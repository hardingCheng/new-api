package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const modelQuotaPoolRedisPrefix = "model_quota_pool"

type ModelQuotaPoolUsage struct {
	Rule      ratio_setting.ModelQuotaPoolRule `json:"rule"`
	Scope     string                           `json:"scope"`
	Metric    string                           `json:"metric,omitempty"`
	PeriodKey string                           `json:"period_key"`
	Limit     int64                            `json:"limit"`
	Used      int64                            `json:"used"`
	Remaining int64                            `json:"remaining"`
}

type ModelQuotaPoolSettlement struct {
	ActualQuota          int
	ActualTotalTokens    int
	HasActualQuota       bool
	HasActualTotalTokens bool
}

var modelQuotaPoolConsumeScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[2]) == 1 then
  local guard = redis.call("GET", KEYS[2]) or "0:0"
  local separator = string.find(guard, ":", 1, true)
  local used_before = tonumber(string.sub(guard, 1, separator - 1)) or 0
  local used_after = tonumber(string.sub(guard, separator + 1)) or used_before
  local limit = tonumber(ARGV[1])
  return {1, used_before, used_after, limit - used_after}
end
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local limit = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local amount = tonumber(ARGV[3])
if limit <= 0 then
  return {1, current, current, 0}
end
if amount <= 0 then
  amount = 1
end
if current >= limit then
  return {0, current, current, limit - current}
end
if current + amount > limit then
  return {0, current, current, limit - current}
end
local next = redis.call("INCRBY", KEYS[1], amount)
if current == 0 and ttl > 0 then
  redis.call("EXPIRE", KEYS[1], ttl)
end
redis.call("SET", KEYS[2], tostring(current) .. ":" .. tostring(next), "EX", ttl)
return {1, current, next, limit - next}
`)

var modelQuotaPoolFinalizeReservationScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[2]) == 0 then
  return 0
end
if redis.call("EXISTS", KEYS[1]) == 1 then
  local current = tonumber(redis.call("GET", KEYS[1]) or "0")
  local delta = tonumber(ARGV[1])
  local next = current + delta
  if next < 0 then
    next = 0
  end
  if next ~= current then
    redis.call("INCRBY", KEYS[1], next - current)
  end
end
redis.call("DEL", KEYS[2])
return 1
`)

var modelQuotaPoolAdjustOnceScript = redis.NewScript(`
if redis.call("SETNX", KEYS[2], "1") == 0 then
  return 0
end
local marker_ttl = 3888000
local pool_ttl = tonumber(redis.call("TTL", KEYS[1]) or "-1")
if pool_ttl > marker_ttl then
  marker_ttl = pool_ttl
end
redis.call("EXPIRE", KEYS[2], marker_ttl)
if redis.call("EXISTS", KEYS[1]) == 0 then
  return 1
end
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local delta = tonumber(ARGV[1])
local next = current + delta
if next < 0 then
  next = 0
end
if next ~= current then
  redis.call("INCRBY", KEYS[1], next - current)
end
return 1
`)

