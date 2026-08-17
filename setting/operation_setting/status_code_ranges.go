package operation_setting

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/relaykit/types"
)

type StatusCodeRange struct {
	Start int
	End   int
}

var defaultAutomaticDisableStatusCodeRanges = []StatusCodeRange{{Start: 401, End: 401}}

// retry for 1xx, 3xx, 4xx(except 400/408), 5xx, and no retry for 2xx.
// 超时类状态码（504/524）也重试：多渠道池里超时上限是渠道属性而不是请求属性，
// 同一个请求换一条上限更高的渠道往往能正常完成，不重试等于把某条渠道的链路上限
// 直接暴露成客户可见的失败。
var defaultAutomaticRetryStatusCodeRanges = []StatusCodeRange{
	{Start: 100, End: 199},
	{Start: 300, End: 399},
	{Start: 401, End: 407},
	{Start: 409, End: 499},
	{Start: 500, End: 599},
}

// 熔断失败状态码：专供熔断失败计数使用，与自动重试、自动禁用状态码完全解耦，单独维护。
// 默认值与重试默认串一致：反复超时的渠道应当被熔断暂时移出轮询，而不是每次都拖满一次超时。
var defaultChannelBreakerFailureStatusCodeRanges = []StatusCodeRange{
	{Start: 100, End: 199},
	{Start: 300, End: 399},
	{Start: 401, End: 407},
	{Start: 409, End: 499},
	{Start: 500, End: 599},
}

var (
	automaticDisableStatusCodeRanges      atomic.Value
	automaticRetryStatusCodeRanges        atomic.Value
	channelBreakerFailureStatusCodeRanges atomic.Value
)

func init() {
	SetAutomaticDisableStatusCodeRanges(defaultAutomaticDisableStatusCodeRanges)
	SetAutomaticRetryStatusCodeRanges(defaultAutomaticRetryStatusCodeRanges)
	SetChannelBreakerFailureStatusCodeRanges(defaultChannelBreakerFailureStatusCodeRanges)
}

// 状态码维度不再硬编码禁止重试：是否重试完全由 AutomaticRetryStatusCodes 配置决定，
// 这样部署方能按自己的渠道池情况调整，而不是被一份写死的名单挡住。
// 流式已经开始推送之后的失败仍由 alwaysSkipRetryCodes（bad_response_body）拦住，
// 不会出现客户端收到两段响应体的情况。
var alwaysSkipRetryStatusCodes = map[int]struct{}{}

var alwaysSkipRetryCodes = map[types.ErrorCode]struct{}{
	types.ErrorCodeBadResponseBody: {},
}

func AutomaticDisableStatusCodesToString() string {
	return statusCodeRangesToString(GetAutomaticDisableStatusCodeRanges())
}

func AutomaticDisableStatusCodesFromString(s string) error {
	ranges, err := ParseHTTPStatusCodeRanges(s)
	if err != nil {
		return err
	}
	SetAutomaticDisableStatusCodeRanges(ranges)
	return nil
}

func ShouldDisableByStatusCode(code int) bool {
	return shouldMatchStatusCodeRanges(GetAutomaticDisableStatusCodeRanges(), code)
}

func AutomaticRetryStatusCodesToString() string {
	return statusCodeRangesToString(GetAutomaticRetryStatusCodeRanges())
}

func AutomaticRetryStatusCodesFromString(s string) error {
	ranges, err := ParseHTTPStatusCodeRanges(s)
	if err != nil {
		return err
	}
	SetAutomaticRetryStatusCodeRanges(ranges)
	return nil
}

func IsAlwaysSkipRetryStatusCode(code int) bool {
	_, exists := alwaysSkipRetryStatusCodes[code]
	return exists
}

func IsAlwaysSkipRetryCode(errorCode types.ErrorCode) bool {
	_, exists := alwaysSkipRetryCodes[errorCode]
	return exists
}

func ShouldRetryByStatusCode(code int) bool {
	if IsAlwaysSkipRetryStatusCode(code) {
		return false
	}
	return shouldMatchStatusCodeRanges(GetAutomaticRetryStatusCodeRanges(), code)
}

func ChannelBreakerFailureStatusCodesToString() string {
	return statusCodeRangesToString(GetChannelBreakerFailureStatusCodeRanges())
}

func ChannelBreakerFailureStatusCodesFromString(s string) error {
	ranges, err := ParseHTTPStatusCodeRanges(s)
	if err != nil {
		return err
	}
	SetChannelBreakerFailureStatusCodeRanges(ranges)
	return nil
}

