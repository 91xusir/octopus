package handlers

import (
	"net/http"
	"time"

	"github.com/91xusir/octopus/internal/conf"
	"github.com/91xusir/octopus/internal/server/middleware"
	"github.com/91xusir/octopus/internal/server/resp"
	"github.com/91xusir/octopus/internal/server/router"
	"github.com/91xusir/octopus/internal/update"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/update").
		Use(middleware.Auth()).
		AddRoute(
			router.NewRoute("", http.MethodGet).
				Handle(latest),
		).
		AddRoute(
			router.NewRoute("/now-version", http.MethodGet).
				Handle(getNowVersion),
		).
		AddRoute(
			router.NewRoute("", http.MethodPost).
				Handle(updateFunc),
		)
}

func latest(c *gin.Context) {
	latestInfo, err := update.GetLatestInfo()
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, *latestInfo)
}

func getNowVersion(c *gin.Context) {
	resp.Success(c, conf.Version)
}

func updateFunc(c *gin.Context) {
	err := update.UpdateCore()
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 先把成功响应写回客户端再重启: 重启会立刻关闭 HTTP 服务, 先重启会让前端
	// 收到连接中断而把更新误判为失败。留出一秒让响应送达, 前端随后自行刷新页面。
	resp.Success(c, "update success")
	go func() {
		time.Sleep(time.Second)
		update.RestartSelf()
	}()
}
