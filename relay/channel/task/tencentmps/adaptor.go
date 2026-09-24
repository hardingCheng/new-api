package tencentmps

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// 对客请求 → 任务计划
// ============================

// modelSpec 是从对客模型名解出的能力系列与输出档位。
type modelSpec struct {
	Family     string
	Resolution string
	ShortEdge  int
}

// parseModelName 解析 video-enhance-1080p 这类模型名；不认识的返回 false。
func parseModelName(name string) (modelSpec, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, family := range []string{familyEnhance, familyRestore} {
		if !strings.HasPrefix(name, family+"-") {
			continue
		}
		resolution := strings.TrimPrefix(name, family+"-")
		short, ok := resolutionShortEdge[resolution]
		if !ok {
			return modelSpec{}, false
		}
		return modelSpec{Family: family, Resolution: resolution, ShortEdge: short}, true
	}
	return modelSpec{}, false
}

// jobPlan 是一次增强任务提交前算好的全部要素：用什么模板、处理哪个视频、预扣多少秒。
type jobPlan struct {
	Spec       modelSpec
	Scene      string
	TemplateID int
	SourceURL  string
	Seconds    int
}

// sourceVideoURL 取源视频地址。首选 OpenAI 视频接口的 input_reference，
// 兼容 metadata.video / metadata.video_url 与 content 数组里的 video_url 项。
func sourceVideoURL(req *relaycommon.TaskSubmitReq) string {
	if u := strings.TrimSpace(req.InputReference); u != "" {
		return u
	}
	for _, key := range []string{"video", "video_url", "input_video"} {
		if u, _ := req.Metadata[key].(string); strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u)
		}
	}
	for _, item := range req.Content {
		if item.VideoURL != nil && strings.TrimSpace(item.VideoURL.URL) != "" {
			return strings.TrimSpace(item.VideoURL.URL)
		}
	}
	return ""
}

// validateSourceURL 只放行公网 http(s) 地址：上游是拉取式的，base64 / data URI 它读不到。
func validateSourceURL(raw string) error {
	if raw == "" {
		return errors.New("input_reference (source video url) is required")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("source video must be a publicly reachable http(s) url")
	}
	return nil
}

// planFromRequest 把对客请求翻译成任务计划；任何一处不满足都在提交前 400，不打上游。
func planFromRequest(req *relaycommon.TaskSubmitReq) (*jobPlan, error) {
	spec, ok := parseModelName(req.Model)
	if !ok {
		return nil, fmt.Errorf("unsupported model %q, expected one of %s", req.Model, strings.Join(ModelList, ", "))
	}

	source := sourceVideoURL(req)
	if err := validateSourceURL(source); err != nil {
		return nil, err
	}

	// 时长是预扣费的乘数。上游在提交时不会告诉我们源视频多长，只能由调用方声明；
	// 任务完成后再按上游回报的实际输出时长与帧率结算差额。
	seconds := relaycommon.EffectiveTaskDuration(*req)
	if relaycommon.HasAutoTaskDuration(*req) || seconds <= 0 {
		return nil, fmt.Errorf("seconds (source video duration, 1-%d) is required", relaycommon.MaxTaskDurationSeconds)
	}
	if seconds > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("seconds must not exceed %d", relaycommon.MaxTaskDurationSeconds)
	}

	plan := &jobPlan{Spec: spec, SourceURL: source, Seconds: seconds}
	switch spec.Family {
	case familyRestore:
		plan.TemplateID = restoreTemplates[spec.Resolution]
	default:
		scene := strings.ToLower(strings.TrimSpace(stringFromMetadata(req.Metadata, "scene")))
		if scene == "" {
			scene = SceneReal
		}
		templates, ok := enhanceTemplates[scene]
		if !ok {
			return nil, fmt.Errorf("unsupported scene %q, expected one of %s", scene, strings.Join(sceneOrder, ", "))
		}
		plan.Scene = scene
		plan.TemplateID = templates[spec.Resolution]
	}
	if plan.TemplateID == 0 {
		return nil, fmt.Errorf("no template for %s", req.Model)
	}
	return plan, nil
}

func stringFromMetadata(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return value
}

// ============================
// 上游请求 / 响应形状
// ============================

type processMediaRequest struct {
	InputInfo        mediaInputInfo        `json:"InputInfo"`
	OutputStorage    taskOutputStorage     `json:"OutputStorage"`
	MediaProcessTask mediaProcessTaskInput `json:"MediaProcessTask"`
}

type mediaInputInfo struct {
	Type         string       `json:"Type"`
	UrlInputInfo urlInputInfo `json:"UrlInputInfo"`
}

