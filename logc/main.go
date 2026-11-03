// Package main 是 logc 客户端代理服务的入口。
//
// 职责：
//   - 部署在每台需要被集中收集日志的机器上（传统模式 / Docker Daemon / K8s DaemonSet）
//   - 接收 logs 服务端的查询请求，通过 Source 抽象层读取本机或容器的日志
//   - 自动向 logs 注册并保持心跳，上报存活状态
//   - Docker/K8s 模式：自动发现目标容器并上报虚拟客户端注册
//
// Source 模式通过 YDSZ_LOG_SOURCE 环境变量控制：
//   - file（默认）：传统文件系统模式，与原行为完全兼容
//   - docker：通过 Docker socket 采集容器 stdout 日志
//   - k8s：通过 K8s API 采集 Pod 容器日志
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	filectl "ydsz-trace/logc/controllers/file"
	"ydsz-trace/logc/controllers/register"
	"ydsz-trace/logc/routers"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/perf"
	"ydsz-trace/pkg/source"
)

// activeSource 全局激活的日志源实例（由工厂创建，中间件注入到请求上下文）。
var (
	activeSource source.Source
	// sampler 端侧日志采样器（V2.2-11），nil 时各读取路径直通
	sampler *perf.Sampler
	// guardian 资源护栏（V2.2-11）
	guardian *perf.Guardian
)

func main() {
	// 定义变量，用于接收命令行的参数值。
	var server string
	var vkey string
	var confPath string
	flag.StringVar(&server, "s", "", "日志服务地址（ip:port）")
	flag.StringVar(&vkey, "v", "", "客户端认证密钥")
	flag.StringVar(&confPath, "c", "conf/app.conf", "INI 配置文件路径")
	flag.Parse()

	// 加载配置文件。
	cfg, err := config.Load(confPath)
	if err != nil {
		log.Printf("加载配置失败: %v，使用内置默认值", err)
		cfg = config.NewDefault()
	}

	// 密钥/服务地址优先级：命令行 > 环境变量 > 配置文件 > 内置默认。
	if vkey == "" {
		vkey = config.EnvOrConfig("YDSZ_CLIENT_KEY", cfg.String("key"), "123456")
	}
	if server == "" {
		server = config.EnvOrConfig("YDSZ_LOG_SERVER", cfg.String("logs"), "127.0.0.1:2021")
	}

	// 弱密钥告警
	if vkey == "123456" || vkey == "ydsz_trace_key" {
		log.Printf("[security] 警告：logc 使用默认预共享密钥（%v），建议通过 YDSZ_CLIENT_KEY 环境变量配置强密钥", vkey)
	}

	// ===== Source 初始化 =====
	src, srcErr := source.CreateSourceFromEnv()
	if srcErr != nil {
		log.Fatalf("创建日志源失败: %v", srcErr)
	}
	activeSource = src
	log.Printf("[source] 已激活: type=%s description=%s", src.Info().Type, src.Info().Description)

	// 位点定期落盘（V2.2-08，FR-102）：内存位点每 30s 写一次磁盘，
	// 保证 logc 重启后能断点续读，同时避免每行都写盘。
	startOffsetFlusher(src)

	// 常驻接收源（V2.2-10，FR-101）：syslog / HTTP push 需要提前监听端口，
	// 否则检索请求到来时才开始接收会丢失数据。
	startReceivingSources(src)

	// 端侧资源护栏（V2.2-11，FR-105/106）：高流量时按比例采样低优先级日志，
	// 内存超阈值时自动降级采样率，避免 Agent 影响业务进程。
	sampler = perf.NewSampler()
	filectl.SetSampler(sampler)
	guardian = perf.NewGuardian(sampler)
	guardian.Start()
	log.Printf("[perf] 采样率=%.2f 内存上限=%sMB（0 表示未配置）",
		sampler.Rate(), os.Getenv("YDSZ_MAX_MEM_MB"))

	// 若配置文件指定了 source
	if cfgSource := cfg.String("source"); cfgSource != "" && os.Getenv("YDSZ_LOG_SOURCE") == "" {
		src2, err2 := source.CreateSource(source.FactoryConfig{
			Type:    source.SourceType(cfgSource),
			Options: map[string]string{"root_dir": cfg.StringOr("logroot", "")},
		})
		if err2 == nil {
			activeSource = src2
			log.Printf("[source] 配置文件覆盖: type=%s", cfgSource)
		}
	}

	// 注册鉴权密钥（与服务端 getRegisterKey 一致）
	regKey := os.Getenv("YDSZ_REGISTER_KEY")
	if regKey == "" {
		regKey = cfg.StringOr("register_key", "")
	}

	// ===== 基础注册（传统模式的本地 IP 注册）=====
	log.Printf("logc register -server=%v -vkey=%v\n", server, vkey)
	register.RegisterLocalIp(server, vkey, regKey)

	// ===== 启动心跳续约 =====
	register.StartGlobalHeartbeat(server, vkey, regKey)

	// ===== Docker / K8s 模式：启动自动发现与虚拟客户端注册 =====
	autoRegisterCtx, autoRegisterCancel := context.WithCancel(context.Background())
	defer autoRegisterCancel()

	if activeSource.Info().Type != string(source.SourceTypeFile) {
		log.Printf("[source] 启动自动发现: type=%s", activeSource.Info().Type)
		go runAutoDiscovery(autoRegisterCtx, server, vkey)
	}

	// ===== 启动 HTTP 服务 =====
	routers.SetActiveSource(activeSource)
	port := cfg.StringOr("httpport", "2020")
	handler := routers.SetupRouter(cfg)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	go gracefulShutdown(srv)

	log.Printf("logc 启动成功，监听端口 %s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP服务启动失败: %v", err)
	}
}

