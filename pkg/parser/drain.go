package parser

import (
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// knownTemplate 已学习的日志模板（叶节点上的候选组）。
type knownTemplate struct {
	id       int64
	tokens   []string
	paramIdx []int // 参数位的位置
	count    atomic.Int64
}

// trieNode 前缀树节点，用于按 token 数快速引导搜索。
type trieNode struct {
	children  map[string]*trieNode
	templates []*knownTemplate
}

// Drain 简化版 Drain3 算法：增量式在线模板挖掘。
//
// 数据结构：
//   - 前缀树按 token 数分层
//   - 每个叶节点下挂一个模板组
//   - 新日志按 token 数去对应层，再按相似度匹配/合并
//
// 并发安全：内部互斥锁，不支持并发写。
type Drain struct {
	mu        sync.Mutex
	root      *trieNode
	maxLen    int
	simThresh float64
	id2tmpl   map[int64]*knownTemplate
	nextID    atomic.Int64
}

// NewDrain 创建 Drain 实例。
func NewDrain() *Drain {
	return &Drain{
		root: &trieNode{
			children:  make(map[string]*trieNode),
			templates: nil,
		},
		maxLen:    50,
		simThresh: 0.4,
		id2tmpl:   make(map[int64]*knownTemplate),
	}
}

// isParamToken 判断 token 是否为可变参数位。
// 覆盖：纯数字、数字序列、UUID、IPv4/IPv6、文件路径、十六进制值。
func isParamToken(token string) bool {
	if token == "" {
		return false
	}
	// 纯数字
	allDigit := true
	hexOnly := true
	for _, c :=range token {
		if c < '0' || c > '9' {
			allDigit = false
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == ':' || c == '.') {
			hexOnly = false
		}
	}
	if allDigit {
		return true
	}
	// UUID 格式（含 - 分隔）
	if isUUID(token) {
		return true
	}
	// IPv4 或 IPv6
	if isIP(token) {
		return true
	}
	// 文件路径（以 / 或 \ 开头）
	if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "\\") || strings.HasPrefix(token, "C:") || strings.HasPrefix(token, "D:") {
		return true
	}
	// 十六进制值（如 0x 开头或纯十六进制长串）
	if strings.HasPrefix(strings.ToLower(token), "0x") {
		return true
	}
	if hexOnly && utf8.RuneCountInString(token) >= 8 {
		return true
	}
	// 主机名:port 格式
	if strings.Count(token, ":") >= 1 {
		parts := strings.Split(token, ":")
		allNumericParts := true
		for _, p := range parts {
			if p == "" {
				continue
			}
			for _, c := range p {
				if c < '0' || c > '9' {
					allNumericParts = false
					break
				}
			}
			if !allNumericParts {
				break
			}
		}
		if allNumericParts {
			return true
		}
	}
	return false
}

// isUUID 检查是否为 UUID 格式。
func isUUID(s string) bool {
	// 8-4-4-4-12 格式
	parts := strings.Split(s, "-")
	if len(parts) == 5 {
		lens := []int{8, 4, 4, 4, 12}
		ok := true
		for i, part := range parts {
			if utf8.RuneCountInString(part) != lens[i] {
				ok = false
				break
			}
			for _, c := range part {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
					ok = false
					break
				}
			}
		}
		return ok
	}
	return false
}

// isIP 检查是否为 IPv4 或 IPv6 地址。
func isIP(s string) bool {
	// 简化检查：IPv4 包含 3 个点，IPv6 包含冒号
	if strings.Count(s, ".") == 3 {
		parts := strings.Split(s, ".")
		ok := true
		for _, p := range parts {
			if p == "" {
				ok = false
				break
			}
			for _, c := range p {
				if c < '0' || c > '9' {
					ok = false
					break
				}
			}
		}
		return ok
	}
	if strings.Contains(s, ":") && len(s) >= 7 {
		// IPv6 简化检查
		ok := true
		for _, c := range s {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == ':') {
				ok = false
				break
			}
		}
		return ok
	}
	return false
}

// tokenize 将日志行分词，maxLen 限制最大 token 数。
func (d *Drain) tokenize(line string) []string {
	raw := strings.Fields(line)
	if len(raw) > d.maxLen {
		raw = raw[:d.maxLen]
	}
	return raw
}

