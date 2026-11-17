package sampler

import (
	"context"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/kouleen/etcd-admin/internal/etcdcli"
)

// Sample 一个时间点上的集群关键指标快照。
type Sample struct {
	Ts       int64 `json:"ts"`
	DbSize   int64 `json:"dbSize"`
	Revision int64 `json:"revision"`
	Keys     int64 `json:"keys"`
	Leases   int64 `json:"leases"`
	Members  int64 `json:"members"`
}

// Sampler 周期性采集集群指标并在内存中保留固定长度的环形序列，
// 为前端提供轻量时序图数据（frp 隧道无法访问 etcd 的 HTTP /metrics）。
type Sampler struct {
	manager *etcdcli.Manager

	mu       sync.RWMutex
	samples  []Sample
	capacity int
}

func New(manager *etcdcli.Manager, capacity int) *Sampler {
	return &Sampler{
		manager:  manager,
		samples:  make([]Sample, 0, capacity),
		capacity: capacity,
	}
}

// Run 阻塞运行采样循环，直到 ctx 被取消。
func (s *Sampler) Run(ctx context.Context, interval time.Duration) {
	// 启动后立即采一次，避免页面初始无数据。
	s.collect(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.collect(ctx)
		}
	}
}

func (s *Sampler) collect(ctx context.Context) {
	cli := s.manager.Get()
	if cli == nil {
		return
	}

	endpoints := s.manager.Info().Endpoints
	if len(endpoints) == 0 {
		return
	}

	sample := Sample{Ts: time.Now().UnixMilli()}

	if st, err := cli.Status(ctx, endpoints[0]); err == nil {
		sample.DbSize = st.DbSize
		sample.Revision = st.Header.Revision
	} else {
		return // 连接异常时不产生脏数据点
	}

	if kr, err := cli.Get(ctx, "", clientv3.WithPrefix(), clientv3.WithKeysOnly(), clientv3.WithLimit(1)); err == nil {
		sample.Keys = kr.Count
	}
	if lr, err := cli.Leases(ctx); err == nil {
		sample.Leases = int64(len(lr.Leases))
	}
	if mr, err := cli.MemberList(ctx); err == nil {
		sample.Members = int64(len(mr.Members))
	}

	s.mu.Lock()
	s.samples = append(s.samples, sample)
	if len(s.samples) > s.capacity {
		s.samples = s.samples[len(s.samples)-s.capacity:]
	}
	s.mu.Unlock()
}

// Series 返回采样序列的副本。
func (s *Sampler) Series() []Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Sample, len(s.samples))
	copy(out, s.samples)
	return out
}

// Latest 返回最近一个样本。
func (s *Sampler) Latest() (Sample, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.samples) == 0 {
		return Sample{}, false
	}
	return s.samples[len(s.samples)-1], true
}
