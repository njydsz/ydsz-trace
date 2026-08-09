// Package controllers 包含 logc 客户端的公共控制器（健康检查、Source 状态等）。
package controllers

import (
	"net/http"
	"time"

	"ydsz-trace/pkg/source"

	"github.com/gin-gonic/gin"
)

// HealthResp 健康检查响应结构。
type HealthResp struct {
	// Status 状态标识：ok / ready
	Status string `json:"status"`
	// App 服务标识
	App string `json:"app"`
	// Time 当前服务器本地时间
	Time string `json:"time"`
	// Source 激活的 Source 摘要（仅就绪探针返回）
	Source *source.SourceInfo `json:"source,omitempty"`
}

// Main 返回一行纯文本，表示服务存活。
func Main(c *gin.Context) {
	c.String(http.StatusOK, "Ydsz Trace logc agent is running.")
}

// Health 返回存活探针结果（K8s liveness probe 使用）。
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ok",
		App:    "ydsz-trace-logc",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// Ready 返回就绪探针结果（K8s readiness probe 使用）。
func Ready(c *gin.Context) {
	s := sourceFromContext(c)
	c.JSON(http.StatusOK, HealthResp{
		Status: "ready",
		App:    "ydsz-trace-logc",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
		Source: sourceInfoPtr(s),
	})
}

// SourceInfo 返回激活 Source 的详细信息（调试接口）。
func SourceInfo(c *gin.Context) {
	s := sourceFromContext(c)
	if s == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "source not initialized"})
		return
	}
	c.JSON(http.StatusOK, s.Info())
}

// SourceTargets 返回当前 Source 发现的目标列表（Docker / K8s 模式）。
// file 模式下返回空列表。
func SourceTargets(c *gin.Context) {
	s := sourceFromContext(c)
	if s == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "source not initialized"})
		return
	}
	// file 模式不提供动态目标
	if s.Info().Type == string(source.SourceTypeFile) {
		c.JSON(http.StatusOK, gin.H{"type": "file", "targets": []source.DiscoveryTarget{}})
		return
	}
	ctx := c.Request.Context()
	ch, err := s.Discover(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 等待 snapshot 或超时
	select {
	case evt := <-ch:
		if evt.Type == "snapshot" {
			c.JSON(http.StatusOK, gin.H{"type": s.Info().Type, "targets": evt.Targets})
			return
		}
		// 非 snapshot：收集前 N 个事件
		var targets []source.DiscoveryTarget
		targets = append(targets, evt.Targets...)
		c.JSON(http.StatusOK, gin.H{"type": s.Info().Type, "targets": targets})
	case <-time.After(5 * time.Second):
		c.JSON(http.StatusOK, gin.H{"type": s.Info().Type, "targets": []source.DiscoveryTarget{}, "timeout": true})
	}
}

func sourceFromContext(c *gin.Context) source.Source {
	v, exists := c.Get("__source__")
	if !exists {
		return nil
	}
	s, ok := v.(source.Source)
	if !ok {
		return nil
	}
	return s
}

func sourceInfoPtr(s source.Source) *source.SourceInfo {
	if s == nil {
		return nil
	}
	info := s.Info()
	return &info
}
