package service

import (
	"context"
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

func CheckAndConsumeModelQuotaPool(ctx *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if ctx == nil || info == nil {
		return nil
	}
	if info.ModelQuotaPoolChecked {
		return nil
	}
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
			rollbackModelQuotaPool(consumed)
			info.ModelQuotaPools = applied
			return types.NewErrorWithStatusCode(
				fmt.Errorf("模型限量池检查失败: %w", err),
				types.ErrorCodeModelQuotaPoolUnavailable,
				http.StatusInternalServerError,
				types.ErrOptionWithSkipRetry(),
			)
		}
		if !allowed {
			rollbackModelQuotaPool(consumed)
			info.ModelQuotaPools = applied
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

func SettleModelQuotaPool(info *relaycommon.RelayInfo) {
	if info == nil || len(info.ModelQuotaPools) == 0 {
		return
	}
	info.ModelQuotaPoolSettled = true
}

func RollbackModelQuotaPool(info *relaycommon.RelayInfo) {
	if info == nil || info.ModelQuotaPoolSettled || len(info.ModelQuotaPools) == 0 {
		return
	}
	rollbackModelQuotaPool(modelQuotaPoolConsumedFromMatches(info.ModelQuotaPools))
	info.ModelQuotaPoolSettled = true
	info.ModelQuotaPools = nil
}

func RollbackTaskModelQuotaPool(task *model.Task) {
	if task == nil || task.PrivateData.BillingContext == nil || len(task.PrivateData.BillingContext.ModelQuotaPools) == 0 {
		return
	}
	rollbackModelQuotaPool(modelQuotaPoolConsumedFromMatches(task.PrivateData.BillingContext.ModelQuotaPools))
	task.PrivateData.BillingContext.ModelQuotaPools = nil
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

func rollbackModelQuotaPool(consumed map[string]int64) {
	if len(consumed) == 0 || !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx := context.Background()
	for key, amount := range consumed {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if amount <= 0 {
			amount = 1
		}
		if err := common.RDB.DecrBy(ctx, key, amount).Err(); err != nil {
			common.SysError("rollback model quota pool failed: " + err.Error())
		}
	}
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
