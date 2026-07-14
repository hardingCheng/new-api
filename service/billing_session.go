package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// BillingSession — 统一计费会话
// ---------------------------------------------------------------------------

// BillingSession 封装单次请求的预扣费/结算/退款生命周期。
// 实现 relaycommon.BillingSettler 接口。
type BillingSession struct {
	relayInfo        *relaycommon.RelayInfo
	funding          FundingSource
	preConsumedQuota int   // 实际预扣额度（信任用户可能为 0）
	tokenConsumed    int   // 令牌额度实际扣减量
	extraReserved    int   // 发送前补充预扣的额度（订阅退款时需要单独回滚）
	adjustmentID     int64 // 预扣时原子创建的持久计费生命周期记录
	trusted          bool  // 是否命中信任额度旁路
	fundingSettled   bool  // funding.Settle 已成功，资金来源已提交
	settled          bool  // Settle 全部完成（资金 + 令牌）
	refunded         bool  // Refund 已调用
	mu               sync.Mutex
}

// Settle 根据实际消耗额度进行结算。
// 资金来源和令牌额度由同一条持久调整记录原子结算。
func (s *BillingSession) Settle(actualQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled {
		return nil
	}
	delta := actualQuota - s.preConsumedQuota
	requestID := strings.TrimSpace(s.relayInfo.RequestId)
	if requestID == "" {
		requestID = common.NewRequestId()
		s.relayInfo.RequestId = requestID
	}
	tokenDelta := delta
	if s.relayInfo.IsPlayground {
		tokenDelta = 0
	}
	var adjustment *model.BillingAdjustment
	var err error
	if s.adjustmentID > 0 {
		for attempt := 0; attempt < 3; attempt++ {
			adjustment, err = model.FinalizeBillingReservation(s.adjustmentID, actualQuota, !s.relayInfo.IsPlayground, false)
			if err == nil {
				break
			}
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
			}
		}
	} else {
		params := model.BillingAdjustmentParams{
			AdjustmentKey:  "relay-settle:" + requestID,
			UserID:         s.relayInfo.UserId,
			TokenID:        s.relayInfo.TokenId,
			SubscriptionID: s.relayInfo.SubscriptionId,
			FundingSource:  s.funding.Source(),
			FundingDelta:   delta,
			TokenDelta:     tokenDelta,
		}
		for attempt := 0; attempt < 3; attempt++ {
			adjustment, err = model.EnsureBillingAdjustment(params)
			if err == nil {
				break
			}
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
			}
		}
	}
	if err != nil {
		s.relayInfo.BillingSettlementPending = true
		s.relayInfo.BillingSettlementError = err.Error()
		return err
	}
	// The final outcome is durable before it is applied. From here on the
	// reconciler owns retries, so request failure cleanup must not refund again.
	s.fundingSettled = true
	s.settled = true
	if err = model.ApplyBillingAdjustment(adjustment.ID); err != nil {
		s.relayInfo.BillingSettlementPending = true
		s.relayInfo.BillingSettlementError = err.Error()
		// The adjustment is durable and will be retried by billing_reconcile.
		// Treat the request lifecycle as settled so failure defers cannot refund
		// the pre-consume while the queued delta is still pending.
		return nil
	}
	s.relayInfo.BillingSettlementPending = false
	s.relayInfo.BillingSettlementError = ""
	// 3) 更新 relayInfo 上的订阅 PostDelta（用于日志）
	if s.funding.Source() == BillingSourceSubscription {
		s.relayInfo.SubscriptionPostDelta += int64(delta)
	}
	return nil
}

// Refund 退还所有预扣费。调整记录先持久化，实际退款失败时由后台重试。
func (s *BillingSession) Refund(c *gin.Context) error {
	s.mu.Lock()
	if s.settled || s.refunded || !s.needsRefundLocked() {
		s.mu.Unlock()
		return nil
	}
	requestID := strings.TrimSpace(s.relayInfo.RequestId)
	if requestID == "" {
		requestID = common.NewRequestId()
		s.relayInfo.RequestId = requestID
	}
	tokenDelta := -s.tokenConsumed
	if s.relayInfo.IsPlayground {
		tokenDelta = 0
	}
	var adjustment *model.BillingAdjustment
	var err error
	if s.adjustmentID > 0 {
		for attempt := 0; attempt < 3; attempt++ {
			adjustment, err = model.FinalizeBillingReservation(s.adjustmentID, 0, !s.relayInfo.IsPlayground, true)
			if err == nil {
				break
			}
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
			}
		}
	} else {
		params := model.BillingAdjustmentParams{
			AdjustmentKey:  "relay-refund:" + requestID,
			UserID:         s.relayInfo.UserId,
			TokenID:        s.relayInfo.TokenId,
			SubscriptionID: s.relayInfo.SubscriptionId,
			FundingSource:  s.funding.Source(),
			FundingDelta:   -s.preConsumedQuota,
			TokenDelta:     tokenDelta,
		}
		if subscription, ok := s.funding.(*SubscriptionFunding); ok {
			params.SubscriptionPreConsumeRequestID = subscription.requestId
			params.SubscriptionPreConsumed = int(subscription.preConsumed)
			params.SubscriptionExtraReserved = s.extraReserved
			params.FundingDelta = -int(subscription.preConsumed) - s.extraReserved
		}
		for attempt := 0; attempt < 3; attempt++ {
			adjustment, err = model.EnsureBillingAdjustment(params)
			if err == nil {
				break
			}
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
			}
		}
	}
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.refunded = true
	s.mu.Unlock()

	logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费（token_quota=%s, funding=%s）",
		s.relayInfo.UserId, logger.FormatQuota(s.tokenConsumed), s.funding.Source()))
	if err := model.ApplyBillingAdjustment(adjustment.ID); err != nil {
		s.relayInfo.BillingSettlementPending = true
		s.relayInfo.BillingSettlementError = err.Error()
		logger.LogWarn(c, "请求退款已进入后台重试: "+err.Error())
	}
	return nil
}

