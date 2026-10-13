// Package services v2.0 语义服务门面层（facade）。
//
// 统一封装模板解析、异常检测、模板聚类等引擎，供 Controller 调用。
// 所有 Service 实例为并发安全，可在 HTTP Handler 间共享。
package services

import (
	"hash/fnv"
	"regexp"
	"sort"
	"strings"
	"sync"

	"ydsz-trace/pkg/parser"
	"ydsz-trace/pkg/schema"
	"ydsz-trace/pkg/stats"
)

// ParserService 模板解析门面。
type ParserService struct {
	drain *parser.LogParser
}

// NewParserService 初始化 ParserService。
func NewParserService() *ParserService {
	return &ParserService{
		drain: parser.New(),
	}
}

// ParseLine 解析单行日志，返回模板 + 字段。
func (s *ParserService) ParseLine(line string) parser.ParseResult {
	return s.drain.ParseLine(line)
}

// BatchParse 批量解析（并发，默认 4 协程）。
func (s *ParserService) BatchParse(lines []string) []parser.ParseResult {
	workers := 4
	return s.drain.ParseBatch(lines, workers)
}

// AllTemplates 返回全部已知模板。
func (s *ParserService) AllTemplates() []parser.Template {
	return s.drain.AllTemplates()
}

// TemplateCount 返回已知模板数量。
func (s *ParserService) TemplateCount() int {
	return s.drain.TemplateCount()
}

// AnomalyService 异常检测门面（包装 pkg/stats.AnomalyEngine）。
type AnomalyService struct {
	engine *stats.AnomalyEngine
	mu     sync.Mutex
}

// NewAnomalyService 创建 AnomalyService 单例。
func NewAnomalyService() *AnomalyService {
	return &AnomalyService{
		engine: stats.NewAnomalyEngine(),
	}
}

// UpdateAndDetect 更新基线并检测异常。
//
// series 的 key = templateHash，value = 当前桶计数。
// ts 为时间戳（格式 "2006-01-02 15:04:05"）。
// 返回异常事件列表（无异常返回 nil）。
func (s *AnomalyService) UpdateAndDetect(series map[string]int, ts string) []stats.AnomalyEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 更新所有序列的基线
	for hash, count := range series {
		s.engine.UpdateBaseline(hash, 0, count)
	}

	// 检测异常
	return s.engine.DetectAnomalies(series, ts)
}

// GetBaseline 读取指定模板的基线状态。
func (s *AnomalyService) GetBaseline(templateHash string, itemID int64) (*stats.Baseline, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.engine.GetBaseline(templateHash, itemID)
}

// ClusterService 模板聚类门面。
//
// 将日志行按模板 hash 分组，输出 schema.LogTemplate。
type ClusterService struct{}

// NewClusterService 创建 ClusterService。
func NewClusterService() *ClusterService {
	return &ClusterService{}
}

// GroupLines 将日志行按模板 hash 分组。
//
// lines 为原始日志行。
// maxLines 限制处理的行数上限（0 或负数表示不限制）。
// 返回 schema.LogTemplate 切片，按命中数降序排列。
func (s *ClusterService) GroupLines(lines []string, maxLines int) []schema.LogTemplate {
	if len(lines) == 0 {
		return nil
	}

	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	type groupAcc struct {
		pattern   string
		count     int
		paramIdx  []int
		firstTime string
		lastTime  string
	}

	m := map[string]*groupAcc{}

	for _, line := range lines {
		pattern := normalizeLine(line)
		tid := hashTemplate(pattern)

		g, exists := m[tid]
		if !exists {
			idx := extractParamIndices(line, pattern)
			m[tid] = &groupAcc{
				pattern:   pattern,
				count:     1,
				paramIdx:  idx,
				firstTime: extractTimestamp(line),
				lastTime:  extractTimestamp(line),
			}
		} else {
			g.count++
			g.lastTime = extractTimestamp(line)
		}
	}

	templates := make([]schema.LogTemplate, 0, len(m))
	for tid, acc := range m {
		templates = append(templates, schema.LogTemplate{
			ID:        tid,
			Pattern:   acc.pattern,
			ParamIdx:  acc.paramIdx,
			Count:     acc.count,
			FirstTime: acc.firstTime,
			LastTime:  acc.lastTime,
		})
	}

	// 按 Count 降序排序
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Count > templates[j].Count
	})

	return templates
}

// --- 内部工具函数 ---

// 预编译正则表达式。
var (
	reNormalizeNum  = regexp.MustCompile(`\b\d+\.\d+\b|\b\d+\b`)
	reNormalizeIPv4 = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	reNormalizeUUID = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	reNormalizePath = regexp.MustCompile(`(/[^\s:]+)+`)
	reNormalizeDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}(?:[T ]\d{2}:\d{2}:\d{2})?`)
)

// hashTemplate 计算模板的 fnv32a hash（字符串形式）。
func hashTemplate(pattern string) string {
	h := fnv.New32a()
	h.Write([]byte(pattern))
	return formatHash(h.Sum32())
}

// formatHash 将 uint32 转为 8 位十六进制字符串。
func formatHash(v uint32) string {
	const digits = "0123456789abcdef"
	buf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		buf[i] = digits[v&0xf]
		v >>= 4
	}
	return string(buf)
}

// normalizeLine 将日志行中的参数替换为 <*>，返回模板。
func normalizeLine(line string) string {
	result := reNormalizeIPv4.ReplaceAllString(line, "<*>")
	result = reNormalizeUUID.ReplaceAllString(result, "<*>")
	result = reNormalizePath.ReplaceAllString(result, "<*>")
	result = reNormalizeDate.ReplaceAllString(result, "<*>")
	result = reNormalizeNum.ReplaceAllString(result, "<*>")
	return result
}

// extractParamIndices 提取参数位置索引。
func extractParamIndices(original, pattern string) []int {
	var idx []int
	patternTokens := strings.Fields(pattern)
	origTokens := strings.Fields(original)
	for i, pt := range patternTokens {
		if pt == "<*>" && i < len(origTokens) {
			// 找到 <*> 对应的原始 token 位置
			idx = append(idx, i)
			_ = origTokens[i]
		}
	}
	return idx
}

// extractTimestamp 从日志行提取时间戳。
func extractTimestamp(line string) string {
	m := reNormalizeDate.FindString(line)
	return m
}
