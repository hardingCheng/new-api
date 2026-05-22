package sora

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskbilling"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
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

type responseTask struct {
	ID                 string `json:"id"`
	TaskID             string `json:"task_id,omitempty"` //兼容旧接口
	Object             string `json:"object"`
	Model              string `json:"model"`
	Status             string `json:"status"`
	Progress           int    `json:"progress"`
	CreatedAt          int64  `json:"created_at"`
	CompletedAt        int64  `json:"completed_at,omitempty"`
	ExpiresAt          int64  `json:"expires_at,omitempty"`
	Seconds            string `json:"seconds,omitempty"`
	Size               string `json:"size,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
	Error              *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
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

const defaultSeedance2Duration = 15

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func validateRemixRequest(c *gin.Context) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	if taskErr := relaycommon.NormalizeSeedanceDuration(&req, nil); taskErr != nil {
		return taskErr
	}
	// 存储原始请求到 context，与 ValidateMultipartDirect 路径保持一致
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		if taskErr := validateRemixRequest(c); taskErr != nil {
			return taskErr
		}
		return validateSeedanceReferenceVideoDuration(c, info)
	}
	if taskErr := relaycommon.ValidateMultipartDirect(c, info); taskErr != nil {
		return taskErr
	}
	return validateSeedanceReferenceVideoDuration(c, info)
}

func validateSeedanceReferenceVideoDuration(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	if !taskcommon.IsTaskReferenceVideoBillingRequest(info, req.Model) {
		return nil
	}
	if _, err := taskbilling.ResolveReferenceVideoSeconds(c); err != nil {
		return service.TaskErrorWrapperLocal(err, "reference_video_duration_probe_failed", http.StatusBadRequest)
	}
	return nil
}

// EstimateBilling 根据用户请求的 seconds 和 size 计算 OtherRatios。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// remix 路径的 OtherRatios 已在 ResolveOriginTask 中设置
	if info.Action == constant.TaskActionRemix {
		return nil
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	if !taskcommon.IsTaskDurationBillingRequest(info, req.Model) {
		return nil
	}

	seconds := req.Duration
	if seconds == 0 {
		seconds, _ = strconv.Atoi(strings.TrimSpace(req.Seconds))
	}

	if isSeedanceModel(info.UpstreamModelName) || isSeedanceModel(info.OriginModelName) || isSeedanceModel(req.Model) {
		if seconds <= 0 && isSeedance2Request(info, req.Model) {
			seconds = defaultSeedance2Duration
		}
		if seconds <= 0 {
			return nil
		}
		ratios := map[string]float64{
			"seconds": float64(seconds),
		}
		if taskcommon.IsTaskReferenceVideoBillingRequest(info, req.Model) {
			referenceSeconds := taskbilling.EstimateReferenceVideoSeconds(c)
			ratios["seconds"] = float64(seconds + referenceSeconds)
		}
		return ratios
	}

	if seconds <= 0 {
		seconds = 4
	}

	size := req.Size
	if size == "" {
		size = "720x1280"
	}

	ratios := map[string]float64{
		"seconds": float64(seconds),
		"size":    1,
	}
	if taskcommon.IsTaskReferenceVideoBillingRequest(info, req.Model) {
		referenceSeconds := taskbilling.EstimateReferenceVideoSeconds(c)
		ratios["seconds"] = float64(seconds + referenceSeconds)
	}
	if size == "1792x1024" || size == "1024x1792" {
		ratios["size"] = 1.666667
	}
	return ratios
}

func isSeedanceModel(modelName string) bool {
	return relaycommon.IsSeedanceTaskModel(modelName)
}

func isSeedance2Request(info *relaycommon.RelayInfo, requestModel string) bool {
	if taskcommon.IsSeedance2Model(requestModel) {
		return true
	}
	if info == nil {
		return false
	}
	return taskcommon.IsSeedance2Model(info.UpstreamModelName) ||
		taskcommon.IsSeedance2Model(info.OriginModelName)
}

func parseIntValue(v interface{}) (int, bool) {
	switch value := v.(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(value))
		return n, err == nil
	}
	return 0, false
}

func normalizeSeedanceDuration(bodyMap map[string]interface{}, defaultDuration int) {
	if bodyMap == nil {
		return
	}
	if duration, ok := parseIntValue(bodyMap["duration"]); ok {
		bodyMap["duration"] = relaycommon.ClampSeedanceDuration(duration)
		delete(bodyMap, "seconds")
		return
	}
	if seconds, ok := parseIntValue(bodyMap["seconds"]); ok {
		bodyMap["duration"] = relaycommon.ClampSeedanceDuration(seconds)
	} else if defaultDuration > 0 {
		bodyMap["duration"] = defaultDuration
	}
	delete(bodyMap, "seconds")
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
			if isSeedanceModel(info.UpstreamModelName) || isSeedanceModel(info.OriginModelName) {
				if taskReq, err := relaycommon.GetTaskRequest(c); err == nil && taskReq.Duration > 0 {
					bodyMap["duration"] = taskReq.Duration
					delete(bodyMap, "seconds")
				} else {
					defaultDuration := 0
					if isSeedance2Request(info, "") {
						defaultDuration = defaultSeedance2Duration
					}
					normalizeSeedanceDuration(bodyMap, defaultDuration)
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
		seedanceDuration := ""
		if isSeedanceModel(info.UpstreamModelName) || isSeedanceModel(info.OriginModelName) {
			if taskReq, err := relaycommon.GetTaskRequest(c); err == nil && taskReq.Duration > 0 {
				seedanceDuration = strconv.Itoa(taskReq.Duration)
			} else if values := formData.Value["duration"]; len(values) > 0 && strings.TrimSpace(values[0]) != "" {
				if duration, err := strconv.Atoi(strings.TrimSpace(values[0])); err == nil {
					seedanceDuration = strconv.Itoa(relaycommon.ClampSeedanceDuration(duration))
				}
			} else if values := formData.Value["seconds"]; len(values) > 0 && strings.TrimSpace(values[0]) != "" {
				if duration, err := strconv.Atoi(strings.TrimSpace(values[0])); err == nil {
					seedanceDuration = strconv.Itoa(relaycommon.ClampSeedanceDuration(duration))
				}
			} else if isSeedance2Request(info, "") {
				seedanceDuration = strconv.Itoa(defaultSeedance2Duration)
			}
		}
		for key, values := range formData.Value {
			if key == "model" {
				continue
			}
			if seedanceDuration != "" && (key == "seconds" || key == "duration") {
				continue
			}
			for _, v := range values {
				writer.WriteField(key, v)
			}
		}
		if seedanceDuration != "" {
			writer.WriteField("duration", seedanceDuration)
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

	return common.ReaderOnly(storage), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
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

	upstreamID := dResp.ID
	if upstreamID == "" {
		upstreamID = dResp.TaskID
	}
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 使用公开 task_xxxx ID 返回给客户端
	dResp.ID = info.PublicTaskID
	dResp.TaskID = info.PublicTaskID
	c.JSON(http.StatusOK, dResp)
	return upstreamID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
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

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
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
	if data, err = sjson.SetBytes(data, "id", task.TaskID); err != nil {
		return nil, errors.Wrap(err, "set id failed")
	}
	return data, nil
}