// preprocess 去除已知格式的固定前缀（时间戳/级别等），返回剩余 token。
func preprocess(tokens []string) []string {
	if len(tokens) == 0 {
		return tokens
	}
	start := 0
	// 跳过日期: yyyy-MM-dd 或 yyyy/MM/dd
	if len(tokens) > 0 && isDateLike(tokens[start]) {
		start++
		// 可能接着时间 HH:mm:ss[.SSS]
		if start < len(tokens) && isTimeLike(tokens[start]) {
			start++
		}
		// 可能接着时区 Z 或 +0800
		if start < len(tokens) && (tokens[start] == "Z" || strings.HasPrefix(tokens[start], "+") || strings.HasPrefix(tokens[start], "-")) {
			start++
		}
	}
	// 跳过 8 位时间戳 ISO8601 格式 2024-01-01T00:00:00.000000000Z
	if start < len(tokens) && isISOTimestamp(tokens[start]) {
		start++
	}
	// 跳过日志级别
	if start < len(tokens) && isLevelToken(tokens[start]) {
		start++
	}
	// 跳过 线程名 [xxx] 或 (xxx)
	if start < len(tokens && isThreadToken(tokens[start]) {
		start++
	}
	return tokens[start:]
}

// isDateLike 判断是否为日期格式。
func isDateLike(s string) bool {
	// 2024-01-15 或 2024/01/15
	if (strings.Count(s, "-") == 2 || strings.Count(s, "/") == 2) && len(s) >= 10 {
		return true
	}
	return false
}

// isTimeLike 判断是否为时间格式。
func isTimeLike(s string) bool {
	// 10:23:45.123
	return strings.Count(s, ":") >= 2 && len(s) >= 8
}

// isISOTimestamp 判断是否为 ISO8601 时间戳。
func isISOTimestamp(s string) bool {
	return len(s) >= 20 && strings.Contains(s, "T") && (strings.Contains(s, "Z") || strings.Contains(s, "+") || strings.Count(s, "-") >= 3)
}

// isLevelToken 判断是否为日志级别。
func isLevelToken(s string) bool {
	switch strings.ToUpper(s) {
	case "DEBUG", "INFO", "WARN", "WARNING", "ERROR", "FATAL", "TRACE":
		return true
	}
	// 可能带冒号或DOT或结束符
	trimmed := strings.TrimRight(s, ":.l]")
	if isLevelToken(trimmed) {
		return true
	}
	return false
}

// isThreadToken 判断是否为线程名。
func isThreadToken(s string) bool {
	return (strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) ||
		(strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")"))
}

// findNode 根据 token 数前缀找到对应的前缀树节点或创建。
func (d *Drain) findNode(tokens []string) *trieNode {
	node := d.root
	for _, tok := range tokens {
		child, ok := node.children[tok]
		if !ok {
			child = &trieNode{
				children:  make(map[string]*trieNode),
				templates: nil,
			}
			node.children[tok] = child
		}
		node = child
	}
	return node
}

// jaccardSimilarity 计算两个 token 序列的 Jaccard 相似度。
func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	// 位置敏感：只在相同位置不同时视为不匹配
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return 1.0
	}
	match := 0
	for i := 0; i < minLen; i++ {
		if a[i] == b[i] || (isParamToken(a[i]) && isParamToken(b[i])) {
			match++
		}
	}
	return float64(match) / float64(maxLen)
}

// cosineSimilarity 计算 token 级别的余弦相似度（加权匹配）。
func cosineSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	// 使用位置精确匹配
	match := 0
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] == b[i] || (isParamToken(a[i]) && isParamToken(b[i])) {
			match++
		}
	}
	// 余弦相似度
	return float64(match) / (math.Sqrt(float64(len(a))) * math.Sqrt(float64(len(b))))
}

// matchBestTemplate 在模板组中寻找最佳匹配模板。
func (d *Drain) matchBestTemplate(tokens []string, candidates []*knownTemplate) (*knownTemplate, float64) {
	best := (*knownTemplate)(nil)
	bestScore := 0.0
	for _, tmpl := range candidates {
		// 用加权方式结合 cosine 和 jaccard
		cosine := cosineSimilarity(tokens, tmpl.tokens)
		jaccard := jaccardSimilarity(tokens, tmpl.tokens)
		score := 0.6*cosine + 0.4*jaccard
		if score > bestScore {
			bestScore = score
			best = tmpl
		}
	}
	return best, bestScore
}

// extractParams 从 tokens 中提取参数值。
func extractParams(tokens []string) []string {
	var params []string
	for _, tok := range tokens {
		if isParamToken(tok) {
			params = append(params, tok)
		}
	}
	return params
}

