package service

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ChannelBreakerStateClosed   = "closed"
	ChannelBreakerStateOpen     = "open"
	ChannelBreakerStateHalfOpen = "half-open"
)

var (
	channelBreakerMu       sync.Mutex
	channelBreakerStates   = map[string]*channelBreakerState{}
	channelBreakerCooldown = time.Duration(0)
)

type channelBreakerState struct {
	State          string
	Failures       int
	OpenedAt       time.Time
	ProbeStartedAt time.Time
	ProbeInFlight  int
	ProbeTotal     int
	ProbeSuccess   int
}

type ChannelBreakerProbe struct {
	Key string
}

func ShouldExcludeChannelBreaker(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	path := c.Request.URL.Path
	for _, excludePath := range common.GetChannelBreakerExcludePaths() {
		if excludePath != "" && strings.HasPrefix(path, excludePath) {
			return true
		}
	}
	return false
}

func CanUseChannelByBreaker(c *gin.Context, channelError types.ChannelError) bool {
	if !common.IsChannelBreakerEnabled() || ShouldExcludeChannelBreaker(c) || !channelError.AutoBan {
		return true
	}
	key := channelBreakerKey(channelError)

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := loadChannelBreakerStateLocked(key)
	if state == nil || state.State == ChannelBreakerStateClosed {
		return true
	}
	if state.State == ChannelBreakerStateOpen {
		return time.Since(state.OpenedAt) >= getChannelBreakerCooldown()
	}
	if state.State != ChannelBreakerStateHalfOpen {
		return true
	}
	return state.ProbeTotal+state.ProbeInFlight < common.GetChannelBreakerProbeCount()
}

func AcquireChannelBreakerProbe(c *gin.Context, channelError types.ChannelError) bool {
	if !common.IsChannelBreakerEnabled() || ShouldExcludeChannelBreaker(c) || !channelError.AutoBan {
		return true
	}
	key := channelBreakerKey(channelError)

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := loadChannelBreakerStateLocked(key)
	if state == nil || state.State == ChannelBreakerStateClosed {
		return true
	}
	if state.State == ChannelBreakerStateOpen {
		if time.Since(state.OpenedAt) < getChannelBreakerCooldown() {
			return false
		}
		state.State = ChannelBreakerStateHalfOpen
		state.ProbeStartedAt = time.Now()
		state.ProbeInFlight = 0
		state.ProbeTotal = 0
		state.ProbeSuccess = 0
		saveChannelBreakerStateLocked(key, state)
	}
	if state.State == ChannelBreakerStateHalfOpen {
		if state.ProbeTotal+state.ProbeInFlight >= common.GetChannelBreakerProbeCount() {
			return false
		}
		state.ProbeInFlight++
		saveChannelBreakerStateLocked(key, state)
		if c != nil {
			c.Set("channel_breaker_probe", ChannelBreakerProbe{Key: key})
		}
	}
	return true
}

func AllowChannelByBreaker(c *gin.Context, channelError types.ChannelError) bool {
	if !CanUseChannelByBreaker(c, channelError) {
		return false
	}
	return AcquireChannelBreakerProbe(c, channelError)
}

func RecordChannelBreakerFailure(c *gin.Context, channelError types.ChannelError, shouldTrip bool) (bool, string) {
	if !common.IsChannelBreakerEnabled() || ShouldExcludeChannelBreaker(c) || !channelError.AutoBan {
		return false, ""
	}
	key := channelBreakerKey(channelError)

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := getOrCreateChannelBreakerState(key)
	if !shouldTrip {
		if state.State == ChannelBreakerStateHalfOpen {
			recordProbeLocked(state, true)
			return evaluateProbeLocked(key, state)
		}
		return false, ""
	}
	if state.State == ChannelBreakerStateHalfOpen {
		recordProbeLocked(state, false)
		return evaluateProbeLocked(key, state)
	}
	state.Failures++
	failureLimit := common.GetChannelBreakerFailureLimit()
	if state.Failures < failureLimit {
		saveChannelBreakerStateLocked(key, state)
		return false, fmt.Sprintf("channel breaker pending (failures: %d/%d)", state.Failures, failureLimit)
	}
	openBreakerLocked(state)
	saveChannelBreakerStateLocked(key, state)
	return true, "channel breaker opened"
}

func RecordChannelBreakerSuccess(c *gin.Context, channelError types.ChannelError) {
	if !common.IsChannelBreakerEnabled() || ShouldExcludeChannelBreaker(c) {
		return
	}
	key := channelBreakerKey(channelError)

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := loadChannelBreakerStateLocked(key)
	if state == nil {
		return
	}
	if state.State == ChannelBreakerStateHalfOpen {
		recordProbeLocked(state, true)
		_, _ = evaluateProbeLocked(key, state)
		return
	}
	deleteChannelBreakerStateLocked(key)
}

