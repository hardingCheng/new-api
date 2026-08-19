package common

import "sync/atomic"

// 零完成不计费：生成类请求上游故障且零补全输出时不收费（预扣费自动退回）。
// 只豁免上游侧故障——客户端主动断开仍按原口径计费，
// 防止「发大 prompt 后立刻断开」白嫖上游成本。默认关闭。
var zeroCompletionNoChargeFlag atomic.Bool

func IsZeroCompletionNoChargeEnabled() bool {
	return zeroCompletionNoChargeFlag.Load()
}

func SetZeroCompletionNoChargeEnabled(enabled bool) {
	zeroCompletionNoChargeFlag.Store(enabled)
}
