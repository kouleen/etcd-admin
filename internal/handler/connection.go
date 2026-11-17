package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/kouleen/etcd-admin/internal/etcdcli"
)

// ConnectionHandler 处理 etcd 连接的查询、测试与切换。
type ConnectionHandler struct {
	manager *etcdcli.Manager
}

func NewConnectionHandler(m *etcdcli.Manager) *ConnectionHandler {
	return &ConnectionHandler{manager: m}
}

// RequireConnection 业务路由统一守卫：Manager 未持有客户端（已断开/从未连接）时
// 直接返回 503 并中止处理链，避免各 handler 解引用 nil 客户端触发 panic
// （否则只能依赖 Hertz recovery 兜底为 500 并打印堆栈）。
// 连接管理类路由（current/test/connect/disconnect）不挂载该中间件。
func RequireConnection(m *etcdcli.Manager) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		if !m.IsConnected() {
			ctx.AbortWithStatusJSON(consts.StatusServiceUnavailable, APIResp{
				Code:    consts.StatusServiceUnavailable,
				Message: errNotConnected.Error(),
			})
			return
		}
		ctx.Next(c)
	}
}

type connectBody struct {
	Endpoints string `json:"endpoints"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

func (b connectBody) parseEndpoints() []string {
	parts := strings.Split(b.Endpoints, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Current 返回当前连接信息。
// GET /api/connection/current
func (h *ConnectionHandler) Current(c context.Context, ctx *app.RequestContext) {
	info := h.manager.Info()
	cli := h.manager.Get()

	// Manager 未持有可用客户端（已断开连接）。
	if cli == nil {
		ok(ctx, map[string]interface{}{
			"info":      info,
			"reachable": false,
			"connected": false,
		})
		return
	}

	status := map[string]interface{}{
		"info":      info,
		"reachable": false,
		"connected": true,
	}
	if st, err := cli.Status(c, info.Endpoints[0]); err == nil {
		status["reachable"] = true
		status["version"] = st.Version
		status["revision"] = st.Header.Revision
		status["dbSize"] = st.DbSize
		// uint64 标识转字符串，避免前端 Number 精度丢失。
		status["leader"] = strconv.FormatUint(st.Leader, 10)
	}
	ok(ctx, status)
}

// Test 仅探测给定参数能否连通，不改变当前连接。
// POST /api/connection/test
func (h *ConnectionHandler) Test(c context.Context, ctx *app.RequestContext) {
	var body connectBody
	if err := ctx.BindJSON(&body); err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	endpoints := body.parseEndpoints()
	if len(endpoints) == 0 {
		fail(ctx, consts.StatusBadRequest, errEmptyEndpoints)
		return
	}

	probeCtx, cancel := context.WithTimeout(c, 10*time.Second)
	defer cancel()
	st, err := etcdcli.TestConnection(probeCtx, endpoints, strings.TrimSpace(body.Username), body.Password)
	if err != nil {
		fail(ctx, consts.StatusBadGateway, err)
		return
	}
	ok(ctx, map[string]interface{}{
		"reachable": true,
		"version":   st.Version,
		"revision":  st.Header.Revision,
		"dbSize":    st.DbSize,
		"leader":    strconv.FormatUint(st.Leader, 10),
	})
}

// Connect 切换到新的 etcd 连接。
// POST /api/connection/connect
func (h *ConnectionHandler) Connect(c context.Context, ctx *app.RequestContext) {
	var body connectBody
	if err := ctx.BindJSON(&body); err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	endpoints := body.parseEndpoints()
	if len(endpoints) == 0 {
		fail(ctx, consts.StatusBadRequest, errEmptyEndpoints)
		return
	}

	connectCtx, cancel := context.WithTimeout(c, 10*time.Second)
	defer cancel()
	if err := h.manager.Reconnect(connectCtx, endpoints, strings.TrimSpace(body.Username), body.Password); err != nil {
		fail(ctx, consts.StatusBadGateway, err)
		return
	}
	ok(ctx, h.manager.Info())
}

// Disconnect 主动断开当前 etcd 连接，使 Manager 进入未连接状态。
// POST /api/connection/disconnect
func (h *ConnectionHandler) Disconnect(c context.Context, ctx *app.RequestContext) {
	h.manager.Disconnect()
	ok(ctx, nil)
}
