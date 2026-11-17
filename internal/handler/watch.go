package handler

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/kouleen/etcd-admin/internal/etcdcli"
)

// WatchHandler 通过 SSE 把 etcd watch 事件实时推送给浏览器。
type WatchHandler struct {
	manager *etcdcli.Manager
}

func NewWatchHandler(m *etcdcli.Manager) *WatchHandler {
	return &WatchHandler{manager: m}
}

type watchEvent struct {
	Type        string `json:"type"` // PUT / DELETE
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`
	ModRevision int64  `json:"modRevision"`
	Version     int64  `json:"version"`
	// Lease 用字符串承载 int64，防止前端 Number 精度丢失；空串表示无租约。
	Lease     string `json:"lease,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// Watch GET /api/watch?prefix=
// 建立 text/event-stream 长连接，客户端断开时自动退出并释放 watcher。
func (h *WatchHandler) Watch(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	prefix := string(ctx.Query("prefix"))

	// 官方 SSE Writer：自动设置响应头并以 chunked 方式即时 flush。
	sseWriter := sse.NewWriter(ctx)
	defer sseWriter.Close()

	writeEvent := func(payload interface{}) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return sseWriter.WriteEvent("", "", data)
	}

	// 心跳：防止反向代理因空闲超时切断 SSE 连接。
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	watchCtx, cancel := context.WithCancel(c)
	defer cancel()

	watchCh := cli.Watch(watchCtx, prefix, clientv3.WithPrefix())
	if err := writeEvent(map[string]interface{}{"type": "CONNECTED", "prefix": prefix}); err != nil {
		return
	}

	for {
		select {
		case <-c.Done():
			// 浏览器断开连接，请求上下文被取消。
			return
		case <-heartbeat.C:
			if err := sseWriter.WriteKeepAlive(); err != nil {
				return
			}
		case resp, ok := <-watchCh:
			if !ok {
				return
			}
			if err := resp.Err(); err != nil {
				_ = writeEvent(map[string]interface{}{"type": "ERROR", "message": err.Error()})
				return
			}
			for _, ev := range resp.Events {
				item := watchEvent{
					Type:        ev.Type.String(),
					Key:         string(ev.Kv.Key),
					ModRevision: ev.Kv.ModRevision,
					Version:     ev.Kv.Version,
					Lease:       strconv.FormatInt(ev.Kv.Lease, 10),
					Timestamp:   time.Now().UnixMilli(),
				}
				if ev.Type == clientv3.EventTypePut {
					item.Value = string(ev.Kv.Value)
				}
				if err := writeEvent(item); err != nil {
					return
				}
			}
		}
	}
}