func IsChannelBreakerOpenForKey(channelId int, usingKey string) bool {
	key := channelBreakerKey(types.ChannelError{ChannelId: channelId, UsingKey: usingKey})
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := loadChannelBreakerStateLocked(key)
	if state == nil || state.State == ChannelBreakerStateClosed {
		return false
	}
	if state.State == ChannelBreakerStateOpen && time.Since(state.OpenedAt) >= getChannelBreakerCooldown() {
		return false
	}
	if state.State == ChannelBreakerStateHalfOpen && state.ProbeTotal+state.ProbeInFlight < common.GetChannelBreakerProbeCount() {
		return false
	}
	return true
}

func ClearChannelBreaker(channelError types.ChannelError) {
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()
	deleteChannelBreakerStateLocked(channelBreakerKey(channelError))
}

func GetChannelBreakerFailureThreshold() int {
	return common.GetChannelBreakerFailureLimit()
}

func getOrCreateChannelBreakerState(key string) *channelBreakerState {
	state := loadChannelBreakerStateLocked(key)
	if state == nil {
		state = &channelBreakerState{State: ChannelBreakerStateClosed}
		saveChannelBreakerStateLocked(key, state)
	}
	return state
}

func recordProbeLocked(state *channelBreakerState, success bool) {
	if state.ProbeInFlight > 0 {
		state.ProbeInFlight--
	}
	state.ProbeTotal++
	if success {
		state.ProbeSuccess++
	}
}

func evaluateProbeLocked(key string, state *channelBreakerState) (bool, string) {
	probeSuccesses := common.GetChannelBreakerProbeSuccessCount()
	probeRequests := common.GetChannelBreakerProbeCount()
	if state.ProbeSuccess >= probeSuccesses {
		deleteChannelBreakerStateLocked(key)
		return false, "channel breaker closed after probe"
	}
	if state.ProbeTotal >= probeRequests {
		openBreakerLocked(state)
		saveChannelBreakerStateLocked(key, state)
		return true, fmt.Sprintf("channel breaker remains open after probe (%d/%d successes)", state.ProbeSuccess, state.ProbeTotal)
	}
	saveChannelBreakerStateLocked(key, state)
	return false, fmt.Sprintf("channel breaker probing (%d/%d successes, %d/%d completed)", state.ProbeSuccess, probeSuccesses, state.ProbeTotal, probeRequests)
}

func openBreakerLocked(state *channelBreakerState) {
	state.State = ChannelBreakerStateOpen
	state.Failures = 0
	state.OpenedAt = time.Now()
	state.ProbeStartedAt = time.Time{}
	state.ProbeInFlight = 0
	state.ProbeTotal = 0
	state.ProbeSuccess = 0
}

func channelBreakerKey(channelError types.ChannelError) string {
	if channelError.UsingKey == "" {
		return fmt.Sprintf("%d", channelError.ChannelId)
	}
	return fmt.Sprintf("%d:%x", channelError.ChannelId, hashString64(channelError.UsingKey))
}

func hashString64(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

func getChannelBreakerCooldown() time.Duration {
	if channelBreakerCooldown > 0 {
		return channelBreakerCooldown
	}
	return time.Duration(common.GetChannelBreakerCooldownSeconds()) * time.Second
}

func channelBreakerRedisEnabled() bool {
	return common.RedisEnabled && common.RDB != nil
}

func channelBreakerRedisKey(key string) string {
	return "channel_breaker:" + key
}

func loadChannelBreakerStateLocked(key string) *channelBreakerState {
	if channelBreakerRedisEnabled() {
		var state channelBreakerState
		data, err := common.RedisGet(channelBreakerRedisKey(key))
		if err == nil {
			if unmarshalErr := common.UnmarshalJsonStr(data, &state); unmarshalErr == nil {
				return &state
			}
		} else if err != redis.Nil {
			common.SysError("failed to load channel breaker from redis: " + err.Error())
		}
	}
	return channelBreakerStates[key]
}

func saveChannelBreakerStateLocked(key string, state *channelBreakerState) {
	if state == nil {
		deleteChannelBreakerStateLocked(key)
		return
	}
	channelBreakerStates[key] = state
	if !channelBreakerRedisEnabled() {
		return
	}
	data, err := common.Marshal(state)
	if err != nil {
		common.SysError("failed to marshal channel breaker: " + err.Error())
		return
	}
	ttl := getChannelBreakerCooldown() + time.Duration(common.GetChannelBreakerProbeCount()+common.GetChannelBreakerFailureLimit()+60)*time.Second
	if err := common.RDB.Set(context.Background(), channelBreakerRedisKey(key), string(data), ttl).Err(); err != nil {
		common.SysError("failed to save channel breaker to redis: " + err.Error())
	}
}

func deleteChannelBreakerStateLocked(key string) {
	delete(channelBreakerStates, key)
	if channelBreakerRedisEnabled() {
		if err := common.RDB.Del(context.Background(), channelBreakerRedisKey(key)).Err(); err != nil {
			common.SysError("failed to delete channel breaker from redis: " + err.Error())
		}
	}
}
