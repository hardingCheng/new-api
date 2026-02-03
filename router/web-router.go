package router

import (
	"embed"
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

func SetWebRouter(router *gin.Engine, buildFS embed.FS, indexPage []byte, externalFrontendPath string) {
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())

	// 如果设置了外部前端路径，使用外部文件系统，否则使用嵌入的文件系统
	if externalFrontendPath != "" {
		router.Use(static.Serve("/", common.ExternalFolder(externalFrontendPath)))
	} else {
		router.Use(static.Serve("/", common.EmbedFolder(buildFS, "web/dist")))
	}

	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")

		// 如果使用外部前端路径，从文件读取最新的 index.html
		if externalFrontendPath != "" {
			indexPath := externalFrontendPath + "/index.html"
			content, err := os.ReadFile(indexPath)
			if err == nil {
				c.Data(http.StatusOK, "text/html; charset=utf-8", content)
				return
			}
			// 如果读取失败，fallback 到嵌入的版本
			common.SysLog("failed to read external index.html, fallback to embedded version: " + err.Error())
		}

		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	})
}
