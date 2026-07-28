package controller

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
)

// 跨站登录:用户在非归属分站登录成功后,签发一次性令牌并跳回归属分站,
// 由归属分站验令牌建会话。令牌单次有效、短时效、绑定目标域名,单实例内存存储。

const crossLoginTicketTTL = 60 * time.Second

type crossLoginTicket struct {
	userId   int
	host     string
	expireAt time.Time
}

var crossLoginTickets sync.Map // code -> crossLoginTicket

func issueCrossLoginCode(userId int, host string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	code := hex.EncodeToString(buf)
	crossLoginTickets.Store(code, crossLoginTicket{
		userId:   userId,
		host:     host,
		expireAt: time.Now().Add(crossLoginTicketTTL),
	})
	return code, nil
}

// consumeCrossLoginCode 校验并消费令牌;不存在、过期或域名不符均失败,失败也不可重试。
func consumeCrossLoginCode(code string, host string) (int, bool) {
	if code == "" {
		return 0, false
	}
	value, ok := crossLoginTickets.LoadAndDelete(code)
	if !ok {
		return 0, false
	}
	ticket := value.(crossLoginTicket)
	if time.Now().After(ticket.expireAt) {
		return 0, false
	}
	if normalizeRequestHost(host) != ticket.host {
		return 0, false
	}
	return ticket.userId, true
}

func normalizeRequestHost(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.TrimSpace(host))
}

// crossStationLoginURL 判断登录用户是否应回归属分站;是则签发令牌并返回跳转地址,
// 否则返回空串(原地登录)。管理员跨站管理,不迁移。
func crossStationLoginURL(user *model.User, c *gin.Context) string {
	if user.Role >= common.RoleAdminUser {
		return ""
	}
	host := c.Request.Host
	if station := setting.GetStationByHost(host); station != nil && station.Group == user.Group {
		return ""
	}
	home := setting.GetStationDomainByGroup(user.Group)
	if home == "" || home == normalizeRequestHost(host) {
		return ""
	}
	code, err := issueCrossLoginCode(user.Id, home)
	if err != nil {
		common.SysLog("cross login code generation failed: " + err.Error())
		return ""
	}
	return "https://" + home + "/api/user/cross_login?code=" + code
}

const (
	crossLoginSignInPath  = "/sign-in"
	crossLoginConsolePath = "/dashboard"
)

// CrossLogin 归属站点侧入口:验一次性令牌 → 建登录会话 → 跳控制台。
// 只写 refresh cookie,访问令牌由前端启动时用该 cookie 换取;
// 令牌无效一律回登录页,不提示原因。
func CrossLogin(c *gin.Context) {
	userId, ok := consumeCrossLoginCode(c.Query("code"), c.Request.Host)
	if !ok {
		c.Redirect(http.StatusFound, crossLoginSignInPath)
		return
	}
	user, err := model.GetUserById(userId, false)
	if err != nil || user.Status != common.UserStatusEnabled {
		c.Redirect(http.StatusFound, crossLoginSignInPath)
		return
	}
	bundle, err := service.CreateLoginSession(user.Id, "cross_station", c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		common.SysLog("cross login session creation failed: " + err.Error())
		c.Redirect(http.StatusFound, crossLoginSignInPath)
		return
	}
	model.UpdateUserLastLoginAt(user.Id)
	service.WriteRefreshCookie(c, bundle.RefreshToken)
	setAuthNoStore(c)
	recordLoginAudit(user, c)
	c.Redirect(http.StatusFound, crossLoginConsolePath)
}