// NeedsRefund 返回是否存在需要退还的预扣状态。
func (s *BillingSession) NeedsRefund() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsRefundLocked()
}

func (s *BillingSession) needsRefundLocked() bool {
	if s.settled || s.refunded || s.fundingSettled {
		// fundingSettled 时资金来源已提交结算，不能再退预扣费
		return false
	}
	if s.tokenConsumed > 0 {
		return true
	}
	// 订阅可能在 tokenConsumed=0 时仍预扣了额度
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub.preConsumed > 0 {
		return true
	}
	return false
}

// GetPreConsumedQuota 返回实际预扣的额度。
func (s *BillingSession) GetPreConsumedQuota() int {
	return s.preConsumedQuota
}

func (s *BillingSession) Reserve(targetQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.settled || s.refunded || s.trusted || targetQuota <= s.preConsumedQuota {
		return nil
	}

	delta := targetQuota - s.preConsumedQuota
	if delta <= 0 {
		return nil
	}

	if s.adjustmentID <= 0 {
		return errors.New("billing reservation is missing")
	}
	if _, err := model.ExtendBillingReservation(s.adjustmentID, delta, !s.relayInfo.IsPlayground, s.relayInfo.TokenUnlimited, s.relayInfo.TokenKey); err != nil {
		return err
	}

	s.preConsumedQuota += delta
	if !s.relayInfo.IsPlayground {
		s.tokenConsumed += delta
	}
	s.extraReserved += delta
	s.syncRelayInfo()
	return nil
}

// ---------------------------------------------------------------------------
// PreConsume — 统一预扣费入口（含信任额度旁路）
// ---------------------------------------------------------------------------

