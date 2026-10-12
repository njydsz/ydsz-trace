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
	paramIdx []int
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
	allDigit := true
	hexOnly := true
	for i := 0; i < len(token); i++ {
		c := token[i]
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
	if isUUID(token) {
		return true
	}
	if isIP(token) {
		return true
	}
	if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "\\") {
		return true
	}
	if len(token) >= 2 && token[0] == 'C' && token[1] == ':' {
		return true
	}
	if len(token) >= 2 && token[0] == 'D' && token[1] == ':' {
		return true
	}
	if strings.HasPrefix(strings.ToLower(token), "0x") {
		return true
	}
	if hexOnly && utf8.RuneCountInString(token) >= 8 {
		return true
	}
	if strings.Count(token, ":") >= 1 {
		parts := strings.Split(token, ":")
		allNumericParts := true
		for _, p := range parts {
			if p == "" {
				continue
			}
			for i := 0; i < len(p); i++ {
				if p[i] < '0' || p[i] > '9' {
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

// isUUID 检查是否为 UUID 格式（8-4-4-4-12）。
func isUUID(s string) bool {
	parts := strings.Split(s, "-")
	if len(parts) != 5 {
		return false
	}
	lens := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if utf8.RuneCountInString(part) != lens[i] {
			return false
		}
		for j := 0; j < len(part); j++ {
			c := part[j]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// isIP 检查是否为 IPv4 或 IPv6 地址。
func isIP(s string) bool {
	if strings.Count(s, ".") == 3 {
		parts := strings.Split(s, ".")
		ok := true
		for _, p := range parts {
			if p == "" {
				ok = false
				break
			}
			for i := 0; i < len(p); i++ {
				if p[i] < '0' || p[i] > '9' {
					ok = false
					break
				}
			}
		}
		return ok
	}
	if strings.Contains(s, ":") && len(s) >= 7 {
		ok := true
		for i := 0; i < len(s); i++ {
			c := s[i]
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
	if len(tokens) > 0 && isDateLike(tokens[start]) {
		start++
		if start < len(tokens) && isTimeLike(tokens[start]) {
			start++
		}
		if start < len(tokens) && (tokens[start] == "Z" || strings.HasPrefix(tokens[start], "+") || strings.HasPrefix(tokens[start], "-")) {
			start++
		}
	}
	if start < len(tokens) && isISOTimestamp(tokens[start]) {
		start++
	}
	if start < len(tokens) && isLevelToken(tokens[start]) {
		start++
	}
	if start < len(tokens) && isThreadToken(tokens[start]) {
		start++
	}
	return tokens[start:]
}

// isDateLike 判断是否为日期格式 yyyy-MM-dd 或 yyyy/MM/dd。
func isDateLike(s string) bool {
	return (strings.Count(s, "-") == 2 || strings.Count(s, "/") == 2) && len(s) >= 10
}

// isTimeLike 判断是否为时间格式 HH:mm:ss.SSS。
func isTimeLike(s string) bool {
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
	trimmed := strings.TrimRight(s, ":.l]")
	if trimmed != s {
		return isLevelToken(trimmed)
	}
	return false
}

// isThreadToken 判断是否为线程名 [xxx] 或 (xxx)。
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

// jaccardSimilarity 计算两个 token 序列的 Jaccard 相似度（位置敏感）。
func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
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

// cosineSimilarity 计算 token 级别的余弦相似度。
func cosineSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
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
	return float64(match) / (math.Sqrt(float64(len(a))) * math.Sqrt(float64(len(b))))
}

// matchBestTemplate 在模板组中寻找最佳匹配模板。
func (d *Drain) matchBestTemplate(tokens []string, candidates []*knownTemplate) (*knownTemplate, float64) {
	var best *knownTemplate
	bestScore := 0.0
	for _, tmpl := range candidates {
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

// mergeTemplate 将新 token 序列合并到已有模板（公共前缀合并策略）。
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
			if isParamToken(in) {
				result[i] = "<*>"
			} else {
				result[i] = in
			}
		} else if i >= len(incoming) {
			if isParamToken(ex) {
				result[i] = "<*>"
			} else {
				result[i] = "<*>"
			}
		} else if ex == in {
			result[i] = ex
		} else {
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

// extractParamsAt 根据参数位置从 tokens 中提取参数值。
func extractParamsAt(tokens []string, idx []int) []string {
	params := make([]string, 0, len(idx))
	for _, i := range idx {
		if i < len(tokens) {
			params = append(params, tokens[i])
		}
	}
	return params
}

// templateStrImpl 从模板提取模板字符串。
func templateStrImpl(tmpl *knownTemplate) string {
	return strings.Join(tmpl.tokens, " ")
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

// ProcessLine 处理单行日志，返回匹配/创建的模板和提取的参数。
func (d *Drain) ProcessLine(line string) (templateStr string, params []string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	tokens := d.tokenize(line)
	if len(tokens) == 0 {
		return line, nil
	}

	content := preprocess(tokens)
	if len(content) == 0 {
		content = tokens
	}

	prefix := content
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	node := d.findNode(prefix)

	if len(node.templates) == 0 {
		tmpl := d.createTemplate(content)
		node.templates = append(node.templates, tmpl)
		return templateStrImpl(tmpl), extractParams(content)
	}

	best, score := d.matchBestTemplate(content, node.templates)
	if best == nil || score < d.simThresh {
		tmpl := d.createTemplate(content)
		node.templates = append(node.templates, tmpl)
		return templateStrImpl(tmpl), extractParams(content)
	}

	mergedTokens, paramIdx := mergeTemplate(best.tokens, content)
	best.tokens = mergedTokens
	best.paramIdx = paramIdx
	best.count.Add(1)
	return templateStrImpl(best), extractParamsAt(content, paramIdx)
}

// findTemplateByContent 根据原始日志行内容查找匹配的已知模板。
func (d *Drain) findTemplateByContent(line string) (*knownTemplate, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	tokens := d.tokenize(line)
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
	node := d.findNode(prefix)
	if len(node.templates) == 0 {
		return nil, false
	}
	best, score := d.matchBestTemplate(content, node.templates)
	if best == nil || score < d.simThresh {
		return nil, false
	}
	return best, true
}

// AllTemplates 返回 Drain 已知所有模板的拷贝。
func (d *Drain) AllTemplates() []Template {
	d.mu.Lock()
	defer d.mu.Unlock()
	templates := make([]Template, 0, len(d.id2tmpl))
	for _, tmpl := range d.id2tmpl {
		templates = append(templates, Template{
			ID:       tmpl.id,
			Template: templateStrImpl(tmpl),
			Count:    tmpl.count.Load(),
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