// runAutoDiscovery 启动目标发现循环，当 Source 发现新目标时，
// 自动向 logs 服务端发起注册请求。
func runAutoDiscovery(ctx context.Context, server, vkey string) {
	eventCh, err := activeSource.Discover(ctx)
	if err != nil {
		log.Printf("[discovery] 启动失败: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-eventCh:
			if !ok {
				log.Printf("[discovery] channel 关闭，退出")
				return
			}
			handleDiscoveryEvent(ctx, server, vkey, evt)
		}
	}
}

// handleDiscoveryEvent 把 DiscoveryEvent 转化为对 logs 服务端的注册/注销请求。
func handleDiscoveryEvent(ctx context.Context, server, vkey string, evt source.DiscoveryEvent) {
	for _, t := range evt.Targets {
		switch evt.Type {
		case "snapshot":
			// 快照批量注册
			err := register.VirtualClient(&register.VirtualRegisterRequest{
				Server:         server,
				VKey:           vkey,
				SourceType:     t.SourceType,
				Identity:       t.Identity,
				DisplayName:    t.DisplayName,
				LogPath:        t.LogPath,
				LocalLogcPort:  activeSourcePort(),
				Labels:         t.Labels,
			})
			if err != nil {
				log.Printf("[discovery] 批量注册目标 %s 失败: %v", t.DisplayName, err)
			}
		case "add":
			err := register.VirtualClient(&register.VirtualRegisterRequest{
				Server:         server,
				VKey:           vkey,
				SourceType:     t.SourceType,
				Identity:       t.Identity,
				DisplayName:    t.DisplayName,
				LogPath:        t.LogPath,
				LocalLogcPort:  activeSourcePort(),
				Labels:         t.Labels,
			})
			if err != nil {
				log.Printf("[discovery] 注册目标 %s 失败: %v", t.DisplayName, err)
			}
		case "remove":
			err := register.RemoveVirtualClient(server, vkey, t.Identity)
			if err != nil {
				log.Printf("[discovery] 注销目标 %s 失败: %v", t.DisplayName, err)
			}
		}
	}
}

// activeSourcePort 返回 logc HTTP 监听端口（用于 logs 回调地址）。
func activeSourcePort() string {
	port := os.Getenv("YDSZ_LOGC_PORT")
	if port == "" {
		port = "2020"
	}
	return port
}

// gracefulShutdown 监听 SIGTERM/SIGINT，停止心跳并优雅关闭 HTTP 服务。
func gracefulShutdown(srv *http.Server) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，正在优雅关闭...")

	// 退出前最后一次落盘读取位点（V2.2-08），确保重启后不重复分析
	flushOffsetsNow(activeSource)

	// 释放常驻接收端口（V2.2-10）
	stopReceivingSources(activeSource)

	register.StopGlobalHeartbeat()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP服务关闭失败: %v", err)
	} else {
		log.Println("HTTP服务已优雅关闭")
	}
	os.Exit(0)
}
