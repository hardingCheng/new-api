package service

import (
	"context"
	"crypto/sha256"
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
return {1, current, next, limit - next}
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
	consumed := make(map[string]int64, len(matches))
	for _, rule := range matches {
		periodKey, ttl := modelQuotaPoolPeriodKey(rule.Period, time.Now())
		redisKey := modelQuotaPoolRedisKey(rule, periodKey)
		amount := modelQuotaPoolConsumeAmount(rule, info)
		allowed, usedBefore, usedAfter, remaining, err := consumeModelQuotaPool(redisKey, rule.Limit, amount, ttl)
		match := ratio_setting.ModelQuotaPoolMatch{
			Rule:        rule,
			Scope:       rule.Scope,
			Metric:      rule.Metric,
			PeriodKey:   periodKey,
			RedisKey:    redisKey,
			Limit:       rule.Limit,
			Amount:      amount,
			UsedBefore:  usedBefore,
			UsedAfter:   usedAfter,
			Remaining:   remaining,
			Description: modelQuotaPoolDescription(rule),
		}
		applied = append(applied, match)
		if err != nil {
			rollbackErr := rollbackRelayModelQuotaPool(info, consumed)
			info.ModelQuotaPools = pendingModelQuotaPoolRollbackMatches(consumed)
			if rollbackErr == nil {
				info.ModelQuotaPoolSettled = true
				info.ModelQuotaPools = nil
			}
			return types.NewErrorWithStatusCode(
				fmt.Errorf("模型限量池检查失败: %w", err),
				types.ErrorCodeModelQuotaPoolUnavailable,
				http.StatusInternalServerError,
				types.ErrOptionWithSkipRetry(),
			)
		}
		if !allowed {
			rollbackErr := rollbackRelayModelQuotaPool(info, consumed)
			info.ModelQuotaPools = pendingModelQuotaPoolRollbackMatches(consumed)
			if rollbackErr == nil {
				info.ModelQuotaPoolSettled = true
				info.ModelQuotaPools = nil
			}
			message := strings.TrimSpace(rule.Message)
			if message == "" {
				message = fmt.Sprintf("模型 %s 当前周期%s已达上限", info.OriginModelName, modelQuotaPoolMetricText(rule.Metric))
			}
			return types.NewErrorWithStatusCode(
				fmt.Errorf("%s", message),
				types.ErrorCodeModelQuotaPoolExceeded,
				http.StatusTooManyRequests,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		consumed[redisKey] += amount
	}
	info.ModelQuotaPools = applied
	info.ModelQuotaPoolChecked = true
	info.ModelQuotaPoolSettled = false
	return nil
}

func pendingModelQuotaPoolRollbackMatches(consumed map[string]int64) []ratio_setting.ModelQuotaPoolMatch {
	matches := make([]ratio_setting.ModelQuotaPoolMatch, 0, len(consumed))
	for key, amount := range consumed {
		if strings.TrimSpace(key) == "" || amount <= 0 {
			continue
		}
		matches = append(matches, ratio_setting.ModelQuotaPoolMatch{RedisKey: key, Amount: amount})
	}
	return matches
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
	var firstErr error
	if settlement.HasActualQuota && settlement.ActualQuota >= 0 {
		if err := adjustRelayModelQuotaPoolDurably(info, ratio_setting.ModelQuotaPoolMetricQuota, int64(settlement.ActualQuota), "relay:settle:"+requestID); err != nil {
			firstErr = err
		}
	}
	if settlement.HasActualTotalTokens && settlement.ActualTotalTokens >= 0 {
		if err := adjustRelayModelQuotaPoolDurably(info, ratio_setting.ModelQuotaPoolMetricTotalTokens, int64(settlement.ActualTotalTokens), "relay:settle:"+requestID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		// The original reservation is still valid. Keep it instead of letting the
		// request safety defer roll back successful usage when even the durable
		// adjustment row could not be created.
		info.ModelQuotaPoolSettled = true
		common.SysError("persist model quota pool settlement failed; keeping estimate: " + firstErr.Error())
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
	if err := rollbackRelayModelQuotaPool(info, modelQuotaPoolConsumedFromMatches(info.ModelQuotaPools)); err != nil {
		return err
	}
	info.ModelQuotaPoolSettled = true
	info.ModelQuotaPools = nil
	return nil
}

func rollbackRelayModelQuotaPool(info *relaycommon.RelayInfo, consumed map[string]int64) error {
	requestID := "unknown"
	if info != nil {
		requestID = strings.TrimSpace(info.RequestId)
		if requestID == "" {
			requestID = common.NewRequestId()
			info.RequestId = requestID
		}
	}
	var firstErr error
	for key, amount := range consumed {
		if strings.TrimSpace(key) == "" || amount <= 0 {
			continue
		}
		if err := queueModelQuotaPoolAdjustment("relay:rollback:"+requestID, key, -amount); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func RollbackTaskModelQuotaPool(task *model.Task) error {
	if task == nil || task.PrivateData.BillingContext == nil || len(task.PrivateData.BillingContext.ModelQuotaPools) == 0 {
		return nil
	}
	consumed := modelQuotaPoolConsumedFromMatches(task.PrivateData.BillingContext.ModelQuotaPools)
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		taskID = fmt.Sprintf("db-%d", task.ID)
	}
	for key, amount := range consumed {
		if strings.TrimSpace(key) == "" || amount <= 0 {
			continue
		}
		if err := queueModelQuotaPoolAdjustment("task:rollback:"+taskID, key, -amount); err != nil {
			return err
		}
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
		delta := actualAmount - match.Amount
		if delta == 0 {
			continue
		}
		operationPrefix := fmt.Sprintf("task:settle:%s:%s:%d", task.TaskID, metric, i)
		if err := queueModelQuotaPoolAdjustment(operationPrefix, match.RedisKey, delta); err != nil {
			common.SysError("queue task model quota pool adjustment failed: " + err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		match.Amount = actualAmount
		match.UsedAfter += delta
		match.Remaining -= delta
		if match.Remaining < 0 {
			match.Remaining = 0
		}
	}
	task.PrivateData.BillingContext.ModelQuotaPools = matches
	return firstErr
}

func adjustRelayModelQuotaPoolDurably(info *relaycommon.RelayInfo, metric string, actualAmount int64, operationPrefix string) error {
	matches := info.ModelQuotaPools
	for i := range matches {
		match := &matches[i]
		if match.Metric != metric || strings.TrimSpace(match.RedisKey) == "" || match.Amount <= 0 {
			continue
		}
		delta := actualAmount - match.Amount
		if delta == 0 {
			continue
		}
		matchOperationPrefix := fmt.Sprintf("%s:%s:%d", operationPrefix, metric, i)
		if err := queueModelQuotaPoolAdjustment(matchOperationPrefix, match.RedisKey, delta); err != nil {
			info.ModelQuotaPools = matches
			return err
		}
		match.Amount = actualAmount
		match.UsedAfter += delta
		match.Remaining -= delta
		if match.Remaining < 0 {
			match.Remaining = 0
		}
	}
	info.ModelQuotaPools = matches
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
		return err
	}
	if err := applyQuotaPoolAdjustment(adjustment); err != nil {
		common.SysError("model quota pool adjustment queued for retry: " + err.Error())
	}
	return nil
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
	digest := sha256.Sum256([]byte(adjustment.OperationKey))
	markerKey := fmt.Sprintf("%s:adjustment:%x", modelQuotaPoolRedisPrefix, digest[:16])
	if _, err := modelQuotaPoolAdjustOnceScript.Run(context.Background(), common.RDB,
		[]string{adjustment.RedisKey, markerKey}, adjustment.Delta).Result(); err != nil {
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
			if !includeAllUserPools {
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

func consumeModelQuotaPool(key string, limit int64, amount int64, ttl time.Duration) (bool, int64, int64, int64, error) {
	ttlSeconds := int64(ttl.Seconds())
	if ttlSeconds <= 0 {
		ttlSeconds = 1
	}
	if amount <= 0 {
		amount = 1
	}
	result, err := modelQuotaPoolConsumeScript.Run(context.Background(), common.RDB, []string{key}, limit, ttlSeconds, amount).Result()
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
