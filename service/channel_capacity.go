package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/go-redis/redis/v8"
)

// 渠道容量保护（RPM / 最大并发）的分布式计数引擎。
//
// 语义见 CHANNEL_CAPACITY_LIMIT_SPEC.md §2/§5：
//   - RPM：任意连续 60 秒窗口内“已开始派发”的上游尝试数（Redis ZSET 滑动窗口）
//   - 并发：已预留、尚未结束的上游尝试数（ZSET 租约，score 为过期时间，可续租）
//   - 时间一律取 Redis TIME，避免多实例时钟漂移
//   - 多实例共用同一套计数；本文件不提供多实例下的本地 fallback
const (
	channelCapacityWindowMs       = 60_000
	channelCapacityLeaseTTLMs     = 90_000
	channelCapacityRenewInterval  = 30 * time.Second
	channelCapacityRPMKeyTTLMs    = 120_000
	channelCapacityInflightTTLMs  = 180_000
	channelCapacityStatsTTLMs     = 7 * 24 * 3_600_000
	channelCapacityOpTimeout      = 2 * time.Second
	channelCapacityMinRetryMs     = 1_000
	channelCapacityErrLogInterval = 10 // 秒，redis 错误日志限频
	// channelCapacityMaxRenewAge 续租寿命上限：正常流远短于此；handler 卡死或
	// 释放路径被跳过时，续租到点自动停止，名额随后由租约 TTL 回收，
	// 保证不存在被无限续租的“僵尸名额”。
	channelCapacityMaxRenewAge = time.Hour
)

const (
	ChannelCapacityReasonRPM         = "rpm"
	ChannelCapacityReasonConcurrency = "concurrency"
	ChannelCapacityReasonBoth        = "both"
	ChannelCapacityReasonRedisError  = "redis_error"
)

// ChannelCapacityLease 是一次容量预留的生命周期句柄。三个方法均幂等。
type ChannelCapacityLease interface {
	// MarkDispatched 声明请求已进入上游派发边界：之后 Release 只释放并发名额，
	// RPM 记录保留到窗口自然过期。
	MarkDispatched()
	// CancelBeforeDispatch 撤销一次尚未派发的预留：RPM 和并发都回退。
	CancelBeforeDispatch(ctx context.Context) error
	// Release 结束一次已派发的尝试：释放并发名额，不撤销 RPM。
	Release(ctx context.Context) error
}

type ChannelCapacityDecision struct {
	Allowed      bool
	WouldBlock   bool // shadow 模式下：如果 enforce 会被拒绝
	Reason       string
	RetryAfterMs int64
	RPMUsed      int64
	InflightUsed int64
	Lease        ChannelCapacityLease
}

// channelCapacityInstanceID 参与 attempt_id，保证多实例成员名不冲突。
var channelCapacityInstanceID = common.GetRandomString(8)
var channelCapacityAttemptSeq atomic.Uint64

// BuildChannelCapacityAttemptID 生成全局唯一的尝试标识：
// 实例随机 ID + 请求 ID + attempt 序号 + 实例内单调序列。
func BuildChannelCapacityAttemptID(requestID string, attemptIndex int) string {
	return fmt.Sprintf("%s:%s:%d:%d", channelCapacityInstanceID, requestID, attemptIndex, channelCapacityAttemptSeq.Add(1))
}

func channelCapacityRPMKey(channelID int) string {
	// hash tag 保证同渠道的所有 key 在 Redis Cluster 下落在同一 slot
	return fmt.Sprintf("new-api:channel-capacity:v1:{channel:%d}:rpm", channelID)
}

func channelCapacityInflightKey(channelID int) string {
	return fmt.Sprintf("new-api:channel-capacity:v1:{channel:%d}:inflight", channelID)
}

func channelCapacityStatsKey(channelID int) string {
	return fmt.Sprintf("new-api:channel-capacity:v1:{channel:%d}:stats", channelID)
}

