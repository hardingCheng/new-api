package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixedVideoModelPriceDefaultsToPerSecondBilling(t *testing.T) {
	givenVideoModes := ratio_setting.VideoBillingMode2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateVideoBillingModeByJSONString(givenVideoModes))
	})
	require.NoError(t, ratio_setting.UpdateVideoBillingModeByJSONString(`{}`))

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("generated_video_seconds", 15)
	info := &relaycommon.RelayInfo{
		OriginModelName: "seedance-2.0-720p",
		PriceData: types.PriceData{
			UsePrice:   true,
			ModelPrice: 0.8,
			Quota:      400000,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
	}
	info.PriceData.AddOtherRatio("seconds", 15)

	require.Equal(t, ratio_setting.VideoBillingModePerSecond, info.VideoBillingMode())
	applyTaskVideoBillingRatios(context, info)

	assert.Equal(t, 6000000, info.PriceData.Quota)
	assert.Equal(t, 15.0, info.PriceData.OtherRatios()["seconds"])
	assert.Equal(t, 15, context.GetInt("billable_video_seconds"))
}

func TestTaskModel2PublicVideoDtoRedactsUpstreamIdentifiers(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusInProgress,
		Properties: model.Properties{
			OriginModelName:   "public-video-model",
			UpstreamModelName: "provider-secret-model",
		},
		Data: []byte(`{
			"id":"upstream-id",
			"task_id":"upstream-task-id",
			"name":"projects/provider-project/locations/us-central1/publishers/google/models/provider-secret-model/operations/upstream-operation",
			"operationName":"projects/provider-project/locations/us-central1/publishers/google/models/provider-secret-model/operations/upstream-operation",
			"model":"provider-secret-model",
			"usage":{"cost":123},
			"response":{
				"id":"nested-upstream-id",
				"task_id":"nested-upstream-task-id",
				"model":"nested-provider-model",
				"status":"processing",
				"usage":{"tokens":99},
				"operation":{
					"name":"projects/provider-project/locations/us-central1/publishers/google/models/nested-provider-model/operations/nested-upstream-operation"
				}
			},
			"result":{"id":"asset-id","name":"final-video.mp4"}
		}`),
	}

	out := TaskModel2PublicVideoDto(task)

	assert.Equal(t, "public-video-model", out.Model)
	assert.Equal(t, "public-video-model", out.ModelName)
	properties, ok := out.Properties.(model.Properties)
	require.True(t, ok)
	assert.Empty(t, properties.UpstreamModelName)
	assert.Equal(t, "public-video-model", properties.OriginModelName)

	var data map[string]any
	require.NoError(t, common.Unmarshal(out.Data, &data))
	assert.Equal(t, "task_public", data["id"])
	assert.Equal(t, "task_public", data["task_id"])
	assert.Equal(t, "task_public", data["name"])
	assert.Equal(t, "task_public", data["operationName"])
	assert.Equal(t, "public-video-model", data["model"])
	assert.NotContains(t, data, "usage")

	response, ok := data["response"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "task_public", response["id"])
	assert.Equal(t, "task_public", response["task_id"])
	assert.Equal(t, "public-video-model", response["model"])
	assert.NotContains(t, response, "usage")
	operation, ok := response["operation"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "task_public", operation["name"])
	assert.NotContains(t, string(out.Data), "provider-project")
	assert.NotContains(t, string(out.Data), "provider-secret-model")
	assert.NotContains(t, string(out.Data), "upstream-operation")

	result, ok := data["result"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "asset-id", result["id"])
	assert.Equal(t, "final-video.mp4", result["name"])
}

