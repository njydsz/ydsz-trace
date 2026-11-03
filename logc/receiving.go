// logc 子文件：常驻接收源启动（V2.2-10，FR-101）。
//
// 背景：
//   syslog（UDP/TCP）与 HTTP push 是「被动接收」型数据源，
//   必须在服务启动时就监听端口，否则客户端推送的日志无处可去。
//   文件/Docker/K8s 是「主动拉取」型，无需提前启动。
//
// 设计：
//   - 按具体类型断言后调用各自的 Start
//   - 启动失败仅告警不退出：接收能力是增强项，不应影响主流程可用性
//   - 退出时由各自的 Stop 释放端口（注册到 shutdown 流程）
package main

import (
	"log"

	"ydsz-trace/pkg/source"
)

// startReceivingSources 启动需要常驻监听的源。
//
// 拉取型源（file/docker/k8s）无操作，保证函数可安全用于任意类型。
func startReceivingSources(src source.Source) {
	switch s := src.(type) {
	case *source.SyslogSource:
		if err := s.Start(); err != nil {
			log.Printf("[source] syslog 启动失败（不影响主流程）: %v", err)
			return
		}
		log.Printf("[source] syslog 接收已就绪: %s", s.Info().Description)
	case *source.HTTPPushSource:
		if err := s.Start(); err != nil {
			log.Printf("[source] HTTP push 启动失败（不影响主流程）: %v", err)
			return
		}
		log.Printf("[source] HTTP push 接收已就绪: %s", s.Info().Description)
	default:
		// file / docker / k8s / journald 均为主动拉取型，无需提前启动
		log.Printf("[source] %T 为拉取型源，无需常驻监听", src)
	}
}

// stopReceivingSources 停止常驻接收源（退出流程调用）。
func stopReceivingSources(src source.Source) {
	switch s := src.(type) {
	case *source.SyslogSource:
		s.Stop()
		log.Printf("[source] syslog 接收已停止")
	case *source.HTTPPushSource:
		s.Stop()
		log.Printf("[source] HTTP push 接收已停止")
	}
}
