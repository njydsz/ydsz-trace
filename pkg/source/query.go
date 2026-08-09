// Package source 子模块：布尔查询表达式解析与求值。
//
// 支持语法：
//   - 原子词：word / "quoted word" / field:value / field:"quoted value"
//   - 布尔操作符：AND OR NOT（不区分大小写）
//   - 分组：( )
//   - 简写：相邻原子词视为 AND
//   - 支持的字段：level / ip / traceId（其他 fallback 到任意位置匹配）
//
// 示例：
//
//	ERROR AND timeout
//	level:ERROR AND NOT DEBUG
//	(error OR fatal) AND module:auth
//	traceId:abc123 AND ip:10.0.
package source

import (
	"fmt"
	"strings"
)

// ---- Token ----

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokTerm
	tokAnd
	tokOr
	tokNot
	tokLParen
	tokRParen
)

type token struct {
	kind  tokenKind
	value string // raw term or field:value
	field string // non-empty when term is "field:value"
}

// ---- AST ----

type exprNode interface {
	eval(line string) bool
}

type termNode struct {
	field string
	value string
	isNot bool // 保留扩展用
}

func (n *termNode) eval(line string) bool {
	val := strings.ToLower(n.value)
	if n.field == "" {
		return strings.Contains(strings.ToLower(line), val)
	}
	switch strings.ToLower(n.field) {
	case "level":
		// 精确匹配级别，忽略大小写
		lineLevel := strings.ToLower(extractLogLevel(line))
		return lineLevel == val
	case "ip":
		// 前缀匹配（支持 ip:10.0. 截断）
		return matchIPPrefix(line, n.value)
	case "traceid":
		return strings.Contains(strings.ToLower(line), val)
	case "module", "msg", "message":
		return strings.Contains(strings.ToLower(line), val)
	default:
		// 未知字段 fallback 到任意位置
		return strings.Contains(strings.ToLower(line), val)
	}
}

// matchIPPrefix 在日志行中搜索任意字段以前缀 n.value 开头。
func matchIPPrefix(line, prefix string) bool {
	if prefix == "" {
		return false
	}
	// 快速路径：直接 contains
	return strings.Contains(line, prefix)
}

type notNode struct {
	sub exprNode
}

func (n *notNode) eval(line string) bool {
	return !n.sub.eval(line)
}

type andNode struct {
	left, right exprNode
}

func (n *andNode) eval(line string) bool {
	// 短路
	return n.left.eval(line) && n.right.eval(line)
}

type orNode struct {
	left, right exprNode
}

func (n *orNode) eval(line string) bool {
	return n.left.eval(line) || n.right.eval(line)
}

// ---- Tokenizer ----

type tokenizer struct {
	input string
	pos   int
}

func newTokenizer(input string) *tokenizer {
	return &tokenizer{input: input}
}

func (t *tokenizer) skipWhitespace() {
	for t.pos < len(t.input) && (t.input[t.pos] == ' ' || t.input[t.pos] == '\t' || t.input[t.pos] == '\n' || t.input[t.pos] == '\r') {
		t.pos++
	}
}

