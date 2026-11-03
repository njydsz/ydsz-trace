// Package logs 子文件：模型路由策略（V2.1 FR-1304）。
//
// 支持按能力类型（chat/embedding/rerank）配置默认模型与降级链。
// 路由策略：
//   - primary：首选模型
//   - fallback：降级模型列表（按顺序尝试）
//   - 自动故障转移：主模型调用失败后自动切换降级模型
package logs

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

const (
	// modelRouteChat chat 能力的路由 key
	modelRouteChat = "chat"
	// modelRouteEmbedding embedding 能力的路由 key
	modelRouteEmbedding = "embedding"
	// modelRouteRerank rerank 能力的路由 key
	modelRouteRerank = "rerank"
)

// ModelRouter 模型路由器。
//
// 维护各能力类型的默认模型 + 降级链，支持自动故障转移。
type ModelRouter struct {
	mu sync.RWMutex

	// defaults 各能力类型的默认模型 ID
	defaults map[string]string
	// fallbacks 各能力类型的降级模型 ID 列表
	fallbacks map[string][]string
	// failedModels 记录最近失败的模型（用于快速跳过）
	failedModels map[string]time.Time
}

// NewModelRouter 创建模型路由器，从数据库加载配置。
func NewModelRouter() *ModelRouter {
	r := &ModelRouter{
		defaults:     make(map[string]string),
		fallbacks:    make(map[string][]string),
		failedModels: make(map[string]time.Time),
	}
	r.loadFromDB()
	return r
}

// loadFromDB 从 t_model_capability 加载启用的模型。
func (r *ModelRouter) loadFromDB() {
	if models.DB == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, capability := range []string{modelRouteChat, modelRouteEmbedding, modelRouteRerank} {
		list, err := models.GetEnabledModelCapabilities(capability)
		if err != nil || len(list) == 0 {
			continue
		}
		// 第一个为默认，其余为降级
		r.defaults[capability] = list[0].ModelId
		if len(list) > 1 {
			fbs := make([]string, 0, len(list)-1)
			for _, m := range list[1:] {
				fbs = append(fbs, m.ModelId)
			}
			r.fallbacks[capability] = fbs
		}
	}
}

// Resolve 返回指定能力类型的可用模型 ID 列表（默认在前，降级在后）。
//
// 跳过最近 5 分钟内失败过的模型。
func (r *ModelRouter) Resolve(capability string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, 1+len(r.fallbacks[capability]))
	if def, ok := r.defaults[capability]; ok && !r.isRecentlyFailed(def) {
		result = append(result, def)
	}
	for _, fb := range r.fallbacks[capability] {
		if !r.isRecentlyFailed(fb) {
			result = append(result, fb)
		}
	}
	return result
}

// MarkFailed 标记模型失败（触发快速跳过）。
func (r *ModelRouter) MarkFailed(modelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failedModels[modelID] = time.Now()
}

// MarkSuccess 清除模型的失败标记。
func (r *ModelRouter) MarkSuccess(modelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.failedModels, modelID)
}

// isRecentlyFailed 判断模型是否在最近 5 分钟内失败过。
func (r *ModelRouter) isRecentlyFailed(modelID string) bool {
	t, ok := r.failedModels[modelID]
	if !ok {
		return false
	}
	return time.Since(t) < 5*time.Minute
}

// GetDefault 返回指定能力类型的默认模型 ID。
func (r *ModelRouter) GetDefault(capability string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaults[capability]
}

// CallWithFallback 使用降级链调用模型（通用模板方法）。
//
// fn 是实际的调用函数，接收 modelID 返回结果和错误。
// 当 fn 返回错误时，自动尝试下一个降级模型。
func (r *ModelRouter) CallWithFallback(ctx context.Context, capability string,
	fn func(ctx context.Context, modelID string) (interface{}, error)) (interface{}, error) {

	modelsList := r.Resolve(capability)
	if len(modelsList) == 0 {
		return nil, fmt.Errorf("无可用模型: capability=%s", capability)
	}

	var lastErr error
	for _, modelID := range modelsList {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, err := fn(ctx, modelID)
		if err == nil {
			r.MarkSuccess(modelID)
			return result, nil
		}
		lastErr = err
		r.MarkFailed(modelID)
		log.Printf("[router] 模型 %s 调用失败，尝试降级: %v", modelID, err)
	}

	return nil, fmt.Errorf("全部模型调用失败: %w", lastErr)
}

// globalRouter 全局模型路由器实例。
var globalRouter = NewModelRouter()

// GetGlobalRouter 返回全局模型路由器。
func GetGlobalRouter() *ModelRouter {
	return globalRouter
}

// ModelRouterResolveHandler 查询指定能力类型的可用模型列表。
//
// @Summary 模型路由解析
// @Description 查询指定能力类型（chat/embedding/rerank）的可用模型列表，按默认模型 + 降级链顺序返回。
// @Tags ModelRouter
// @Produce json
// @Param capability path string true "能力类型：chat / embedding / rerank"
// @Success 200 {object} api.Response{data=map[string]interface{}} "capability, models, default"
// @Failure 400 {object} api.Response "capability 不能为空"
// @Failure 404 {object} api.Response "无可用模型"
// @Router /logs/model-router/{capability} [get]
func ModelRouterResolveHandler(c *gin.Context) {
	capability := c.Param("capability")
	if capability == "" {
		api.Fail(c, api.CodeBadRequest, "capability 不能为空")
		return
	}

	router := GetGlobalRouter()
	models := router.Resolve(capability)
	if len(models) == 0 {
		api.Fail(c, api.CodeNotFound, "无可用模型: "+capability)
		return
	}

	api.Success(c, "查询成功", gin.H{
		"capability": capability,
		"models":     models,
		"default":    router.GetDefault(capability),
	})
}
