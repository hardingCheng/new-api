package relay

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// arkDownloadableActions 是成片可经本站下载入口取回的任务类型。
var arkDownloadableActions = []string{
	constant.TaskActionImageToVideo,
	constant.TaskActionTextToVideo,
	constant.TaskActionFirstTailToVideo,
	constant.TaskActionReferenceToVideo,
	constant.TaskActionRemix,
	constant.TaskActionAvatarGenerate,
	constant.TaskActionEnhance,
}

const (
	arkVideoStatusQueued    = "queued"
	arkVideoStatusRunning   = "running"
	arkVideoStatusSucceeded = "succeeded"
	arkVideoStatusFailed    = "failed"
	arkVideoFramesPerSecond = 24
	arkVideoArtifactKey     = "video"
)

// TaskModel2ArkVideoDto 构建火山方舟格式的任务查询响应。
// 成片地址给本站域名下带访问凭证的下载链接（不暴露上游地址、不需要令牌），
// 失败时给方舟形状的 error。
func TaskModel2ArkVideoDto(c *gin.Context, task *model.Task) *dto.ArkVideoTaskDto {
	modelName := strings.TrimSpace(task.Properties.OriginModelName)
	if modelName == "" {
		modelName = taskModelName(task)
	}
	out := &dto.ArkVideoTaskDto{
		ID:         task.TaskID,
		Model:      modelName,
		Status:     arkVideoStatus(task.Status),
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		Duration:   taskVideoDuration(task),
		Resolution: arkResolutionFromModelName(modelName),
	}
	switch task.Status {
	case model.TaskStatusSuccess:
		out.FramesPerSecond = arkVideoFramesPerSecond
		if videoURL := arkVideoContentURL(c, task); videoURL != "" {
			out.Content = &dto.ArkVideoTaskContent{VideoURL: videoURL}
		}
		out.Usage = arkVideoUsage(task.Data)
	case model.TaskStatusFailure:
		message := strings.TrimSpace(task.FailReason)
		if message == "" {
			message = "task failed"
		}
		out.Error = &dto.ArkVideoTaskError{Code: arkFailureCode(message), Message: message}
	}
	return out
}

func arkVideoStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitting, model.TaskStatusSubmitted, model.TaskStatusQueued:
		return arkVideoStatusQueued
	case model.TaskStatusSuccess:
		return arkVideoStatusSucceeded
	case model.TaskStatusFailure:
		return arkVideoStatusFailed
	default:
		return arkVideoStatusRunning
	}
}

// arkVideoContentURL 给成功任务一个不需要令牌的下载链接：后台配置了任务公开地址就用配置的，
// 没配就用本次请求的站点域名（多站共用一个后端，各站回各自的域名）。
func arkVideoContentURL(c *gin.Context, task *model.Task) string {
	if task == nil || task.Status != model.TaskStatusSuccess || strings.TrimSpace(task.GetResultURL()) == "" {
		return ""
	}
	// 与下载入口放行的任务类型保持一致（见 controller 里 legacy 视频的判定），否则给出去的链接打不开。
	if !slices.Contains(arkDownloadableActions, constant.NormalizeTaskAction(task.Action)) {
		return ""
	}
	// 只认专门配置的任务公开地址；站点地址可能还是安装默认值（localhost），不能拿来对外。
	if strings.TrimSpace(system_setting.TaskPublicAddress) != "" {
		if contentURL, err := service.BuildTaskArtifactContentURL(task.TaskID, arkVideoArtifactKey); err == nil {
			return contentURL
		}
	}
	access, err := service.IssueTaskArtifactAccess(task.TaskID, arkVideoArtifactKey)
	if err != nil || c == nil || c.Request == nil {
		return ""
	}
	host := strings.TrimSpace(c.Request.Host)
	if host == "" {
		return ""
	}
	scheme := "https"
	if proto := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))); proto == "http" || proto == "https" {
		scheme = proto
	}
	return fmt.Sprintf("%s://%s/v1/tasks/%s/artifacts/%s/content?%s=%s",
		scheme, host, url.PathEscape(task.TaskID), arkVideoArtifactKey,
		service.TaskArtifactAccessQueryParameter, url.QueryEscape(access))
}

// arkVideoUsage 从上游回写的数据里取 token 用量（按 token 计费的线路才有），没有就不输出。
func arkVideoUsage(data json.RawMessage) *dto.ArkVideoTaskUsage {
	if len(data) == 0 {
		return nil
	}
	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		return nil
	}
	total := intFromAny(usage["total_tokens"])
	completion := intFromAny(usage["completion_tokens"])
	if total <= 0 && completion <= 0 {
		return nil
	}
	if total <= 0 {
		total = completion
	}
	if completion <= 0 {
		completion = total
	}
	return &dto.ArkVideoTaskUsage{CompletionTokens: completion, TotalTokens: total}
}

func arkResolutionFromModelName(modelName string) string {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	for _, resolution := range []string{"480p", "720p", "1080p", "4k"} {
		if strings.HasSuffix(lower, "-"+resolution) {
			return resolution
		}
	}
	return ""
}

// arkFailureCode 按失败原因给一个方舟风格的错误码，调用方据此区分「素材/内容问题」和「服务问题」。
func arkFailureCode(reason string) string {
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "sensitive"), strings.Contains(lower, "privacy"),
		strings.Contains(lower, "real person"), strings.Contains(lower, "审核"), strings.Contains(lower, "敏感"):
		return "SensitiveContentDetected"
	case strings.Contains(lower, "invalidparameter"), strings.Contains(lower, "download"),
		strings.Contains(lower, "image_url"), strings.Contains(lower, "invalid"):
		return "InvalidParameter"
	default:
		return "InternalServiceError"
	}
}