type urlInputInfo struct {
	Url string `json:"Url"`
}

type taskOutputStorage struct {
	Type             string           `json:"Type"`
	CosOutputStorage cosOutputStorage `json:"CosOutputStorage"`
}

type cosOutputStorage struct {
	Bucket string `json:"Bucket"`
	Region string `json:"Region"`
}

type mediaProcessTaskInput struct {
	TranscodeTaskSet []transcodeTaskInput `json:"TranscodeTaskSet"`
}

type transcodeTaskInput struct {
	Definition        int               `json:"Definition"`
	OverrideParameter overrideParameter `json:"OverrideParameter"`
}

type overrideParameter struct {
	StdExtInfo string `json:"StdExtInfo"`
}

// stdExtInfo 是模板之外的扩展参数，上游要求以 JSON 字符串形式塞在 StdExtInfo 里。
type stdExtInfo struct {
	VideoInfo      stdExtVideoInfo `json:"video_info"`
	UseInternalCos int             `json:"use_internal_cos"`
}

type stdExtVideoInfo struct {
	Short int `json:"short"`
}

type apiError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

type processMediaResponse struct {
	Response struct {
		RequestId string    `json:"RequestId"`
		TaskId    string    `json:"TaskId"`
		Error     *apiError `json:"Error"`
	} `json:"Response"`
}

type describeTaskDetailResponse struct {
	Response struct {
		RequestId    string              `json:"RequestId"`
		Error        *apiError           `json:"Error"`
		Status       string              `json:"Status"`
		TaskType     string              `json:"TaskType"`
		WorkflowTask *workflowTaskResult `json:"WorkflowTask"`
		ScheduleTask *scheduleTaskResult `json:"ScheduleTask"`
	} `json:"Response"`
}

type workflowTaskResult struct {
	Status                string               `json:"Status"`
	ErrCode               int                  `json:"ErrCode"`
	Message               string               `json:"Message"`
	MediaProcessResultSet []mediaProcessResult `json:"MediaProcessResultSet"`
}

type mediaProcessResult struct {
	Type          string               `json:"Type"`
	TranscodeTask *transcodeTaskResult `json:"TranscodeTask"`
}

type scheduleTaskResult struct {
	Status            string           `json:"Status"`
	ErrCode           int              `json:"ErrCode"`
	Message           string           `json:"Message"`
	ActivityResultSet []activityResult `json:"ActivityResultSet"`
}

type activityResult struct {
	ActivityType    string `json:"ActivityType"`
	ActivityResItem struct {
		TranscodeTask *transcodeTaskResult `json:"TranscodeTask"`
	} `json:"ActivityResItem"`
}

type transcodeTaskResult struct {
	Status     string           `json:"Status"`
	ErrCode    int              `json:"ErrCode"`
	ErrCodeExt string           `json:"ErrCodeExt"`
	Message    string           `json:"Message"`
	Progress   int              `json:"Progress"`
	Output     *transcodeOutput `json:"Output"`
}

type transcodeOutput struct {
	Path            string            `json:"Path"`
	Duration        float64           `json:"Duration"`
	Width           int               `json:"Width"`
	Height          int               `json:"Height"`
	VideoStreamSet  []videoStreamItem `json:"VideoStreamSet"`
	CallBackExtInfo string            `json:"CallBackExtInfo"`
	OutputStorage   *outputStorage    `json:"OutputStorage"`
}

type videoStreamItem struct {
	Fps float64 `json:"Fps"`
}

type outputStorage struct {
	Type             string            `json:"Type"`
	CosOutputStorage *cosOutputStorage `json:"CosOutputStorage"`
}

// ============================
// Adaptor
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	baseURL     string
	apiKey      string
	payload     []byte // 提交请求体；签名必须覆盖与实际发送完全一致的字节
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	if a.baseURL == "" {
		a.baseURL = defaultEndpoint
	}
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if taskErr := relaycommon.ValidateTaskDurationBounds(req); taskErr != nil {
		return taskErr
	}
	plan, err := planFromRequest(&req)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	// 渠道密钥配错是我们的问题，不该等到预扣费之后才发现
	if _, err := parseChannelKey(a.apiKey); err != nil {
		return service.TaskErrorWrapperLocal(err, "channel_config_error", http.StatusInternalServerError)
	}
	// 与公共校验保持一致：把时长归一到 duration/seconds 两个字段
	req.Duration = plan.Seconds
	req.Seconds = strconv.Itoa(plan.Seconds)
	c.Set("task_request", req)
	info.Action = constant.TaskActionEnhance
	return nil
}

