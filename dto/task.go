package dto

import (
	"encoding/json"

	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
)

type OpenAIVideoError = relaydto.OpenAIVideoError

type TaskError struct {
	// NoRetry prevents duplicate upstream work after a response has been accepted.
	NoRetry    bool   `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Data       any    `json:"data"`
	StatusCode int    `json:"-"`
	LocalError bool   `json:"-"`
	Error      error  `json:"-"`
}

type TaskData interface {
	SunoDataResponse | []SunoDataResponse | string | any
}

const TaskSuccessCode = "success"

type TaskResponse[T TaskData] struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func (t *TaskResponse[T]) IsSuccess() bool {
	return t.Code == TaskSuccessCode
}

type TaskDto struct {
	ID                   int64           `json:"id"`
	CreatedAt            int64           `json:"created_at"`
	UpdatedAt            int64           `json:"updated_at"`
	TaskID               string          `json:"task_id"`
	UpstreamTaskID       string          `json:"upstream_task_id,omitempty"`
	Platform             string          `json:"platform"`
	UserId               int             `json:"user_id"`
	Group                string          `json:"group"`
	ChannelId            int             `json:"channel_id"`
	ChannelName          string          `json:"channel_name,omitempty"`
	Quota                int             `json:"quota"`
	RefundQuota          int             `json:"refund_quota,omitempty"`
	Action               string          `json:"action"`
	Status               string          `json:"status"`
	FailReason           string          `json:"fail_reason"`
	ResultURL            string          `json:"result_url,omitempty"` // 任务结果 URL（视频地址等）
	URL                  string          `json:"url,omitempty"`
	VideoURL             string          `json:"video_url,omitempty"`
	SubmitTime           int64           `json:"submit_time"`
	StartTime            int64           `json:"start_time"`
	FinishTime           int64           `json:"finish_time"`
	Progress             string          `json:"progress"`
	Properties           any             `json:"properties"`
	Username             string          `json:"username,omitempty"`
	ModelName            string          `json:"model_name,omitempty"`
	VideoDuration        int             `json:"video_duration,omitempty"`
	Data                 json.RawMessage `json:"data"`
	Timestamp2String     string          `json:"timestamp2string,omitempty"`
	Key                  string          `json:"key,omitempty"`
	LegacyVideoAvailable bool            `json:"legacy_video_available,omitempty"`
	// ResultDiscarded marks a synchronous result that was returned inline and
	// never persisted; the UI must not offer artifact retrieval for it.
	ResultDiscarded bool           `json:"result_discarded,omitempty"`
	AdminInfo       *TaskAdminInfo `json:"admin_info,omitempty"`
	RootInfo        *TaskRootInfo  `json:"root_info,omitempty"`
}

// VideoTaskPublicDto 是 /v1/videos/{task_id} 对外返回的精简结构，
// 去除了 platform / user_id / group / channel_id / channel_name / quota /
// refund_quota / username / key 等内部计费与归属字段，避免泄露给调用方。
type VideoTaskPublicDto struct {
	// ID 对外返回 task_xxxx 字符串（与 OpenAI Video API 一致），
	// 不再暴露数据库自增主键，避免下游按字符串解析数字 id 失败。
	ID string `json:"id"`
	// Object/Model/CompletedAt/Seconds/Size/Error 为 OpenAI Video API 兼容字段，
	// 让 OpenAI SDK 直连也能正常解析；OpenAI SDK 会忽略下方 new-api 扩展字段。
	Object      string            `json:"object"`
	Model       string            `json:"model,omitempty"`
	CompletedAt int64             `json:"completed_at,omitempty"`
	Seconds     string            `json:"seconds,omitempty"`
	Size        string            `json:"size,omitempty"`
	Error       *OpenAIVideoError `json:"error,omitempty"`
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
	TaskID      string            `json:"task_id"`
	Action      string            `json:"action"`
	Status      string            `json:"status"`
	FailReason  string            `json:"fail_reason"`
	ResultURL   string            `json:"result_url,omitempty"`
	URL         string            `json:"url,omitempty"`
	VideoURL    string            `json:"video_url,omitempty"`
	SubmitTime  int64             `json:"submit_time"`
	StartTime   int64             `json:"start_time"`
	FinishTime  int64             `json:"finish_time"`
	// Progress 输出为数字（0-100），与 OpenAI Video API 一致，
	// 避免下游按 int 解析 "100%" 字符串失败。
	Progress         int             `json:"progress"`
	Properties       any             `json:"properties"`
	ModelName        string          `json:"model_name,omitempty"`
	VideoDuration    int             `json:"video_duration,omitempty"`
	Data             json.RawMessage `json:"data"`
	Timestamp2String string          `json:"timestamp2string,omitempty"`
}

type TaskPluginInfo struct {
	Key     string                `json:"key"`
	Name    string                `json:"name"`
	Version string                `json:"version,omitempty"`
	Author  *TaskPluginAuthorInfo `json:"author,omitempty"`
}

type TaskPluginAuthorInfo struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type TaskPluginRuntimeInfo struct {
	Key        string `json:"key"`
	Version    string `json:"version"`
	APIVersion int    `json:"api_version"`
	Generation uint64 `json:"generation"`
}

type TaskAdminInfo struct {
	RequestID   string          `json:"request_id,omitempty"`
	RequestPath string          `json:"request_path,omitempty"`
	TaskPlugin  *TaskPluginInfo `json:"task_plugin,omitempty"`
}

type TaskRootInfo struct {
	TaskPlugin     *TaskPluginRuntimeInfo `json:"task_plugin,omitempty"`
	UpstreamTaskID string                 `json:"upstream_task_id,omitempty"`
	NodeName       string                 `json:"node_name,omitempty"`
}

type FetchReq struct {
	IDs []string `json:"ids"`
}
