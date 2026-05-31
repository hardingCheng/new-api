package service

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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
	RuleId         string
	RuleName       string
	Group          string
	Model          string
	CooldownSecs   int
	RuleProbeCount int
	RuleProbeNeed  int
}

type ChannelBreakerProbe struct {
	Key string
}

type ChannelBreakerStatus struct {
	StateKey              string     `json:"state_key"`
	ChannelId             int        `json:"channel_id"`
	KeyHash               string     `json:"key_hash,omitempty"`
	State                 string     `json:"state"`
	Failures              int        `json:"failures"`
	OpenedAt              *time.Time `json:"opened_at,omitempty"`
	ProbeStartedAt        *time.Time `json:"probe_started_at,omitempty"`
	ProbeInFlight         int        `json:"probe_in_flight"`
	ProbeTotal            int        `json:"probe_total"`
	ProbeSuccess          int        `json:"probe_success"`
	CooldownSeconds       int        `json:"cooldown_seconds"`
	CooldownRemainingSecs int        `json:"cooldown_remaining_seconds"`
	RuleId                string     `json:"rule_id,omitempty"`
	RuleName              string     `json:"rule_name,omitempty"`
	Group                 string     `json:"group,omitempty"`
	Model                 string     `json:"model,omitempty"`
	RuleProbeCount        int        `json:"rule_probe_count,omitempty"`
	RuleProbeSuccessCount int        `json:"rule_probe_success_count,omitempty"`
}

type channelBreakerRuntimeRule struct {
	Id                   string
	Name                 string
	Enabled              bool
	FailureLimit         int
	Cooldown             time.Duration
	ProbeCount           int
	ProbeSuccessCount    int
	FailureStatusCodes   []operation_setting.StatusCodeRange
	FailureKeywords      []string
	ExcludePaths         []string
	DisableBreaker       bool
	OnlyKeyBreaker       bool
	IgnoreClientError4xx bool
}

func ShouldExcludeChannelBreaker(c *gin.Context) bool {
	return shouldExcludeChannelBreakerByRule(c, resolveChannelBreakerRule(c, types.ChannelError{}))
}

func CanUseChannelByBreaker(c *gin.Context, channelError types.ChannelError) bool {
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) || !channelError.AutoBan {
		return true
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
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
		return time.Since(state.OpenedAt) >= rule.Cooldown
	}
	if state.State != ChannelBreakerStateHalfOpen {
		return true
	}
	return state.ProbeTotal+state.ProbeInFlight < rule.ProbeCount
}

