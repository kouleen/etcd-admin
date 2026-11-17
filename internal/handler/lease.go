package handler

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/kouleen/etcd-admin/internal/etcdcli"
)

// leaseIDToString 把 int64 租约 ID 转成字符串，
// 避免超过 JS Number.MAX_SAFE_INTEGER 的 ID 在前端丢失精度。
func leaseIDToString(id int64) string {
	return strconv.FormatInt(id, 10)
}

// LeaseHandler 处理租约（Lease）相关请求。
type LeaseHandler struct {
	manager *etcdcli.Manager
}

func NewLeaseHandler(m *etcdcli.Manager) *LeaseHandler {
	return &LeaseHandler{manager: m}
}

type leaseItem struct {
	// ID 用字符串承载 int64，防止 JavaScript Number 精度丢失。
	ID         string   `json:"id"`
	TTL        int64    `json:"ttl"`
	GrantedTTL int64    `json:"grantedTtl"`
	Keys       []string `json:"keys"`
}

// List 列出所有租约及其剩余 TTL 与绑定的键。
// GET /api/leases
func (h *LeaseHandler) List(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	resp, err := cli.Leases(c)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	items := make([]leaseItem, 0, len(resp.Leases))
	for _, lease := range resp.Leases {
		item := leaseItem{ID: leaseIDToString(int64(lease.ID)), Keys: []string{}}
		ttl, err := cli.TimeToLive(c, lease.ID, clientv3.WithAttachedKeys())
		if err == nil {
			item.TTL = ttl.TTL
			item.GrantedTTL = ttl.GrantedTTL
			for _, k := range ttl.Keys {
				item.Keys = append(item.Keys, string(k))
			}
		}
		items = append(items, item)
	}
	ok(ctx, map[string]interface{}{"items": items})
}

// Grant 创建一个指定 TTL（秒）的租约。
// POST /api/leases  body: {ttl}
func (h *LeaseHandler) Grant(c context.Context, ctx *app.RequestContext) {
	var body struct {
		TTL int64 `json:"ttl"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	if body.TTL <= 0 {
		fail(ctx, consts.StatusBadRequest, errInvalidTTL)
		return
	}
	resp, err := h.manager.Get().Grant(c, body.TTL)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, map[string]interface{}{"id": leaseIDToString(int64(resp.ID)), "ttl": resp.TTL})
}

// KeepAlive 对租约做一次续期。
// POST /api/leases/:id/keepalive
func (h *LeaseHandler) KeepAlive(c context.Context, ctx *app.RequestContext) {
	id, err := parseLeaseID(ctx)
	if err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	resp, err := h.manager.Get().KeepAliveOnce(c, clientv3.LeaseID(id))
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, map[string]interface{}{"id": leaseIDToString(int64(resp.ID)), "ttl": resp.TTL})
}

// Revoke 吊销租约，绑定该租约的键会被一并删除。
// DELETE /api/leases/:id
func (h *LeaseHandler) Revoke(c context.Context, ctx *app.RequestContext) {
	id, err := parseLeaseID(ctx)
	if err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	if _, err := h.manager.Get().Revoke(c, clientv3.LeaseID(id)); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

func parseLeaseID(ctx *app.RequestContext) (int64, error) {
	return strconv.ParseInt(ctx.Param("id"), 10, 64)
}