// preConsume atomically persists the recovery action and reserves token plus
// wallet/subscription quota before the upstream request starts.
func (s *BillingSession) preConsume(c *gin.Context, quota int) *types.NewAPIError {
	effectiveQuota := quota

	// ---- 信任额度旁路 ----
	if s.shouldTrust(c) {
		s.trusted = true
		effectiveQuota = 0
		logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足, 信任且不需要预扣费 (funding=%s)", s.relayInfo.UserId, s.funding.Source()))
	} else if effectiveQuota > 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 需要预扣费 %s (funding=%s)", s.relayInfo.UserId, logger.FormatQuota(effectiveQuota), s.funding.Source()))
	}

	requestID := strings.TrimSpace(s.relayInfo.RequestId)
	if requestID == "" {
		requestID = common.NewRequestId()
		s.relayInfo.RequestId = requestID
	}
	params := model.BillingReservationParams{
		RequestID:             requestID,
		UserID:                s.relayInfo.UserId,
		TokenID:               s.relayInfo.TokenId,
		TokenKey:              s.relayInfo.TokenKey,
		TokenUnlimited:        s.relayInfo.TokenUnlimited,
		TokenBillingEnabled:   !s.relayInfo.IsPlayground,
		FundingSource:         s.funding.Source(),
		Amount:                effectiveQuota,
		SubscriptionModelName: s.relayInfo.OriginModelName,
	}
	result, err := model.ReserveBillingQuota(params)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no active subscription") || strings.Contains(errMsg, "subscription quota insufficient") {
			return types.NewErrorWithStatusCode(fmt.Errorf("订阅额度不足或未配置订阅: %s", errMsg), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if strings.Contains(errMsg, "user quota is not enough") {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if strings.Contains(errMsg, "token quota is not enough") {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	if result == nil || result.Adjustment == nil {
		return types.NewError(errors.New("billing reservation result is missing"), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	s.adjustmentID = result.Adjustment.ID
	s.preConsumedQuota = effectiveQuota
	if !s.relayInfo.IsPlayground {
		s.tokenConsumed = effectiveQuota
	}
	if subscription, ok := s.funding.(*SubscriptionFunding); ok && result.Subscription != nil {
		subscription.subscriptionId = result.Subscription.UserSubscriptionId
		subscription.preConsumed = result.Subscription.PreConsumed
		subscription.AmountTotal = result.Subscription.AmountTotal
		subscription.AmountUsedAfter = result.Subscription.AmountUsedAfter
		if planInfo, planErr := model.GetSubscriptionPlanInfoByUserSubscriptionId(result.Subscription.UserSubscriptionId); planErr == nil && planInfo != nil {
			subscription.PlanId = planInfo.PlanId
			subscription.PlanTitle = planInfo.PlanTitle
		}
	}

	// ---- 同步 RelayInfo 兼容字段 ----
	s.syncRelayInfo()

	return nil
}

// shouldTrust 统一信任额度检查，适用于钱包和订阅。
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 异步任务（ForcePreConsume=true）必须预扣全额，不允许信任旁路
	if s.relayInfo.ForcePreConsume {
		return false
	}

	trustQuota := common.GetTrustQuota()
	if trustQuota <= 0 {
		return false
	}

	// 检查令牌是否充足
	tokenTrusted := s.relayInfo.TokenUnlimited
	if !tokenTrusted {
		tokenQuota := c.GetInt("token_quota")
		tokenTrusted = tokenQuota > trustQuota
	}
	if !tokenTrusted {
		return false
	}

	switch s.funding.Source() {
	case BillingSourceWallet:
		return s.relayInfo.UserQuota > trustQuota
	case BillingSourceSubscription:
		// 订阅不能启用信任旁路。原因：
		// 订阅预扣记录要求正数额度，不能用零额度建立可信的退款凭据。
		return false
	default:
		return false
	}
}

// syncRelayInfo 将 BillingSession 的状态同步到 RelayInfo 的兼容字段上。
func (s *BillingSession) syncRelayInfo() {
	info := s.relayInfo
	info.FinalPreConsumedQuota = s.preConsumedQuota
	info.BillingSource = s.funding.Source()

	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		info.SubscriptionId = sub.subscriptionId
		info.SubscriptionPreConsumed = sub.preConsumed + int64(s.extraReserved)
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = sub.AmountTotal
		info.SubscriptionAmountUsedAfterPreConsume = sub.AmountUsedAfter + int64(s.extraReserved)
		info.SubscriptionPlanId = sub.PlanId
		info.SubscriptionPlanTitle = sub.PlanTitle
	} else {
		info.SubscriptionId = 0
		info.SubscriptionPreConsumed = 0
	}
}

// ---------------------------------------------------------------------------
// NewBillingSession 工厂 — 根据计费偏好创建会话并处理回退
// ---------------------------------------------------------------------------

// NewBillingSession 根据用户计费偏好创建 BillingSession，处理 subscription_first / wallet_first 的回退。
func NewBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	if relayInfo == nil {
		return nil, types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	pref := common.NormalizeBillingPreference(relayInfo.UserSetting.BillingPreference)

	// 钱包路径需要先检查用户额度
	tryWallet := func() (*BillingSession, *types.NewAPIError) {
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if userQuota <= 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if userQuota-preConsumedQuota < 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", logger.FormatQuota(userQuota), logger.FormatQuota(preConsumedQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		relayInfo.UserQuota = userQuota

		session := &BillingSession{
			relayInfo: relayInfo,
			funding:   &WalletFunding{},
		}
		if apiErr := session.preConsume(c, preConsumedQuota); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	trySubscription := func() (*BillingSession, *types.NewAPIError) {
		subConsume := int64(preConsumedQuota)
		if subConsume <= 0 {
			subConsume = 1
		}
		session := &BillingSession{
			relayInfo: relayInfo,
			funding: &SubscriptionFunding{
				requestId: relayInfo.RequestId,
			},
		}
		// 订阅必须至少预扣 1，确保存在可恢复的订阅预扣记录。
		if apiErr := session.preConsume(c, int(subConsume)); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	switch pref {
	case "subscription_only":
		return trySubscription()
	case "wallet_only":
		return tryWallet()
	case "wallet_first":
		session, err := tryWallet()
		if err != nil {
			if err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return trySubscription()
			}
			return nil, err
		}
		return session, nil
	case "subscription_first":
		fallthrough
	default:
		hasSub, subCheckErr := model.HasActiveUserSubscription(relayInfo.UserId)
		if subCheckErr != nil {
			return nil, types.NewError(subCheckErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if !hasSub {
			return tryWallet()
		}
		session, apiErr := trySubscription()
		if apiErr != nil {
			if apiErr.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				// 仅当用户的活跃订阅允许钱包回退时才回退到钱包，否则返回订阅额度不足错误
				allowOverflow, overflowErr := model.UserActiveSubscriptionsAllowWalletOverflow(relayInfo.UserId)
				if overflowErr != nil {
					return nil, types.NewError(overflowErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
				}
				if allowOverflow {
					return tryWallet()
				}
				return nil, apiErr
			}
			return nil, apiErr
		}
		return session, nil
	}
}
