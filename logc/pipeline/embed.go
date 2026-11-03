// Package pipeline logc 端侧管道（PRD 6.2 L0→L1→L2→L3）。
//
// EmbedPipeline 封装 pkg/embedding.Embedder，负责模板文本的向量化（L2 层）。
package pipeline

import (
	"context"

	"ydsz-trace/pkg/embedding"
)

// EmbedPipeline 端侧向量化管道（L2 嵌入层）。
//
// 封装 pkg/embedding.Embedder，提供：
//   - 单模板向量化 EmbedTemplate
//   - 批量模板向量化 EmbedBatch
//
// embedder 为 nil 时所有方法自动跳过（返回 nil，不报错），
// 符合 PRD 6.2 "向量化为可选阶段" 的设计约束。
type EmbedPipeline struct {
	embedder embedding.Embedder
}

// NewEmbedPipeline 创建向量化管道实例。
//
// embedder 可为 nil（向量化阶段自动跳过）。
func NewEmbedPipeline(embedder embedding.Embedder) *EmbedPipeline {
	return &EmbedPipeline{
		embedder: embedder,
	}
}

// EmbedTemplate 对单个模板文本生成向量。
//
// embedder 不可用时返回 (nil, nil)，表示跳过该阶段。
func (p *EmbedPipeline) EmbedTemplate(template string) ([]float32, error) {
	if p.embedder == nil {
		return nil, nil
	}
	if template == "" {
		return nil, embedding.ErrEmptyInput
	}

	ctx, cancel := context.WithTimeout(context.Background(), embedTimeout)
	defer cancel()

	vec, err := p.embedder.Embed(ctx, template)
	if err != nil {
		return nil, err
	}
	return vec, nil
}

// EmbedBatch 批量向量化模板文本。
//
// 返回与输入顺序一致的向量切片。embedder 不可用时返回 nil。
// 批量接口使用 Embedder.EmbedBatch 以减少 HTTP 往返。
func (p *EmbedPipeline) EmbedBatch(templates []string) ([][]float32, error) {
	if p.embedder == nil {
		return nil, nil
	}
	if len(templates) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), embedTimeout)
	defer cancel()

	// EmbedBatch 接口接收 [][]string（每个元素为一组文本）
	inputs := make([][]string, len(templates))
	for i, t := range templates {
		inputs[i] = []string{t}
	}

	vecs, err := p.embedder.EmbedBatch(ctx, inputs)
	if err != nil {
		return nil, err
	}
	return vecs, nil
}

// Dimension 返回向量维度（embedder 不可用时返回 0）。
func (p *EmbedPipeline) Dimension() int {
	if p.embedder == nil {
		return 0
	}
	return p.embedder.Dimension()
}
