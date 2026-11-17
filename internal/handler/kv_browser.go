package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// 单次扫描的最大键数，避免超大集群把内存打满。
const scanCap = 5000

// kvTreeNode 键树节点，目录与叶子复用同一结构。
type kvTreeNode struct {
	Name     string        `json:"name"`
	Path     string        `json:"path"`
	Dir      bool          `json:"dir"`
	Key      string        `json:"key,omitempty"`
	Lease    string        `json:"lease,omitempty"`
	Children []*kvTreeNode `json:"children,omitempty"`
}

// Tree 按 "/" 分层返回键树，供前端做类文件目录式浏览。
// GET /api/kv/tree?prefix=
func (h *KVHandler) Tree(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	prefix := string(ctx.Query("prefix"))

	resp, err := cli.Get(c, prefix, clientv3.WithPrefix(), clientv3.WithKeysOnly(), clientv3.WithLimit(scanCap))
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}

	root := &kvTreeNode{Name: "root", Path: "", Dir: true}
	for _, kv := range resp.Kvs {
		full := string(kv.Key)
		segments := strings.Split(full, "/")
		node := root
		path := ""
		for i, seg := range segments {
			if i > 0 {
				path += "/"
			}
			path += seg

			next := findChild(node.Children, seg)
			if next == nil {
				next = &kvTreeNode{Name: seg, Path: path}
				node.Children = append(node.Children, next)
			}
			if i == len(segments)-1 {
				// 该节点本身也是一个键（可能同时拥有子节点，即既是叶子又是目录）。
				next.Key = full
				next.Lease = leaseIDToString(kv.Lease)
			}
			node = next
		}
	}
	markDirAndSort(root)

	ok(ctx, map[string]interface{}{
		"root":      root,
		"total":     resp.Count,
		"scanned":   len(resp.Kvs),
		"truncated": resp.Count > int64(len(resp.Kvs)),
	})
}

func findChild(children []*kvTreeNode, name string) *kvTreeNode {
	for _, ch := range children {
		if ch.Name == name {
			return ch
		}
	}
	return nil
}

// markDirAndSort 递归标记目录属性并按名称排序，目录排在文件前面。
func markDirAndSort(node *kvTreeNode) {
	if len(node.Children) > 0 {
		node.Dir = true
	}
	for _, ch := range node.Children {
		markDirAndSort(ch)
	}
	sort.SliceStable(node.Children, func(i, j int) bool {
		a, b := node.Children[i], node.Children[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return a.Name < b.Name
	})
}

// Search 在键名（可选含值）中做包含匹配搜索。
// GET /api/kv/search?keyword=&prefix=&inValue=
func (h *KVHandler) Search(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	keyword := string(ctx.Query("keyword"))
	prefix := string(ctx.Query("prefix"))
	inValue := string(ctx.Query("inValue")) == "1" || string(ctx.Query("inValue")) == "true"
	if keyword == "" {
		fail(ctx, consts.StatusBadRequest, fmt.Errorf("keyword 不能为空"))
		return
	}

	limit := int64(scanCap)
	opts := []clientv3.OpOption{clientv3.WithPrefix(), clientv3.WithLimit(limit)}
	if !inValue {
		opts = append(opts, clientv3.WithKeysOnly())
	}
	resp, err := cli.Get(c, prefix, opts...)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}

	items := make([]kvItem, 0)
	low := strings.ToLower(keyword)
	for _, kv := range resp.Kvs {
		keyStr := string(kv.Key)
		matched := strings.Contains(strings.ToLower(keyStr), low)
		if !matched && inValue {
			matched = strings.Contains(strings.ToLower(string(kv.Value)), low)
		}
		if !matched {
			continue
		}
		items = append(items, kvItem{
			Key:            keyStr,
			Value:          string(kv.Value),
			CreateRevision: kv.CreateRevision,
			ModRevision:    kv.ModRevision,
			Version:        kv.Version,
			Lease:          leaseIDToString(kv.Lease),
		})
	}
	ok(ctx, map[string]interface{}{"items": items, "matched": len(items), "scanned": len(resp.Kvs)})
}

