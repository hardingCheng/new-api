package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// useStationTestConfigs 装两个站点：z.example.com 归 z 组，cd.example.com 归 cd 组。
func useStationTestConfigs(t *testing.T) {
	t.Helper()
	previous := setting.StationConfigs2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateStationConfigsByJsonString(previous))
	})
	require.NoError(t, setting.UpdateStationConfigsByJsonString(
		`{"z.example.com":{"group":"z"},"cd.example.com":{"group":"cd"}}`,
	))
}

// 账号归属别的站点时，登录不能在当前站建会话，而要发一次性令牌把浏览器交回归属站点。
func TestSetupLoginHandsOffAccountOwnedByAnotherStation(t *testing.T) {
	useStationTestConfigs(t)

	previousDB := model.DB
	previousRedis := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
	})

	user := &model.User{
		Username: "z-station-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "z", AuthVersion: 1, AffCode: "aff-z-station-user",
	}
	require.NoError(t, db.Create(user).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "https://cd.example.com/api/user/login", nil)

	setupLogin(user, c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			CrossLoginURL string `json:"cross_login_url"`
			AccessToken   string `json:"access_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.NotEmpty(t, response.Data.CrossLoginURL)
	assert.Empty(t, response.Data.AccessToken, "交接响应里不能带本站访问令牌")

	handoff, err := url.Parse(response.Data.CrossLoginURL)
	require.NoError(t, err)
	assert.Equal(t, "z.example.com", handoff.Host)
	assert.Equal(t, "/api/user/cross_login", handoff.Path)

	// 本站不留会话，也不写 refresh cookie
	var sessions int64
	require.NoError(t, db.Model(&model.UserSession{}).Count(&sessions).Error)
	assert.Zero(t, sessions)
	for _, cookie := range recorder.Result().Cookies() {
		assert.NotEqual(t, service.RefreshCookieName, cookie.Name)
	}

	// 交接令牌指向本人，且在归属站点只能消费一次
	// (绑定域名/过期的行为见 cross_login_test.go；注意失败的尝试也会烧掉令牌)
	code := handoff.Query().Get("code")
	require.NotEmpty(t, code)
	userId, ok := consumeCrossLoginCode(code, "z.example.com")
	require.True(t, ok)
	assert.Equal(t, user.Id, userId)
	_, ok = consumeCrossLoginCode(code, "z.example.com")
	assert.False(t, ok, "令牌单次有效")
}

func TestCrossStationLoginURLKeepsUsersInPlace(t *testing.T) {
	useStationTestConfigs(t)

	cases := []struct {
		name string
		user model.User
		host string
	}{
		{
			name: "管理员跨站管理，不迁移",
			user: model.User{Id: 1, Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "z"},
			host: "cd.example.com",
		},
		{
			name: "已经在归属站点上",
			user: model.User{Id: 2, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "z"},
			host: "z.example.com",
		},
		{
			name: "分组没有对应站点",
			user: model.User{Id: 3, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"},
			host: "cd.example.com",
		},
		{
			name: "请求域名不是已配置站点",
			user: model.User{Id: 4, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"},
			host: "1.2.3.4:3000",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "https://"+tc.host+"/api/user/login", nil)
			assert.Empty(t, crossStationLoginURL(&tc.user, c))
		})
	}
}
