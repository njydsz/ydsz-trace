// Package metrics 提供 Prometheus 指标暴露。
//
// 底层使用 github.com/prometheus/client_golang，提供 Counter/Gauge/Histogram/Summary，
// 可由 /metrics 通过 promhttp.Handler 抓取。全局单例 Global() 自动注册到 prometheus.Registry，
// 暴露：
//
//   - ydsz_queries_total / ydsz_queries_success / ydsz_queries_failed（counter）
//   - ydsz_query_duration_seconds（histogram, seconds）
//   - ydsz_clients_total / ydsz_clients_online（gauge）
//   - ydsz_http_requests_total（counter, 按 method+code 分）
//   - ydsz_http_request_duration_seconds（histogram, seconds）
//   - Go runtime 指标（promhttp handler 自动附带）
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "ydsz"

// Collector 指标收集器（全局单例，自动注册）。
type Collector struct {
	reg *prometheus.Registry

	queryTotal    prometheus.Counter
	querySuccess  prometheus.Counter
	queryFailed   prometheus.Counter
	queryDuration prometheus.Histogram

	clientTotal  prometheus.Gauge
	clientOnline prometheus.Gauge

	httpRequests *prometheus.CounterVec
	httpDuration prometheus.Histogram
}

var globalCollector *Collector

// ensureGlobal 惰性初始化全局单例，并基于独立的 Registry（避免 default registry 自带 Go collector 冲突）。
func ensureGlobal() *Collector {
	if globalCollector != nil {
		return globalCollector
	}
	reg := prometheus.NewRegistry()
	// 显式附带 Go runtime 指标（对应原实现 ydsz_go_goroutines / ydsz_go_mem_alloc_bytes 等）
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{Namespace: namespace}))

	queryDurationBuckets := prometheus.DefBuckets
	httpDurationBuckets := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

	c := &Collector{
		reg: reg,

		queryTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "queries_total", Help: "查询总次数",
		}),
		querySuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "queries_success", Help: "成功查询次数",
		}),
		queryFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "queries_failed", Help: "失败查询次数",
		}),
		queryDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace, Name: "query_duration_seconds",
			Help:    "查询耗时分布（秒）",
			Buckets: queryDurationBuckets,
		}),

		clientTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "clients_total", Help: "注册客户端总数",
		}),
		clientOnline: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "clients_online", Help: "在线客户端数",
		}),

		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "http_requests_total", Help: "HTTP 请求总数",
		}, []string{"method", "code"}),
		httpDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace, Name: "http_request_duration_seconds",
			Help:    "HTTP 请求耗时分布（秒）",
			Buckets: httpDurationBuckets,
		}),
	}
	reg.MustRegister(
		c.queryTotal, c.querySuccess, c.queryFailed, c.queryDuration,
		c.clientTotal, c.clientOnline, c.httpRequests, c.httpDuration,
	)
	globalCollector = c
	return c
}

// Global 返回全局 Collector 单例（惰性初始化）。
func Global() *Collector { return ensureGlobal() }

// QueryStarted 查询总次数 +1。
func (c *Collector) QueryStarted() { c.queryTotal.Inc() }

// QuerySucceeded 记录成功查询及耗时。
func (c *Collector) QuerySucceeded(duration time.Duration) {
	c.querySuccess.Inc()
	c.queryDuration.Observe(duration.Seconds())
}

// QueryFailed 记录失败查询及耗时。
func (c *Collector) QueryFailed(duration time.Duration) {
	c.queryFailed.Inc()
	c.queryDuration.Observe(duration.Seconds())
}

// UpdateClientStats 更新客户端统计（总数与在线数）。
func (c *Collector) UpdateClientStats(total, online int64) {
	c.clientTotal.Set(float64(total))
	c.clientOnline.Set(float64(online))
}

// HTTPRequestRecorded 记录一次 HTTP 请求（状态码 + 耗时）。
//
// 为兼容旧调用方保留该签名；method 维度由中间件内部记录时补齐。
func (c *Collector) HTTPRequestRecorded(statusCode int, duration time.Duration) {
	code := strconv.Itoa(statusCode)
	c.httpRequests.WithLabelValues("--", code).Inc()
	c.httpDuration.Observe(duration.Seconds())
}

// HTTPRequestRecordedWithMethod 内部使用，记录含 method 的请求计数。
func (c *Collector) HTTPRequestRecordedWithMethod(method string, statusCode int, duration time.Duration) {
	code := strconv.Itoa(statusCode)
	c.httpRequests.WithLabelValues(method, code).Inc()
	c.httpDuration.Observe(duration.Seconds())
}

// Handler 返回 /metrics HTTP handler。
func (c *Collector) Handler() http.Handler {
	return promhttp.HandlerFor(c.reg, promhttp.HandlerOpts{})
}

// GinHandler 是 Gin 中间件-friendly 的适配器。
func (c *Collector) GinHandler() gin.HandlerFunc {
	h := promhttp.HandlerFor(c.reg, promhttp.HandlerOpts{})
	return func(ctx *gin.Context) { h.ServeHTTP(ctx.Writer, ctx.Request) }
}

// HTTPMetricsMiddleware 返回 Gin 中间件，按方法+状态码统计请求。
func HTTPMetricsMiddleware() gin.HandlerFunc {
	c := Global()
	return func(ctx *gin.Context) {
		start := time.Now()
		ctx.Next()
		duration := time.Since(start)
		c.HTTPRequestRecordedWithMethod(ctx.Request.Method, ctx.Writer.Status(), duration)
	}
}
