// Package metrics 提供 Prometheus 风格的运行时指标暴露。
//
// 当前实现为纯文本文本格式（Prometheus exposition format），
// 可直接由 Prometheus server 抓取；后续可替换为 prometheus/client_grafana 原生 SDK。
package metrics

import (
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Collector 指标收集器。
type Collector struct {
	mu sync.RWMutex

	// 应用启动时间
	startTime time.Time

	// 查询计数
	queryTotal   int64
	querySuccess int64
	queryFailure int64

	// 查询耗时累计（毫秒）
	queryDurationMs int64

	// 客户端管理
	clientTotal  int64
	clientOnline int64

	// HTTP 请求计数
	httpRequestsTotal   int64
	httpRequests4xx     int64
	httpRequests5xx     int64
	httpRequestDuration int64
}

// globalCollector 全局唯一收集器实例。
var globalCollector = &Collector{startTime: time.Now()}

// Global 返回全局 Collector 单例。
func Global() *Collector {
	return globalCollector
}

// QueryStarted 记录一次查询开始。
func (c *Collector) QueryStarted() {
	c.mu.Lock()
	c.queryTotal++
	c.mu.Unlock()
}

// QuerySucceeded 记录一次查询成功，duration 为耗时。
func (c *Collector) QuerySucceeded(duration time.Duration) {
	c.mu.Lock()
	c.querySuccess++
	c.queryDurationMs += duration.Milliseconds()
	c.mu.Unlock()
}

// QueryFailed 记录一次查询失败。
func (c *Collector) QueryFailed(duration time.Duration) {
	c.mu.Lock()
	c.queryFailure++
	c.queryDurationMs += duration.Milliseconds()
	c.mu.Unlock()
}

// UpdateClientStats 更新客户端统计数据。
func (c *Collector) UpdateClientStats(total, online int64) {
	c.mu.Lock()
	c.clientTotal = total
	c.clientOnline = online
	c.mu.Unlock()
}

// HTTPRequestRecorded 记录一次 HTTP 请求。
func (c *Collector) HTTPRequestRecorded(statusCode int, duration time.Duration) {
	c.mu.Lock()
	c.httpRequestsTotal++
	c.httpRequestDuration += duration.Milliseconds()
	if statusCode >= 400 && statusCode < 500 {
		c.httpRequests4xx++
	} else if statusCode >= 500 {
		c.httpRequests5xx++
	}
	c.mu.Unlock()
}

// Handler 返回 /metrics HTTP handler，暴露 Prometheus 格式指标。
func (c *Collector) Handler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		c.mu.RLock()
		uptime := time.Since(c.startTime).Seconds()
		queryTotal := c.queryTotal
		querySuccess := c.querySuccess
		queryFailure := c.queryFailure
		queryDuration := c.queryDurationMs
		clientTotal := c.clientTotal
		clientOnline := c.clientOnline
		httpTotal := c.httpRequestsTotal
		http4xx := c.httpRequests4xx
		http5xx := c.httpRequests5xx
		httpDuration := c.httpRequestDuration
		c.mu.RUnlock()

		// 计算平均耗时
		var avgQueryMs, avgHTTPMs float64
		if queryTotal > 0 {
			avgQueryMs = float64(queryDuration) / float64(queryTotal)
		}
		if httpTotal > 0 {
			avgHTTPMs = float64(httpDuration) / float64(httpTotal)
		}

		// Go runtime 指标
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		// 构造 Prometheus exposition format 输出
		var b string
		b += "# HELP ydsz_uptime_seconds 应用运行时长（秒）\n"
		b += "# TYPE ydsz_uptime_seconds counter\n"
		b += fmt.Sprintf("ydsz_uptime_seconds %.2f\n", uptime)

		b += "\n# HELP ydsz_queries_total 查询总次数\n"
		b += "# TYPE ydsz_queries_total counter\n"
		b += fmt.Sprintf("ydsz_queries_total %d\n", queryTotal)

		b += "\n# HELP ydsz_queries_success 成功查询次数\n"
		b += "# TYPE ydsz_queries_success counter\n"
		b += fmt.Sprintf("ydsz_queries_success %d\n", querySuccess)

		b += "\n# HELP ydsz_queries_failed 失败查询次数\n"
		b += "# TYPE ydsz_queries_failed counter\n"
		b += fmt.Sprintf("ydsz_queries_failed %d\n", queryFailure)

		b += "\n# HELP ydsz_query_duration_ms 查询平均耗时（毫秒）\n"
		b += "# TYPE ydsz_query_duration_ms gauge\n"
		b += fmt.Sprintf("ydsz_query_duration_ms %.2f\n", avgQueryMs)

		b += "\n# HELP ydsz_clients_total 注册客户端总数\n"
		b += "# TYPE ydsz_clients_total gauge\n"
		b += fmt.Sprintf("ydsz_clients_total %d\n", clientTotal)

		b += "\n# HELP ydsz_clients_online 在线客户端数\n"
		b += "# TYPE ydsz_clients_online gauge\n"
		b += fmt.Sprintf("ydsz_clients_online %d\n", clientOnline)

		b += "\n# HELP ydsz_http_requests_total HTTP 请求总数\n"
		b += "# TYPE ydsz_http_requests_total counter\n"
		b += fmt.Sprintf("ydsz_http_requests_total %d\n", httpTotal)

		b += "\n# HELP ydsz_http_requests_4xx HTTP 4xx 错误数\n"
		b += "# TYPE ydsz_http_requests_4xx counter\n"
		b += fmt.Sprintf("ydsz_http_requests_4xx %d\n", http4xx)

		b += "\n# HELP ydsz_http_requests_5xx HTTP 5xx 错误数\n"
		b += "# TYPE ydsz_http_requests_5xx counter\n"
		b += fmt.Sprintf("ydsz_http_requests_5xx %d\n", http5xx)

		b += "\n# HELP ydsz_http_request_duration_ms HTTP 请求平均耗时（毫秒）\n"
		b += "# TYPE ydsz_http_request_duration_ms gauge\n"
		b += fmt.Sprintf("ydsz_http_request_duration_ms %.2f\n", avgHTTPMs)

		b += "\n# HELP ydsz_go_goroutines 当前 goroutine 数量\n"
		b += "# TYPE ydsz_go_goroutines gauge\n"
		b += fmt.Sprintf("ydsz_go_goroutines %d\n", runtime.NumGoroutine())

		b += "\n# HELP ydsz_go_mem_alloc_bytes 当前堆分配字节数\n"
		b += "# TYPE ydsz_go_mem_alloc_bytes gauge\n"
		b += fmt.Sprintf("ydsz_go_mem_alloc_bytes %d\n", memStats.Alloc)

		b += "\n# HELP ydsz_go_mem_sys_bytes 从系统获取的内存字节数\n"
		b += "# TYPE ydsz_go_mem_sys_bytes gauge\n"
		b += fmt.Sprintf("ydsz_go_mem_sys_bytes %d\n", memStats.Sys)

		b += "\n# HELP ydsz_go_gc_total GC 执行总次数\n"
		b += "# TYPE ydsz_go_gc_total counter\n"
		b += fmt.Sprintf("ydsz_go_gc_total %d\n", memStats.NumGC)

		ctx.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		ctx.String(http.StatusOK, b)
	}
}

// HTTPMetricsMiddleware 返回 Gin 中间件，自动记录每个请求的状态码与耗时。
func HTTPMetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		Global().HTTPRequestRecorded(c.Writer.Status(), duration)
	}
}
