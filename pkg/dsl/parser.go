package dsl

import (
	"strconv"
	"strings"
)

// parser DSL 递归下降语法分析器。
//
// 语法规则（优先级从低到高）：
//
//	expr    → andExpr ("OR" andExpr)*
//	andExpr → notExpr ("AND" notExpr)*
//	notExpr → "NOT" notExpr | primary
//	primary → "(" expr ")" | fieldExpr | keyword
//	fieldExpr → IDENT ":" value
//	           | IDENT ":" (">"|"<"|">="|"<=") NUMBER   // 如 duration:>100
//	           | IDENT (">"|"<"|">="|"<=") NUMBER        // 裸比较
//	keyword → IDENT | NUMBER | STRING
type parser struct {
	tokens []Token
	pos    int
}

// parse 将 Token 序列解析为 AST 根节点。
func parse(tokens []Token) (Expr, error) {
	if len(tokens) == 0 {
		return nil, ErrParseEOF
	}
	p := &parser{tokens: tokens}
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if !p.atEOF() {
		return nil, ErrParseSyntax
	}
	return expr, nil
}

// current 返回当前 Token。
func (p *parser) current() Token {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return Token{Type: TokenEOF}
}

// atEOF 检查是否已消费到 EOF。
func (p *parser) atEOF() bool {
	return p.current().Type == TokenEOF
}

// advance 前进到下一个 Token 并返回当前 Token。
func (p *parser) advance() Token {
	tok := p.current()
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return tok
}

// parseOr 解析 OR 表达式（最低优先级）。
func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.current().Type == TokenOr {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &OrExpr{Left: left, Right: right}
	}
	return left, nil
}

// parseAnd 解析 AND 表达式。
func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.current().Type == TokenAnd {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &AndExpr{Left: left, Right: right}
	}
	return left, nil
}

// parseNot 解析 NOT 表达式。
func (p *parser) parseNot() (Expr, error) {
	if p.current().Type == TokenNot {
		p.advance()
		inner, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &NotExpr{Inner: inner}, nil
	}
	return p.parsePrimary()
}

// parsePrimary 解析括号分组、字段表达式或裸词。
func (p *parser) parsePrimary() (Expr, error) {
	tok := p.current()
	switch tok.Type {
	case TokenLParen:
		p.advance()
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.current().Type != TokenRParen {
			return nil, ErrParseSyntax
		}
		p.advance()
		return &Group{Inner: expr}, nil
	case TokenString:
		p.advance()
		return &Keyword{Value: stripQuotes(tok.Value)}, nil
	case TokenIdent:
		return p.tryParseFieldExpr()
	case TokenNumber:
		p.advance()
		return &Keyword{Value: tok.Value}, nil
	default:
		return nil, ErrParseSyntax
	}
}

// tryParseFieldExpr 尝试解析字段表达式或退化为裸词 Keyword。
func (p *parser) tryParseFieldExpr() (Expr, error) {
	tok := p.current()
	field := tok.Value

	// 检查是否整体包含冒号（某些 lexer 输出会将其合并为 IDENT）
	if strings.Contains(field, ":") {
		parts := strings.SplitN(field, ":", 2)
		fname := strings.TrimSpace(parts[0])
		fval := strings.TrimSpace(parts[1])
		p.advance()
		return p.classifyField(fname, fval), nil
	}

	// 纯通配符（不含冒号的 *xxx*）
	if strings.Contains(field, "*") {
		p.advance()
		return &FieldLike{Field: "*", Value: field}, nil
	}

	// 查看下一个 token 决定路径
	if p.pos+1 >= len(p.tokens) {
		p.advance()
		return &Keyword{Value: field}, nil
	}

	next := p.tokens[p.pos+1]
	switch next.Type {
	case TokenColon:
		return p.parseAfterColon(field)
	case TokenGt, TokenLt, TokenGte, TokenLte:
		return p.parseAfterComparison(field)
	default:
		p.advance()
		return &Keyword{Value: field}, nil
	}
}

// parseAfterColon 处理 "field:" 后跟 value 或比较运算符的情况。
func (p *parser) parseAfterColon(field string) (Expr, error) {
	p.advance() // 消费 ident
	p.advance() // 消费 ':'

	// 形如 duration:>100 → 冒号后跟比较操作符
	switch p.current().Type {
	case TokenGt, TokenLt, TokenGte, TokenLte:
		opTok := p.advance()
		numTok := p.current()
		if numTok.Type != TokenNumber {
			return nil, ErrParseSyntax
		}
		p.advance()
		return makeFieldCmp(field, opTok.Type, numTok.Value)
	}

	// 形如 level:ERROR 或 path:/api/*
	valueTok := p.current()
	var value string
	switch valueTok.Type {
	case TokenIdent:
		value = valueTok.Value
	case TokenNumber:
		value = valueTok.Value
	case TokenString:
		value = stripQuotes(valueTok.Value)
	default:
		return nil, ErrParseEOF
	}
	p.advance()
	return p.classifyField(field, value), nil
}

// parseAfterComparison 处理裸比较，如 status>=500（无冒号）。
func (p *parser) parseAfterComparison(field string) (Expr, error) {
	p.advance()               // 消费 ident
	opTok := p.advance()      // 消费比较操作符
	numTok := p.current()
	if numTok.Type != TokenNumber {
		return nil, ErrParseSyntax
	}
	p.advance()
	return makeFieldCmp(field, opTok.Type, numTok.Value)
}

// makeFieldCmp 构建 FieldCmp 节点的便捷函数。
func makeFieldCmp(field string, opType TokenType, numStr string) (Expr, error) {
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return nil, ErrParseSyntax
	}
	return &FieldCmp{Field: field, Op: OpFromToken(opType), Value: num}, nil
}

// classifyField 根据值形式区分为等值、通配或裸词。
func (p *parser) classifyField(field, value string) Expr {
	if strings.Contains(value, "*") {
		return &FieldLike{Field: field, Value: value}
	}
	if value == "" {
		// 形如 "level:" 无值 → 当作裸词 field
		return &Keyword{Value: field}
	}
	return &FieldEq{Field: field, Value: value}
}

// OpFromToken 从 Token 类型映射比较操作符字符串。
func OpFromToken(t TokenType) string {
	switch t {
	case TokenGt:
		return ">"
	case TokenLt:
		return "<"
	case TokenGte:
		return ">="
	case TokenLte:
		return "<="
	default:
		return "="
	}
}

// stripQuotes 去除字符串首尾引号。
func stripQuotes(s string) string {
	if len(s) >= 2 {
		c := s[0]
		if c == '"' || c == '\'' {
			return s[1 : len(s)-1]
		}
	}
	return s
}
