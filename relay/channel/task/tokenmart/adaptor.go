package tokenmart

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// 上游为异步视频生成服务：POST /v1/video/generate 创建任务，
// GET /v1/video/tasks/{id} 查询；参考素材走 content 数组的 image_url 项
// （原生多参考图，不会把首张素材当首帧），画幅由 ratio 决定、与素材比例无关。
//
// 对外契约与其他视频渠道一致：客户仍按 image / aspect_ratio / seconds 提交，
// 由本适配器翻译成上游形状，客户无需改动。

const (
	// 时长边界同时是计费乘数的边界；越界必须在提交前本地 400。
	MinDurationSeconds     = 1
	MaxDurationSeconds     = 15
	defaultDurationSeconds = 5
	defaultResolution      = "480p"
	defaultRatio           = "16:9"
)

// ============================
// Request / Response structures
// ============================

type requestPayload struct {
	Model      string                        `json:"model"`
	Content    []relaycommon.TaskContentItem `json:"content"`
	Resolution string                        `json:"resolution"`
	Duration   int                           `json:"duration"`
	Ratio      string                        `json:"ratio"`
}

type upstreamTask struct {
	ID              string          `json:"id"`
	Status          string          `json:"status"`
	Model           string          `json:"model"`
	DurationSeconds int             `json:"duration_seconds"`
	Outputs         []string        `json:"outputs"`
	Error           json.RawMessage `json:"error"`
}

type taskEnvelope struct {
	Task upstreamTask `json:"task"`
}

// 同步错误 envelope：{"error":{"message":...,"type":...},"request_id":...}
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
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

// EstimateBilling 按输出秒数计费，参考素材不额外计费（上游对带素材的请求单价更低）。
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
	return fmt.Sprintf("%s/v1/video/generate", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
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
	// 分辨率由对客模型名决定（见 resolutionFromModelName），映射后的上游模型名不带档位后缀，
	// 所以要在取用 UpstreamModelName 之前把 resolution 定下来。
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

	var envelope taskEnvelope
	if err := common.Unmarshal(responseBody, &envelope); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if envelope.Task.ID == "" {
		var errEnvelope errorEnvelope
		_ = common.Unmarshal(responseBody, &errEnvelope)
		message := errEnvelope.Error.Message
		if message == "" {
			message = fmt.Sprintf("task id is empty, body: %s", responseBody)
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
	return envelope.Task.ID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/video/tasks/%s", baseUrl, taskID)

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

// resolutionFromModelName 从对客模型名的档位后缀取分辨率（如 seedance-2.0-mini-480p → 480p）。
// 分辨率决定上游成本（token 数按像素数增长），因此只认模型名，不接受请求参数覆盖 ——
// 否则客户可以在低价档的模型上要高分辨率，把成本抬到售价之上。
func resolutionFromModelName(modelName string) string {
	name := strings.ToLower(strings.TrimSpace(modelName))
	for _, res := range []string{"480p", "720p", "1080p", "4k"} {
		if strings.HasSuffix(name, "-"+res) {
			return res
		}
	}
	return ""
}

func convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{Model: req.Model}

	// 参考素材：上游按 content 数组里的 image_url 项做原生多参考图
	for _, imgURL := range req.ImageValues() {
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
	// 画幅：对外用 aspect_ratio，兼容直接传 ratio 的调用方
	if req.AspectRatio != "" {
		r.Ratio = req.AspectRatio
	} else if req.Ratio != "" {
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
	// 分辨率以模型名档位为准，压过 metadata 里的任何取值
	if res := resolutionFromModelName(req.Model); res != "" {
		r.Resolution = res
	}
	if r.Resolution == "" {
		r.Resolution = defaultResolution
	}
	if r.Ratio == "" {
		r.Ratio = defaultRatio
	}
	return &r, nil
}

// upstreamFailureReason 从上游的 error 字段取失败原因，兼容对象与字符串两种形状。
func upstreamFailureReason(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var asString string
	if err := common.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asObject struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(raw, &asObject); err == nil {
		if asObject.Code != "" && asObject.Message != "" {
			return asObject.Code + ": " + asObject.Message
		}
		if asObject.Message != "" {
			return asObject.Message
		}
	}
	return string(raw)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var envelope taskEnvelope
	if err := common.Unmarshal(respBody, &envelope); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	t := envelope.Task

	taskResult := relaycommon.TaskInfo{Code: 0}
	switch t.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = taskcommon.ProgressQueued
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = taskcommon.ProgressInProgress
	case "completed", "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = taskcommon.ProgressComplete
		if len(t.Outputs) > 0 {
			taskResult.Url = t.Outputs[0]
		}
	case "failed", "cancelled", "canceled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = taskcommon.ProgressComplete
		reason := upstreamFailureReason(t.Error)
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
	var envelope taskEnvelope
	if err := common.Unmarshal(originTask.Data, &envelope); err != nil {
		return nil, errors.Wrap(err, "unmarshal task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	if len(envelope.Task.Outputs) > 0 {
		openAIVideo.SetMetadata("url", envelope.Task.Outputs[0])
	}
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if reason := upstreamFailureReason(envelope.Task.Error); reason != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{Message: reason}
	}

	return common.Marshal(openAIVideo)
}
