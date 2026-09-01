package dmc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
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

// 上游为异步视频生成服务：POST /v2/video_generation 创建任务，
// GET /v2/query/video_generation/{task_id} 查询；输出固定 768P，
// 名义时长整数 1–15 秒，按输出秒数计费。

const (
	// 上游硬限制，同时是计费乘数的边界；越界必须在提交前本地 400。
	MinDurationSeconds     = 1
	MaxDurationSeconds     = 15
	defaultDurationSeconds = 5
	defaultResolution      = "768P"
	defaultRatio           = "16:9"
)

// ============================
// Request / Response structures
// ============================

type requestPayload struct {
	Model       string                        `json:"model"`
	Content     []relaycommon.TaskContentItem `json:"content"`
	Resolution  string                        `json:"resolution"`
	Duration    int                           `json:"duration"`
	Ratio       string                        `json:"ratio"`
	CallbackURL string                        `json:"callback_url,omitempty"`
}

type createResponse struct {
	TaskID string `json:"task_id"`
}

type upstreamError struct {
	Code    string `json:"code"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

type upstreamTask struct {
	ID       string        `json:"id"`
	Status   string        `json:"status"`
	Progress *float64      `json:"progress"`
	Error    upstreamError `json:"error"`
	Content  struct {
		URL string `json:"url"`
	} `json:"content"`
}

type queryResponse struct {
	Task upstreamTask `json:"task"`
}

// 同步错误 envelope：{"type":"error","error":{"type":...,"message":...}}
type errorEnvelope struct {
	Error upstreamError `json:"error"`
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

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	if taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	payload, err := convertToRequestPayload(&req)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if payload.Duration < MinDurationSeconds || payload.Duration > MaxDurationSeconds {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("duration must be an integer from %d through %d", MinDurationSeconds, MaxDurationSeconds),
			"invalid_duration", http.StatusBadRequest)
	}
	return nil
}

// EstimateBilling 上游按输出秒数计费，参考素材不额外计费。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) (map[string]float64, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	payload, err := convertToRequestPayload(&req)
	if err != nil {
		return nil, err
	}
	// duration 可能来自 metadata，绕过标准 DTO 校验路径，这里必须再次约束
	if payload.Duration < MinDurationSeconds || payload.Duration > MaxDurationSeconds {
		return nil, fmt.Errorf("duration must be an integer from %d through %d", MinDurationSeconds, MaxDurationSeconds)
	}
	c.Set("generated_video_seconds", payload.Duration)
	c.Set("billable_video_seconds", payload.Duration)
	return map[string]float64{"seconds": float64(payload.Duration)}, nil
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v2/video_generation", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	// 上游支持幂等创建；带上用户前缀转发，避免不同用户的 Key 在共享上游账号内互撞
	if idem := c.GetHeader("Idempotency-Key"); idem != "" {
		req.Header.Set("Idempotency-Key", fmt.Sprintf("u%d-%s", info.UserId, idem))
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	payload, err := convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		payload.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = payload.Model
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var cResp createResponse
	if err := common.Unmarshal(responseBody, &cResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if cResp.TaskID == "" {
		var envelope errorEnvelope
		_ = common.Unmarshal(responseBody, &envelope)
		message := envelope.Error.Message
		if message == "" {
			message = fmt.Sprintf("task_id is empty, body: %s", responseBody)
		}
		statusCode := resp.StatusCode
		if statusCode == http.StatusOK {
			statusCode = http.StatusInternalServerError
		}
		taskErr = service.TaskErrorWrapper(errors.New(message), "upstream_error", statusCode)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return cResp.TaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v2/query/video_generation/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
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

func convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{Model: req.Model}

	// 通用图片入参：不带 role 的单图上游按首帧处理
	for _, imgURL := range req.Images {
		r.Content = append(r.Content, relaycommon.TaskContentItem{
			Type:     "image_url",
			ImageURL: &relaycommon.TaskMediaURL{URL: imgURL},
		})
	}

	if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	if len(req.Content) > 0 {
		r.Content = req.Content
	}
	if req.Ratio != "" {
		r.Ratio = req.Ratio
	}

	// 上游要求 content 恰好一个文本项，文本一律以 prompt 字段为准
	items := make([]relaycommon.TaskContentItem, 0, len(r.Content)+1)
	for _, item := range r.Content {
		if item.Type == "text" {
			continue
		}
		items = append(items, item)
	}
	items = append(items, relaycommon.TaskContentItem{Type: "text", Text: req.Prompt})
	r.Content = items

	if sec := relaycommon.EffectiveTaskDuration(*req); sec > 0 {
		r.Duration = sec
	}
	if r.Duration == 0 {
		r.Duration = defaultDurationSeconds
	}
	if r.Resolution == "" {
		r.Resolution = defaultResolution
	}
	if r.Ratio == "" {
		r.Ratio = defaultRatio
	}
	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var qResp queryResponse
	if err := common.Unmarshal(respBody, &qResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	t := qResp.Task

	taskResult := relaycommon.TaskInfo{Code: 0}
	switch t.Status {
	case "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = taskcommon.ProgressQueued
	case "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = taskcommon.ProgressInProgress
		if t.Progress != nil && *t.Progress > 0 {
			pct := int(*t.Progress * 100)
			// 上游 running 阶段 progress 可能已经是 1（结果发布中）；非终态不能写 100%，
			// 否则会被轮询器的待处理条件（progress != '100%'）跳过导致任务搁浅
			if pct > 99 {
				pct = 99
			}
			if pct < 1 {
				pct = 1
			}
			taskResult.Progress = fmt.Sprintf("%d%%", pct)
		}
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = taskcommon.ProgressComplete
		taskResult.Url = t.Content.URL
	case "failed", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = taskcommon.ProgressComplete
		reason := t.Error.Message
		if t.Error.Code != "" {
			reason = t.Error.Code + ": " + reason
		}
		if reason == "" {
			reason = t.Status
		}
		taskResult.Reason = reason
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = taskcommon.ProgressInProgress
	}
	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var qResp queryResponse
	if err := common.Unmarshal(originTask.Data, &qResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal dmc task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	if qResp.Task.Content.URL != "" {
		openAIVideo.SetMetadata("url", qResp.Task.Content.URL)
	}
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if qResp.Task.Status == "failed" || qResp.Task.Status == "cancelled" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: qResp.Task.Error.Message,
			Code:    qResp.Task.Error.Code,
		}
	}

	return common.Marshal(openAIVideo)
}
