package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseHTTPStatusCodeRanges_CommaSeparated(t *testing.T) {
	ranges, err := ParseHTTPStatusCodeRanges("401,403,500-599")
	require.NoError(t, err)
	require.Equal(t, []StatusCodeRange{
		{Start: 401, End: 401},
		{Start: 403, End: 403},
		{Start: 500, End: 599},
	}, ranges)
}

func TestParseHTTPStatusCodeRanges_MergeAndNormalize(t *testing.T) {
	ranges, err := ParseHTTPStatusCodeRanges("500-505,504,401,403,402")
	require.NoError(t, err)
	require.Equal(t, []StatusCodeRange{
		{Start: 401, End: 403},
		{Start: 500, End: 505},
	}, ranges)
}

func TestParseHTTPStatusCodeRanges_Invalid(t *testing.T) {
	_, err := ParseHTTPStatusCodeRanges("99,600,foo,500-400,500-")
	require.Error(t, err)
}

func TestParseHTTPStatusCodeRanges_NoComma_IsInvalid(t *testing.T) {
	_, err := ParseHTTPStatusCodeRanges("401 403")
	require.Error(t, err)
}

func TestShouldDisableByStatusCode(t *testing.T) {
	orig := GetAutomaticDisableStatusCodeRanges()
	t.Cleanup(func() { SetAutomaticDisableStatusCodeRanges(orig) })

	SetAutomaticDisableStatusCodeRanges([]StatusCodeRange{
		{Start: 401, End: 403},
		{Start: 500, End: 599},
	})

	require.True(t, ShouldDisableByStatusCode(401))
	require.True(t, ShouldDisableByStatusCode(403))
	require.False(t, ShouldDisableByStatusCode(404))
	require.True(t, ShouldDisableByStatusCode(500))
	require.False(t, ShouldDisableByStatusCode(200))
}

func TestShouldRetryByStatusCode(t *testing.T) {
	orig := GetAutomaticRetryStatusCodeRanges()
	t.Cleanup(func() { SetAutomaticRetryStatusCodeRanges(orig) })

	SetAutomaticRetryStatusCodeRanges([]StatusCodeRange{
		{Start: 429, End: 429},
		{Start: 500, End: 599},
	})

	require.True(t, ShouldRetryByStatusCode(429))
	require.True(t, ShouldRetryByStatusCode(500))
	require.True(t, ShouldRetryByStatusCode(504))
	require.True(t, ShouldRetryByStatusCode(524))
	require.False(t, ShouldRetryByStatusCode(400))
	require.False(t, ShouldRetryByStatusCode(200))
}

// 超时类状态码没有硬编码短路，完全由配置决定：配置覆盖到就重试，配置排除掉就不重试。
func TestShouldRetryByStatusCode_TimeoutCodesFollowConfig(t *testing.T) {
	orig := GetAutomaticRetryStatusCodeRanges()
	t.Cleanup(func() { SetAutomaticRetryStatusCodeRanges(orig) })

	SetAutomaticRetryStatusCodeRanges([]StatusCodeRange{
		{Start: 500, End: 503},
		{Start: 505, End: 523},
		{Start: 525, End: 599},
	})

	require.False(t, ShouldRetryByStatusCode(504))
	require.False(t, ShouldRetryByStatusCode(524))
	require.True(t, ShouldRetryByStatusCode(503))
	require.True(t, ShouldRetryByStatusCode(525))
}

func TestShouldRetryByStatusCode_Default(t *testing.T) {
	require.False(t, ShouldRetryByStatusCode(200))
	require.False(t, ShouldRetryByStatusCode(400))
	require.True(t, ShouldRetryByStatusCode(401))
	require.False(t, ShouldRetryByStatusCode(408))
	require.True(t, ShouldRetryByStatusCode(429))
	require.True(t, ShouldRetryByStatusCode(500))
	// 多渠道池里超时上限是渠道属性，换一条上限更高的渠道往往能成功，所以默认重试
	require.True(t, ShouldRetryByStatusCode(504))
	require.True(t, ShouldRetryByStatusCode(524))
	require.True(t, ShouldRetryByStatusCode(599))
}

func TestIsAlwaysSkipRetryStatusCode(t *testing.T) {
	// 状态码维度不再有硬编码的禁止重试名单
	require.False(t, IsAlwaysSkipRetryStatusCode(504))
	require.False(t, IsAlwaysSkipRetryStatusCode(524))
	require.False(t, IsAlwaysSkipRetryStatusCode(500))
}
