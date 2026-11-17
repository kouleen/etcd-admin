package handler

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/kouleen/etcd-admin/internal/etcdcli"
	"github.com/kouleen/etcd-admin/internal/sampler"
)

// MetricsHandler 输出后端采样得到的集群指标时序与实时概览。
type MetricsHandler struct {
	manager *etcdcli.Manager
	sampler *sampler.Sampler
}

func NewMetricsHandler(m *etcdcli.Manager, s *sampler.Sampler) *MetricsHandler {
	return &MetricsHandler{manager: m, sampler: s}
}

// Series 返回内存中的指标采样序列。
// GET /api/metrics/series
func (h *MetricsHandler) Series(c context.Context, ctx *app.RequestContext) {
	ok(ctx, map[string]interface{}{"samples": h.sampler.Series()})
}

// Overview 聚合一次页面所需的全部实时指标。
// GET /api/metrics/overview
func (h *MetricsHandler) Overview(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	info := h.manager.Info()

	type endpointMetric struct {
		Endpoint string `json:"endpoint"`
		Version  string `json:"version"`
		Healthy  bool   `json:"healthy"`
		DBSize   int64  `json:"dbSize"`
		Revision int64  `json:"revision"`
		// Leader 为 uint64 标识，转字符串防止前端 Number 精度丢失。
		Leader    string `json:"leader"`
		IsLearner bool   `json:"isLearner"`
		Error     string `json:"error,omitempty"`
	}

	endpoints := make([]endpointMetric, 0, len(info.Endpoints))
	for _, ep := range info.Endpoints {
		item := endpointMetric{Endpoint: ep}
		if st, err := cli.Status(c, ep); err == nil {
			item.Healthy = true
			item.Version = st.Version
			item.DBSize = st.DbSize
			item.Revision = st.Header.Revision
			item.Leader = strconv.FormatUint(st.Leader, 10)
			item.IsLearner = st.IsLearner
		} else {
			item.Error = err.Error()
		}
		endpoints = append(endpoints, item)
	}

	keys := int64(0)
	if kr, err := cli.Get(c, "", clientv3.WithPrefix(), clientv3.WithKeysOnly(), clientv3.WithLimit(1)); err == nil {
		keys = kr.Count
	}
	leaseCount := 0
	if lr, err := cli.Leases(c); err == nil {
		leaseCount = len(lr.Leases)
	}
	memberCount := 0
	if mr, err := cli.MemberList(c); err == nil {
		memberCount = len(mr.Members)
	}
	alarmCount := 0
	if ar, err := cli.AlarmList(c); err == nil {
		alarmCount = len(ar.Alarms)
	}

	latest, hasLatest := h.sampler.Latest()
	ok(ctx, map[string]interface{}{
		"endpoints": endpoints,
		"keys":      keys,
		"leases":    leaseCount,
		"members":   memberCount,
		"alarms":    alarmCount,
		"latest":    latest,
		"hasLatest": hasLatest,
		"samples":   h.sampler.Series(),
	})
}
