package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"

	"github.com/kouleen/etcd-admin/internal/etcdcli"
	"github.com/kouleen/etcd-admin/internal/handler"
	"github.com/kouleen/etcd-admin/internal/sampler"
)

// Register 注册全部 API 路由与静态页面资源。
// 前端资源经 go:embed 编译进二进制（见 internal/router/static.go 与 web/web.go）。
// 路由分两层：
//   - 公共路由（health/status/connection/*）：未连接时也可访问，负责探测与建立连接
//   - 业务路由（protected 子组）：统一经 handler.RequireConnection 守卫，
//     未持有 etcd 客户端时返回 503，避免 handler 解引用 nil 客户端 panic
//
// staticCacheMaxAge 控制 /static 资源的浏览器缓存（秒）：
//   - 0：开发模式，强制 no-cache 每次回源
//   - 正数：生产模式，public, max-age=N, immutable，配合资源 URL 的 ?v= 版本号失效
//
// 根路由 "/" 根据当前 etcd 连接状态动态返回：
//   - 已连接: 内嵌的 index.html（主控制台）
//   - 未连接: 内嵌的 entry.html（连接入口表单）
func Register(h *server.Hertz, m *etcdcli.Manager, sp *sampler.Sampler, staticCacheMaxAge int) {
	kv := handler.NewKVHandler(m)
	lease := handler.NewLeaseHandler(m)
	cluster := handler.NewClusterHandler(m)
	watch := handler.NewWatchHandler(m)
	conn := handler.NewConnectionHandler(m)
	auth := handler.NewAuthHandler(m)
	metrics := handler.NewMetricsHandler(m, sp)

	// /static 静态资源缓存策略（由环境变量 STATIC_CACHE_MAX_AGE 控制）：
	//   - 0（默认，开发模式）：强制 no-cache，浏览器每次回源，便于频繁改 JS/CSS
	//   - >0（生产模式）：长缓存 + immutable。资源 URL 均带 ?v= 版本号，
	//     内容变更时版本号随 HTML 一起升级为新 URL，旧缓存自动失效
	h.Use(func(c context.Context, ctx *app.RequestContext) {
		if strings.HasPrefix(string(ctx.Path()), "/static/") {
			if staticCacheMaxAge > 0 {
				ctx.Header("Cache-Control", fmt.Sprintf("public, max-age=%d, immutable", staticCacheMaxAge))
			} else {
				ctx.Header("Cache-Control", "no-cache, no-store, must-revalidate")
				ctx.Header("Pragma", "no-cache")
				ctx.Header("Expires", "0")
			}
		}
		ctx.Next(c)
	})

	api := h.Group("/api")
	{
		api.GET("/health", func(c context.Context, ctx *app.RequestContext) {
			ctx.JSON(200, handler.APIResp{Code: 0, Message: "ok", Data: map[string]string{"status": "ok"}})
		})

		// 连接状态（轻量：entry 页用）
		api.GET("/status", func(c context.Context, ctx *app.RequestContext) {
			ctx.JSON(200, handler.APIResp{Code: 0, Message: "ok", Data: map[string]interface{}{
				"connected": m.IsConnected(),
				"info":      m.Info(),
			}})
		})

		// 连接管理：断连状态下仍须可用（探测/建立/断开连接），不挂连接守卫
		api.GET("/connection/current", conn.Current)
		api.POST("/connection/test", conn.Test)
		api.POST("/connection/connect", conn.Connect)
		api.POST("/connection/disconnect", conn.Disconnect)
	}

	// 业务路由统一挂载连接守卫：Manager 未持有客户端时返回 503 并中止，
	// 避免 handler 解引用 nil 客户端触发 panic。子组 basePath 仍为 /api，路径不变。
	protected := api.Group("/", handler.RequireConnection(m))
	{
		// 集群状态、告警与维护
		protected.GET("/cluster/status", cluster.Status)
		protected.GET("/cluster/alarms", cluster.Alarms)
		protected.POST("/cluster/alarms/disarm", cluster.Disarm)
		protected.POST("/maintenance/compact", cluster.Compact)
		protected.POST("/maintenance/defrag", cluster.Defrag)

		// 键值管理
		protected.GET("/kv", kv.List)
		protected.GET("/kv/tree", kv.Tree)
		protected.GET("/kv/search", kv.Search)
		protected.GET("/kv/history", kv.History)
		protected.GET("/kv/detail", kv.Detail)
		protected.GET("/kv/export", kv.Export)
		protected.PUT("/kv", kv.Put)
		protected.DELETE("/kv", kv.Delete)
		protected.POST("/kv/import", kv.Import)
		protected.POST("/kv/batch-delete", kv.BatchDelete)

		// 实时监听（SSE）
		protected.GET("/watch", watch.Watch)

		// 租约管理
		protected.GET("/leases", lease.List)
		protected.POST("/leases", lease.Grant)
		protected.POST("/leases/:id/keepalive", lease.KeepAlive)
		protected.DELETE("/leases/:id", lease.Revoke)

		// 认证、用户与角色
		protected.GET("/auth/status", auth.Status)
		protected.POST("/auth/enable", auth.Enable)
		protected.POST("/auth/disable", auth.Disable)
		protected.GET("/auth/users", auth.Users)
		protected.POST("/auth/users", auth.UserCreate)
		protected.DELETE("/auth/users", auth.UserDelete)
		protected.POST("/auth/users/password", auth.UserChangePassword)
		protected.POST("/auth/users/roles", auth.GrantRole)
		protected.DELETE("/auth/users/roles", auth.RevokeRole)
		protected.GET("/auth/roles", auth.Roles)
		protected.POST("/auth/roles", auth.RoleCreate)
		protected.DELETE("/auth/roles", auth.RoleDelete)
		protected.POST("/auth/roles/permissions", auth.GrantPermission)
		protected.DELETE("/auth/roles/permissions", auth.RevokePermission)

		// 指标监控
		protected.GET("/metrics/series", metrics.Series)
		protected.GET("/metrics/overview", metrics.Overview)
	}

	// 前端静态资源：全部来自编译期 go:embed 内嵌的 web.FS，运行时不读取磁盘
	h.GET("/static/*filepath", embeddedStatic)
	h.HEAD("/static/*filepath", embeddedStatic)

	// 独立入口页：始终可访问（方便前端断开连接后直接跳转回来）
	h.GET("/entry", func(c context.Context, ctx *app.RequestContext) {
		writeEmbeddedHTML(ctx, "entry.html")
	})
	h.HEAD("/entry", func(c context.Context, ctx *app.RequestContext) {
		writeEmbeddedHTML(ctx, "entry.html")
	})

	// 根路由：按 etcd 连接状态动态分发
	h.GET("/", func(c context.Context, ctx *app.RequestContext) {
		target := "entry.html"
		if m.IsConnected() {
			target = "index.html"
		}
		writeEmbeddedHTML(ctx, target)
	})
}
