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
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
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

// issueChatDumpTicket 走真实 handler 换票，返回票面值。
func issueChatDumpTicket(t *testing.T, userId int) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/chatdump/viewer_ticket", nil)
	c.Set("id", userId)

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
	return ticket
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

	ticket := issueChatDumpTicket(t, root.Id)

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

func TestChatDumpViewerCookieOnlyAdmitsEnabledRoot(t *testing.T) {
	db := useChatDumpViewerTestDB(t)

	cases := []struct {
		name     string
		role     int
		status   int
		admitted bool
	}{
		{"启用中的 root", common.RoleRootUser, common.UserStatusEnabled, true},
		{"普通用户", common.RoleCommonUser, common.UserStatusEnabled, false},
		{"管理员但不是 root", common.RoleAdminUser, common.UserStatusEnabled, false},
		{"被禁用的 root", common.RoleRootUser, common.UserStatusDisabled, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := createChatDumpUser(t, db, "dump-"+tc.name, tc.role, tc.status)
			viewer, ok := consumeChatDumpTicket(issueChatDumpTicket(t, user.Id))
			require.True(t, ok)
			assert.Equal(t, tc.admitted, chatDumpViewerAuthorized(viewer))
		})
	}

	assert.False(t, chatDumpViewerAuthorized("not-a-viewer-token"))
	assert.False(t, chatDumpViewerAuthorized(""))
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
		http.MethodGet, "/_dump/?ticket="+issueChatDumpTicket(t, root.Id), nil))
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
