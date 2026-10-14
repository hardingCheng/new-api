package sora

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for text type
	ImageURL *ImageURL `json:"image_url,omitempty"` // for image_url type
}

type ImageURL struct {
	URL string `json:"url"`
}

// flexString 兼容上游将 id 返回为字符串或数字两种情况：
// 部分 Sora 兼容渠道（如 seedance）会把 id 作为数字返回，
// 直接用 string 解析会报 "cannot unmarshal number into Go struct field ... of type string"。
type flexString string

func (s *flexString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*s = ""
		return nil
	}
	if data[0] == '"' {
		var str string
		if err := common.Unmarshal(data, &str); err != nil {
			return err
		}
		*s = flexString(str)
		return nil
	}
	// 非字符串（数字等）原样作为字符串保存
	*s = flexString(data)
	return nil
}

type responseTask struct {
	ID                    flexString `json:"id"`
	TaskID                string     `json:"task_id,omitempty"` //兼容旧接口
	Object                string     `json:"object"`
	Model                 string     `json:"model"`
	Status                string     `json:"status"`
	Progress              int        `json:"progress"`
	CreatedAt             int64      `json:"created_at"`
	CompletedAt           int64      `json:"completed_at,omitempty"`
	ExpiresAt             int64      `json:"expires_at,omitempty"`
	Seconds               string     `json:"seconds,omitempty"`
	Size                  string     `json:"size,omitempty"`
	RemixedFromVideoID    string     `json:"remixed_from_video_id,omitempty"`
	ReferenceVideoSeconds int        `json:"reference_video_seconds,omitempty"`
	Error                 *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
	// 上游原样带回的火山方舟用量（Seedance 2.x 按 token 计费）。
	Usage *struct {
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// seedance25VideoInputRatio 是 Seedance 2.5 含参考视频输入时相对基准单价的倍率
// （官方 480p/720p 70→42 元/百万 token，1080p 77→46 元/百万 token）。
func seedance25VideoInputRatio(is1080p bool) float64 {
	if is1080p {
		return 46.0 / 77.0
	}
	return 42.0 / 70.0
}

// seedance25Content 把 prompt 和参考素材组装成火山方舟格式的 content[]：视频中转的 Ark 线路
// 只读 content[]（顶层 prompt 不参与）。素材来自 images（字符串或 {url,type,role} 对象）、
// image、input_reference；字符串一律视为参考图，未指定 role 时按素材类型给 reference_*。
func seedance25Content(bodyMap map[string]any) []any {
	content := make([]any, 0)
	if prompt, _ := bodyMap["prompt"].(string); strings.TrimSpace(prompt) != "" {
		content = append(content, map[string]any{"type": "text", "text": prompt})
	}
	appendMedia := func(url, mediaType, role string) {
		url = strings.TrimSpace(url)
		if url == "" {
			return
		}
		kind := "image"
		if mediaType == "video" || mediaType == "audio" {
			kind = mediaType
		}
		if role == "" {
			role = "reference_" + kind
		}
		item := map[string]any{
			"type":        kind + "_url",
			kind + "_url": map[string]any{"url": url},
			"role":        role,
		}
		// 真人图交给中转先过白再生成
		if mediaType == "person" {
			item["subject_type"] = "person"
		}
		content = append(content, item)
	}
	appendValue := func(raw any) {
		switch v := raw.(type) {
		case string:
			appendMedia(v, "image", "")
		case map[string]any:
			url, _ := v["url"].(string)
			mediaType, _ := v["type"].(string)
			role, _ := v["role"].(string)
			appendMedia(url, mediaType, role)
		}
	}
	for _, key := range []string{"images", "image", "input_reference"} {
		switch v := bodyMap[key].(type) {
		case []any:
			for _, raw := range v {
				appendValue(raw)
			}
		default:
			appendValue(v)
		}
	}
	return content
}

// seedance25ReferenceVideoURLs 取出请求里的参考视频：火山格式 content[] 的 video_url，
// 以及 images 列表里 type 为 video 的条目。
func seedance25ReferenceVideoURLs(req relaycommon.TaskSubmitReq, body []byte) []string {
	urls := relaycommon.ExtractReferenceVideoURLs(req)
	for _, item := range gjson.GetBytes(body, `images.#(type=="video")#.url`).Array() {
		if url := strings.TrimSpace(item.String()); url != "" && !slices.Contains(urls, url) {
			urls = append(urls, url)
		}
	}
	return urls
}

// seedance25OutputPixels 是各分辨率档输出画面的像素数（各画幅比例像素数基本一致，按 16:9 计）。
func seedance25OutputPixels(modelName string) float64 {
	name := strings.ToLower(modelName)
	switch {
	case strings.Contains(name, "1080p"):
		return 1920 * 1080
	case strings.Contains(name, "480p"):
		return 854 * 480
	default:
		return 1280 * 720
	}
}

// seedance25SettlementTokens 返回 Seedance 2.5 任务用于结算的 token 数：优先上游
// total_tokens；上游不回传时（如 LibLib 线路）按官方计费公式估算：
// (24 × (输出秒数 + 参考视频秒数) + 1) × 输出像素 / 1024。结果饱和到 int32 范围内。
func seedance25SettlementTokens(resTask responseTask, modelName string, referenceSeconds int) int {
	if resTask.Usage != nil {
		tokens := resTask.Usage.TotalTokens
		if tokens <= 0 {
			tokens = resTask.Usage.CompletionTokens
		}
		if tokens > 0 {
			return int(min(tokens, math.MaxInt32))
		}
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(resTask.Seconds)), 64)
	if err != nil || seconds <= 0 || seconds > relaycommon.MaxTaskDurationSeconds {
		return 0
	}
	referenceSeconds = min(max(referenceSeconds, 0), relaycommon.MaxTaskDurationSeconds)
	frames := 24*(seconds+float64(referenceSeconds)) + 1
	return int(min(frames*seedance25OutputPixels(modelName)/1024, math.MaxInt32))
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func validateRemixRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	if taskErr := relaycommon.ValidateTaskDurationBoundsForModel(req, info); taskErr != nil {
		return taskErr
	}
	// 存储原始请求到 context，与 ValidateMultipartDirect 路径保持一致
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		return validateRemixRequest(c, info)
	}
	return relaycommon.ValidateMultipartDirect(c, info)
}