// History 沿 revision 向前回溯一个键的历史版本（不含已被 compact 压缩的版本）。
// GET /api/kv/history?key=&limit=
func (h *KVHandler) History(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	key := string(ctx.Query("key"))
	if key == "" {
		fail(ctx, consts.StatusBadRequest, errEmptyKey)
		return
	}
	limit, _ := strconv.Atoi(string(ctx.Query("limit")))
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	cur, err := cli.Get(c, key)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	if len(cur.Kvs) == 0 {
		fail(ctx, consts.StatusNotFound, errKeyNotFound)
		return
	}

	entries := []kvItem{toKVItem(cur.Kvs[0])}
	curKV := cur.Kvs[0]
	rev := curKV.ModRevision

	// 逐步向前取该 key 在更早 revision 下的状态，直到触及创建版本或达到数量上限。
	for len(entries) < limit && rev > curKV.CreateRevision {
		resp, err := cli.Get(c, key, clientv3.WithRev(rev-1))
		if err != nil {
			fail(ctx, consts.StatusInternalServerError, err)
			return
		}
		if len(resp.Kvs) == 0 {
			break
		}
		older := resp.Kvs[0]
		if older.ModRevision >= rev {
			// 防御性兜底：保证 revision 严格递减，避免死循环。
			rev--
			continue
		}
		entries = append(entries, toKVItem(older))
		rev = older.ModRevision
	}
	ok(ctx, map[string]interface{}{"entries": entries})
}

func toKVItem(kv *mvccpb.KeyValue) kvItem {
	return kvItem{
		Key:            string(kv.Key),
		Value:          string(kv.Value),
		CreateRevision: kv.CreateRevision,
		ModRevision:    kv.ModRevision,
		Version:        kv.Version,
		Lease:          leaseIDToString(kv.Lease),
	}
}

// importItem 导入文件中的单条键值。
// LeaseID 用字符串承载，兼容超出 JS 安全整数范围的租约 ID。
type importItem struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	LeaseID string `json:"leaseId,omitempty"`
}

// Import 批量导入键值。
// POST /api/kv/import  body: {overwrite: bool, items: [{key, value, leaseId?}]}
func (h *KVHandler) Import(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	var body struct {
		Overwrite bool         `json:"overwrite"`
		Items     []importItem `json:"items"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	if len(body.Items) == 0 {
		fail(ctx, consts.StatusBadRequest, fmt.Errorf("导入内容为空"))
		return
	}

	var success, skipped int
	failed := make([]map[string]string, 0)
	for _, item := range body.Items {
		if item.Key == "" {
			skipped++
			continue
		}
		leaseID, err := parseLeaseIDString(item.LeaseID)
		if err != nil {
			failed = append(failed, map[string]string{"key": item.Key, "error": "无效的 leaseId: " + err.Error()})
			continue
		}
		putOpts := make([]clientv3.OpOption, 0, 1)
		if leaseID > 0 {
			putOpts = append(putOpts, clientv3.WithLease(leaseID))
		}

		if body.Overwrite {
			if _, err := cli.Put(c, item.Key, item.Value, putOpts...); err != nil {
				failed = append(failed, map[string]string{"key": item.Key, "error": err.Error()})
				continue
			}
			success++
			continue
		}

		// 非覆盖模式：仅当 key 不存在时写入。
		txn, err := cli.Txn(c).
			If(clientv3.Compare(clientv3.CreateRevision(item.Key), "=", 0)).
			Then(clientv3.OpPut(item.Key, item.Value, putOpts...)).
			Commit()
		if err != nil {
			failed = append(failed, map[string]string{"key": item.Key, "error": err.Error()})
			continue
		}
		if txn.Succeeded {
			success++
		} else {
			skipped++
		}
	}
	ok(ctx, map[string]interface{}{"success": success, "skipped": skipped, "failed": failed})
}

// Export 导出前缀下全部键值为 JSON 文件。
// GET /api/kv/export?prefix=
func (h *KVHandler) Export(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	prefix := string(ctx.Query("prefix"))

	resp, err := cli.Get(c, prefix, clientv3.WithPrefix(), clientv3.WithLimit(scanCap))
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	items := make([]importItem, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		items = append(items, importItem{Key: string(kv.Key), Value: string(kv.Value), LeaseID: leaseIDToString(kv.Lease)})
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}

	fileName := "etcd-export.json"
	if prefix != "" {
		safe := strings.NewReplacer("/", "_", " ", "_").Replace(strings.Trim(prefix, "/"))
		if safe != "" {
			fileName = "etcd-" + safe + ".json"
		}
	}
	ctx.Response.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	ctx.Data(consts.StatusOK, "application/json; charset=utf-8", payload)
}
