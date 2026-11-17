package etcdcli

import (
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Client 对 etcd v3 客户端的轻量封装，由 Manager 统一管理生命周期。
type Client struct {
	*clientv3.Client
}
