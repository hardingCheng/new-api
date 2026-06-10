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
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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

func validateRemixRequest(c *gin.Context) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	// 存储原始请求到 context，与 ValidateMultipartDirect 路径保持一致
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		return validateRemixRequest(c)
	}
	return relaycommon.ValidateMultipartDirect(c, info)
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

	seconds := relaycommon.EffectiveTaskDuration(req)
	if seconds <= 0 {
		seconds = 4
	}
	referenceSeconds := 0
	if relaycommon.IsSeedanceVideoModel(req.Model) || relaycommon.IsSeedanceVideoModel(info.OriginModelName) {
		referenceSeconds = service.SumReferenceVideoDurationSeconds(c, relaycommon.ExtractReferenceVideoURLs(req))
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
	return ratios
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
			if req, err := relaycommon.GetTaskRequest(c); err == nil && relaycommon.EffectiveTaskDuration(req) > 0 {
				bodyMap["seconds"] = req.Seconds
				if _, exists := bodyMap["duration"]; exists {
					bodyMap["duration"] = req.Duration
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
		hasDuration := relaycommon.EffectiveTaskDuration(taskReq) > 0
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
			writer.WriteField("seconds", taskReq.Seconds)
			if _, exists := formData.Value["duration"]; exists {
				writer.WriteField("duration", strconv.Itoa(taskReq.Duration))
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

	upstreamID := string(dResp.ID)
	if upstreamID == "" {
		upstreamID = dResp.TaskID
	}
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 使用公开 task_xxxx ID 返回给客户端
	dResp.ID = flexString(info.PublicTaskID)
	dResp.TaskID = info.PublicTaskID
	// 用对外模型名覆盖上游真实模型名，避免泄露映射后的上游模型（如 wp/seedance-2.0-fast-480p）
	if info.OriginModelName != "" {
		dResp.Model = info.OriginModelName
	}
	if referenceSeconds := c.GetInt("reference_video_seconds"); referenceSeconds > 0 {
		dResp.ReferenceVideoSeconds = referenceSeconds
	}
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
		if data, err = sjson.SetBytes(data, "status", dto.VideoStatusCompleted); err != nil {
			return nil, errors.Wrap(err, "set status failed")
		}
		if data, err = sjson.SetBytes(data, "progress", 100); err != nil {
			return nil, errors.Wrap(err, "set progress failed")
		}
	case model.TaskStatusFailure:
		if data, err = sjson.SetBytes(data, "status", dto.VideoStatusFailed); err != nil {
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

	return data, nil
}
