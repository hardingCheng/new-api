package types

import "fmt"

type GroupRatioInfo struct {
	GroupRatio        float64
	GroupSpecialRatio float64
	HasSpecialRatio   bool
	UserOverrideRatio float64
	HasUserOverride   bool
}

type PriceData struct {
	FreeModel            bool
	ModelPrice           float64
	ModelRatio           float64
	CompletionRatio      float64
	CacheRatio           float64
	CacheCreationRatio   float64
	CacheCreation5mRatio float64
	CacheCreation1hRatio float64
	ImageRatio           float64
	AudioRatio           float64
	AudioCompletionRatio float64
	OtherRatios          map[string]float64
	UsePrice             bool
	Quota                int // 按次计费的最终额度（MJ / Task）
	QuotaToPreConsume    int // 按量计费的预消耗额度
	GroupRatioInfo       GroupRatioInfo
	// VideoRefMode 用户级「参考视频秒数」定价模式：""=无 / factor / price / flat / cap。
	// 仅作用于参考视频那部分秒数，不影响生成秒数。
	VideoRefMode string
	// VideoRefValue 对应模式的数值（倍率 / 每秒单价$ / 整段总价$ / 封顶秒数）。
	VideoRefValue float64
	// VideoRefApplyGroupRatio 仅对 price/flat 生效：参考固定价是否也乘分组倍率（默认 false=绝对值）。
	VideoRefApplyGroupRatio bool
}

func (p *PriceData) AddOtherRatio(key string, ratio float64) {
	if p.OtherRatios == nil {
		p.OtherRatios = make(map[string]float64)
	}
	if ratio <= 0 {
		return
	}
	p.OtherRatios[key] = ratio
}

func (p *PriceData) ToSetting() string {
	return fmt.Sprintf("ModelPrice: %f, ModelRatio: %f, CompletionRatio: %f, CacheRatio: %f, GroupRatio: %f, UsePrice: %t, CacheCreationRatio: %f, CacheCreation5mRatio: %f, CacheCreation1hRatio: %f, QuotaToPreConsume: %d, ImageRatio: %f, AudioRatio: %f, AudioCompletionRatio: %f", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, p.UsePrice, p.CacheCreationRatio, p.CacheCreation5mRatio, p.CacheCreation1hRatio, p.QuotaToPreConsume, p.ImageRatio, p.AudioRatio, p.AudioCompletionRatio)
}