// EstimateBilling 根据用户请求的 seconds 和 size 计算 OtherRatios。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	ratios, _ := a.EstimateBillingValidated(c, info)
	return ratios
}

func (a *TaskAdaptor) EstimateBillingValidated(c *gin.Context, info *relaycommon.RelayInfo) (map[string]float64, error) {
	// remix 路径的 OtherRatios 已在 ResolveOriginTask 中设置
	if info.Action == constant.TaskActionRemix {
		return nil, nil
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	// Seedance 2.5 按倍率（token）定价时，完成后按上游 total_tokens 重算，
	// 重算会连乘全部 OtherRatios，所以这里不能放 seconds，只放参考视频折扣。
	if relaycommon.IsSeedance25RelayModel(info, req.Model) && !info.PriceData.UsePrice {
		var body []byte
		if storage, err := common.GetBodyStorage(c); err == nil {
			body, _ = storage.Bytes()
		}
		videoURLs := seedance25ReferenceVideoURLs(req, body)
		if len(videoURLs) == 0 {
			return nil, nil
		}
		// 参考视频秒数只用于上游不回传 token 时的估算，读不到时按 0 计、不拦请求。
		if referenceSeconds, err := service.SumReferenceVideoDurationSeconds(c, videoURLs); err == nil && referenceSeconds > 0 {
			c.Set(relaycommon.TaskPropertyReferenceVideoSecondsKey, referenceSeconds)
		}
		is1080p := strings.Contains(strings.ToLower(info.OriginModelName+" "+info.UpstreamModelName+" "+req.Model), "1080p")
		return map[string]float64{"video_input": seedance25VideoInputRatio(is1080p)}, nil
	}

	seconds := relaycommon.EffectiveTaskDuration(req)
	if seconds <= 0 {
		seconds = 4
	}
	referenceSeconds := 0
	if relaycommon.IsSeedanceRelayModel(info, req.Model) {
		referenceSeconds, err = service.SumReferenceVideoDurationSeconds(c, relaycommon.ExtractReferenceVideoURLs(req))
		if err != nil {
			return nil, err
		}
	}
	billableSeconds := seconds + referenceSeconds
	c.Set("generated_video_seconds", seconds)
	c.Set("reference_video_seconds", referenceSeconds)
	c.Set("billable_video_seconds", billableSeconds)

	size := req.Size
	if size == "" {
		size = "720x1280"
	}

	ratios := map[string]float64{
		// Only billing uses generated + reference seconds. BuildRequestBody sends
		// req.Seconds/req.Duration, i.e. the generated video duration only.
		"seconds": float64(billableSeconds),
		"size":    1,
	}
	return ratios, nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.Action == constant.TaskActionRemix {
		return fmt.Sprintf("%s/v1/videos/%s/remix", a.baseURL, info.OriginTaskID), nil
	}
	return fmt.Sprintf("%s/v1/videos", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	cachedBody, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read_body_bytes_failed")
	}
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "application/json") {
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(cachedBody, &bodyMap); err == nil {
			bodyMap["model"] = info.UpstreamModelName
			if req, err := relaycommon.GetTaskRequest(c); err == nil {
				duration := relaycommon.EffectiveTaskDuration(req)
				if relaycommon.IsSeedance25AutoDuration(info, req) {
					duration = -1
				}
				if duration != 0 {
					bodyMap["seconds"] = strconv.Itoa(duration)
					_, hasDuration := bodyMap["duration"]
					if hasDuration || relaycommon.IsSeedanceVideoModel(req.Model) ||
						relaycommon.IsSeedanceVideoModel(info.OriginModelName) ||
						relaycommon.IsSeedanceVideoModel(info.UpstreamModelName) {
						bodyMap["duration"] = duration
					}
				}
			}
			if req, err := relaycommon.GetTaskRequest(c); err == nil {
				if relaycommon.ShouldRewriteGrokImagineReferenceToImageURL(info, req) {
					relaycommon.RewriteGrokImagineReferenceToImageURL(bodyMap)
				} else {
					relaycommon.FillMissingGrokImagineInputReferenceMap(info, req, bodyMap)
					relaycommon.FillGrokImagineVideo15PreviewImages(info, req, bodyMap)
				}
			}
			// seedance 系上游只认 image 字段（字符串或数组），images / input_reference 会被静默忽略：
			// 归并成 image，避免客户按 images 传参后参考图被丢弃、却照常按秒计费。
			// 仅限 seedance —— 同一适配器还服务其他视频渠道，它们各有自己的参考图字段协议。
			// Seedance 2.5 的上游是自有视频中转的 Ark 线路，只读火山格式 content[]：
			// 客户已传 content 时原样透传，否则由 prompt 和参考素材组装。
			if req, err := relaycommon.GetTaskRequest(c); err == nil && relaycommon.IsSeedance25RelayModel(info, req.Model) {
				if items, _ := bodyMap["content"].([]any); len(items) == 0 {
					if content := seedance25Content(bodyMap); len(content) > 0 {
						bodyMap["content"] = content
					}
				}
				delete(bodyMap, "images")
				delete(bodyMap, "image")
				delete(bodyMap, "input_reference")
			} else if err == nil && relaycommon.IsSeedanceRelayModel(info, req.Model) {
				if value := req.ImageUpstreamValue(); value != nil {
					bodyMap["image"] = value
					delete(bodyMap, "images")
					delete(bodyMap, "input_reference")
				}
			}
			if newBody, err := common.Marshal(bodyMap); err == nil {
				return bytes.NewReader(newBody), nil
			}
		}
		return bytes.NewReader(cachedBody), nil
	}

	if strings.Contains(contentType, "multipart/form-data") {
		formData, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return bytes.NewReader(cachedBody), nil
		}
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		writer.WriteField("model", info.UpstreamModelName)
		taskReq, _ := relaycommon.GetTaskRequest(c)
		duration := relaycommon.EffectiveTaskDuration(taskReq)
		if relaycommon.IsSeedance25AutoDuration(info, taskReq) {
			duration = -1
		}
		hasDuration := duration != 0
		_, hasDurationField := formData.Value["duration"]
		writeDuration := hasDurationField || relaycommon.IsSeedanceVideoModel(taskReq.Model) ||
			relaycommon.IsSeedanceVideoModel(info.OriginModelName) ||
			relaycommon.IsSeedanceVideoModel(info.UpstreamModelName)
		hasInputReference := false
		for key, values := range formData.Value {
			if key == "model" {
				continue
			}
			if hasDuration && (key == "seconds" || key == "duration") {
				continue
			}
			if key == "input_reference" && len(values) > 0 {
				hasInputReference = true
			}
			for _, v := range values {
				writer.WriteField(key, v)
			}
		}
		if hasDuration {
			writer.WriteField("seconds", strconv.Itoa(duration))
			if writeDuration {
				writer.WriteField("duration", strconv.Itoa(duration))
			}
		}
		if !hasInputReference && relaycommon.ShouldFillGrokImagineInputReference(info, taskReq) {
			for _, value := range taskReq.InputReferenceValues() {
				writer.WriteField("input_reference", value)
			}
		}
		for fieldName, fileHeaders := range formData.File {
			for _, fh := range fileHeaders {
				f, err := fh.Open()
				if err != nil {
					continue
				}
				ct := fh.Header.Get("Content-Type")
				if ct == "" || ct == "application/octet-stream" {
					buf512 := make([]byte, 512)
					n, _ := io.ReadFull(f, buf512)
					ct = http.DetectContentType(buf512[:n])
					// Re-open after sniffing so the full content is copied below
					f.Close()
					f, err = fh.Open()
					if err != nil {
						continue
					}
				}
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fh.Filename))
				h.Set("Content-Type", ct)
				part, err := writer.CreatePart(h)
				if err != nil {
					f.Close()
					continue
				}
				io.Copy(part, f)
				f.Close()
			}
		}
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &buf, nil
	}

	return common.NewReplayableBodyReader(storage), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) ParseResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*channel.TaskSubmitResponse, *dto.TaskError) {
	taskID, taskData, clientResponse, taskErr := a.doResponse(c, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}
	return &channel.TaskSubmitResponse{UpstreamTaskID: taskID, TaskData: taskData, ClientResponse: clientResponse}, nil
}