// 单脚本原子完成：清理过期记录 → 读取用量 → 判限 → 按启用维度写入。
// 不能拆成 Go 侧 ZCARD → 判断 → ZADD，并发下会超卖。
// KEYS: [1]=rpm zset, [2]=inflight zset, [3]=stats hash
// ARGV: [1]=rpm_limit, [2]=conc_limit, [3]=enforce(0/1), [4]=attempt_id,
//
//	[5]=window_ms, [6]=lease_ttl_ms, [7]=rpm_key_ttl_ms, [8]=inflight_key_ttl_ms, [9]=stats_ttl_ms
//
// 返回 {allowed, reason, retry_after_ms, rpm_used, inflight_used}
var channelCapacityAcquireScript = redis.NewScript(`
local time = redis.call("TIME")
local now_ms = tonumber(time[1]) * 1000 + math.floor(tonumber(time[2]) / 1000)
local rpm_limit = tonumber(ARGV[1])
local conc_limit = tonumber(ARGV[2])
local enforce = tonumber(ARGV[3]) == 1
local attempt_id = ARGV[4]
local window_ms = tonumber(ARGV[5])
local lease_ttl_ms = tonumber(ARGV[6])

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", now_ms - window_ms)
redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", now_ms)

local rpm_used = redis.call("ZCARD", KEYS[1])
local inflight_used = redis.call("ZCARD", KEYS[2])

local rpm_limited = rpm_limit > 0 and rpm_used >= rpm_limit
local conc_limited = conc_limit > 0 and inflight_used >= conc_limit

local retry_after_ms = 0
if rpm_limited then
  local oldest = redis.call("ZRANGE", KEYS[1], 0, 0, "WITHSCORES")
  if oldest[2] then
    retry_after_ms = math.floor(tonumber(oldest[2])) + window_ms - now_ms
  end
end
if (rpm_limited or conc_limited) and retry_after_ms < 1000 then
  retry_after_ms = 1000
end

local reason = ""
if rpm_limited and conc_limited then
  reason = "both"
elseif rpm_limited then
  reason = "rpm"
elseif conc_limited then
  reason = "concurrency"
end

if enforce and reason ~= "" then
  redis.call("HINCRBY", KEYS[3], "reject_" .. reason, 1)
  redis.call("PEXPIRE", KEYS[3], tonumber(ARGV[9]))
  return {0, reason, retry_after_ms, rpm_used, inflight_used}
end

if rpm_limit > 0 then
  redis.call("ZADD", KEYS[1], now_ms, attempt_id)
  redis.call("PEXPIRE", KEYS[1], tonumber(ARGV[7]))
end
if conc_limit > 0 then
  redis.call("ZADD", KEYS[2], now_ms + lease_ttl_ms, attempt_id)
  redis.call("PEXPIRE", KEYS[2], tonumber(ARGV[8]))
end
if reason == "" then
  redis.call("HINCRBY", KEYS[3], "allowed", 1)
else
  redis.call("HINCRBY", KEYS[3], "shadow_block_" .. reason, 1)
end
redis.call("PEXPIRE", KEYS[3], tonumber(ARGV[9]))
return {1, reason, retry_after_ms, rpm_used, inflight_used}
`)

// 续租：只用 ZADD XX 更新仍存在的 member，member 已被释放/回收时返回 0，
// 绝不重新创建；同时刷新 inflight key TTL，避免长流期间 key 先于 member 消失。
// KEYS: [1]=inflight zset；ARGV: [1]=attempt_id, [2]=lease_ttl_ms, [3]=inflight_key_ttl_ms
var channelCapacityRenewScript = redis.NewScript(`
local time = redis.call("TIME")
local now_ms = tonumber(time[1]) * 1000 + math.floor(tonumber(time[2]) / 1000)
if redis.call("ZSCORE", KEYS[1], ARGV[1]) == false then
  return 0
end
redis.call("ZADD", KEYS[1], "XX", now_ms + tonumber(ARGV[2]), ARGV[1])
redis.call("PEXPIRE", KEYS[1], tonumber(ARGV[3]))
return 1
`)

