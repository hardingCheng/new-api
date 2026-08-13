package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStationConfigs(t *testing.T) {
	err := UpdateStationConfigsByJsonString(`{
		"Z.Open-API.ai ": {
			"group": "z",
			"oauth": {"github": {"client_id": "id-z", "client_secret": "sec-z"}},
			"brand": {
				"system_name": "Z站", "logo": "https://z/logo.png", "home_page_content": "<h1>z</h1>", "home_page_theme": "prism",
				"notice": "z 公告", "about": "z 关于",
				"announcements": [{"content": "上新", "type": "info"}]
			}
		},
		"d.open-api.ai": {
			"oauth": {"github": {"client_id": "id-d", "client_secret": ""}}
		}
	}`)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if s := GetStationByHost("z.open-api.ai"); s == nil || s.Group != "z" || s.Brand.SystemName != "Z站" {
		t.Fatalf("expected z station with normalized host key, got %+v", s)
	}
	if theme := ResolveHomePageTheme(GetStationByHost("z.open-api.ai")); theme != HomePageThemePrism {
		t.Fatalf("expected prism home page theme, got %q", theme)
	}
	if s := GetStationByHost("z.open-api.ai:443"); s == nil {
		t.Fatal("expected port to be stripped from host")
	}
	if s := GetStationByHost("z.open-api.ai"); s.Brand.Notice != "z 公告" || s.Brand.About != "z 关于" ||
		len(s.Brand.Announcements) != 1 || s.Brand.Announcements[0]["content"] != "上新" {
		t.Fatalf("expected notice/about/announcements parsed, got %+v", s.Brand)
	}
	if s := GetStationByHost("154.201.87.117:13000"); s != nil {
		t.Fatalf("expected nil for unconfigured host, got %+v", s)
	}
	if s := GetStationByHost(""); s != nil {
		t.Fatal("expected nil for empty host")
	}

	if client, ok := GetStationOAuthClient("z.open-api.ai", "github"); !ok || client.ClientId != "id-z" || client.ClientSecret != "sec-z" {
		t.Fatalf("expected z github client, got %+v ok=%v", client, ok)
	}
	if _, ok := GetStationOAuthClient("d.open-api.ai", "github"); ok {
		t.Fatal("expected incomplete credentials to fall back to global")
	}
	if _, ok := GetStationOAuthClient("z.open-api.ai", "discord"); ok {
		t.Fatal("expected unconfigured provider to fall back to global")
	}

	if got := GetStationDomainByGroup("z"); got != "z.open-api.ai" {
		t.Fatalf("expected z group to resolve to its station domain, got %q", got)
	}
	if got := GetStationDomainByGroup("hz"); got != "" {
		t.Fatalf("expected group without station to resolve empty, got %q", got)
	}
	if got := GetStationDomainByGroup(""); got != "" {
		t.Fatalf("expected empty group to resolve empty, got %q", got)
	}

	if err := UpdateStationConfigsByJsonString(""); err != nil {
		t.Fatalf("empty string should reset configs: %v", err)
	}
	if s := GetStationByHost("z.open-api.ai"); s != nil {
		t.Fatal("expected configs cleared after empty update")
	}
	if got := StationConfigs2JsonString(); got != "{}" {
		t.Fatalf("expected {} after reset, got %s", got)
	}

	if err := UpdateStationConfigsByJsonString("{not json"); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestHomePageThemeValidationAndFallback(t *testing.T) {
	require.NoError(t, UpdateStationConfigsByJsonString(`{
		"default.example.com": {"brand": {"home_page_theme": "default"}},
		"empty.example.com": {"brand": {}}
	}`))
	t.Cleanup(func() {
		require.NoError(t, UpdateStationConfigsByJsonString(""))
	})

	assert.Equal(t, HomePageThemeDefault, ResolveHomePageTheme(GetStationByHost("default.example.com")))
	assert.Equal(t, HomePageThemeDefault, ResolveHomePageTheme(GetStationByHost("empty.example.com")))
	assert.Equal(t, HomePageThemeDefault, ResolveHomePageTheme(nil))

	err := UpdateStationConfigsByJsonString(`{
		"invalid.example.com": {"brand": {"home_page_theme": "future"}}
	}`)
	require.ErrorContains(t, err, `unsupported home page theme "future"`)
	assert.Nil(t, GetStationByHost("invalid.example.com"))
}