func CheckAndConsumeModelQuotaPool(ctx *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if ctx == nil || info == nil {
		return nil
	}
	if info.ModelQuotaPoolChecked {
		return nil
	}
	// Pool rules use the public model requested by the client. Upstream mappings
	// are routing details and may change between retries without changing usage.
	matches := ratio_setting.MatchModelQuotaPoolRules(info.UserId, info.OriginModelName)
	if len(matches) == 0 {
		info.ModelQuotaPoolChecked = true
		return nil
	}
	if !common.RedisEnabled || common.RDB == nil {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("模型限量池需要 Redis 才能启用"),
			types.ErrorCodeModelQuotaPoolUnavailable,
			http.StatusInternalServerError,
			types.ErrOptionWithSkipRetry(),
		)
	}

	applied := make([]ratio_setting.ModelQuotaPoolMatch, 0, len(matches))
	requestID := strings.TrimSpace(info.RequestId)
	if requestID == "" {
		requestID = common.NewRequestId()
		info.RequestId = requestID
	}
	for index, rule := range matches {
		periodKey, ttl := modelQuotaPoolPeriodKey(rule.Period, time.Now())
		redisKey := modelQuotaPoolRedisKey(rule, periodKey)
		amount := modelQuotaPoolConsumeAmount(rule, info)
		operationDigest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", requestID, index, redisKey)))
		operationKey := fmt.Sprintf("relay:reserve:%x", operationDigest[:16])
		reservation, err := model.EnsureQuotaPoolReservation(operationKey, redisKey, amount)
		if err != nil || reservation.Status != model.QuotaPoolAdjustmentStatusReserved {
			if err == nil {
				err = errors.New("model quota pool reservation was already finalized")
			}
			info.ModelQuotaPools = applied
			rollbackErr := RollbackModelQuotaPool(info)
			if rollbackErr != nil {
				err = errors.Join(err, rollbackErr)
			}
			return types.NewErrorWithStatusCode(
				fmt.Errorf("模型限量池检查失败: %w", err),
				types.ErrorCodeModelQuotaPoolUnavailable,
				http.StatusInternalServerError,
				types.ErrOptionWithSkipRetry(),
			)
		}
		allowed, usedBefore, usedAfter, remaining, err := consumeModelQuotaPool(redisKey, reservation.GuardKey, rule.Limit, amount, ttl)
		match := ratio_setting.ModelQuotaPoolMatch{
			Rule:          rule,
			ReservationID: reservation.ID,
			Scope:         rule.Scope,
			Metric:        rule.Metric,
			PeriodKey:     periodKey,
			RedisKey:      redisKey,
			Limit:         rule.Limit,
			Amount:        amount,
			UsedBefore:    usedBefore,
			UsedAfter:     usedAfter,
			Remaining:     remaining,
			Description:   modelQuotaPoolDescription(rule),
		}
		applied = append(applied, match)
		if err != nil {
			info.ModelQuotaPools = applied
			rollbackErr := RollbackModelQuotaPool(info)
			if rollbackErr != nil {
				err = errors.Join(err, rollbackErr)
			}
			return types.NewErrorWithStatusCode(
				fmt.Errorf("模型限量池检查失败: %w", err),
				types.ErrorCodeModelQuotaPoolUnavailable,
				http.StatusInternalServerError,
				types.ErrOptionWithSkipRetry(),
			)
		}
		if !allowed {
			info.ModelQuotaPools = applied
			rollbackErr := RollbackModelQuotaPool(info)
			message := strings.TrimSpace(rule.Message)
			if message == "" {
				message = fmt.Sprintf("模型 %s 当前周期%s已达上限", info.OriginModelName, modelQuotaPoolMetricText(rule.Metric))
			}
			apiErr := types.NewErrorWithStatusCode(
				fmt.Errorf("%s", message),
				types.ErrorCodeModelQuotaPoolExceeded,
				http.StatusTooManyRequests,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
			if rollbackErr != nil {
				return types.NewErrorWithStatusCode(
					errors.Join(apiErr, rollbackErr),
					types.ErrorCodeModelQuotaPoolUnavailable,
					http.StatusInternalServerError,
					types.ErrOptionWithSkipRetry(),
				)
			}
			return apiErr
		}
	}
	info.ModelQuotaPools = applied
	info.ModelQuotaPoolChecked = true
	info.ModelQuotaPoolSettled = false
	return nil
}

