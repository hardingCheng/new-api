package router

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// useChatDumpViewerTestDB 建一个只装 users 表的内存库，并在用例结束后清空
// 票/浏览期两张内存表，避免用例之间互相看见对方的凭据。
func useChatDumpViewerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	common.SessionSecret = "chatdump-viewer-test-secret"
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
		chatDumpGrants.Lock()
		chatDumpGrants.tickets = make(map[string]chatDumpGrant)
		chatDumpGrants.sessions = make(map[string]chatDumpGrant)
		chatDumpGrants.Unlock()
	})
	return db
}

func createChatDumpUser(t *testing.T, db *gorm.DB, username string, role, status int) *model.User {
	t.Helper()
	user := &model.User{
		Username: username, Password: "unused", Role: role,
		Status: status, Group: "default", AuthVersion: 1,
		AffCode: "aff-" + username, // aff_code 是唯一索引，同一用例建多个用户时不能都留空
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

// issueChatDumpTicket 走真实 handler 换票，返回票面值与它绑定的登录会话。
func issueChatDumpTicket(t *testing.T, userId int) (string, string) {
	t.Helper()
	bundle, err := service.CreateLoginSession(userId, "password", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/chatdump/viewer_ticket", nil)
	c.Set("id", userId)
	c.Set("session_id", bundle.Session.SID)

	ChatDumpViewerTicket(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			URL       string `json:"url"`
			ExpiresIn int    `json:"expires_in"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, int(chatDumpTicketTTL.Seconds()), response.Data.ExpiresIn)

	parsed, err := url.Parse(response.Data.URL)
	require.NoError(t, err)
	assert.Equal(t, chatDumpCookiePath+"/", parsed.Path)
	ticket := parsed.Query().Get("ticket")
	require.NotEmpty(t, ticket)
	return ticket, bundle.Session.SID
}

func firstTicket(ticket string, _ string) string { return ticket }

// consumeChatDumpTicketFor 换票并立刻兑成浏览期令牌。
func consumeChatDumpTicketFor(t *testing.T, userId int) (string, bool) {
	t.Helper()
	ticket, _ := issueChatDumpTicket(t, userId)
	return consumeChatDumpTicket(ticket)
}

func TestChatDumpTicketRequiresLoggedInUser(t *testing.T) {
	useChatDumpViewerTestDB(t)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/chatdump/viewer_ticket", nil)

	ChatDumpViewerTicket(c)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestChatDumpTicketIsSingleUse(t *testing.T) {
	db := useChatDumpViewerTestDB(t)
	root := createChatDumpUser(t, db, "dump-root", common.RoleRootUser, common.UserStatusEnabled)

	ticket, _ := issueChatDumpTicket(t, root.Id)

	viewer, ok := consumeChatDumpTicket(ticket)
	require.True(t, ok)
	require.NotEmpty(t, viewer)

	_, ok = consumeChatDumpTicket(ticket)
	assert.False(t, ok, "同一张票不能换第二次")

	_, ok = consumeChatDumpTicket("no-such-ticket")
	assert.False(t, ok)

	_, ok = consumeChatDumpTicket("")
	assert.False(t, ok)
}

func TestChatDumpTicketExpires(t *testing.T) {
	db := useChatDumpViewerTestDB(t)
	root := createChatDumpUser(t, db, "dump-root-expiry", common.RoleRootUser, common.UserStatusEnabled)

	expired := "expired-ticket"
	chatDumpGrants.Lock()
	chatDumpGrants.tickets[expired] = chatDumpGrant{
		userId:    root.Id,
		expiresAt: time.Now().Add(-time.Second),
	}
	chatDumpGrants.Unlock()

	_, ok := consumeChatDumpTicket(expired)
	assert.False(t, ok, "过期的票不能换浏览期")
}

func TestChatDumpViewerCookieOnlyAdmitsRoot(t *testing.T) {
	db := useChatDumpViewerTestDB(t)

	cases := []struct {
		name     string
		role     int
		admitted bool
	}{
		{"root", common.RoleRootUser, true},
		{"普通用户", common.RoleCommonUser, false},
		{"管理员但不是 root", common.RoleAdminUser, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := createChatDumpUser(t, db, "dump-"+tc.name, tc.role, common.UserStatusEnabled)
			viewer, ok := consumeChatDumpTicketFor(t, user.Id)
			require.True(t, ok)
			assert.Equal(t, tc.admitted, chatDumpViewerAuthorized(viewer))
		})
	}

	assert.False(t, chatDumpViewerAuthorized("not-a-viewer-token"))
	assert.False(t, chatDumpViewerAuthorized(""))
}

// 拿到浏览期之后账号才被禁用：cookie 必须立刻失效。
// （禁用中的账号压根建不出登录会话，所以只能先拿票再禁用）
func TestChatDumpViewerCookieDiesWhenRootGetsDisabled(t *testing.T) {
	db := useChatDumpViewerTestDB(t)
	root := createChatDumpUser(t, db, "dump-root-disable", common.RoleRootUser, common.UserStatusEnabled)

	viewer, ok := consumeChatDumpTicketFor(t, root.Id)
	require.True(t, ok)
	require.True(t, chatDumpViewerAuthorized(viewer))

	require.NoError(t, db.Model(&model.User{}).Where("id = ?", root.Id).
		Update("status", common.UserStatusDisabled).Error)

	assert.False(t, chatDumpViewerAuthorized(viewer), "账号被禁用后浏览期必须失效")
}

func TestChatDumpAuthExchangesTicketForViewerCookie(t *testing.T) {
	db := useChatDumpViewerTestDB(t)
	root := createChatDumpUser(t, db, "dump-root-gate", common.RoleRootUser, common.UserStatusEnabled)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/_dump")
	group.Use(chatDumpAuth)
	group.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "index") })
	group.GET("/list", func(c *gin.Context) { c.String(http.StatusOK, "list") })

	// 带票打开查看页：换成浏览期 cookie，并把票从地址栏洗掉
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/_dump/?ticket="+firstTicket(issueChatDumpTicket(t, root.Id)), nil))
	require.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, chatDumpCookiePath+"/", recorder.Header().Get("Location"))
	setCookie := recorder.Header().Get("Set-Cookie")
	require.Contains(t, setCookie, chatDumpCookieName+"=")
	assert.Contains(t, setCookie, "HttpOnly")
	assert.Contains(t, setCookie, "Path="+chatDumpCookiePath)

	viewerCookie := ""
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == chatDumpCookieName {
			viewerCookie = cookie.Value
		}
	}
	require.NotEmpty(t, viewerCookie)

	// 带 cookie 调数据接口：放行
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/_dump/list", nil)
	request.AddCookie(&http.Cookie{Name: chatDumpCookieName, Value: viewerCookie})
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "list", recorder.Body.String())
}

func TestChatDumpAuthRejectsRequestsWithoutCredentials(t *testing.T) {
	useChatDumpViewerTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/_dump")
	group.Use(chatDumpAuth)
	group.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "index") })
	group.GET("/list", func(c *gin.Context) { c.String(http.StatusOK, "list") })

	// 数据接口：不暴露路由存在，直接 404
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/_dump/list", nil))
	assert.Equal(t, http.StatusNotFound, recorder.Code)

	// 浏览器直接打开查看页：回登录页
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/_dump/", nil))
	require.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, "/sign-in", recorder.Header().Get("Location"))

	// 伪造的票也换不到浏览期
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/_dump/?ticket=forged", nil))
	require.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, "/sign-in", recorder.Header().Get("Location"))
	assert.False(t, strings.Contains(recorder.Header().Get("Set-Cookie"), chatDumpCookieName))
}

// codex 审查指出的问题：浏览期 cookie 原本只看"账号还是不是启用中的 root"，
// root 登出/被踢下线后这张 cookie 还能再用 8 小时。这里钉住修复后的行为。
func TestChatDumpViewerCookieDiesWithItsLoginSession(t *testing.T) {
	db := useChatDumpViewerTestDB(t)
	root := createChatDumpUser(t, db, "dump-root-revoke", common.RoleRootUser, common.UserStatusEnabled)

	ticket, sid := issueChatDumpTicket(t, root.Id)
	viewer, ok := consumeChatDumpTicket(ticket)
	require.True(t, ok)
	require.True(t, chatDumpViewerAuthorized(viewer), "刚换出来应该能用")

	// root 登出（会话作废）
	_, err := model.RevokeUserSession(root.Id, sid, "user_logout")
	require.NoError(t, err)

	assert.False(t, chatDumpViewerAuthorized(viewer), "原会话作废后浏览期必须立刻失效")
}

// codex 复查怀疑：改密码只升级用户的 AuthVersion、不动 SID，浏览期是否还有效？
// 结论用测试定：会话上盖着签发时的 AuthVersion，与用户当前版本不一致即作废。
func TestChatDumpViewerCookieDiesWhenAuthVersionAdvances(t *testing.T) {
	db := useChatDumpViewerTestDB(t)
	root := createChatDumpUser(t, db, "dump-root-authver", common.RoleRootUser, common.UserStatusEnabled)

	viewer, ok := consumeChatDumpTicketFor(t, root.Id)
	require.True(t, ok)
	require.True(t, chatDumpViewerAuthorized(viewer))

	// 模拟改密码：只把用户的安全版本 +1，会话行原样不动
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", root.Id).
		Update("auth_version", root.AuthVersion+1).Error)

	assert.False(t, chatDumpViewerAuthorized(viewer), "安全版本前进后浏览期必须失效")
}