// 对齐设计示例：原价每秒 0.1 美元，生成 3 秒，参考 15 秒。
// basePerSec = 0.1 × QuotaPerUnit × groupRatio(1) = 0.1 × 500000 = 50000。
func TestReferenceVideoCost(t *testing.T) {
	const basePerSec = 50000.0 // 0.1 美元/秒 的每秒额度
	const refSec = 15.0
	const size = 1.0

	const groupRatio = 0.8 // VIP 八折

	cases := []struct {
		name  string
		mode  string
		value float64
		apply bool
		want  float64 // 参考部分额度
	}{
		{"无规则全额", ratio_setting.VideoRefModeNone, 0, false, basePerSec * refSec},                                        // 750000
		{"倍率半价", ratio_setting.VideoRefModeFactor, 0.5, false, basePerSec * refSec * 0.5},                               // 375000
		{"倍率免费", ratio_setting.VideoRefModeFactor, 0, false, 0},                                                         // 0
		{"固定单价0.02-不折扣", ratio_setting.VideoRefModePrice, 0.02, false, 0.02 * common.QuotaPerUnit * refSec},             // 150000
		{"固定单价0.02-跟折扣", ratio_setting.VideoRefModePrice, 0.02, true, 0.02 * common.QuotaPerUnit * refSec * groupRatio}, // 120000
		{"整段固定1元-不折扣", ratio_setting.VideoRefModeFlat, 1, false, 1 * common.QuotaPerUnit},                               // 500000
		{"整段固定1元-跟折扣", ratio_setting.VideoRefModeFlat, 1, true, 1 * common.QuotaPerUnit * groupRatio},                   // 400000
		{"封顶5秒", ratio_setting.VideoRefModeCap, 5, false, basePerSec * 5},                                               // 250000
	}
	for _, tc := range cases {
		got := referenceVideoCost(tc.mode, tc.value, refSec, basePerSec, size, groupRatio, tc.apply)
		if got != tc.want {
			t.Fatalf("%s: referenceVideoCost = %v, want %v", tc.name, got, tc.want)
		}
	}

	// 无参考秒：任何模式都不收参考费。
	for _, mode := range []string{
		ratio_setting.VideoRefModeNone, ratio_setting.VideoRefModeFactor,
		ratio_setting.VideoRefModePrice, ratio_setting.VideoRefModeFlat, ratio_setting.VideoRefModeCap,
	} {
		if got := referenceVideoCost(mode, 1, 0, basePerSec, size, groupRatio, true); got != 0 {
			t.Fatalf("无参考秒模式 %q: 应为 0，实际 %v", mode, got)
		}
	}
}

// 锁定总额度（生成 + 参考），并证明「无规则」与旧逻辑（base×(gen+ref)×size）完全一致。
func TestReferenceVideoTotalQuota(t *testing.T) {
	const basePerSec = 50000.0 // 0.1 美元/秒
	const genSec = 3.0
	const refSec = 15.0
	const size = 1.0
	genCost := basePerSec * genSec * size

	total := func(mode string, value float64) int {
		return int(genCost + referenceVideoCost(mode, value, refSec, basePerSec, size, 1.0, false))
	}

	cases := []struct {
		name  string
		mode  string
		value float64
		want  int // 总额度（QuotaPerUnit=500000 => $1=500000）
	}{
		{"无规则=旧逻辑", ratio_setting.VideoRefModeNone, 0, 900000},      // $1.8，等于 base×(3+15)
		{"倍率半价", ratio_setting.VideoRefModeFactor, 0.5, 525000},     // $1.05
		{"倍率免费", ratio_setting.VideoRefModeFactor, 0, 150000},       // $0.3（只收生成3秒）
		{"固定单价0.02", ratio_setting.VideoRefModePrice, 0.02, 300000}, // $0.6
		{"整段固定1元", ratio_setting.VideoRefModeFlat, 1, 650000},       // $1.3
		{"封顶5秒", ratio_setting.VideoRefModeCap, 5, 400000},          // $0.8
	}
	for _, tc := range cases {
		if got := total(tc.mode, tc.value); got != tc.want {
			t.Fatalf("%s: 总额度=%d, 期望=%d", tc.name, got, tc.want)
		}
	}

	// 关键不回归断言：无规则总额度 == 旧逻辑 base×(gen+ref)×size。
	if total(ratio_setting.VideoRefModeNone, 0) != int(basePerSec*(genSec+refSec)*size) {
		t.Fatal("无规则路径与旧计费逻辑不一致")
	}
}
