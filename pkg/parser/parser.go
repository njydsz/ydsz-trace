package parser

import (
	"strings"
	"sync"
)

// Template 日志模板结构。
type Template struct {
	// ID 模板唯一标识
	ID int64

	// Template 模板串（参数替换为 <*>）
	Template string

	// Count 已匹配的日志行数
	Count int64
}

// ParseResult 单行日志解析结果。
type ParseResult struct {
	// TemplateID 匹配到的模板唯一标识（0 表示未匹配到已知模板）
	TemplateID int64

	// Template 模板串（参数替换为 <*>）
	Template string

	// Params 提取的参数值（原始 token）
	Params []string

	// Fields 抽取的命名字段
	Fields map[string]string

	// Format 匹配到的内置格式名称（空 = 由 Drain 发现）
	Format string

	// Confidence 模板匹配置信度 [0,1]
	Confidence float64
}

// LogParser 模板解析器实例（并发安全）。
type LogParser struct {
	mu        sync.RWMutex
	templates map[string]*Template
	drain     *Drain
	formats   []FormatMatcher
}

// New 创建一个新的 LogParser 实例。
func New() *LogParser {
	return &LogParser{
		templates: make(map[string]*Template),
		drain:     NewDrain(),
		formats:   builtinFormats(),
	}
}

// ParseLine 解析单行日志，返回模板与字段。
//
// 流程：
// 1. 预抽取通用字段（时间戳/级别/TraceID 等）
// 2. 尝试内置格式匹配（JSON/Spring Boot/Nginx...）
// 3. 格式匹配成功：返回格式名和抽取的字段
// 4. 格式匹配失败：走 Drain 算法，增量挖掘模板
func (p *LogParser) ParseLine(line string) ParseResult {
	// 预抽取通用字段
	preFields := ExtractFields(line)

	// 尝试内置格式匹配
	if fmtFields, fmtName, matched := MatchFormat(line); matched {
		// 合并预抽取字段（内置格式优先）
		for k, v := range fmtFields {
			preFields[k] = v
		}
		templateStr := maskParams(line, fmtFields)
		// 格式匹配也记录到 Drain 以便模板管理
		_, drainParams := p.drain.ProcessLine(line)
		var tmplID int64
		if t, found := p.drain.findTemplateByContent(line); found {
			tmplID = t.id
		}
		return ParseResult{
			TemplateID: tmplID,
			Template:   templateStr,
			Params:     drainParams,
			Fields:     preFields,
			Format:     fmtName,
			Confidence: 1.0,
		}
	}

	// Drain 算法挖掘
	templateStr, params := p.drain.ProcessLine(line)
	confidence := 0.6
	if len(params) > 0 {
		confidence = 0.5 + 0.1*float64(min(len(params), 5))
		if confidence > 0.95 {
			confidence = 0.95
		}
	}

	// 获取匹配到的模板 ID
	var tmplID int64
	if tmpl, found := p.drain.findTemplateByContent(line); found {
		tmplID = tmpl.id
	}

	return ParseResult{
		TemplateID: tmplID,
		Template:   templateStr,
		Params:     params,
		Fields:     preFields,
		Format:     "",
		Confidence: confidence,
	}
}

// ParseBatch 并发解析多行日志。
//
// workers 指定并发协程数，若 <1 则默认为 1。
func (p *LogParser) ParseBatch(lines []string, workers int) []ParseResult {
	if workers < 1 {
		workers = 1
	}
	n := len(lines)
	if n == 0 {
		return nil
	}
	results := make([]ParseResult, n)

	if workers == 1 || n <= workers {
		for i, line := range lines {
			results[i] = p.ParseLine(line)
		}
		return results
	}

	// 并发解析
	batchSize := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		start := w * batchSize
		end := start + batchSize
		if end > n {
			end = n
		}
		if start >= n {
			break
		}
		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			for i := s; i < e; i++ {
				results[i] = p.ParseLine(lines[i])
			}
		}(start, end)
	}
	wg.Wait()
	return results
}

// GetTemplate 根据日志行查找已知模板。
// 若 Drain 中已存在相似模板则返回（second=true），否则返回 false。
func (p *LogParser) GetTemplate(line string) (*Template, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tokens := p.drain.tokenize(line)
	if len(tokens) == 0 {
		return nil, false
	}
	content := preprocess(tokens)
	if len(content) == 0 {
		content = tokens
	}
	prefix := content
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	node := p.drain.findNode(prefix)
	if len(node.templates) == 0 {
		return nil, false
	}
	best, score := p.drain.matchBestTemplate(content, node.templates)
	if best == nil || score < p.drain.simThresh {
		return nil, false
	}
	return &Template{
		ID:       best.id,
		Template: strings.Join(best.tokens, " "),
		Count:    best.count.Load(),
	}, true
}

// AllTemplates 返回当前全部已知模板副本。
func (p *LogParser) AllTemplates() []Template {
	return p.drain.AllTemplates()
}

// TemplateCount 返回已知模板数量。
func (p *LogParser) TemplateCount() int {
	return p.drain.TemplateCount()
}

// maskParams 根据内置格式已提取的字段值，将原始行中的具体值替换为 <*>，
// 从而生成模板字符串（参数化表示）。
func maskParams(line string, fields map[string]string) string {
	result := line
	// 按值的长度降序替换，避免短值先替换导致问题
	type pair struct {
		k, v string
	}
	pairs := make([]pair, 0, len(fields))
	for k, v := range fields {
		if v != "" && len(v) > 2 {
			pairs = append(pairs, pair{k, v})
		}
	}
	// 简单按值长度降序
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if len(pairs[j].v) > len(pairs[i].v) {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	for _, p := range pairs {
		// 仅替换完整单词/值的出现（避免误替换短子串）
		result = strings.ReplaceAll(result, p.v, "<*>")
	}
	return result
}

// min 返回两整数中较小值。
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
