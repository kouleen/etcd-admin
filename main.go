package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"

	"github.com/kouleen/etcd-admin/internal/config"
	"github.com/kouleen/etcd-admin/internal/etcdcli"
	"github.com/kouleen/etcd-admin/internal/router"
	"github.com/kouleen/etcd-admin/internal/sampler"
)

func main() {
	cfg := config.Load()

	// 本项目仅作为 etcd 客户端，启动时连接远程 etcd，失败不退出进程，
	// 允许稍后在「连接设置」页重新切换连接。
	manager, err := etcdcli.NewManager(cfg)
	if err != nil {
		log.Printf("警告：初始连接 etcd 失败 (%v): %v，可在页面连接设置中重连", cfg.EtcdEndpoints, err)
		manager = etcdcli.NewLazyManager(cfg)
	} else {
		log.Printf("已连接 etcd 集群: %v", cfg.EtcdEndpoints)
	}

	// 指标采样器：每 10 秒一个点，保留最近 90 个点（约 15 分钟）。
	sp := sampler.New(manager, 90)
	samplerCtx, stopSampler := context.WithCancel(context.Background())
	go sp.Run(samplerCtx, 10*time.Second)

	hlog.SetLevel(hlog.LevelInfo)
	h := server.Default(server.WithHostPorts(cfg.ListenAddr))
	router.Register(h, manager, sp, cfg.StaticCacheMaxAge)

	go func() {
		if err := h.Run(); err != nil {
			log.Fatalf("Hertz 服务退出: %v", err)
		}
	}()

	// 优雅退出：收到中断信号后停止采样、停止接收新请求并释放资源。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务...")
	stopSampler()
	if err := h.Shutdown(context.Background()); err != nil {
		log.Printf("优雅关闭异常: %v", err)
	}
	if cli := manager.Get(); cli != nil {
		_ = cli.Close()
	}
}
