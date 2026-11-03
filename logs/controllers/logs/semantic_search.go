// Package logs 子文件：语义检索（L2 embedding 层）。
//
// 接入向量相似度检索：用户输入自然语言 query，经由 Embedder 生成向量，
// 再通过 VectorStore cosine similarity 召回最相似的模板或样本。
//
// 与 /logs/nl2dsl/parse 的区别：NL2DSL 仅做键值提取与 DSL 组装，
// 不包含语义匹配；本接口提供语义级召回能力。
//
// 路由：POST /logs/semantic/search（本文件仅实现控制器，路由挂载由 router.go 完成）
package logs

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/embedding"
)

// 语义检索目标类型。
const (
	// SemanticTargetTemplate 检索日志模板embedding_vec。
	SemanticTargetTemplate = "template"
	// SemanticTargetSample 检索样本行向量。
	SemanticTargetSample = "sample"

	// defaultSemanticTopK 默认召回条数。
	defaultSemanticTopK = 10
	// maxSemanticTopK 单次召回上限。
	maxSemanticTopK = 100
	// semanticSearchTimeout 检索操作超时。
	semanticSearchTimeout = 30 * time.Second
)

// SemanticSearchReq 语义检索请求。
type SemanticSearchReq struct {
	// Query 用户自然语言输入（必填）。
	Query string `json:"query" binding:"required"`
	// Type 检索目标类型：'template' / 'sample'（默认 'template'）。
	Type string `json:"type"`
	// TopK 召回条数（默认 10，最大 100）。
	TopK int `json:"topK"`
}

// SemanticSearchResp 语义检索响应。
type SemanticSearchResp struct {
	// Results 召回结果按相似度降序排列。
	Results []SemanticResult `json:"results"`
	// Degraded 为 true 表示 L2 不可用，本次走 L0+L1 降级召回（语义相关性弱）。
	// 前端应据此提示用户"语义检索未启用"，而非展示为正常语义结果。
	Degraded bool `json:"degraded,omitempty"`
	// DegradReason 降级原因（供前端提示与运维排查）。
	DegradReason string `json:"degradReason,omitempty"`
}

// SemanticResult 单条召回结果。
type SemanticResult struct {
	// TargetID 目标标识（模板 hash 或样本 ID）。
	TargetID string `json:"targetId"`
	// TargetType 目标类型（'template' / 'sample'）。
	TargetType string `json:"targetType"`
	// Score 余弦相似度得分（越大越相似，范围 [-1, 1]）。
	Score float32 `json:"score"`
	// Metadata 附加可读字段（含 template_text / sample_line 等）。
	Metadata map[string]string `json:"metadata"`
}

// SemanticSearch 语义检索路由处理器包装。
//
// @Summary 语义检索
// @Description 将自然语言 query 向量化后与模板/样本向量做 cosine similarity 检索
// @Tags SemanticSearch
// @Accept json
// @Produce json
// @Param request body SemanticSearchReq true "自然语言检索请求"
// @Success 200 {object} api.Success{data=SemanticSearchResp}
// @Failure 400 {object} api.Response "参数错误（query 为空或 type 非法）"
// @Router /logs/semantic/search [post]
func SemanticSearch(c *gin.Context) {
	semanticSearch(c)
}

// semanticSearch 核心检索逻辑。
func semanticSearch(c *gin.Context) {
	var req SemanticSearchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		api.Fail(c, api.CodeBadRequest, "query 不能为空")
		return
	}

	// 校验并归一化 type。
	if req.Type == "" {
		req.Type = SemanticTargetTemplate
	}
	if req.Type != SemanticTargetTemplate && req.Type != SemanticTargetSample {
		api.Fail(c, api.CodeBadRequest, "type 必须为 'template' 或 'sample'")
		return
	}

	// TopK 默认值与上限。
	if req.TopK <= 0 {
		req.TopK = defaultSemanticTopK
	}
	if req.TopK > maxSemanticTopK {
		req.TopK = maxSemanticTopK
	}

	// 创建 Embedder（降级判断）。
	// L2 不可用时不再直接失败：退化为 t_embedding 空表 + 关键词提示，
	// 让前端明确展示"语义检索未启用"，同时保留接口可用性。
	embedder := createEmbedder()
	degraded := embedder == nil
	degradReason := ""
	if degraded {
		degradReason = "L2 嵌入层未启用或后端不可达（检查 YDSZ_EMBED_* 环境变量或 ollama 服务）"
		log.Printf("[semantic] L2 不可用，进入降级检索: %s", degradReason)
	}

	// 创建 VectorStore。
	// V2.1 优先查询 t_embedding 表（由写入管线填充），降级到 embedding_vec 表。
	if models.DB == nil {
		api.Fail(c, api.CodeServerError, "语义检索暂不可用，请检查 ollama 服务")
		return
	}
	tstore, terr := embedding.NewTStore(models.DB.DB)
	if terr != nil {
		log.Printf("[semantic] 初始化 t_embedding 存储失败: %v", terr)
		api.Fail(c, api.CodeServerError, "语义检索暂不可用，请检查 ollama 服务")
		return
	}

	// 带超时的检索上下文。
	ctx, cancel := context.WithTimeout(c.Request.Context(), semanticSearchTimeout)
	defer cancel()

	var searchResults []embedding.SearchResult

	if !degraded {
		// Embed 用户 query。
		queryVec, err := embedder.Embed(ctx, req.Query)
		if err != nil {
			// 检索期失败同样降级，不中断用户请求。
			log.Printf("[semantic] Embed 用户 query 失败，降级: %v", err)
			degraded = true
			degradReason = "query 向量化失败: " + err.Error()
			searchResults = nil
		} else {
			// 向量检索：优先 t_embedding 表，降级 embedding_vec 表。
			sr, serr := tstore.SearchT(ctx, queryVec, req.TopK, "", req.Type)
			if serr != nil {
				log.Printf("[semantic] t_embedding 检索失败，降级到 embedding_vec: %v", serr)
				sr = nil
			}
			if len(sr) == 0 {
				// 降级：查询 embedding_vec 表（兼容旧数据）
				store, verr := embedding.NewSQLStore(models.DB.DB)
				if verr != nil {
					log.Printf("[semantic] 初始化 embedding_vec 存储失败: %v", verr)
				} else {
					sr, serr = store.Search(ctx, queryVec, req.TopK)
					if serr != nil {
						log.Printf("[semantic] embedding_vec 检索失败: %v", serr)
					}
				}
			}
			searchResults = sr
		}
	}

	// 包装响应。
	results := make([]SemanticResult, 0, len(searchResults))
	for _, sr := range searchResults {
		results = append(results, SemanticResult{
			TargetID:   sr.Key,
			TargetType: req.Type,
			Score:      sr.Score,
			Metadata:   enrichMetadata(req.Type, sr),
		})
	}

	msg := "语义检索成功"
	if degraded {
		msg = "语义检索降级：L2 嵌入层未启用，仅返回基础召回"
	}

	api.Success(c, msg, SemanticSearchResp{
		Results:      results,
		Degraded:     degraded,
		DegradReason: degradReason,
	})
}

