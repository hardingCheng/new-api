package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func withGroupEffortRatio(t *testing.T, jsonStr string) {
	t.Helper()
	saved := GroupEffortRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupEffortRatioByJSONString(saved))
	})
	require.NoError(t, UpdateGroupEffortRatioByJSONString(jsonStr))
}

func TestGetGroupEffortRatioMatchesGroupAndLevel(t *testing.T) {
	withGroupEffortRatio(t, `{"claudemax分组":{"max":2,"xhigh":1.5}}`)

	ratio, ok := GetGroupEffortRatio("claudemax分组", "max")
	require.True(t, ok)
	require.Equal(t, 2.0, ratio)

	ratio, ok = GetGroupEffortRatio("claudemax分组", "xhigh")
	require.True(t, ok)
	require.Equal(t, 1.5, ratio)

	_, ok = GetGroupEffortRatio("claudemax分组", "high")
	require.False(t, ok, "unconfigured level must not match")

	_, ok = GetGroupEffortRatio("default", "max")
	require.False(t, ok, "other groups must not inherit the rule")

	_, ok = GetGroupEffortRatio("claudemax分组", "")
	require.False(t, ok, "empty effort never matches")
}

func TestGetGroupEffortRatioIgnoresCaseAndWhitespace(t *testing.T) {
	withGroupEffortRatio(t, `{"claudemax分组":{"MAX":2}}`)

	ratio, ok := GetGroupEffortRatio("claudemax分组", " max ")
	require.True(t, ok)
	require.Equal(t, 2.0, ratio)
}

func TestUpdateGroupEffortRatioEmptyStringClearsRules(t *testing.T) {
	withGroupEffortRatio(t, `{"claudemax分组":{"max":2}}`)

	require.NoError(t, UpdateGroupEffortRatioByJSONString(""))
	_, ok := GetGroupEffortRatio("claudemax分组", "max")
	require.False(t, ok)
	require.Equal(t, "{}", GroupEffortRatio2JSONString())
}

func TestCheckGroupEffortRatioRejectsInvalidConfig(t *testing.T) {
	cases := map[string]string{
		"not json":          `{"claudemax分组":`,
		"flat map":          `{"claudemax分组":2}`,
		"zero ratio":        `{"claudemax分组":{"max":0}}`,
		"negative ratio":    `{"claudemax分组":{"max":-1}}`,
		"empty level":       `{"claudemax分组":{"  ":2}}`,
		"case-colliding":    `{"claudemax分组":{"max":2,"MAX":3}}`,
		"non-numeric ratio": `{"claudemax分组":{"max":"2"}}`,
	}
	for name, jsonStr := range cases {
		t.Run(name, func(t *testing.T) {
			require.Error(t, CheckGroupEffortRatio(jsonStr))
			// Update 与 Check 同一口径:坏配置不能进内存
			saved := GroupEffortRatio2JSONString()
			require.Error(t, UpdateGroupEffortRatioByJSONString(jsonStr))
			require.Equal(t, saved, GroupEffortRatio2JSONString())
		})
	}

	require.NoError(t, CheckGroupEffortRatio(`{}`))
	require.NoError(t, CheckGroupEffortRatio(`{"claudemax分组":{"max":2},"other":{"low":0.5}}`))
}
