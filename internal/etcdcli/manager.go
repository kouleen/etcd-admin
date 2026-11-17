package etcdcli

import (
	"context"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/kouleen/etcd-admin/internal/config"
)

// ConnectionInfo 当前连接的只读信息。
type ConnectionInfo struct {
	Endpoints []string `json:"endpoints"`
	Username  string   `json:"username"`
	HasAuth   bool     `json:"hasAuth"`
}

// Manager 持有当前 etcd 客户端，支持运行时安全切换连接。
// 切换过程采用“先建新连、成功后替换”的策略，失败时保留旧连接。
type Manager struct {
	mu        sync.RWMutex
	cli       *Client
	endpoints []string
	username  string
	password  string
}

// NewManager 根据初始配置建立连接。
func NewManager(cfg *config.Config) (*Manager, error) {
	cli, err := dial(cfg.EtcdEndpoints, cfg.EtcdUsername, cfg.EtcdPassword)
	if err != nil {
		return nil, err
	}
	return &Manager{
		cli:       cli,
		endpoints: append([]string(nil), cfg.EtcdEndpoints...),
		username:  cfg.EtcdUsername,
		password:  cfg.EtcdPassword,
	}, nil
}

// Get 返回当前客户端。
func (m *Manager) Get() *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cli
}

// Info 返回当前连接信息（不含密码明文）。
func (m *Manager) Info() ConnectionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ConnectionInfo{
		Endpoints: append([]string(nil), m.endpoints...),
		Username:  m.username,
		HasAuth:   m.username != "",
	}
}

// IsConnected 判断当前是否持有可用客户端。
func (m *Manager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cli != nil
}

// Disconnect 关闭并释放底层客户端，使 Manager 进入未连接状态。
// 后续 API 调用需先调用 Reconnect 恢复。
func (m *Manager) Disconnect() {
	m.mu.Lock()
	old := m.cli
	m.cli = nil
	m.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

// Reconnect 用新参数建立连接，成功后关闭并替换旧客户端。
func (m *Manager) Reconnect(ctx context.Context, endpoints []string, username, password string) error {
	cli, err := dial(endpoints, username, password)
	if err != nil {
		return err
	}

	m.mu.Lock()
	old := m.cli
	m.cli = cli
	m.endpoints = append([]string(nil), endpoints...)
	m.username = username
	m.password = password
	m.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}
	return nil
}

// NewLazyManager 在启动探测失败时使用：不做连通性校验直接构造客户端，
// 底层 gRPC 会在后台持续重连，用户也可在连接设置页手动切换。
func NewLazyManager(cfg *config.Config) *Manager {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.EtcdEndpoints,
		DialTimeout: 8 * time.Second,
		Username:    cfg.EtcdUsername,
		Password:    cfg.EtcdPassword,
	})
	if err != nil {
		// 参数级错误（极少见），仍返回空 manager，等待页面重连。
		return &Manager{endpoints: append([]string(nil), cfg.EtcdEndpoints...)}
	}
	return &Manager{
		cli:       &Client{cli},
		endpoints: append([]string(nil), cfg.EtcdEndpoints...),
		username:  cfg.EtcdUsername,
		password:  cfg.EtcdPassword,
	}
}

// TestConnection 只做一次性连通性探测，不影响当前连接。
func TestConnection(ctx context.Context, endpoints []string, username, password string) (*clientv3.StatusResponse, error) {
	cli, err := dial(endpoints, username, password)
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	return cli.Status(ctx, endpoints[0])
}

// dial 创建客户端并做一次 Status 探测，确保连接真实可用。
func dial(endpoints []string, username, password string) (*Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 8 * time.Second,
		Username:    username,
		Password:    password,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := cli.Status(ctx, endpoints[0]); err != nil {
		_ = cli.Close()
		return nil, err
	}
	return &Client{cli}, nil
}