func (a *TaskAdaptor) doResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, clientResponse any, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Sora response
	var dResp responseTask
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	upstreamID := string(dResp.ID)
	if upstreamID == "" {
		upstreamID = dResp.TaskID
	}
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}
	// 视频中转在提交阶段就失败（如某条上游登录失效）时仍返回 HTTP 200 + status=failed：
	// 按 502 处理，让 hub 换同模型的其他渠道重试，而不是把必然失败的任务交给客户。
	if relaycommon.IsSeedance25RelayModel(info, info.OriginModelName) && (dResp.Status == "failed" || dResp.Status == "cancelled") {
		reason := "upstream task failed at submission"
		if dResp.Error != nil && strings.TrimSpace(dResp.Error.Message) != "" {
			reason = dResp.Error.Message
		}
		taskErr = service.TaskErrorWrapper(errors.New(reason), "upstream_submit_failed", http.StatusBadGateway)
		return
	}

	// 使用公开 task_xxxx ID 返回给客户端
	dResp.ID = flexString(info.PublicTaskID)
	dResp.TaskID = info.PublicTaskID
	// 用对外模型名覆盖上游真实模型名，避免泄露映射后的上游模型（如 wp/seedance-2.0-fast-480p、prism-3.0-fast-480p）
	if info.OriginModelName != "" {
		dResp.Model = info.OriginModelName
	}
	if referenceSeconds := c.GetInt("reference_video_seconds"); referenceSeconds > 0 {
		dResp.ReferenceVideoSeconds = referenceSeconds
	}
	clientResponse = dResp
	return upstreamID, responseBody, clientResponse, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, task *model.Task, proxy string) (*http.Response, error) {
	return a.fetchTaskBody(baseUrl, key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  constant.NormalizeTaskAction(task.Action),
	}, proxy)
}