// EstimateBilling 按调用方声明的源视频秒数预扣，帧率按基准档（≤30 帧）预估。
// 任务完成后 AdjustBillingOnComplete 用上游回报的实际输出时长与帧率结算差额。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	ratios, _ := a.EstimateBillingValidated(c, info)
	return ratios
}

func (a *TaskAdaptor) EstimateBillingValidated(c *gin.Context, info *relaycommon.RelayInfo) (map[string]float64, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	plan, err := planFromRequest(&req)
	if err != nil {
		return nil, err
	}
	c.Set("generated_video_seconds", plan.Seconds)
	c.Set("billable_video_seconds", plan.Seconds)
	return map[string]float64{"seconds": float64(plan.Seconds), "fps": 1}, nil
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/", nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, _ *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	plan, err := planFromRequest(&req)
	if err != nil {
		return nil, err
	}
	cred, err := parseChannelKey(a.apiKey)
	if err != nil {
		return nil, err
	}
	payload, err := buildProcessMediaPayload(plan, cred)
	if err != nil {
		return nil, err
	}
	a.payload = payload
	return bytes.NewReader(payload), nil
}

// buildProcessMediaPayload 组装 ProcessMedia 请求体：URL 输入、走临时链接输出、单个转码（增强）任务。
func buildProcessMediaPayload(plan *jobPlan, cred credentials) ([]byte, error) {
	ext, err := common.Marshal(stdExtInfo{
		VideoInfo:      stdExtVideoInfo{Short: plan.Spec.ShortEdge},
		UseInternalCos: 1,
	})
	if err != nil {
		return nil, err
	}
	body := processMediaRequest{
		InputInfo: mediaInputInfo{Type: "URL", UrlInputInfo: urlInputInfo{Url: plan.SourceURL}},
		// 输出实际落在上游的临时存储并以临时链接返回；这里的桶只是为了过上游校验
		OutputStorage: taskOutputStorage{
			Type:             "COS",
			CosOutputStorage: cosOutputStorage{Bucket: cred.Bucket, Region: cred.Region},
		},
		MediaProcessTask: mediaProcessTaskInput{
			TranscodeTaskSet: []transcodeTaskInput{{
				Definition:        plan.TemplateID,
				OverrideParameter: overrideParameter{StdExtInfo: string(ext)},
			}},
		},
	}
	return common.Marshal(body)
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	cred, err := parseChannelKey(a.apiKey)
	if err != nil {
		return err
	}
	if a.payload == nil {
		return errors.New("request payload not built")
	}
	signRequest(req, cred, actionProcessMedia, a.payload, time.Now())
	return nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) ParseResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*channel.TaskSubmitResponse, *taskdto.TaskError) {
	taskID, taskData, clientResponse, taskErr := a.doResponse(c, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}
	return &channel.TaskSubmitResponse{UpstreamTaskID: taskID, TaskData: taskData, ClientResponse: clientResponse}, nil
}

func (a *TaskAdaptor) doResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, clientResponse any, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	upstreamTaskID, apiErr, err := parseSubmitResponse(responseBody)
	if err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if apiErr != nil {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", apiErr.Code, apiErr.Message), "upstream_error", statusForErrorCode(apiErr.Code))
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	clientResponse = ov
	return upstreamTaskID, responseBody, clientResponse, nil
}

// parseSubmitResponse 解 ProcessMedia 响应。上游出错时 HTTP 仍是 200，错误放在 Response.Error 里。
func parseSubmitResponse(body []byte) (taskID string, apiErr *apiError, err error) {
	var resp processMediaResponse
	if err := common.Unmarshal(body, &resp); err != nil {
		return "", nil, err
	}
	if resp.Response.Error != nil {
		return "", resp.Response.Error, nil
	}
	if strings.TrimSpace(resp.Response.TaskId) == "" {
		return "", nil, errors.New("task id is empty")
	}
	return resp.Response.TaskId, nil, nil
}

// statusForErrorCode 把上游错误码翻成对客状态码：调用方参数问题 400，我们的密钥问题 401，
// 上游限流 429，其余一律当上游故障 502。
func statusForErrorCode(code string) int {
	switch {
	case strings.HasPrefix(code, "InvalidParameter"), strings.HasPrefix(code, "MissingParameter"):
		return http.StatusBadRequest
	case strings.HasPrefix(code, "AuthFailure"), strings.HasPrefix(code, "UnauthorizedOperation"):
		return http.StatusUnauthorized
	case strings.HasPrefix(code, "RequestLimitExceeded"), strings.HasPrefix(code, "LimitExceeded"):
		return http.StatusTooManyRequests
	default:
		return http.StatusBadGateway
	}
}