// 派发前取消：RPM 和并发都回退。
// KEYS: [1]=rpm zset, [2]=inflight zset；ARGV: [1]=attempt_id
var channelCapacityCancelScript = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
redis.call("ZREM", KEYS[2], ARGV[1])
return 1
`)

var channelCapacityRedisErrors atomic.Int64
var channelCapacityLastErrLogUnix atomic.Int64

// channelCapacityLogGate 渠道容量事件日志限频：满载时逐条打日志会形成日志风暴，
// 精确计数在 stats hash 里，日志只负责提示。
var channelCapacityLogGate sync.Map // channelID -> unix 秒

const channelCapacityLogGateSeconds = 30

// ChannelCapacityShouldLog 返回该渠道现在是否应该输出一条容量事件日志。
func ChannelCapacityShouldLog(channelID int) bool {
	now := time.Now().Unix()
	actual, loaded := channelCapacityLogGate.LoadOrStore(channelID, now)
	if !loaded {
		return true
	}
	last, _ := actual.(int64)
	if now-last < channelCapacityLogGateSeconds {
		return false
	}
	return channelCapacityLogGate.CompareAndSwap(channelID, actual, now)
}

// ChannelCapacityRedisErrorCount 返回进程内累计的容量引擎 Redis 错误数（可观测性用）。
func ChannelCapacityRedisErrorCount() int64 {
	return channelCapacityRedisErrors.Load()
}

func recordChannelCapacityRedisError(op string, err error) {
	channelCapacityRedisErrors.Add(1)
	now := time.Now().Unix()
	last := channelCapacityLastErrLogUnix.Load()
	if now-last >= channelCapacityErrLogInterval && channelCapacityLastErrLogUnix.CompareAndSwap(last, now) {
		common.SysError(fmt.Sprintf("channel capacity redis %s failed: %s", op, err.Error()))
	}
}

func channelCapacityRedisAvailable() bool {
	return common.RedisEnabled && common.RDB != nil
}

// noopChannelCapacityLease 用于未触发任何 Redis 预留的场景（无限额 / redis 不可用放行）。
type noopChannelCapacityLease struct{}

func (noopChannelCapacityLease) MarkDispatched()                            {}
func (noopChannelCapacityLease) CancelBeforeDispatch(context.Context) error { return nil }
func (noopChannelCapacityLease) Release(context.Context) error              { return nil }

type redisChannelCapacityLease struct {
	channelID   int
	attemptID   string
	hasRPM      bool
	hasInflight bool

	dispatched atomic.Bool
	finished   atomic.Bool
	stopRenew  chan struct{}
	stopOnce   sync.Once
}

func (l *redisChannelCapacityLease) MarkDispatched() {
	l.dispatched.Store(true)
}

func (l *redisChannelCapacityLease) stop() {
	l.stopOnce.Do(func() { close(l.stopRenew) })
}

// opContext 释放/取消不能复用可能已被客户端取消的请求 context。
func (l *redisChannelCapacityLease) opContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), channelCapacityOpTimeout)
}

func (l *redisChannelCapacityLease) CancelBeforeDispatch(ctx context.Context) error {
	if l.dispatched.Load() {
		// 已派发的尝试不允许撤销 RPM，降级为普通释放
		return l.Release(ctx)
	}
	if l.finished.Swap(true) {
		return nil
	}
	l.stop()
	if !channelCapacityRedisAvailable() {
		return nil
	}
	opCtx, cancel := l.opContext(ctx)
	defer cancel()
	err := channelCapacityCancelScript.Run(opCtx, common.RDB,
		[]string{channelCapacityRPMKey(l.channelID), channelCapacityInflightKey(l.channelID)},
		l.attemptID).Err()
	if err != nil && err != redis.Nil {
		// 失败交给窗口过期 / 租约 TTL 兜底，不影响客户请求
		recordChannelCapacityRedisError("cancel", err)
		return err
	}
	return nil
}

func (l *redisChannelCapacityLease) Release(ctx context.Context) error {
	if l.finished.Swap(true) {
		return nil
	}
	l.stop()
	if !l.hasInflight || !channelCapacityRedisAvailable() {
		return nil
	}
	opCtx, cancel := l.opContext(ctx)
	defer cancel()
	err := common.RDB.ZRem(opCtx, channelCapacityInflightKey(l.channelID), l.attemptID).Err()
	if err != nil && err != redis.Nil {
		recordChannelCapacityRedisError("release", err)
		return err
	}
	return nil
}

func (l *redisChannelCapacityLease) startRenewLoop() {
	gopool.Go(func() {
		ticker := time.NewTicker(channelCapacityRenewInterval)
		defer ticker.Stop()
		deadline := time.Now().Add(channelCapacityMaxRenewAge)
		for {
			select {
			case <-l.stopRenew:
				return
			case <-ticker.C:
				if time.Now().After(deadline) {
					common.SysError(fmt.Sprintf("channel capacity lease renew aged out: channel_id=%d attempt_id=%s", l.channelID, l.attemptID))
					return
				}
				if !channelCapacityRedisAvailable() {
					continue
				}
				opCtx, cancel := context.WithTimeout(context.Background(), channelCapacityOpTimeout)
				alive, err := channelCapacityRenewScript.Run(opCtx, common.RDB,
					[]string{channelCapacityInflightKey(l.channelID)},
					l.attemptID, channelCapacityLeaseTTLMs, channelCapacityInflightTTLMs).Int64()
				cancel()
				if err != nil {
					// 续租失败不切断客户响应；连续失败最终由租约 TTL 回收
					recordChannelCapacityRedisError("renew", err)
					continue
				}
				if alive == 0 {
					// member 已被释放或回收，续租使命结束
					return
				}
			}
		}
	})
}

// TryAcquireChannelCapacity 为一次即将开始的上游尝试原子申请容量。
//
// mode 取 operation_setting.ChannelCapacityMode*：
//   - shadow：始终放行并写入真实计数，达限时 WouldBlock=true
//   - enforce：达限时拒绝且不写入任何记录
//
// capacity 无任何限额（HasLimit()==false）时直接放行，零 Redis 开销。
// Redis 不可用时：shadow 放行（fail open），enforce 拒绝（fail closed，Reason=redis_error）。
func TryAcquireChannelCapacity(ctx context.Context, channelID int, capacity *dto.ChannelCapacitySettings, mode string, attemptID string) ChannelCapacityDecision {
	allowedNoop := ChannelCapacityDecision{Allowed: true, Lease: noopChannelCapacityLease{}}
	if mode != operation_setting.ChannelCapacityModeShadow && mode != operation_setting.ChannelCapacityModeEnforce {
		return allowedNoop
	}
	if !capacity.HasLimit() {
		return allowedNoop
	}
	enforce := mode == operation_setting.ChannelCapacityModeEnforce
	if !channelCapacityRedisAvailable() {
		recordChannelCapacityRedisError("acquire", fmt.Errorf("redis unavailable"))
		if enforce {
			return ChannelCapacityDecision{
				Allowed:      false,
				Reason:       ChannelCapacityReasonRedisError,
				RetryAfterMs: channelCapacityMinRetryMs,
				Lease:        noopChannelCapacityLease{},
			}
		}
		return ChannelCapacityDecision{Allowed: true, Reason: ChannelCapacityReasonRedisError, Lease: noopChannelCapacityLease{}}
	}

	enforceFlag := 0
	if enforce {
		enforceFlag = 1
	}
	if ctx == nil {
		ctx = context.Background()
	}
	opCtx, cancel := context.WithTimeout(ctx, channelCapacityOpTimeout)
	defer cancel()
	raw, err := channelCapacityAcquireScript.Run(opCtx, common.RDB,
		[]string{channelCapacityRPMKey(channelID), channelCapacityInflightKey(channelID), channelCapacityStatsKey(channelID)},
		capacity.RPM, capacity.MaxConcurrency, enforceFlag, attemptID,
		channelCapacityWindowMs, channelCapacityLeaseTTLMs,
		channelCapacityRPMKeyTTLMs, channelCapacityInflightTTLMs, channelCapacityStatsTTLMs).Result()
	if err != nil {
		recordChannelCapacityRedisError("acquire", err)
		if enforce {
			return ChannelCapacityDecision{
				Allowed:      false,
				Reason:       ChannelCapacityReasonRedisError,
				RetryAfterMs: channelCapacityMinRetryMs,
				Lease:        noopChannelCapacityLease{},
			}
		}
		return ChannelCapacityDecision{Allowed: true, Reason: ChannelCapacityReasonRedisError, Lease: noopChannelCapacityLease{}}
	}

	values, ok := raw.([]interface{})
	if !ok || len(values) < 5 {
		recordChannelCapacityRedisError("acquire", fmt.Errorf("unexpected script result: %v", raw))
		if enforce {
			return ChannelCapacityDecision{
				Allowed:      false,
				Reason:       ChannelCapacityReasonRedisError,
				RetryAfterMs: channelCapacityMinRetryMs,
				Lease:        noopChannelCapacityLease{},
			}
		}
		return ChannelCapacityDecision{Allowed: true, Reason: ChannelCapacityReasonRedisError, Lease: noopChannelCapacityLease{}}
	}
	allowed, _ := values[0].(int64)
	reason, _ := values[1].(string)
	retryAfterMs, _ := values[2].(int64)
	rpmUsed, _ := values[3].(int64)
	inflightUsed, _ := values[4].(int64)

	decision := ChannelCapacityDecision{
		Allowed:      allowed == 1,
		WouldBlock:   allowed == 1 && reason != "",
		Reason:       reason,
		RetryAfterMs: retryAfterMs,
		RPMUsed:      rpmUsed,
		InflightUsed: inflightUsed,
	}
	if !decision.Allowed {
		decision.Lease = noopChannelCapacityLease{}
		return decision
	}

	lease := &redisChannelCapacityLease{
		channelID:   channelID,
		attemptID:   attemptID,
		hasRPM:      capacity.RPM > 0,
		hasInflight: capacity.MaxConcurrency > 0,
		stopRenew:   make(chan struct{}),
	}
	if lease.hasInflight {
		lease.startRenewLoop()
	}
	decision.Lease = lease
	return decision
}