func (a *TaskAdaptor) fetchTaskBody(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(task *model.Task, _ *http.Response, respBody []byte) (*relaycommon.TaskInfo, error) {
	taskResult, err := a.parseTaskResult(respBody)
	if err != nil || taskResult.Status != model.TaskStatusSuccess || task == nil {
		return taskResult, err
	}
	if !relaycommon.IsSeedance25RelayModel(nil, task.Properties.OriginModelName) &&
		!relaycommon.IsSeedance25RelayModel(nil, task.Properties.UpstreamModelName) {
		return taskResult, nil
	}
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return taskResult, nil
	}
	// 只有按倍率定价的任务会用 token 重算；按秒定价的快照会忽略它。
	modelName := task.Properties.OriginModelName + " " + task.Properties.UpstreamModelName
	taskResult.TotalTokens = seedance25SettlementTokens(resTask, modelName, task.Properties.ReferenceVideoSeconds)
	return taskResult, nil
}

func (a *TaskAdaptor) parseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch resTask.Status {
	case "queued", "pending":
		taskResult.Status = model.TaskStatusQueued
	case "processing", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
	case "completed":
		taskResult.Status = model.TaskStatusSuccess
		// Url intentionally left empty — the caller constructs the proxy URL using the public task ID
	case "failed", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	default:
	}
	if resTask.Progress > 0 && resTask.Progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	data := task.Data
	var err error

	// 空数据兜底：上游从未回写 task.Data（如刚提交、或超时清理把状态置为终态但未回写 body）时，
	// 透传空 body 会让下游解析失败，这里给一个最小可解析对象。
	if len(data) == 0 {
		data = []byte("{}")
	}

	// 替换为对外公开的 task ID（不暴露上游真实 ID）
	if data, err = sjson.SetBytes(data, "id", task.TaskID); err != nil {
		return nil, errors.Wrap(err, "set id failed")
	}

	// 用对外模型名覆盖上游真实模型名，避免泄露映射后的上游模型
	if task.Properties.OriginModelName != "" {
		if data, err = sjson.SetBytes(data, "model", task.Properties.OriginModelName); err != nil {
			return nil, errors.Wrap(err, "set model failed")
		}
	}

	// 上游回显的 task_id（部分渠道如 veo 会带）是上游内部 ID，替换为对外公开 ID。
	// 仅在字段存在时替换，避免给本不返回 task_id 的渠道凭空加字段。
	if gjson.GetBytes(data, "task_id").Exists() {
		if data, err = sjson.SetBytes(data, "task_id", task.TaskID); err != nil {
			return nil, errors.Wrap(err, "set task_id failed")
		}
	}

	// 仅在数据库已是终态（成功/失败）时，才用权威值覆盖 task.Data。
	// 非终态（排队/处理中）必须保持上游原生状态字符串（如 seedance 的 "processing"），
	// 否则像 Apigod 这类只认上游原生状态的下游会把 "in_progress" 判为 unknown。
	// 终态覆盖用于纠正一个 stale 场景：超时清理器把 DB 置为 FAILURE、progress=100%，
	// 但不会回写 task.Data；若纯透传，下游会一直看到 "processing" 而永远轮询。
	switch task.Status {
	case model.TaskStatusSuccess:
		if data, err = sjson.SetBytes(data, "status", relaydto.VideoStatusCompleted); err != nil {
			return nil, errors.Wrap(err, "set status failed")
		}
		if data, err = sjson.SetBytes(data, "progress", 100); err != nil {
			return nil, errors.Wrap(err, "set progress failed")
		}
	case model.TaskStatusFailure:
		if data, err = sjson.SetBytes(data, "status", relaydto.VideoStatusFailed); err != nil {
			return nil, errors.Wrap(err, "set status failed")
		}
		if data, err = sjson.SetBytes(data, "progress", 100); err != nil {
			return nil, errors.Wrap(err, "set progress failed")
		}
		reason := task.FailReason
		if reason == "" {
			reason = "task failed"
		}
		if data, err = sjson.SetBytes(data, "error.message", reason); err != nil {
			return nil, errors.Wrap(err, "set error.message failed")
		}
	}

	// 任务成功时，确保顶层与 metadata 中的 url/video_url/result_url 都存在
	if task.Status == model.TaskStatusSuccess {
		var respMap map[string]any
		if err := common.Unmarshal(data, &respMap); err == nil {
			var resultURL string
			if existingURL, ok := respMap["url"].(string); ok && existingURL != "" {
				resultURL = existingURL
			} else if existingVideoURL, ok := respMap["video_url"].(string); ok && existingVideoURL != "" {
				resultURL = existingVideoURL
			} else if task.PrivateData.ResultURL != "" {
				resultURL = task.PrivateData.ResultURL
			}
			if resultURL != "" {
				for _, field := range []string{"url", "video_url", "result_url", "metadata.url", "metadata.video_url", "metadata.result_url"} {
					if data, err = sjson.SetBytes(data, field, resultURL); err != nil {
						return nil, errors.Wrapf(err, "set %s failed", field)
					}
				}
			}
		}
	}

	// status 兜底：非终态透传上游原生状态，但若上游 body 缺 status 字段（如空数据兜底场景），
	// 下游会解析失败，这里用 DB 状态映射出的 OpenAI 标准状态补上。
	if !gjson.GetBytes(data, "status").Exists() {
		if data, err = sjson.SetBytes(data, "status", task.Status.ToVideoStatus()); err != nil {
			return nil, errors.Wrap(err, "set status fallback failed")
		}
	}

	if task.Properties.ReferenceVideoSeconds > 0 {
		if data, err = sjson.SetBytes(data, "reference_video_seconds", task.Properties.ReferenceVideoSeconds); err != nil {
			return nil, errors.Wrap(err, "set reference_video_seconds failed")
		}
	}

	// 透传上游原生结构时，删除可能携带的内部计费字段，避免把上游成本
	// （如 xAI 的 usage.cost_in_usd_ticks）暴露给调用方。
	if gjson.GetBytes(data, "usage").Exists() {
		if data, err = sjson.DeleteBytes(data, "usage"); err != nil {
			return nil, errors.Wrap(err, "delete usage failed")
		}
	}

	return data, nil
}