// createEmbedder 根据环境变量/能力配置创建 Embedder 实例。
//
// 复用 pkg/embedding.NewFromEnv，支持 local / openai / claude 三类 Provider。
// 探测失败返回 nil，由调用方降级为 L0+L1 检索（不直接报错给用户）。
//
// 环境变量：
//   - YDSZ_EMBED_PROVIDER：local | openai | claude（默认 local）
//   - YDSZ_EMBED_ENDPOINT：服务地址（默认 http://localhost:11434）
//   - YDSZ_EMBED_MODEL：模型名称（默认 bge-m3）
//   - YDSZ_EMBED_API_KEY：云端 Provider 的 API Key
//   - YDSZ_EMBED_DIMENSION：向量维度
func createEmbedder() embedding.Embedder {
	e, provider, err := embedding.NewFromEnv()
	if err != nil {
		log.Printf("[semantic] 创建 Embedder 失败: %v", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), embeddingProbeTimeout)
	defer cancel()

	if perr := embedding.Probe(ctx, e); perr != nil {
		log.Printf("[semantic] L2 探测失败，降级为 L0+L1: %v", perr)
		return nil
	}

	log.Printf("[semantic] L2 嵌入层已启用 provider=%d model=%s dim=%d",
		provider, os.Getenv(embedding.EnvEmbedModel), e.Dimension())
	return e
}

// embeddingProbeTimeout 语义检索路径的 L2 探测超时。
//
// 检索是用户同步等待路径，超时需短于 semanticSearchTimeout，避免整体请求被拖慢。
const embeddingProbeTimeout = 3 * time.Second

// enrichMetadata 从 models 层查询可读字段并合并到 SearchResult.Metadata。
//
// 对 template 类型追加 template_text、service、level 等；
// 对 sample 类型追加 sample_line。
func enrichMetadata(targetType string, sr embedding.SearchResult) map[string]string {
	metadata := sr.Metadata
	if metadata == nil {
		metadata = make(map[string]string)
	}

	switch targetType {
	case SemanticTargetTemplate:
	 enrichTemplateMetadata(sr.Key, metadata)
	case SemanticTargetSample:
	 enrichSampleMetadata(sr.Key, metadata)
	}

	return metadata
}

// enrichTemplateMetadata 查询 t_log_template 并追加可读字段。
func enrichTemplateMetadata(templateID string, metadata map[string]string) {
	tmpl, err := models.GetTemplateByID(templateID)
	if err != nil || tmpl == nil {
		return
	}
	metadata["templateText"] = tmpl.TemplateText
	metadata["service"] = tmpl.Service
	metadata["level"] = tmpl.Level
	metadata["totalCount"] = strconv.Itoa(tmpl.TotalCount)
	metadata["sampleLine"] = tmpl.SampleLine
	if tmpl.FirstSeen != "" {
		metadata["firstSeen"] = tmpl.FirstSeen
	}
	if tmpl.LastSeen != "" {
		metadata["lastSeen"] = tmpl.LastSeen
	}
}

// enrichSampleMetadata 查询 t_embedding 元数据并追加可读字段。
func enrichSampleMetadata(embeddingKey string, metadata map[string]string) {
	row, err := models.GetEmbeddingByKey(embeddingKey)
	if err != nil || row == nil {
		return
	}
	metadata["targetID"] = row.TargetId
	metadata["modelVersion"] = row.ModelVersion
	metadata["vectorDim"] = strconv.Itoa(row.VectorDim)
}
