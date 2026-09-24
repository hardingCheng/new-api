package vidu

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func avatarTestContext(t *testing.T, body string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "vidu-s1-540p",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeVidu,
			ChannelBaseUrl: "https://api.vidu.cn",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	return c, info
}

func avatarPayloadFrom(t *testing.T, body io.Reader) avatarRequestPayload {
	t.Helper()
	raw, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload avatarRequestPayload
	require.NoError(t, common.Unmarshal(raw, &payload))
	return payload
}

// 数字人模型走上游独立的提交端点，字段按上游形状翻译。
func TestAvatarRequestRoutingAndPayload(t *testing.T) {
	c, info := avatarTestContext(t, `{
		"model":"vidu-s1-540p",
		"image":"https://example.com/portrait.png",
		"prompt":"大家好，欢迎光临",
		"metadata":{"voice":"voice_42","timeline":"{\"actions\":[]}"}
	}`)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	assert.Equal(t, constant.TaskActionAvatarGenerate, info.Action)

	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.vidu.cn/ent/v2/avatar-video/generations", url)

	reqBody, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := avatarPayloadFrom(t, reqBody)

	assert.Equal(t, "vidu-s1", payload.Model, "上游只认不带档位后缀的模型名")
	assert.Equal(t, "https://example.com/portrait.png", payload.Image)
	assert.Equal(t, "大家好，欢迎光临", payload.Text)
	assert.Equal(t, "voice_42", payload.Voice)
	assert.Equal(t, `{"actions":[]}`, payload.Timeline)
	assert.Equal(t, "540p", payload.Resolution)
}

// 分辨率决定上游单价，只能由对客模型名的档位决定：
// 客户在 540p 档的模型上塞更高分辨率必须无效，否则成本会被抬到售价之上。
func TestAvatarResolutionLockedByModelName(t *testing.T) {
	c, info := avatarTestContext(t, `{
		"model":"vidu-s1-540p",
		"image":"https://example.com/portrait.png",
		"prompt":"你好",
		"metadata":{"resolution":"720p"}
	}`)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	reqBody, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	assert.Equal(t, "540p", avatarPayloadFrom(t, reqBody).Resolution)
}

// 给了音频就不该再要求提示词：按音频对口型的请求本来就没有文本。
func TestAvatarAudioOnlyRequestNeedsNoPrompt(t *testing.T) {
	c, info := avatarTestContext(t, `{
		"model":"vidu-s1-720p",
		"image":"https://example.com/portrait.png",
		"metadata":{"audio":"https://example.com/speech.mp3"}
	}`)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	reqBody, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)

	payload := avatarPayloadFrom(t, reqBody)
	assert.Equal(t, "https://example.com/speech.mp3", payload.Audio)
	assert.Empty(t, payload.Text)
	assert.Equal(t, "720p", payload.Resolution)
}

// 人像图和讲话内容都是上游硬性要求，缺了要在本地就拦掉。
func TestAvatarRequestRejectsMissingInputs(t *testing.T) {
	adaptor := &TaskAdaptor{}

	c, info := avatarTestContext(t, `{"model":"vidu-s1-540p","prompt":"你好"}`)
	adaptor.Init(info)
	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr, "缺人像图必须拒绝")
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)

	c, info = avatarTestContext(t, `{"model":"vidu-s1-540p","image":"https://example.com/portrait.png"}`)
	adaptor.Init(info)
	taskErr = adaptor.ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr, "音频和文本都缺必须拒绝")
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

// 常规视频生成的四个端点不受数字人分流影响。
func TestRegularViduRoutingUnaffected(t *testing.T) {
	c, info := avatarTestContext(t, `{"model":"viduq1","images":["https://example.com/a.png"],"prompt":"a fox"}`)
	info.OriginModelName = "viduq1"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	assert.Equal(t, constant.TaskActionImageToVideo, info.Action)

	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.vidu.cn/ent/v2/img2video", url)
}

