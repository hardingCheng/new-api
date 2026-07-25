package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// GetChannelEconomics 渠道经营核算(每渠道的进货档位、赚亏结论与算式)。
// 数据由宿主机上的独立监控服务计算(渠道表只读),这里仅做管理端代理;
// 监控服务不可达时返回 success=false,渠道页的经营列静默降级,不影响其他功能。
func GetChannelEconomics(c *gin.Context) {
	url := os.Getenv("CHANNEL_ECONOMICS_URL")
	if url == "" {
		url = "http://host.docker.internal:13100/api/economics"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "经营数据服务不可达"})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "经营数据读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": json.RawMessage(body)})
}