func (t *tokenizer) next() (token, error) {
	t.skipWhitespace()
	if t.pos >= len(t.input) {
		return token{kind: tokEOF}, nil
	}

	ch := t.input[t.pos]

	// 括号
	if ch == '(' {
		t.pos++
		return token{kind: tokLParen}, nil
	}
	if ch == ')' {
		t.pos++
		return token{kind: tokRParen}, nil
	}

	// 引号字符串
	if ch == '"' || ch == '\'' {
		quote := ch
		t.pos++
		start := t.pos
		for t.pos < len(t.input) && t.input[t.pos] != quote {
			t.pos++
		}
		if t.pos >= len(t.input) {
			return token{}, fmt.Errorf("未关闭的引号")
		}
		val := t.input[start:t.pos]
		t.pos++
		return token{kind: tokTerm, value: val}, nil
	}

	// 字或单词（读取到空格、括号、引号为止）
	start := t.pos
	for t.pos < len(t.input) {
		c := t.input[t.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '(' || c == ')' || c == '"' || c == '\'' {
			break
		}
		t.pos++
	}
	word := t.input[start:t.pos]
	if word == "" {
		return token{}, fmt.Errorf("在 %d 处遇到空白/括号不足", t.pos)
	}

	upper := strings.ToUpper(word)
	switch upper {
	case "AND":
		return token{kind: tokAnd}, nil
	case "OR":
		return token{kind: tokOr}, nil
	case "NOT":
		return token{kind: tokNot}, nil
	}

	// 检查是否为 field:value
	if idx := strings.Index(word, ":"); idx > 0 && idx < len(word)-1 {
		field := word[:idx]
		value := word[idx+1:]
		return token{kind: tokTerm, value: value, field: field}, nil
	}

	return token{kind: tokTerm, value: word}, nil
}

// ---- Parser (recursive descent) ----
// Grammar:
//   expr   := orExpr
//   orExpr := andExpr (OR andExpr)*
//   andExpr:= unary (AND? unary)*    // 省略 AND 视为 AND
//   unary  := NOT unary | primary
//   primary:= '(' expr ')' | term

type parser struct {
	tok token
	lex *tokenizer
	err error
}

func (p *parser) advance() {
	if p.err != nil {
		return
	}
	t, err := p.lex.next()
	if err != nil {
		p.err = err
		return
	}
	p.tok = t
}

func (p *parser) parse() (exprNode, error) {
	p.advance()
	if p.tok.kind == tokEOF {
		return nil, fmt.Errorf("空查询表达式")
	}
	n := p.parseOr()
	if p.err != nil {
		return nil, p.err
	}
	if p.tok.kind != tokEOF {
		return nil, fmt.Errorf("在 %q 后仍有未解析的 token", p.tok.value)
	}
	return n, nil
}

func (p *parser) parseOr() exprNode {
	n := p.parseAnd()
	for p.tok.kind == tokOr && p.err == nil {
		p.advance()
		r := p.parseAnd()
		n = &orNode{left: n, right: r}
	}
	return n
}

func (p *parser) parseAnd() exprNode {
	n := p.parseUnary()
	for p.err == nil {
		switch p.tok.kind {
		case tokAnd:
			p.advance()
			r := p.parseUnary()
			n = &andNode{left: n, right: r}
		case tokTerm, tokNot, tokLParen:
			// 省略 AND 视为 AND（implicit AND）
			r := p.parseUnary()
			n = &andNode{left: n, right: r}
		default:
			return n
		}
	}
	return n
}

func (p *parser) parseUnary() exprNode {
	if p.tok.kind == tokNot {
		p.advance()
		sub := p.parseUnary()
		return &notNode{sub: sub}
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() exprNode {
	switch p.tok.kind {
	case tokLParen:
		p.advance()
		n := p.parseOr()
		if p.tok.kind != tokRParen {
			p.err = fmt.Errorf("期待 ')'")
			return nil
		}
		p.advance()
		return n
	case tokTerm:
		t := &termNode{value: p.tok.value, field: p.tok.field}
		p.advance()
		return t
	case tokEOF:
		p.err = fmt.Errorf("查询不完整")
		return nil
	default:
		p.err = fmt.Errorf("意外 token: %q", p.tok.value)
		return nil
	}
}

// ---- Public API ----

// CompileQuery 编译布尔查询表达式字符串；返回可重用的求值函数。
// 空字符串返回 nil（表示无查询条件，匹配所有）。
func CompileQuery(q string) (func(line string) bool, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	p := &parser{lex: newTokenizer(q)}
	n, err := p.parse()
	if err != nil {
		return nil, fmt.Errorf("查询表达式解析失败: %w", err)
	}
	return n.eval, nil
}

// MustCompileQuery 编译查询表达式，失败返回"总是为假"的过滤器（保守语义）。
// 便于 CLI/配置层快速接入。
func MustCompileQuery(q string) func(line string) bool {
	fn, err := CompileQuery(q)
	if err != nil {
		// 返回永远 false 的过滤器——即不返回任何匹配
		return func(line string) bool { return false }
	}
	return fn
}

// IsValidQuery 在编译失败时返回错误。
func IsValidQuery(q string) error {
	_, err := CompileQuery(q)
	return err
}
