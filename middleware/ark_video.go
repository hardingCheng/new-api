package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// ArkVideoTasksPath 是火山方舟格式视频任务入口的路径前缀（与方舟官方路径一致，
// 调用方只改 Base URL 即可把请求指到本站）。
const ArkVideoTasksPath = "/api/v3/contents/generations/tasks"

const arkFramesPerSecond = 24

// arkDroppedFields 是方舟特有、本站不转发的字段：回调地址会让上游绕过本站直接回调调用方；
// 档位、有效期、草稿模式只影响上游侧，与本站计费无关；frames 已折算成 duration。
var arkDroppedFields = []string{"callback_url", "service_tier", "execution_expires_after", "draft", "frames"}

var arkResolutionSuffixes = []string{"-480p", "-720p", "-1080p", "-4k"}

// arkTextCommandPattern 匹配旧版方舟写在提示词末尾的文本参数，如 "--duration 5 --ratio 16:9"。
var arkTextCommandPattern = regexp.MustCompile(`\s*--(ratio|rt|duration|dur|resolution|rs|seed|camerafixed|cf|watermark|wm|fps)\s+(\S+)`)

// ArkVideoSubmit 把方舟格式的提交请求体整理成内部视频任务请求：
// prompt 取 content 里的文本项，frames 折算成 duration，裸模型名按 resolution 补上档位后缀，
// 提示词末尾的 "--duration 5" 一类文本参数提升为字段，并去掉不转发的字段。
func ArkVideoSubmit() gin.HandlerFunc {
	return func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyArkVideoAPI, true)
		if !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") {
			abortWithArkError(c, http.StatusBadRequest, "InvalidParameter", "request body must be application/json")
			return
		}
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			abortWithArkError(c, http.StatusBadRequest, "InvalidParameter", "failed to read request body")
			return
		}
		raw, err := storage.Bytes()
		if err != nil {
			abortWithArkError(c, http.StatusBadRequest, "InvalidParameter", "failed to read request body")
			return
		}
		normalized, err := NormalizeArkVideoRequest(raw)
		if err != nil {
			abortWithArkError(c, http.StatusBadRequest, "InvalidParameter", err.Error())
			return
		}
		// 替换缓存的请求体：清掉旧存储，后续 GetBodyStorage 会从新字节重建。
		common.CleanupBodyStorage(c)
		c.Set(common.KeyRequestBody, normalized)
		c.Request.ContentLength = int64(len(normalized))
		c.Next()
	}
}

// ArkVideoFetch 只做标记，让查询响应按方舟形状输出。
func ArkVideoFetch() gin.HandlerFunc {
	return func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyArkVideoAPI, true)
		c.Next()
	}
}

// ArkVideoNotSupported 给方舟有、本站不提供的操作（任务列表、取消）一个方舟形状的明确错误。
func ArkVideoNotSupported() gin.HandlerFunc {
	return func(c *gin.Context) {
		abortWithArkError(c, http.StatusBadRequest, "NotSupported",
			"this operation is not supported; submit with POST "+ArkVideoTasksPath+" and query with GET "+ArkVideoTasksPath+"/{id}")
	}
}

func abortWithArkError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ArkErrorBody(status, code, message))
}

// ArkErrorBody 组装方舟格式的错误体：{"error":{"code","message","param","type"}}。
func ArkErrorBody(status int, code, message string) gin.H {
	errorType := "BadRequest"
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		errorType = "Unauthorized"
	case status == http.StatusNotFound:
		errorType = "NotFound"
	case status == http.StatusTooManyRequests:
		errorType = "TooManyRequests"
	case status >= http.StatusInternalServerError:
		errorType = "InternalServerError"
	}
	return gin.H{"error": gin.H{
		"code":    code,
		"message": message,
		"param":   "",
		"type":    errorType,
	}}
}

// NormalizeArkVideoRequest 是提交体整理的纯函数部分，便于单测。
func NormalizeArkVideoRequest(raw []byte) ([]byte, error) {
	var body map[string]any
	if err := common.Unmarshal(raw, &body); err != nil || body == nil {
		return nil, errors.New("request body must be a JSON object")
	}

	modelName, _ := body["model"].(string)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, errors.New("model is required")
	}

	content, _ := body["content"].([]any)
	prompt, _ := body["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		for _, item := range content {
			if m, ok := item.(map[string]any); ok && m["type"] == "text" {
				if text, ok := m["text"].(string); ok && strings.TrimSpace(text) != "" {
					prompt = text
					break
				}
			}
		}
	}
	prompt, commands := splitArkTextCommands(prompt)
	if prompt == "" {
		return nil, errors.New("content must contain a text item with the prompt")
	}
	body["prompt"] = prompt
	// content 里的文本项同步成清理后的提示词，避免文本参数被当成画面描述转发给上游。
	for _, item := range content {
		if m, ok := item.(map[string]any); ok && m["type"] == "text" {
			m["text"] = prompt
		}
	}
	for key, value := range commands {
		if _, exists := body[key]; !exists {
			body[key] = value
		}
	}

	if resolution, ok := body["resolution"].(string); ok {
		resolution = strings.ToLower(strings.TrimSpace(resolution))
		lowerModel := strings.ToLower(modelName)
		hasSuffix := slices.ContainsFunc(arkResolutionSuffixes, func(suffix string) bool {
			return strings.HasSuffix(lowerModel, suffix)
		})
		if resolution != "" && !hasSuffix {
			modelName += "-" + resolution
		}
	}
	body["model"] = modelName

	// frames 是计费乘数（时长）的另一种写法，先限界再折算，越界直接拒绝。
	if rawFrames, hasFrames := body["frames"]; hasFrames {
		if _, hasDuration := body["duration"]; !hasDuration {
			frames, ok := rawFrames.(float64)
			maxFrames := float64(relaycommon.MaxTaskDurationSeconds*arkFramesPerSecond + 1)
			if !ok || frames != float64(int(frames)) || frames <= 1 || frames > maxFrames {
				return nil, fmt.Errorf("frames must be an integer from %d through %d", arkFramesPerSecond+1, int(maxFrames))
			}
			body["duration"] = (int(frames) - 1 + arkFramesPerSecond/2) / arkFramesPerSecond
		}
	}

	for _, key := range arkDroppedFields {
		delete(body, key)
	}
	return common.Marshal(body)
}

// splitArkTextCommands 把提示词末尾的 "--duration 5 --ratio 16:9" 一类文本参数拆出来，
// 返回清理后的提示词和对应的字段值（duration/seed 为整数，开关为布尔，其余为字符串）。
func splitArkTextCommands(prompt string) (string, map[string]any) {
	commands := map[string]any{}
	matches := arkTextCommandPattern.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return strings.TrimSpace(prompt), commands
	}
	for _, match := range matches {
		key, value := strings.ToLower(match[1]), strings.TrimSpace(match[2])
		switch key {
		case "ratio", "rt":
			commands["ratio"] = value
		case "duration", "dur":
			if seconds, err := strconv.Atoi(value); err == nil {
				commands["duration"] = seconds
			}
		case "resolution", "rs":
			commands["resolution"] = strings.ToLower(value)
		case "seed":
			if seed, err := strconv.Atoi(value); err == nil {
				commands["seed"] = seed
			}
		case "camerafixed", "cf":
			commands["camera_fixed"] = strings.EqualFold(value, "true")
		case "watermark", "wm":
			commands["watermark"] = strings.EqualFold(value, "true")
		}
	}
	return strings.TrimSpace(arkTextCommandPattern.ReplaceAllString(prompt, "")), commands
}
