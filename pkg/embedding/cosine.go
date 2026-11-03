// Package embedding 提供文本向量化能力（L2 嵌入层）。
//
// 接口层零外部依赖；HTTP 调用只在实现层内部。
//
// 默认 Provider 为 Local（ollama），确保"数据不出内网"（P0-06）。
// 云端 embedding（OpenAI 等）仅作为可选 Provider。
package embedding

import (
	"math"
)

// CosineSimilarity 计算两个向量的余弦相似度。
//
// 返回值范围 [-1, 1]，值越大越相似。
// 对零向量或不等长输入安全（返回 0，不 panic）。
//
// V2.1 优化：循环展开 + 边界检查消除，提升 SIMD 友好性。
func CosineSimilarity(a, b []float32) float32 {
	n := len(a)
	if n != len(b) || n == 0 {
		return 0
	}

	var dot, normA, normB float64

	// 4-way 循环展开，减少分支预测失败
	i := 0
	for ; i+4 <= n; i += 4 {
		a0, a1, a2, a3 := a[i], a[i+1], a[i+2], a[i+3]
		b0, b1, b2, b3 := b[i], b[i+1], b[i+2], b[i+3]

		dot += float64(a0*b0) + float64(a1*b1) + float64(a2*b2) + float64(a3*b3)
		normA += float64(a0*a0) + float64(a1*a1) + float64(a2*a2) + float64(a3*a3)
		normB += float64(b0*b0) + float64(b1*b1) + float64(b2*b2) + float64(b3*b3)
	}

	// 处理剩余元素
	for ; i < n; i++ {
		ai := a[i]
		bi := b[i]
		dot += float64(ai * bi)
		normA += float64(ai * ai)
		normB += float64(bi * bi)
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
