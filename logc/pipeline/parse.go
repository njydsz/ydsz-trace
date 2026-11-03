// Package pipeline logc 端侧管道（PRD 6.2 L0→L1→L2→L3）。
//
// ParsePipeline 封装 pkg/parser.LogParser，负责日志行的模板解析与多行合并。
package pipeline

import (
	"strings"
	"sync"

	"ydsz-trace/pkg/parser"
)

// ParsePipeline 端侧解析管道（L0 模板解析层）。
//
// 封装 pkg/parser.LogParser，提供：
//   - 单行解析 ProcessLine
//   - 并发批解析 ProcessBatch
//   - 上下文行合并（多行日志合并为一个模板）
//
// 该结构为并发安全（内部 LogParser 已保证线程安全）。
type ParsePipeline struct {
	parser *parser.LogParser
}

// NewParsePipeline 创建解析管道实例。
func NewParsePipeline() *ParsePipeline {
	return &ParsePipeline{
		parser: parser.New(),
	}
}

// ProcessLine 解析单行日志，返回模板解析结果。
//
// 与 parser.LogParser.ParseLine 行为一致，包装为管道接口。
func (p *ParsePipeline) ProcessLine(line string) (parser.ParseResult, error) {
	if strings.TrimSpace(line) == "" {
		return parser.ParseResult{}, ErrEmptyLine
	}
	return p.parser.ParseLine(line), nil
}

// ProcessBatch 并发解析多行日志，返回与输入顺序一致的结果切片。
//
// workers 指定并发协程数，<=0 时使用默认值 defaultWorkerCount。
func (p *ParsePipeline) ProcessBatch(lines []string) []parser.ParseResult {
	if len(lines) == 0 {
		return nil
	}
	return p.parser.ParseBatch(lines, defaultWorkerCount)
}

// MergeContextLines 将多行日志合并为一个模板（上下文行合并）。
//
// 适用场景：Java 异常堆栈、多行 SQL 等跨行日志。
// 算法：
//  1. 过滤空行与纯堆栈行（以 tab/空格 + "at " 开头）
//  2. 取首行作为主模板行
//  3. 若存在异常类型行（含 Exception/Error），将其附加到模板
//
// 返回合并后的模板串与包含的行数。
func (p *ParsePipeline) MergeContextLines(lines []string) (template string, lineCount int) {
	if len(lines) == 0 {
		return "", 0
	}

	// 过滤空行
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return "", 0
	}

	// 取首行作为主模板
	mainLine := filtered[0]
	result := p.parser.ParseLine(mainLine)

	// 如果存在异常类型行，附加到模板（最多附加 2 行）
	extraCount := 0
	for _, line := range filtered[1:] {
		if extraCount >= 2 {
			break
		}
		upper := strings.ToUpper(line)
		if strings.Contains(upper, "EXCEPTION") || strings.Contains(upper, "ERROR") {
			// 提取异常类型（取第一个空格前的词）
			parts := strings.Fields(line)
			if len(parts) > 0 {
				result.Template = result.Template + " " + parts[0]
				extraCount++
			}
		}
	}

	return result.Template, len(filtered)
}

// TemplateCount 返回当前已知模板数量。
func (p *ParsePipeline) TemplateCount() int {
	return p.parser.TemplateCount()
}

// AllTemplates 返回全部已知模板副本。
func (p *ParsePipeline) AllTemplates() []parser.Template {
	return p.parser.AllTemplates()
}

// 确保 sync 包被引用（ProcessBatch 内部使用 WaitGroup 由 parser 包处理）。
var _ = sync.WaitGroup{}
