package handler

import (
	"context"
	"errors"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/kouleen/etcd-admin/internal/etcdcli"
)

// ClusterHandler 处理集群状态、告警与维护类请求。
type ClusterHandler struct {
	manager *etcdcli.Manager
}

func NewClusterHandler(m *etcdcli.Manager) *ClusterHandler {
	return &ClusterHandler{manager: m}
}

type endpointStatus struct {
	Endpoint string `json:"endpoint"`
	Version  string `json:"version"`
	DBSize   int64  `json:"dbSize"`
	Revision int64  `json:"revision"`
	// Leader 为 uint64 标识，统一转字符串避免前端 Number 精度丢失。
	Leader    string `json:"leader"`
	IsLearner bool   `json:"isLearner"`
	Healthy   bool   `json:"healthy"`
	Error     string `json:"error,omitempty"`
}

type memberInfo struct {
	// ID 用字符串承载 uint64，原因同 endpointStatus.Leader。
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	PeerURLs   []string `json:"peerUrls"`
	ClientURLs []string `json:"clientUrls"`
	IsLearner  bool     `json:"isLearner"`
}

// Status 汇总各端点状态与成员列表。
// GET /api/cluster/status
func (h *ClusterHandler) Status(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	endpoints := h.manager.Info().Endpoints

	statuses := make([]endpointStatus, 0, len(endpoints))
	for _, ep := range endpoints {
		item := endpointStatus{Endpoint: ep}
		resp, err := cli.Status(c, ep)
		if err != nil {
			item.Error = err.Error()
			statuses = append(statuses, item)
			continue
		}
		item.Healthy = true
		item.Version = resp.Version
		item.DBSize = resp.DbSize
		item.Revision = resp.Header.Revision
		item.Leader = strconv.FormatUint(resp.Leader, 10)
		item.IsLearner = resp.IsLearner
		statuses = append(statuses, item)
	}

	members := make([]memberInfo, 0)
	memberResp, err := cli.MemberList(c)
	if err == nil {
		for _, m := range memberResp.Members {
			members = append(members, memberInfo{
				ID:         strconv.FormatUint(m.ID, 10),
				Name:       m.Name,
				PeerURLs:   m.PeerURLs,
				ClientURLs: m.ClientURLs,
				IsLearner:  m.IsLearner,
			})
		}
	}
	ok(ctx, map[string]interface{}{
		"endpoints": statuses,
		"members":   members,
	})
}

// Alarms 列出集群当前告警。
// GET /api/cluster/alarms
func (h *ClusterHandler) Alarms(c context.Context, ctx *app.RequestContext) {
	resp, err := h.manager.Get().AlarmList(c)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	alarms := make([]map[string]interface{}, 0, len(resp.Alarms))
	for _, a := range resp.Alarms {
		alarms = append(alarms, map[string]interface{}{
			"memberID": strconv.FormatUint(a.MemberID, 10),
			"alarm":    a.Alarm.String(),
		})
	}
	ok(ctx, map[string]interface{}{"items": alarms})
}

// Disarm 解除告警（默认解除全部 NOSPACE 告警）。
// POST /api/cluster/alarms/disarm  body: {memberID?}
func (h *ClusterHandler) Disarm(c context.Context, ctx *app.RequestContext) {
	var body struct {
		MemberID uint64 `json:"memberId"`
	}
	_ = ctx.BindJSON(&body)

	resp, err := h.manager.Get().AlarmDisarm(c, &clientv3.AlarmMember{
		MemberID: body.MemberID,
		Alarm:    pb.AlarmType_NOSPACE,
	})
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, map[string]interface{}{"remaining": len(resp.Alarms)})
}

// Compact 压缩历史版本，释放存储空间。
// POST /api/maintenance/compact  body: {revision}
func (h *ClusterHandler) Compact(c context.Context, ctx *app.RequestContext) {
	var body struct {
		Revision int64 `json:"revision"`
	}
	if err := ctx.BindJSON(&body); err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	if body.Revision <= 0 {
		fail(ctx, consts.StatusBadRequest, errors.New("revision 必须大于 0"))
		return
	}
	if _, err := h.manager.Get().Compact(c, body.Revision); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, map[string]interface{}{"revision": body.Revision})
}

// Defrag 对所有端点进行碎片整理。
// POST /api/maintenance/defrag
func (h *ClusterHandler) Defrag(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	results := make([]map[string]interface{}, 0)
	for _, ep := range h.manager.Info().Endpoints {
		resp, err := cli.Defragment(c, ep)
		row := map[string]interface{}{"endpoint": ep}
		if err != nil {
			row["error"] = err.Error()
		} else {
			row["ok"] = true
			// 不同 etcd 版本下 Defragment 响应的 Header 可能为 nil，需做空值防护。
			if resp != nil && resp.Header != nil {
				row["revision"] = resp.Header.Revision
			}
		}
		results = append(results, row)
	}
	ok(ctx, map[string]interface{}{"results": results})
}