func AcquireChannelBreakerProbe(c *gin.Context, channelError types.ChannelError) bool {
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) || !channelError.AutoBan {
		return true
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
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
		if time.Since(state.OpenedAt) < rule.Cooldown {
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
		if state.ProbeTotal+state.ProbeInFlight >= rule.ProbeCount {
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
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) || !channelError.AutoBan {
		return false, ""
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
		return false, ""
	}
	key := channelBreakerKey(channelError)

	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	state := getOrCreateChannelBreakerState(key, c, rule)
	if !shouldTrip {
		if state.State == ChannelBreakerStateHalfOpen {
			recordProbeLocked(state, true)
			return evaluateProbeLocked(key, state, rule)
		}
		return false, ""
	}
	if state.State == ChannelBreakerStateHalfOpen {
		recordProbeLocked(state, false)
		return evaluateProbeLocked(key, state, rule)
	}
	state.Failures++
	failureLimit := rule.FailureLimit
	if state.Failures < failureLimit {
		saveChannelBreakerStateLocked(key, state)
		return false, fmt.Sprintf("channel breaker pending (failures: %d/%d)", state.Failures, failureLimit)
	}
	openBreakerLocked(state)
	saveChannelBreakerStateLocked(key, state)
	return true, "channel breaker opened"
}

func RecordChannelBreakerSuccess(c *gin.Context, channelError types.ChannelError) {
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || shouldExcludeChannelBreakerByRule(c, rule) {
		return
	}
	channelError, ok := normalizeChannelBreakerTarget(channelError, rule)
	if !ok {
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
		_, _ = evaluateProbeLocked(key, state, rule)
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

func ShouldTripChannelBreakerWithRule(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	rule := resolveChannelBreakerRule(c, channelError)
	if !rule.Enabled || rule.DisableBreaker {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if rule.IgnoreClientError4xx && err.StatusCode >= 400 && err.StatusCode <= 499 {
		return false
	}
	if shouldMatchStatusCodeRanges(rule.FailureStatusCodes, err.StatusCode) {
		return true
	}
	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, rule.FailureKeywords, true)
	return search
}

func ClearChannelBreaker(channelError types.ChannelError) {
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()
	deleteChannelBreakerStateLocked(channelBreakerKey(channelError))
}

func ClearChannelBreakerByStateKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()
	if loadChannelBreakerStateLocked(key) == nil {
		return false
	}
	deleteChannelBreakerStateLocked(key)
	return true
}

func ListChannelBreakerStatuses() []ChannelBreakerStatus {
	channelBreakerMu.Lock()
	defer channelBreakerMu.Unlock()

	keys := listChannelBreakerKeysLocked()
	statuses := make([]ChannelBreakerStatus, 0, len(keys))
	for _, key := range keys {
		state := loadChannelBreakerStateLocked(key)
		if state == nil || state.State == ChannelBreakerStateClosed {
			continue
		}
		statuses = append(statuses, buildChannelBreakerStatus(key, state))
	}
	return statuses
}

func GetChannelBreakerFailureThreshold() int {
	return common.GetChannelBreakerFailureLimit()
}

func getOrCreateChannelBreakerState(key string, c *gin.Context, rule channelBreakerRuntimeRule) *channelBreakerState {
	state := loadChannelBreakerStateLocked(key)
	if state == nil {
		state = &channelBreakerState{State: ChannelBreakerStateClosed}
		saveChannelBreakerStateLocked(key, state)
	}
	state.RuleId = rule.Id
	state.RuleName = rule.Name
	state.Group = channelBreakerContextGroup(c)
	state.Model = channelBreakerContextModel(c)
	state.CooldownSecs = int(rule.Cooldown.Seconds())
	state.RuleProbeCount = rule.ProbeCount
	state.RuleProbeNeed = rule.ProbeSuccessCount
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

func evaluateProbeLocked(key string, state *channelBreakerState, rule channelBreakerRuntimeRule) (bool, string) {
	probeSuccesses := rule.ProbeSuccessCount
	probeRequests := rule.ProbeCount
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
	return fmt.Sprintf("%d:%s", channelError.ChannelId, ChannelBreakerKeyHash(channelError.UsingKey))
}

func ChannelBreakerKeyHash(key string) string {
	return fmt.Sprintf("%x", hashString64(key))
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

func listChannelBreakerKeysLocked() []string {
	keySet := make(map[string]struct{})
	for key := range channelBreakerStates {
		keySet[key] = struct{}{}
	}
	if channelBreakerRedisEnabled() {
		iter := common.RDB.Scan(context.Background(), 0, "channel_breaker:*", 100).Iterator()
		for iter.Next(context.Background()) {
			key := strings.TrimPrefix(iter.Val(), "channel_breaker:")
			if key != "" {
				keySet[key] = struct{}{}
			}
		}
		if err := iter.Err(); err != nil {
			common.SysError("failed to scan channel breaker keys from redis: " + err.Error())
		}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	return keys
}

func buildChannelBreakerStatus(key string, state *channelBreakerState) ChannelBreakerStatus {
	channelId, keyHash := parseChannelBreakerKey(key)
	cooldown := getChannelBreakerCooldown()
	if state.CooldownSecs > 0 {
		cooldown = time.Duration(state.CooldownSecs) * time.Second
	}
	status := ChannelBreakerStatus{
		StateKey:              key,
		ChannelId:             channelId,
		KeyHash:               keyHash,
		State:                 state.State,
		Failures:              state.Failures,
		ProbeInFlight:         state.ProbeInFlight,
		ProbeTotal:            state.ProbeTotal,
		ProbeSuccess:          state.ProbeSuccess,
		CooldownSeconds:       int(cooldown.Seconds()),
		RuleId:                state.RuleId,
		RuleName:              state.RuleName,
		Group:                 state.Group,
		Model:                 state.Model,
		RuleProbeCount:        state.RuleProbeCount,
		RuleProbeSuccessCount: state.RuleProbeNeed,
	}
	if !state.OpenedAt.IsZero() {
		openedAt := state.OpenedAt
		status.OpenedAt = &openedAt
		remaining := cooldown - time.Since(state.OpenedAt)
		if remaining > 0 {
			status.CooldownRemainingSecs = int(remaining.Seconds())
			if remaining%time.Second != 0 {
				status.CooldownRemainingSecs++
			}
		}
	}
	if !state.ProbeStartedAt.IsZero() {
		probeStartedAt := state.ProbeStartedAt
		status.ProbeStartedAt = &probeStartedAt
	}
	return status
}

func parseChannelBreakerKey(key string) (int, string) {
	parts := strings.SplitN(key, ":", 2)
	channelId, _ := strconv.Atoi(parts[0])
	if len(parts) == 2 {
		return channelId, parts[1]
	}
	return channelId, ""
}

func resolveChannelBreakerRule(c *gin.Context, channelError types.ChannelError) channelBreakerRuntimeRule {
	fallback := defaultChannelBreakerRuntimeRule()
	if !fallback.Enabled {
		return fallback
	}
	rules := common.GetChannelBreakerRules()
	if len(rules) == 0 {
		return fallback
	}
	group := channelBreakerContextGroup(c)
	modelName := channelBreakerContextModel(c)
	channelId := strconv.Itoa(channelError.ChannelId)
	bestPriority := -1
	bestIndex := -1
	for i, rule := range rules {
		if !rule.Enabled {
			continue
		}
		priority := channelBreakerRulePriority(rule, group, modelName, channelId)
		if priority > bestPriority {
			bestPriority = priority
			bestIndex = i
		}
	}
	if bestIndex < 0 {
		return fallback
	}
	return runtimeRuleFromConfig(rules[bestIndex], fallback)
}

func channelBreakerRulePriority(rule common.ChannelBreakerRule, group, modelName, channelId string) int {
	switch rule.Scope {
	case common.ChannelBreakerScopeChannel:
		if channelId != "" && containsString(rule.Targets, channelId) {
			return 4
		}
	case common.ChannelBreakerScopeModel:
		if modelName != "" && containsString(rule.Targets, modelName) {
			return 3
		}
	case common.ChannelBreakerScopeGroup:
		if group != "" && containsString(rule.Targets, group) {
			return 2
		}
	case common.ChannelBreakerScopeGlobal:
		return 1
	}
	return -1
}

func runtimeRuleFromConfig(rule common.ChannelBreakerRule, fallback channelBreakerRuntimeRule) channelBreakerRuntimeRule {
	runtimeRule := fallback
	runtimeRule.Id = rule.Id
	runtimeRule.Name = rule.Name
	runtimeRule.Enabled = rule.Enabled
	runtimeRule.FailureLimit = rule.FailureLimit
	runtimeRule.Cooldown = time.Duration(rule.CooldownSeconds) * time.Second
	runtimeRule.ProbeCount = rule.ProbeCount
	runtimeRule.ProbeSuccessCount = rule.ProbeSuccessCount
	runtimeRule.DisableBreaker = rule.DisableBreaker
	runtimeRule.OnlyKeyBreaker = rule.OnlyKeyBreaker
	runtimeRule.IgnoreClientError4xx = rule.IgnoreClientError4xx
	if ranges, err := operation_setting.ParseHTTPStatusCodeRanges(rule.FailureStatusCodes); err == nil && len(ranges) > 0 {
		runtimeRule.FailureStatusCodes = ranges
	}
	if keywords := common.ParseChannelBreakerList(rule.FailureKeywords); len(keywords) > 0 {
		runtimeRule.FailureKeywords = normalizeKeywords(keywords)
	}
	if paths := common.ParseChannelBreakerList(rule.ExcludePaths); len(paths) > 0 {
		runtimeRule.ExcludePaths = paths
	}
	return runtimeRule
}

func defaultChannelBreakerRuntimeRule() channelBreakerRuntimeRule {
	return channelBreakerRuntimeRule{
		Id:                 "global-default",
		Name:               "全局默认规则",
		Enabled:            common.IsChannelBreakerEnabled(),
		FailureLimit:       common.GetChannelBreakerFailureLimit(),
		Cooldown:           getChannelBreakerCooldown(),
		ProbeCount:         common.GetChannelBreakerProbeCount(),
		ProbeSuccessCount:  common.GetChannelBreakerProbeSuccessCount(),
		FailureStatusCodes: operation_setting.GetAutomaticDisableStatusCodeRanges(),
		FailureKeywords:    operation_setting.GetAutomaticDisableKeywords(),
		ExcludePaths:       common.GetChannelBreakerExcludePaths(),
	}
}

func shouldExcludeChannelBreakerByRule(c *gin.Context, rule channelBreakerRuntimeRule) bool {
	if rule.DisableBreaker {
		return true
	}
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	path := c.Request.URL.Path
	for _, excludePath := range rule.ExcludePaths {
		if excludePath != "" && strings.HasPrefix(path, excludePath) {
			return true
		}
	}
	return false
}

func normalizeChannelBreakerTarget(channelError types.ChannelError, rule channelBreakerRuntimeRule) (types.ChannelError, bool) {
	if rule.OnlyKeyBreaker && channelError.UsingKey == "" {
		return channelError, false
	}
	return channelError, true
}

func channelBreakerContextGroup(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup); group != "" {
		return group
	}
	if group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup); group != "" {
		return group
	}
	return common.GetContextKeyString(c, constant.ContextKeyUserGroup)
}

func channelBreakerContextModel(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func normalizeKeywords(keywords []string) []string {
	normalized := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" {
			normalized = append(normalized, keyword)
		}
	}
	return normalized
}

func shouldMatchStatusCodeRanges(ranges []operation_setting.StatusCodeRange, code int) bool {
	if code < 100 || code > 599 {
		return false
	}
	for _, r := range ranges {
		if code < r.Start {
			return false
		}
		if code <= r.End {
			return true
		}
	}
	return false
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
