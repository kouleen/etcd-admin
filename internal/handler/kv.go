package handler

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/kouleen/etcd-admin/internal/etcdcli"
)

// KVHandler 处理键值读写相关请求。
type KVHandler struct {
	manager *etcdcli.Manager
}

func NewKVHandler(m *etcdcli.Manager) *KVHandler {
	return &KVHandler{manager: m}
}

// kvItem 列表/详情中返回的键值条目
type kvItem struct {
	Key            string `json:"key"`
	Value          string `json:"value,omitempty"`
	CreateRevision int64  `json:"createRevision"`
	ModRevision    int64  `json:"modRevision"`
	Version        int64  `json:"version"`
	// Lease 可能是超过 JS 安全整数范围的 int64，序列化为字符串；空串表示无租约。
	Lease string `json:"lease,omitempty"`
}

// parseLeaseIDString 解析前端以字符串形式回传的租约 ID，空串或 0 视为无租约。
func parseLeaseIDString(s string) (clientv3.LeaseID, error) {
	if s == "" || s == "0" {
		return 0, nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return clientv3.LeaseID(id), nil
}

// List 按前缀分页列出键。
// GET /api/kv?prefix=&limit=&keysOnly=&start=
// start 为上一页返回的 nextKey 游标，传入后从该 key 之后继续扫描。
func (h *KVHandler) List(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	prefix := string(ctx.Query("prefix"))
	start := string(ctx.Query("start"))
	limit, _ := strconv.ParseInt(string(ctx.Query("limit")), 10, 64)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	keysOnly := string(ctx.Query("keysOnly")) == "1" || string(ctx.Query("keysOnly")) == "true"

	opts := []clientv3.OpOption{clientv3.WithLimit(limit)}
	if start != "" {
		// 游标分页：限定在前缀区间内，从 start 开始。
		opts = append(opts, clientv3.WithRange(clientv3.GetPrefixRangeEnd(prefix)))
	} else {
		opts = append(opts, clientv3.WithPrefix())
	}
	if keysOnly {
		opts = append(opts, clientv3.WithKeysOnly())
	}

	queryKey := prefix
	if start != "" {
		queryKey = start
	}
	resp, err := cli.Get(c, queryKey, opts...)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}

	items := make([]kvItem, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		items = append(items, kvItem{
			Key:            string(kv.Key),
			Value:          string(kv.Value),
			CreateRevision: kv.CreateRevision,
			ModRevision:    kv.ModRevision,
			Version:        kv.Version,
			Lease:          leaseIDToString(kv.Lease),
		})
	}
	ok(ctx, map[string]interface{}{
		"items":    items,
		"count":    resp.Count,
		"more":     resp.More,
		"revision": resp.Header.Revision,
		"keysOnly": keysOnly,
		"nextKey":  nextKey(resp),
	})
}

// nextKey 在分页未拉完时返回下一页起始 key，供前端继续加载。
func nextKey(resp *clientv3.GetResponse) string {
	if !resp.More || len(resp.Kvs) == 0 {
		return ""
	}
	return string(resp.Kvs[len(resp.Kvs)-1].Key) + "\x00"
}

// Detail 查询单个键的完整内容。
// GET /api/kv/detail?key=
func (h *KVHandler) Detail(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	key := string(ctx.Query("key"))
	if key == "" {
		fail(ctx, consts.StatusBadRequest, errEmptyKey)
		return
	}
	resp, err := cli.Get(c, key)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	if len(resp.Kvs) == 0 {
		fail(ctx, consts.StatusNotFound, errKeyNotFound)
		return
	}
	kv := resp.Kvs[0]
	ok(ctx, kvItem{
		Key:            string(kv.Key),
		Value:          string(kv.Value),
		CreateRevision: kv.CreateRevision,
		ModRevision:    kv.ModRevision,
		Version:        kv.Version,
		Lease:          leaseIDToString(kv.Lease),
	})
}

// Put 写入或更新一个键。
// PUT /api/kv  body: {key, value, leaseId?}，leaseId 以字符串承载避免精度丢失。
func (h *KVHandler) Put(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	var body struct {
		Key     string `json:"key"`
		Value   string `json:"value"`
		LeaseID string `json:"leaseId"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	if body.Key == "" {
		fail(ctx, consts.StatusBadRequest, errEmptyKey)
		return
	}

	leaseID, err := parseLeaseIDString(body.LeaseID)
	if err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	opts := make([]clientv3.OpOption, 0, 1)
	if leaseID > 0 {
		opts = append(opts, clientv3.WithLease(leaseID))
	}
	resp, err := cli.Put(c, body.Key, body.Value, opts...)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, map[string]interface{}{"revision": resp.Header.Revision})
}

// Delete 删除单个键或按前缀删除。
// DELETE /api/kv?key=&byPrefix=
func (h *KVHandler) Delete(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	key := string(ctx.Query("key"))
	if key == "" {
		fail(ctx, consts.StatusBadRequest, errEmptyKey)
		return
	}
	var opts []clientv3.OpOption
	if strings.EqualFold(string(ctx.Query("byPrefix")), "true") || string(ctx.Query("byPrefix")) == "1" {
		opts = append(opts, clientv3.WithPrefix())
	}
	resp, err := cli.Delete(c, key, opts...)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, map[string]interface{}{"deleted": resp.Deleted})
}

// BatchDelete 一次删除多个键。
// POST /api/kv/batch-delete  body: {keys: []string}
func (h *KVHandler) BatchDelete(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	var body struct {
		Keys []string `json:"keys"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	if len(body.Keys) == 0 {
		fail(ctx, consts.StatusBadRequest, errEmptyKey)
		return
	}

	// 用事务保证批量删除的原子性。
	ops := make([]clientv3.Op, 0, len(body.Keys))
	for _, k := range body.Keys {
		if k == "" {
			continue
		}
		ops = append(ops, clientv3.OpDelete(k))
	}
	if len(ops) == 0 {
		fail(ctx, consts.StatusBadRequest, errEmptyKey)
		return
	}
	resp, err := cli.Txn(c).Then(ops...).Commit()
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	var deleted int64
	for _, r := range resp.Responses {
		if dr := r.GetResponseDeleteRange(); dr != nil {
			deleted += dr.GetDeleted()
		}
	}
	ok(ctx, map[string]interface{}{"deleted": deleted, "committed": resp.Succeeded})
}
