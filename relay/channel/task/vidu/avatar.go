package vidu

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"unicode"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/pkg/errors"
)

// 数字人（非实时 · 视频文件版）：一张人像图 + 一段音频或一段文本 → 人像开口说话的视频。
//
// 上游的提交端点与常规视频生成不同（/ent/v2/avatar-video/generations），请求体也另一套
// （单张 image、audio/text 二选一），但任务轮询与结果解析共用同一套
// （/ent/v2/tasks/{id}/creations），所以这里只补提交这一段。
//
// 上游另有一条「实时交互」产品线，走 WebSocket 控制链 + 阿里 RTC 媒体流，
// 音视频不经过本服务，无法按量计费与限流，不在此适配。

const (
	// avatarModelPrefix 是数字人系列的对客模型名前缀（如 vidu-s1-540p）。
	avatarModelPrefix = "vidu-s1"
	// avatarDefaultResolution 是模型名未带档位后缀时的兜底分辨率。
	avatarDefaultResolution = "540p"
	// avatarMaxTextRunes 是上游对讲话文本的长度上限。
	avatarMaxTextRunes = 10000
)

// avatarResolutions 是上游支持的分辨率档位，也是模型名允许的后缀。
var avatarResolutions = []string{"540p", "720p"}

type avatarRequestPayload struct {
	Model       string `json:"model"`
	Image       string `json:"image"`
	Audio       string `json:"audio,omitempty"`
	Text        string `json:"text,omitempty"`
	Voice       string `json:"voice,omitempty"`
	Timeline    string `json:"timeline,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
	CallbackUrl string `json:"callback_url,omitempty"`
}

// avatarSubmitResponse 只取提交响应里计费要用的部分。
type avatarSubmitResponse struct {
	TaskId  string `json:"task_id"`
	State   string `json:"state"`
	Credits int    `json:"credits"`
}

// IsAvatarModel 判断对客模型名是否走数字人链路。
func IsAvatarModel(modelName string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelName)), avatarModelPrefix)
}

// avatarResolutionFromModelName 从对客模型名的档位后缀取分辨率（vidu-s1-720p → 720p）。
// 分辨率决定上游单价，因此只认模型名，不接受请求参数覆盖 —— 否则客户可以在低价档的
// 模型上要高分辨率，把成本抬到售价之上。
func avatarResolutionFromModelName(modelName string) string {
	name := strings.ToLower(strings.TrimSpace(modelName))
	for _, res := range avatarResolutions {
		if strings.HasSuffix(name, "-"+res) {
			return res
		}
	}
	return ""
}

// avatarUpstreamModelName 把对客模型名还原成上游认的模型名：上游只认不带档位后缀的
// vidu-s1，档位通过 resolution 字段传。
func avatarUpstreamModelName(modelName string) string {
	name := strings.TrimSpace(modelName)
	for _, res := range avatarResolutions {
		if strings.HasSuffix(strings.ToLower(name), "-"+res) {
			return name[:len(name)-len(res)-1]
		}
	}
	return name
}

// avatarAudioInput 取请求里的音频输入。上游 audio 优先于 text，这里保持一致。
func avatarAudioInput(req *relaycommon.TaskSubmitReq) string {
	if req == nil || req.Metadata == nil {
		return ""
	}
	audio, _ := req.Metadata["audio"].(string)
	return strings.TrimSpace(audio)
}

// estimateSpeechSeconds 按文本估算配音时长（秒，向上取整）。
//
// 文本模式下上游合成完才知道真实时长，而预扣费必须发生在提交之前，所以这里先估一个：
// 中日韩按字数（约 4 字/秒），拉丁字母按词数（约 2.5 词/秒）。宁可估高不估低 ——
// 估高了提交后会按上游返回的实际用量调低，估低了则会预扣不足。
func estimateSpeechSeconds(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}

	cjkCount := 0
	latinWords := 0
	inLatinWord := false

	for _, r := range text {
		switch {
		case isCJK(r):
			cjkCount++
			inLatinWord = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inLatinWord {
				latinWords++
				inLatinWord = true
			}
		default:
			inLatinWord = false
		}
	}

	seconds := float64(cjkCount)/4.0 + float64(latinWords)/2.5
	if seconds <= 0 {
		return 0
	}
	return avatarBillableSeconds(int(math.Ceil(seconds)))
}

// avatarBillableSeconds 把秒数收敛到计费可接受的区间。无论秒数来自文本估算、音频文件
// 头部还是上游返回，都是用户或上游可控的数字，直接拿去当计费乘数有溢出风险。
func avatarBillableSeconds(seconds int) int {
	if seconds <= 0 {
		return 0
	}
	if seconds > relaycommon.MaxTaskDurationSeconds {
		return relaycommon.MaxTaskDurationSeconds
	}
	return seconds
}

func isCJK(r rune) bool {
	switch {
	case r >= 0x3040 && r <= 0x30FF: // 日文假名
		return true
	case r >= 0x3400 && r <= 0x4DBF: // 汉字扩展 A
		return true
	case r >= 0x4E00 && r <= 0x9FFF: // 汉字基本区
		return true
	case r >= 0xAC00 && r <= 0xD7AF: // 韩文音节
		return true
	case r >= 0xF900 && r <= 0xFAFF: // 汉字兼容
		return true
	}
	return false
}

// validateAvatarRequest 校验数字人提交的必填项。上游对这三项的要求是硬性的，
// 在本地拦掉可以避免一次无谓的上游往返，也避免文本超长的请求进入计费估算。
func validateAvatarRequest(req *relaycommon.TaskSubmitReq) *taskdto.TaskError {
	if len(req.ImageValues()) == 0 && strings.TrimSpace(req.Image) == "" {
		return service.TaskErrorWrapperLocal(errors.New("image is required for avatar video"), "invalid_request", http.StatusBadRequest)
	}
	if avatarAudioInput(req) == "" && strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(errors.New("either audio or prompt is required for avatar video"), "invalid_request", http.StatusBadRequest)
	}
	if n := len([]rune(req.Prompt)); n > avatarMaxTextRunes {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("prompt must not exceed %d characters", avatarMaxTextRunes),
			"invalid_request", http.StatusBadRequest)
	}
	return nil
}

// convertToAvatarPayload 把对客请求翻译成上游数字人的请求体。
func convertToAvatarPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) *avatarRequestPayload {
	image := strings.TrimSpace(req.Image)
	if images := req.ImageValues(); len(images) > 0 {
		image = images[0]
	}

	r := avatarRequestPayload{Image: image}
	// metadata 承载 audio / voice / timeline 等上游独有字段
	_ = taskcommon.UnmarshalMetadata(req.Metadata, &r)
	if strings.TrimSpace(r.Image) == "" {
		r.Image = image
	}
	// 讲话内容一律以 prompt 为准；上游 audio 优先于 text，保持不变透传
	if text := strings.TrimSpace(req.Prompt); text != "" {
		r.Text = text
	}

	// 上游只认不带档位后缀的模型名，档位走 resolution 字段
	r.Model = avatarUpstreamModelName(req.Model)
	if info != nil && info.IsModelMapped && info.UpstreamModelName != "" {
		r.Model = info.UpstreamModelName
	}
	// 分辨率以模型名档位为准，压过 metadata 里的任何取值
	if res := avatarResolutionFromModelName(req.Model); res != "" {
		r.Resolution = res
	}
	if r.Resolution == "" {
		r.Resolution = avatarDefaultResolution
	}
	return &r
}