// 文本模式下的预扣秒数：宁可估高不估低，估低了会预扣不足。
func TestEstimateSpeechSeconds(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int
	}{
		{"空文本", "   ", 0},
		{"中文按字数", "大家好欢迎光临本店", 3},             // 9 字 / 4 = 2.25 → 3
		{"英文按词数", "hello there my friend", 2}, // 4 词 / 2.5 = 1.6 → 2
		{"中英混合", "你好 world", 1},               // 2/4 + 1/2.5 = 0.9 → 1
		{"标点不计入", "你好，。！你好", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, estimateSpeechSeconds(tc.text))
		})
	}
}

// 计费秒数必须有上界：文本估算和上游返回都是不可信输入。
func TestAvatarBillableSecondsBounded(t *testing.T) {
	assert.Equal(t, 0, avatarBillableSeconds(0))
	assert.Equal(t, 0, avatarBillableSeconds(-5))
	assert.Equal(t, 42, avatarBillableSeconds(42))
	assert.Equal(t, relaycommon.MaxTaskDurationSeconds, avatarBillableSeconds(relaycommon.MaxTaskDurationSeconds+1))
	assert.Equal(t, relaycommon.MaxTaskDurationSeconds, avatarBillableSeconds(1<<30))
}

// 文本模式的预扣：按估算秒数出 ratio，并落下按秒计费所需的上下文。
func TestEstimateBillingTextMode(t *testing.T) {
	c, info := avatarTestContext(t, `{
		"model":"vidu-s1-540p",
		"image":"https://example.com/portrait.png",
		"prompt":"大家好欢迎光临"
	}`)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	ratios, err := adaptor.EstimateBillingValidated(c, info)
	require.NoError(t, err)
	require.NotNil(t, ratios)
	assert.Equal(t, float64(2), ratios["seconds"]) // 7 字 / 4 = 1.75 → 2
	assert.Equal(t, 2, c.GetInt("generated_video_seconds"))
}

// 常规视频生成不经过数字人的计费估算。
func TestEstimateBillingSkipsNonAvatarAction(t *testing.T) {
	c, info := avatarTestContext(t, `{"model":"viduq1","images":["https://example.com/a.png"],"prompt":"a fox"}`)
	info.OriginModelName = "viduq1"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	ratios, err := adaptor.EstimateBillingValidated(c, info)
	require.NoError(t, err)
	assert.Nil(t, ratios)
}

// 上游返回的是积分消耗，必须按换算还原成秒；换算未确认时一律不动计费，
// 否则会拿没核对过的倍数多扣客户。
func TestAdjustBillingOnSubmitConvertsUpstreamCredits(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionAvatarGenerate},
	}

	// 换算未确认（默认 0）：保持预扣的估算秒数不变
	require.Zero(t, avatarCreditsPerSecond, "换算未经实测确认前必须保持为 0")
	assert.Nil(t, adaptor.AdjustBillingOnSubmit(info, []byte(`{"task_id":"1","credits":30}`)))

	// 换算确认后：积分按每秒消耗还原成秒
	original := avatarCreditsPerSecond
	avatarCreditsPerSecond = 4
	defer func() { avatarCreditsPerSecond = original }()

	ratios := adaptor.AdjustBillingOnSubmit(info, []byte(`{"task_id":"1","state":"created","credits":30}`))
	require.NotNil(t, ratios)
	assert.Equal(t, float64(8), ratios["seconds"], "30 积分 / 4 每秒 = 7.5 → 向上取整 8 秒")

	// 上游返回的数字同样不可信，必须收敛到上界
	ratios = adaptor.AdjustBillingOnSubmit(info, []byte(`{"task_id":"1","credits":999999999}`))
	require.NotNil(t, ratios)
	assert.Equal(t, float64(relaycommon.MaxTaskDurationSeconds), ratios["seconds"])

	// 缺字段、非数字人任务都不得改动计费
	assert.Nil(t, adaptor.AdjustBillingOnSubmit(info, []byte(`{"task_id":"1"}`)))
	assert.Nil(t, adaptor.AdjustBillingOnSubmit(info, []byte(`not json`)))
	assert.Nil(t, adaptor.AdjustBillingOnSubmit(
		&relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionImageToVideo}},
		[]byte(`{"credits":30}`)))
}