func GetChannelBreakerFailureStatusCodeRanges() []StatusCodeRange {
	ranges, ok := channelBreakerFailureStatusCodeRanges.Load().([]StatusCodeRange)
	if !ok {
		return nil
	}
	return copyStatusCodeRanges(ranges)
}

func SetChannelBreakerFailureStatusCodeRanges(ranges []StatusCodeRange) {
	channelBreakerFailureStatusCodeRanges.Store(copyStatusCodeRanges(ranges))
}

func GetAutomaticDisableStatusCodeRanges() []StatusCodeRange {
	ranges, ok := automaticDisableStatusCodeRanges.Load().([]StatusCodeRange)
	if !ok {
		return nil
	}
	return copyStatusCodeRanges(ranges)
}

func SetAutomaticDisableStatusCodeRanges(ranges []StatusCodeRange) {
	automaticDisableStatusCodeRanges.Store(copyStatusCodeRanges(ranges))
}

func GetAutomaticRetryStatusCodeRanges() []StatusCodeRange {
	ranges, ok := automaticRetryStatusCodeRanges.Load().([]StatusCodeRange)
	if !ok {
		return nil
	}
	return copyStatusCodeRanges(ranges)
}

func SetAutomaticRetryStatusCodeRanges(ranges []StatusCodeRange) {
	automaticRetryStatusCodeRanges.Store(copyStatusCodeRanges(ranges))
}

func copyStatusCodeRanges(ranges []StatusCodeRange) []StatusCodeRange {
	if len(ranges) == 0 {
		return []StatusCodeRange{}
	}
	return append([]StatusCodeRange(nil), ranges...)
}

func statusCodeRangesToString(ranges []StatusCodeRange) string {
	if len(ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r.Start == r.End {
			parts = append(parts, strconv.Itoa(r.Start))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d-%d", r.Start, r.End))
	}
	return strings.Join(parts, ",")
}

func shouldMatchStatusCodeRanges(ranges []StatusCodeRange, code int) bool {
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

func ParseHTTPStatusCodeRanges(input string) ([]StatusCodeRange, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	input = strings.NewReplacer("，", ",").Replace(input)
	segments := strings.Split(input, ",")

	var ranges []StatusCodeRange
	var invalid []string

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		r, err := parseHTTPStatusCodeToken(seg)
		if err != nil {
			invalid = append(invalid, seg)
			continue
		}
		ranges = append(ranges, r)
	}

	if len(invalid) > 0 {
		return nil, fmt.Errorf("invalid http status code rules: %s", strings.Join(invalid, ", "))
	}
	if len(ranges) == 0 {
		return nil, nil
	}

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start == ranges[j].Start {
			return ranges[i].End < ranges[j].End
		}
		return ranges[i].Start < ranges[j].Start
	})

	merged := []StatusCodeRange{ranges[0]}
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if r.Start <= last.End+1 {
			if r.End > last.End {
				last.End = r.End
			}
			continue
		}
		merged = append(merged, r)
	}

	return merged, nil
}

func parseHTTPStatusCodeToken(token string) (StatusCodeRange, error) {
	token = strings.TrimSpace(token)
	token = strings.ReplaceAll(token, " ", "")
	if token == "" {
		return StatusCodeRange{}, fmt.Errorf("empty token")
	}

	if strings.Contains(token, "-") {
		parts := strings.Split(token, "-")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return StatusCodeRange{}, fmt.Errorf("invalid range token: %s", token)
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return StatusCodeRange{}, fmt.Errorf("invalid range start: %s", token)
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return StatusCodeRange{}, fmt.Errorf("invalid range end: %s", token)
		}
		if start > end {
			return StatusCodeRange{}, fmt.Errorf("range start > end: %s", token)
		}
		if start < 100 || end > 599 {
			return StatusCodeRange{}, fmt.Errorf("range out of bounds: %s", token)
		}
		return StatusCodeRange{Start: start, End: end}, nil
	}

	code, err := strconv.Atoi(token)
	if err != nil {
		return StatusCodeRange{}, fmt.Errorf("invalid status code: %s", token)
	}
	if code < 100 || code > 599 {
		return StatusCodeRange{}, fmt.Errorf("status code out of bounds: %s", token)
	}
	return StatusCodeRange{Start: code, End: code}, nil
}
