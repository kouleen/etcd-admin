package config

import (
	"os"
	"strconv"
	"strings"
)

// Config 运行时配置，全部来自环境变量，避免硬编码。
type Config struct {
	// ListenAddr Hertz HTTP 服务监听地址
	ListenAddr string
	// EtcdEndpoints etcd 集群端点列表
	EtcdEndpoints []string
	// EtcdUsername etcd 认证用户名（可选）
	EtcdUsername string
	// EtcdPassword etcd 认证密码（可选）
	EtcdPassword string
	// StaticCacheMaxAge /static 静态资源的浏览器缓存时间（秒）。
	// 0（默认）为开发模式：强制 no-cache，每次请求回源；
	// 正数为生产模式：返回 public, max-age=N, immutable，配合资源 URL 的 ?v= 版本号失效。
	StaticCacheMaxAge int
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// getPositiveInt 读取正整数环境变量；未设置、非法或非正数时回落 0。
func getPositiveInt(key string) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && v > 0 {
		return v
	}
	return 0
}

// Load 从环境变量加载配置。
// ETCD_ENDPOINTS 未设置或为空时返回空端点列表，进程仍正常启动，
// 由用户在连接入口页手动建立连接（见 main.go 的 lazy 连接降级逻辑）。
func Load() *Config {
	var endpoints []string
	if raw := strings.TrimSpace(os.Getenv("ETCD_ENDPOINTS")); raw != "" {
		for _, ep := range strings.Split(raw, ",") {
			if ep = strings.TrimSpace(ep); ep != "" {
				endpoints = append(endpoints, ep)
			}
		}
	}
	return &Config{
		ListenAddr:        getenv("SERVER_ADDR", ":8080"),
		EtcdEndpoints:     endpoints,
		EtcdUsername:      os.Getenv("ETCD_USERNAME"),
		EtcdPassword:      os.Getenv("ETCD_PASSWORD"),
		StaticCacheMaxAge: getPositiveInt("STATIC_CACHE_MAX_AGE"),
	}
}