// isPermanentErrorCode 判断轮询时遇到的错误是否不可能靠重试恢复。
func isPermanentErrorCode(code string) bool {
	return strings.HasPrefix(code, "ResourceNotFound") || strings.HasPrefix(code, "InvalidParameter")
}

// ============================
// 轮询
// ============================

func (a *TaskAdaptor) FetchTask(baseUrl, key string, task *model.Task, proxy string) (*http.Response, error) {
	return a.fetchTaskBody(baseUrl, key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  constant.NormalizeTaskAction(task.Action),
	}, proxy)
}

func (a *TaskAdaptor) fetchTaskBody(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, errors.New("invalid task_id")
	}
	cred, err := parseChannelKey(key)
	if err != nil {
		return nil, err
	}
	baseUrl = strings.TrimRight(baseUrl, "/")
	if baseUrl == "" {
		baseUrl = defaultEndpoint
	}
	payload, err := common.Marshal(map[string]string{"TaskId": taskID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, baseUrl+"/", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	signRequest(req, cred, actionDescribeTaskDetail, payload, time.Now())

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// transcodeResult 从两种任务形状（WorkflowTask / ScheduleTask）里取出增强这一步的结果。
func (r *describeTaskDetailResponse) transcodeResult() *transcodeTaskResult {
	if wf := r.Response.WorkflowTask; wf != nil {
		for _, item := range wf.MediaProcessResultSet {
			if item.TranscodeTask != nil {
				return item.TranscodeTask
			}
		}
	}
	if st := r.Response.ScheduleTask; st != nil {
		for _, item := range st.ActivityResultSet {
			if item.ActivityResItem.TranscodeTask != nil {
				return item.ActivityResItem.TranscodeTask
			}
		}
	}
	return nil
}

func (r *describeTaskDetailResponse) failureMessage() string {
	if wf := r.Response.WorkflowTask; wf != nil && wf.Message != "" && wf.Message != "SUCCESS" {
		return wf.Message
	}
	if st := r.Response.ScheduleTask; st != nil && st.Message != "" && st.Message != "SUCCESS" {
		return st.Message
	}
	return ""
}

// outputURL 取输出视频地址：临时链接优先（CallBackExtInfo 里的 tmpl_url），
// 没有时按输出桶拼公网地址（要求桶可公有读）。
func outputURL(out *transcodeOutput) string {
	if out == nil {
		return ""
	}
	if strings.TrimSpace(out.CallBackExtInfo) != "" {
		var ext struct {
			TmplURL string `json:"tmpl_url"`
		}
		if err := common.Unmarshal([]byte(out.CallBackExtInfo), &ext); err == nil && strings.TrimSpace(ext.TmplURL) != "" {
			return strings.TrimSpace(ext.TmplURL)
		}
	}
	if out.OutputStorage != nil && out.OutputStorage.CosOutputStorage != nil && out.Path != "" {
		cos := out.OutputStorage.CosOutputStorage
		if cos.Bucket != "" && cos.Region != "" {
			return fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s", cos.Bucket, cos.Region, strings.TrimLeft(out.Path, "/"))
		}
	}
	return ""
}

func (a *TaskAdaptor) ParseTaskResult(_ *model.Task, _ *http.Response, respBody []byte) (*relaycommon.TaskInfo, error) {
	return a.parseTaskResult(respBody)
}

func (a *TaskAdaptor) parseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var resp describeTaskDetailResponse
	if err := common.Unmarshal(respBody, &resp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	if apiErr := resp.Response.Error; apiErr != nil {
		if isPermanentErrorCode(apiErr.Code) {
			return relaycommon.FailTaskInfo(fmt.Sprintf("%s: %s", apiErr.Code, apiErr.Message)), nil
		}
		// 鉴权、限流之类的错误下一轮再查，不把任务判死
		return nil, fmt.Errorf("upstream error %s: %s", apiErr.Code, apiErr.Message)
	}

	result := &relaycommon.TaskInfo{Code: 0}
	transcode := resp.transcodeResult()
	switch strings.ToUpper(resp.Response.Status) {
	case "WAITING":
		result.Status = model.TaskStatusQueued
		result.Progress = taskcommon.ProgressQueued
	case "PROCESSING":
		result.Status = model.TaskStatusInProgress
		result.Progress = processingProgress(transcode)
	case "FINISH":
		return finishedTaskInfo(&resp, transcode), nil
	default:
		result.Status = model.TaskStatusInProgress
		result.Progress = taskcommon.ProgressInProgress
	}
	return result, nil
}

// processingProgress 把上游 0-100 的进度转成百分比字串；未完成时封顶 99，
// 免得进度条已满、状态却还是处理中。
func processingProgress(transcode *transcodeTaskResult) string {
	if transcode == nil || transcode.Progress <= 0 {
		return taskcommon.ProgressInProgress
	}
	progress := transcode.Progress
	if progress > 99 {
		progress = 99
	}
	return strconv.Itoa(progress) + "%"
}

func finishedTaskInfo(resp *describeTaskDetailResponse, transcode *transcodeTaskResult) *relaycommon.TaskInfo {
	if transcode == nil {
		reason := resp.failureMessage()
		if reason == "" {
			reason = "upstream task finished without an enhancement result"
		}
		return relaycommon.FailTaskInfo(reason)
	}
	succeeded := strings.EqualFold(transcode.Status, "SUCCESS") || (transcode.ErrCode == 0 && transcode.Output != nil && transcode.Status == "")
	if !succeeded {
		reason := transcode.Message
		if reason == "" || reason == "SUCCESS" {
			reason = transcode.ErrCodeExt
		}
		if reason == "" {
			reason = fmt.Sprintf("upstream task failed (code %d)", transcode.ErrCode)
		}
		return relaycommon.FailTaskInfo(reason)
	}
	videoURL := outputURL(transcode.Output)
	if videoURL == "" {
		return relaycommon.FailTaskInfo("upstream task succeeded but returned no output url")
	}
	return &relaycommon.TaskInfo{
		Code:     0,
		Status:   model.TaskStatusSuccess,
		Progress: taskcommon.ProgressComplete,
		Url:      videoURL,
	}
}

// ============================
// 完成后结算
// ============================

// fpsBillingRatio 是输出帧率对应的计费倍数：≤30 帧 1 倍，≤60 帧 2 倍，再往上 4 倍。
func fpsBillingRatio(fps float64) float64 {
	switch {
	case fps <= fpsTierBase:
		return 1
	case fps <= fpsTierDouble:
		return 2
	default:
		return 4
	}
}

func (out *transcodeOutput) fps() float64 {
	if out == nil {
		return 0
	}
	for _, stream := range out.VideoStreamSet {
		if stream.Fps > 0 {
			return stream.Fps
		}
	}
	return 0
}

// AdjustBillingOnComplete 用上游回报的实际输出（时长、帧率）重算应收额度。
//
// 预扣时只有调用方声明的秒数、帧率按基准档估；上游按实际输出计费，两者对不上就在这里
// 补扣或退还。返回 0 表示维持预扣金额。
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	if task == nil || taskResult == nil || taskResult.Status != model.TaskStatusSuccess {
		return 0
	}
	bc := task.PrivateData.BillingContext
	if bc == nil || task.Quota <= 0 {
		return 0
	}
	var resp describeTaskDetailResponse
	if err := common.Unmarshal(task.Data, &resp); err != nil {
		return 0
	}
	transcode := resp.transcodeResult()
	if transcode == nil || transcode.Output == nil || transcode.Output.Duration <= 0 {
		return 0
	}

	actualSeconds := math.Ceil(transcode.Output.Duration)
	if actualSeconds > relaycommon.MaxTaskDurationSeconds {
		actualSeconds = relaycommon.MaxTaskDurationSeconds
	}
	actualFpsRatio := fpsBillingRatio(transcode.Output.fps())

	estimatedSeconds := bc.OtherRatios["seconds"]
	if estimatedSeconds <= 0 {
		estimatedSeconds = float64(task.Properties.VideoSeconds)
	}
	if estimatedSeconds <= 0 {
		return 0
	}
	estimatedFpsRatio := bc.OtherRatios["fps"]
	if estimatedFpsRatio <= 0 {
		estimatedFpsRatio = 1
	}

	scale := (actualSeconds * actualFpsRatio) / (estimatedSeconds * estimatedFpsRatio)
	if scale == 1 {
		return 0
	}
	actualQuota, _ := common.QuotaFromFloatChecked(float64(task.Quota) * scale)
	if actualQuota <= 0 || actualQuota == task.Quota {
		return 0
	}
	return actualQuota
}

// ============================
// 对客查询响应
// ============================

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var resp describeTaskDetailResponse
	if err := common.Unmarshal(originTask.Data, &resp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if videoURL := outputURL(resp.transcodeResult().outputOrNil()); videoURL != "" {
		openAIVideo.SetMetadata("url", videoURL)
	}
	if originTask.Status == model.TaskStatusFailure && originTask.FailReason != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{Message: originTask.FailReason}
	}
	return common.Marshal(openAIVideo)
}

func (t *transcodeTaskResult) outputOrNil() *transcodeOutput {
	if t == nil {
		return nil
	}
	return t.Output
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}