func SettleModelQuotaPool(info *relaycommon.RelayInfo, settlement ModelQuotaPoolSettlement) error {
	if info == nil || len(info.ModelQuotaPools) == 0 {
		return nil
	}
	requestID := strings.TrimSpace(info.RequestId)
	if requestID == "" {
		requestID = common.NewRequestId()
		info.RequestId = requestID
	}
	matches := info.ModelQuotaPools
	var firstErr error
	for index := range matches {
		match := &matches[index]
		actualAmount := match.Amount
		switch match.Metric {
		case ratio_setting.ModelQuotaPoolMetricQuota:
			if settlement.HasActualQuota && settlement.ActualQuota >= 0 {
				actualAmount = int64(settlement.ActualQuota)
			}
		case ratio_setting.ModelQuotaPoolMetricTotalTokens:
			if settlement.HasActualTotalTokens && settlement.ActualTotalTokens >= 0 {
				actualAmount = int64(settlement.ActualTotalTokens)
			}
		}
		operationPrefix := fmt.Sprintf("relay:settle:%s:%s:%d", requestID, match.Metric, index)
		if err := settleModelQuotaPoolMatch(match, actualAmount, operationPrefix); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	info.ModelQuotaPools = matches
	if firstErr != nil {
		common.SysError("persist or apply model quota pool settlement failed: " + firstErr.Error())
		return firstErr
	}
	info.ModelQuotaPoolSettled = true
	return nil
}

func settleRelayModelQuotaPoolWithAdjuster(info *relaycommon.RelayInfo, settlement ModelQuotaPoolSettlement, adjuster func(string, int64) bool) {
	if settlement.HasActualQuota && settlement.ActualQuota >= 0 {
		adjustRelayModelQuotaPoolWithAdjuster(info, ratio_setting.ModelQuotaPoolMetricQuota, int64(settlement.ActualQuota), adjuster)
	}
	if settlement.HasActualTotalTokens && settlement.ActualTotalTokens >= 0 {
		adjustRelayModelQuotaPoolWithAdjuster(info, ratio_setting.ModelQuotaPoolMetricTotalTokens, int64(settlement.ActualTotalTokens), adjuster)
	}
}

func RollbackModelQuotaPool(info *relaycommon.RelayInfo) error {
	if info == nil || info.ModelQuotaPoolSettled || len(info.ModelQuotaPools) == 0 {
		return nil
	}
	requestID := strings.TrimSpace(info.RequestId)
	if requestID == "" {
		requestID = common.NewRequestId()
		info.RequestId = requestID
	}
	matches := info.ModelQuotaPools
	var firstErr error
	for index := range matches {
		// A zero reservation ID means this match was already finalized during a
		// partially successful settlement. Its durable adjustment must stand.
		if matches[index].ReservationID == 0 {
			continue
		}
		operationPrefix := fmt.Sprintf("relay:rollback:%s:%d", requestID, index)
		if err := settleModelQuotaPoolMatch(&matches[index], 0, operationPrefix); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	info.ModelQuotaPools = matches
	if firstErr != nil {
		return firstErr
	}
	info.ModelQuotaPoolSettled = true
	info.ModelQuotaPools = nil
	return nil
}

func RollbackTaskModelQuotaPool(task *model.Task) error {
	if task == nil || task.PrivateData.BillingContext == nil || len(task.PrivateData.BillingContext.ModelQuotaPools) == 0 {
		return nil
	}
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		taskID = fmt.Sprintf("db-%d", task.ID)
	}
	matches := task.PrivateData.BillingContext.ModelQuotaPools
	var firstErr error
	for index := range matches {
		operationPrefix := fmt.Sprintf("task:rollback:%s:%d", taskID, index)
		if err := settleModelQuotaPoolMatch(&matches[index], 0, operationPrefix); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	task.PrivateData.BillingContext.ModelQuotaPools = matches
	if firstErr != nil {
		return firstErr
	}
	task.PrivateData.BillingContext.ModelQuotaPools = nil
	return nil
}

func AdjustTaskModelQuotaPoolQuota(task *model.Task, actualQuota int) error {
	if task == nil || task.PrivateData.BillingContext == nil || len(task.PrivateData.BillingContext.ModelQuotaPools) == 0 || actualQuota < 0 {
		return nil
	}
	return adjustTaskModelQuotaPool(task, ratio_setting.ModelQuotaPoolMetricQuota, int64(actualQuota))
}

func AdjustTaskModelQuotaPoolTokens(task *model.Task, actualTokens int) error {
	if task == nil || task.PrivateData.BillingContext == nil || len(task.PrivateData.BillingContext.ModelQuotaPools) == 0 || actualTokens < 0 {
		return nil
	}
	return adjustTaskModelQuotaPool(task, ratio_setting.ModelQuotaPoolMetricTotalTokens, int64(actualTokens))
}

func adjustTaskModelQuotaPool(task *model.Task, metric string, actualAmount int64) error {
	if task == nil || task.PrivateData.BillingContext == nil {
		return nil
	}
	matches := task.PrivateData.BillingContext.ModelQuotaPools
	var firstErr error
	for i := range matches {
		match := &matches[i]
		if match.Metric != metric || strings.TrimSpace(match.RedisKey) == "" || match.Amount <= 0 {
			continue
		}
		operationPrefix := fmt.Sprintf("task:settle:%s:%s:%d", task.TaskID, metric, i)
		if err := settleModelQuotaPoolMatch(match, actualAmount, operationPrefix); err != nil {
			common.SysError("queue task model quota pool adjustment failed: " + err.Error())
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	task.PrivateData.BillingContext.ModelQuotaPools = matches
	return firstErr
}

func settleTaskSubmissionModelQuotaPool(task *model.Task, actualQuota int) error {
	if task == nil || task.PrivateData.BillingContext == nil || actualQuota < 0 {
		return nil
	}
	matches := task.PrivateData.BillingContext.ModelQuotaPools
	var firstErr error
	for index := range matches {
		actualAmount := matches[index].Amount
		if matches[index].Metric == ratio_setting.ModelQuotaPoolMetricQuota {
			actualAmount = int64(actualQuota)
		}
		operationPrefix := fmt.Sprintf("task:submission:%s:%s:%d", task.TaskID, matches[index].Metric, index)
		if err := settleModelQuotaPoolMatch(&matches[index], actualAmount, operationPrefix); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	task.PrivateData.BillingContext.ModelQuotaPools = matches
	return firstErr
}

func settleModelQuotaPoolMatch(match *ratio_setting.ModelQuotaPoolMatch, actualAmount int64, operationPrefix string) error {
	if match == nil || strings.TrimSpace(match.RedisKey) == "" || match.Amount <= 0 || actualAmount < 0 {
		return nil
	}
	delta := actualAmount - match.Amount
	if match.ReservationID > 0 {
		if err := finalizeQuotaPoolReservationDurably(match.ReservationID, match.RedisKey, delta, actualAmount); err != nil {
			return err
		}
		match.ReservationID = 0
	} else if delta != 0 {
		if err := queueModelQuotaPoolAdjustment(operationPrefix, match.RedisKey, delta); err != nil {
			return err
		}
	}
	match.Amount = actualAmount
	match.UsedAfter += delta
	match.Remaining -= delta
	if match.Remaining < 0 {
		match.Remaining = 0
	}
	return nil
}

func finalizeQuotaPoolReservationDurably(reservationID int64, redisKey string, delta int64, actualAmount int64) error {
	var adjustment *model.QuotaPoolAdjustment
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		adjustment, err = model.FinalizeQuotaPoolReservation(reservationID, actualAmount)
		if err == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
		}
	}
	if err != nil {
		directErr := applyQuotaPoolReservationDirect(reservationID, redisKey, delta)
		if directErr == nil {
			return nil
		}
		return errors.Join(err, directErr)
	}
	if err := applyQuotaPoolAdjustment(adjustment); err != nil {
		common.SysError("model quota pool reservation queued for retry: " + err.Error())
	}
	return nil
}

func queueModelQuotaPoolAdjustment(operationPrefix string, redisKey string, delta int64) error {
	if strings.TrimSpace(redisKey) == "" || delta == 0 {
		return nil
	}
	digest := sha256.Sum256([]byte(redisKey))
	operationKey := fmt.Sprintf("%s:%x", operationPrefix, digest[:12])
	var adjustment *model.QuotaPoolAdjustment
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		adjustment, err = model.EnsureQuotaPoolAdjustment(operationKey, redisKey, delta)
		if err == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
		}
	}
	if err != nil {
		if directErr := applyQuotaPoolAdjustmentDirect(operationKey, redisKey, delta); directErr == nil {
			return nil
		} else {
			return errors.Join(err, directErr)
		}
	}
	if err := applyQuotaPoolAdjustment(adjustment); err != nil {
		common.SysError("model quota pool adjustment queued for retry: " + err.Error())
	}
	return nil
}

func applyQuotaPoolAdjustmentDirect(operationKey string, redisKey string, delta int64) error {
	if !common.RedisEnabled || common.RDB == nil {
		return errors.New("model quota pool Redis is unavailable")
	}
	digest := sha256.Sum256([]byte(operationKey))
	markerKey := fmt.Sprintf("%s:adjustment:%x", modelQuotaPoolRedisPrefix, digest[:16])
	_, err := modelQuotaPoolAdjustOnceScript.Run(context.Background(), common.RDB,
		[]string{redisKey, markerKey}, delta).Result()
	return err
}

func applyQuotaPoolReservationDirect(reservationID int64, redisKey string, delta int64) error {
	if !common.RedisEnabled || common.RDB == nil {
		return errors.New("model quota pool Redis is unavailable")
	}
	guardKey := model.QuotaPoolReservationGuardKey(reservationID)
	if strings.TrimSpace(redisKey) == "" || guardKey == "" {
		return errors.New("invalid model quota pool reservation")
	}
	_, err := modelQuotaPoolFinalizeReservationScript.Run(context.Background(), common.RDB,
		[]string{redisKey, guardKey}, delta).Result()
	return err
}

func applyQuotaPoolAdjustment(adjustment *model.QuotaPoolAdjustment) error {
	if adjustment == nil || adjustment.Status == model.QuotaPoolAdjustmentStatusSucceeded {
		return nil
	}
	if !common.RedisEnabled || common.RDB == nil {
		err := fmt.Errorf("model quota pool Redis is unavailable")
		model.MarkQuotaPoolAdjustmentFailed(adjustment.ID, err)
		return err
	}
	if adjustment.Status == model.QuotaPoolAdjustmentStatusReserved {
		return errors.New("model quota pool reservation is not finalized")
	}
	var err error
	if adjustment.ReservedAmount > 0 {
		guardKey := adjustment.GuardKey
		if guardKey == "" {
			guardKey = model.QuotaPoolReservationGuardKey(adjustment.ID)
		}
		_, err = modelQuotaPoolFinalizeReservationScript.Run(context.Background(), common.RDB,
			[]string{adjustment.RedisKey, guardKey}, adjustment.Delta).Result()
	} else {
		err = applyQuotaPoolAdjustmentDirect(adjustment.OperationKey, adjustment.RedisKey, adjustment.Delta)
	}
	if err != nil {
		model.MarkQuotaPoolAdjustmentFailed(adjustment.ID, err)
		return err
	}
	if err := model.MarkQuotaPoolAdjustmentSucceeded(adjustment.ID); err != nil {
		model.MarkQuotaPoolAdjustmentFailed(adjustment.ID, err)
		return err
	}
	return nil
}

func adjustRelayModelQuotaPoolWithAdjuster(info *relaycommon.RelayInfo, metric string, actualAmount int64, adjuster func(string, int64) bool) {
	matches := info.ModelQuotaPools
	adjustModelQuotaPoolMatches(matches, metric, actualAmount, adjuster)
	info.ModelQuotaPools = matches
}

func adjustTaskModelQuotaPoolWithAdjuster(task *model.Task, metric string, actualAmount int64, adjuster func(string, int64) bool) {
	matches := task.PrivateData.BillingContext.ModelQuotaPools
	adjustModelQuotaPoolMatches(matches, metric, actualAmount, adjuster)
	task.PrivateData.BillingContext.ModelQuotaPools = matches
}

func adjustModelQuotaPoolMatches(matches []ratio_setting.ModelQuotaPoolMatch, metric string, actualAmount int64, adjuster func(string, int64) bool) {
	for i := range matches {
		match := &matches[i]
		if match.Metric != metric || strings.TrimSpace(match.RedisKey) == "" || match.Amount <= 0 {
			continue
		}
		delta := actualAmount - match.Amount
		if delta == 0 {
			continue
		}
		if adjuster == nil || !adjuster(match.RedisKey, delta) {
			continue
		}
		match.Amount = actualAmount
		match.UsedAfter += delta
		match.Remaining -= delta
		if match.Remaining < 0 {
			match.Remaining = 0
		}
	}
}

func modelQuotaPoolConsumedFromMatches(matches []ratio_setting.ModelQuotaPoolMatch) map[string]int64 {
	consumed := make(map[string]int64, len(matches))
	for _, match := range matches {
		if strings.TrimSpace(match.RedisKey) == "" || match.Amount <= 0 {
			continue
		}
		consumed[match.RedisKey] += match.Amount
	}
	return consumed
}

func GetVisibleModelQuotaPoolUsage(userID int, includeAllUserPools bool) []ModelQuotaPoolUsage {
	cfg := ratio_setting.GetModelQuotaPoolCopy()
	if len(cfg.Rules) == 0 {
		return nil
	}
	now := time.Now()
	items := make([]ModelQuotaPoolUsage, 0, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		if rule.Disabled || rule.Limit <= 0 {
			continue
		}
		if rule.Scope == ratio_setting.ModelQuotaPoolScopeUser {
			if !includeAllUserPools && rule.UserID != userID {
				continue
			}
		} else if rule.Scope != ratio_setting.ModelQuotaPoolScopeGlobal {
			continue
		}
		periodKey, _ := modelQuotaPoolPeriodKey(rule.Period, now)
		used := getModelQuotaPoolUsed(modelQuotaPoolRedisKey(rule, periodKey))
		remaining := rule.Limit - used
		if remaining < 0 {
			remaining = 0
		}
		items = append(items, ModelQuotaPoolUsage{
			Rule:      rule,
			Scope:     rule.Scope,
			Metric:    rule.Metric,
			PeriodKey: periodKey,
			Limit:     rule.Limit,
			Used:      used,
			Remaining: remaining,
		})
	}
	return items
}

func consumeModelQuotaPool(key string, guardKey string, limit int64, amount int64, ttl time.Duration) (bool, int64, int64, int64, error) {
	ttlSeconds := int64(ttl.Seconds())
	if ttlSeconds <= 0 {
		ttlSeconds = 1
	}
	if amount <= 0 {
		amount = 1
	}
	result, err := modelQuotaPoolConsumeScript.Run(context.Background(), common.RDB, []string{key, guardKey}, limit, ttlSeconds, amount).Result()
	if err != nil {
		return false, 0, 0, 0, err
	}
	values, ok := result.([]interface{})
	if !ok || len(values) < 4 {
		return false, 0, 0, 0, fmt.Errorf("unexpected redis script result: %v", result)
	}
	allowed := toRedisInt64(values[0]) == 1
	usedBefore := toRedisInt64(values[1])
	usedAfter := toRedisInt64(values[2])
	remaining := toRedisInt64(values[3])
	if remaining < 0 {
		remaining = 0
	}
	return allowed, usedBefore, usedAfter, remaining, nil
}

func getModelQuotaPoolUsed(key string) int64 {
	if !common.RedisEnabled || common.RDB == nil {
		return 0
	}
	value, err := common.RDB.Get(context.Background(), key).Int64()
	if err != nil {
		if err != redis.Nil {
			common.SysError("get model quota pool usage failed: " + err.Error())
		}
		return 0
	}
	if value < 0 {
		return 0
	}
	return value
}

func modelQuotaPoolConsumeAmount(rule ratio_setting.ModelQuotaPoolRule, info *relaycommon.RelayInfo) int64 {
	switch rule.Metric {
	case ratio_setting.ModelQuotaPoolMetricTotalTokens:
		if info == nil || info.GetEstimateTotalTokens() <= 0 {
			return 1
		}
		return int64(info.GetEstimateTotalTokens())
	case ratio_setting.ModelQuotaPoolMetricQuota:
		if info == nil {
			return 1
		}
		if info.PriceData.Quota > 0 {
			return int64(info.PriceData.Quota)
		}
		if info.PriceData.QuotaToPreConsume > 0 {
			return int64(info.PriceData.QuotaToPreConsume)
		}
		return 1
	case ratio_setting.ModelQuotaPoolMetricRequests:
		fallthrough
	default:
		return 1
	}
}

func modelQuotaPoolMetricText(metric string) string {
	switch metric {
	case ratio_setting.ModelQuotaPoolMetricTotalTokens:
		return "总 Token 消耗"
	case ratio_setting.ModelQuotaPoolMetricQuota:
		return "花费金额"
	case ratio_setting.ModelQuotaPoolMetricRequests:
		fallthrough
	default:
		return "请求次数"
	}
}

func modelQuotaPoolPeriodKey(period string, now time.Time) (string, time.Duration) {
	switch period {
	case ratio_setting.ModelQuotaPoolPeriodMinute:
		start := now.Truncate(time.Minute)
		return start.Format("200601021504"), start.Add(time.Minute).Sub(now)
	case ratio_setting.ModelQuotaPoolPeriodHour:
		start := now.Truncate(time.Hour)
		return start.Format("2006010215"), start.Add(time.Hour).Sub(now)
	case ratio_setting.ModelQuotaPoolPeriodWeek:
		year, week := now.ISOWeek()
		start := startOfISOWeek(year, week, now.Location())
		return fmt.Sprintf("%04dW%02d", year, week), start.AddDate(0, 0, 7).Sub(now)
	case ratio_setting.ModelQuotaPoolPeriodMonth:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start.Format("200601"), start.AddDate(0, 1, 0).Sub(now)
	case ratio_setting.ModelQuotaPoolPeriodDay:
		fallthrough
	default:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return start.Format("20060102"), start.AddDate(0, 0, 1).Sub(now)
	}
}

func modelQuotaPoolRedisKey(rule ratio_setting.ModelQuotaPoolRule, periodKey string) string {
	scopeKey := rule.Scope
	if rule.Scope == ratio_setting.ModelQuotaPoolScopeUser {
		scopeKey = fmt.Sprintf("%s:%d", rule.Scope, rule.UserID)
	}
	return fmt.Sprintf("%s:%s:%s:%s", modelQuotaPoolRedisPrefix, scopeKey, ratio_setting.ModelQuotaPoolRuleKey(rule), periodKey)
}

func modelQuotaPoolDescription(rule ratio_setting.ModelQuotaPoolRule) string {
	if rule.Scope == ratio_setting.ModelQuotaPoolScopeUser {
		return "user_model_pool"
	}
	return "global_model_pool"
}

func startOfISOWeek(year int, week int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, loc)
	weekday := int(jan4.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	week1Start := jan4.AddDate(0, 0, -weekday+1)
	return week1Start.AddDate(0, 0, (week-1)*7)
}

func toRedisInt64(value interface{}) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		var out int64
		_, _ = fmt.Sscan(v, &out)
		return out
	default:
		return 0
	}
}