// mergeTemplate 将新 token 序列合并到已有模板（公共前缀合并策略）。
// 返回新的模板 token 序列和参数位置。
func mergeTemplate(existing, incoming []string) ([]string, []int) {
	minLen := len(existing)
	if len(incoming) < minLen {
		minLen = len(incoming)
	}
	maxLen := len(existing)
	if len(incoming) > maxLen {
		maxLen = len(incoming)
	}
	result := make([]string, maxLen)
	for i := 0; i < maxLen; i++ {
		var ex, in string
		if i < len(existing) {
			ex = existing[i]
		}
		if i < len(incoming) {
			in = incoming[i]
		}
		if i >= len(existing) {
			// 新序列比旧序列长，这个位置直接从incoming来
			if isParamToken(in) {
				result[i] = "<*>"
			} else {
				result[i] = in
			}
		} else if i >= len(incoming) {
			// 旧序列比新序列长
			if isParamToken(ex) {
				result[i] = "<*>"
			} else {
				result[i] = "<*>"
			}
		} else if ex == in {
			result[i] = ex
		} else {
			// 不同位置 → 参数位
			result[i] = "<*>"
		}
	}
	var paramIdx []int
	for i, tok := range result {
		if tok == "<*>" {
			paramIdx = append(paramIdx, i)
		}
	}
	return result, paramIdx
}

// templateString 将模板 token 序列拼接为模板字符串。
func templateString(tokens []string) string {
	return strings.Join(tokens, " ")
}

// ProcessLine 处理单行日志，返回匹配/创建的模板和提取的参数。
// 这是 Drain 算法的核心入口。
func (d *Drain) ProcessLine(line string) (templateStr string, params []string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	tokens := d.tokenize(line)
	if len(tokens) == 0 {
		return line, nil
	}

	// 预处理：去除固定前缀
	content := preprocess(tokens)
	if len(content) == 0 {
		content = tokens
	}

	// 获取 token 数前缀（第一个 token 作为初始引导）
	prefix := content
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	node := d.findNode(prefix)

	if len(node.templates) == 0 {
		// 没有候选模板，直接创建
		tmpl := d.createTemplate(content)
		node.templates = append(node.templates, tmpl)
		return templateStrImpl(tmpl), extractParams(content)
	}

	// 匹配最佳模板
	best, score := d.matchBestTemplate(content, node.templates)
	if best == nil || score < d.simThresh {
		// 没有达到阈值，创建新模板
		tmpl := d.createTemplate(content)
		node.templates = append(node.templates, tmpl)
		return templateStrImpl(tmpl), extractParams(content)
	}

	// 匹配成功，合并到最佳模板
	mergedTokens, paramIdx := mergeTemplate(best.tokens, content)
	best.tokens = mergedTokens
	best.paramIdx = paramIdx
	best.count.Add(1)
	params = extractParamsAt(content, paramIdx)
	return templateStrImpl(best), params
}

// createTemplate 创建新的已知模板。
func (d *Drain) createTemplate(tokens []string) *knownTemplate {
	id := d.nextID.Add(1)
	tmplTokens := make([]string, len(tokens))
	copy(tmplTokens, tokens)
	var paramIdx []int
	for i, tok := range tmplTokens {
		if isParamToken(tok) {
			tmplTokens[i] = "<*>"
			paramIdx = append(paramIdx, i)
		}
	}
	tmpl := &knownTemplate{
		id:       id,
		tokens:   tmplTokens,
		paramIdx: paramIdx,
	}
	tmpl.count.Add(1)
	d.id2tmpl[id] = tmpl
	return tmpl
}

// templateStrImpl 从模板提取模板字符串。
func templateStrImpl(tmpl *knownTemplate) string {
	return strings.Join(tmpl.tokens, " ")
}

// extractParamsAt 根据参数位置从 tokens 中提取参数值。
func extractParamsAt(tokens []string, idx []int) []string {
	params := make([]string, 0, len(idx))
	for _, i := range idx {
		if i < len(tokens) {
			params = append(params, tokens[i])
		}
	}
	return sortParams(params)
}

// sortParams 保持参数原始顺序（这里直接返回即可）。
func sortParams(params []string) []string {
	return params
}

// AllTemplates 返回 Drain 已知所有模板的拷贝。
func (d *Drain) AllTemplates() []Template {
	d.mu.Lock()
	defer d.mu.Unlock()
	templates := make([]Template, 0, len(d.id2tmpl))
	for _, tmpl := range d.id2tmpl {
		templates = append(templates, Template{
			ID:      tmpl.id,
			Template: templateStrImpl(tmpl),
			Count:   tmpl.count.Load(),
		})
	}
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].ID < templates[j].ID
	})
	return templates
}

// TemplateCount 返回已知模板数量。
func (d *Drain) TemplateCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.id2tmpl)
}
