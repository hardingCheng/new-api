package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// 工作台数据由宿主机上的独立监控服务聚合(上游余额/报警/熔断聚合/错误日志,
// 服务只在本机与 docker 网关监听),这里仅做管理端只读代理,与渠道经营列
// (GetChannelEconomics)同一模式:监控服务不可达时返回 success=false,
// 工作台页显示"暂不可用",hub 其余功能不受影响。
func workbenchBaseURL() string {
	if v := os.Getenv("OPS_WORKBENCH_URL"); v != "" {
		return v
	}
	return "http://host.docker.internal:13100"
}

func proxyWorkbench(c *gin.Context, path string, query url.Values) {
	target := workbenchBaseURL() + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "工作台数据服务不可达"})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "工作台数据读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": json.RawMessage(body)})
}

// GetWorkbenchSummary 工作台总览:状态条/报警清单/趋势/容量水位/上游站余额。
func GetWorkbenchSummary(c *gin.Context) {
	proxyWorkbench(c, "/console/api/data", nil)
}

// GetWorkbenchErrors 错误日志页(游标分页),仅透传白名单参数。
func GetWorkbenchErrors(c *gin.Context) {
	query := url.Values{}
	for _, k := range []string{"channel_id", "before_id", "window", "limit"} {
		if v := c.Query(k); v != "" {
			query.Set(k, v)
		}
	}
	proxyWorkbench(c, "/console/api/errors", query)
}
