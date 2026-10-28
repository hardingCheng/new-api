package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// SetArkVideoRouter 注册火山方舟格式的视频任务入口（路径与方舟官方一致），
// 调用方把 Base URL 指到本站即可提交、查询 seedance 任务；计费、选路、重试全部复用视频任务链路。
func SetArkVideoRouter(router *gin.Engine) {
	arkRouter := router.Group(middleware.ArkVideoTasksPath)
	arkRouter.Use(middleware.RouteTag("relay"))
	arkRouter.Use(middleware.TokenAuth())
	{
		arkRouter.POST("",
			middleware.SystemPerformanceCheck(),
			middleware.ArkVideoSubmit(),
			middleware.Distribute(),
			controller.RelayTask,
		)
		arkRouter.GET("/:task_id",
			middleware.ArkVideoFetch(),
			middleware.Distribute(),
			controller.RelayTaskFetch,
		)
		arkRouter.GET("", middleware.ArkVideoNotSupported())
		arkRouter.DELETE("/:task_id", middleware.ArkVideoNotSupported())
	}
}
