package dsl

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// TokenType 词法单元类型。
type TokenType int

// 所有 Token 类型常量。
const (
	TokenError TokenType = iota
	TokenEOF
	TokenIdent   // 标识符或关键字
	TokenNumber  // 数字
	TokenString  // 带引号字符串
	TokenColon   // :
	TokenLParen  // (
	TokenRParen  // )
	TokenAnd     // AND
	TokenOr      // OR
	TokenNot     // NOT
	TokenPipe    // |
	TokenStar    // *
	TokenSlash   // /
	TokenGt      // >
	TokenLt      // <
	TokenGte     // >=
	TokenLte     // <=
)

// Token 单个词法单元。
type Token struct {
	Type  TokenType
	Value string
}

// 关键字映射表。
var keywords = map[string]TokenType{
	"AND": TokenAnd,
	"OR":  TokenOr,
	"NOT": TokenNot,
}

// lexer DSL 词法分析器。
type lexer struct {
	input  string
	pos    int
	start  int
	tokens []Token
}

// tokenize 将 DSL 字符串切片为 Token 序列，末尾追加 TokenEOF。
func tokenize(input string) []Token {
	l := &lexer{input: input}
	l.run()
	l.tokens = append(l.tokens, Token{Type: TokenEOF})
	return l.tokens
}

// run 驱动整个词法扫描过程。
func (l *lexer) run() {
	for l.pos < len(l.input) {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			break
		}
		l.start = l.pos
		c := l.next()
		switch c {
		case ':':
			l.emit(TokenColon)
		case '(':
			l.emit(TokenLParen)
		case ')':
			l.emit(TokenRParen)
		case '|':
			l.emit(TokenPipe)
		case '*':
			l.emit(TokenStar)
		case '/':
			// 如果前一个 token 是冒号，说明跟在 field: 后面，视为路径值的一部分
			if l.lastTokenType() == TokenColon {
				l.readPathValue()
			} else {
				l.emit(TokenSlash)
			}
		case '>':
			if l.peek() == '=' {
				l.next()
				l.emit(TokenGte)
			} else {
				l.emit(TokenGt)
			}
		case '<':
			if l.peek() == '=' {
				l.next()
				l.emit(TokenLte)
			} else {
				l.emit(TokenLt)
			}
		case '-', '+':
			if isDigit(l.peek()) {
				l.readNumber()
			} else {
				l.emit(TokenIdent)
			}
		case '"', '\'':
			l.readString(c)
		default:
			if isDigit(c) {
				l.readNumber()
			} else if isIdentStart(c) {
				l.readIdent()
			} else {
				l.emit(TokenIdent)
			}
		}
	}
}

// lastTokenType 返回已发射的最后一个 Token 的类型，无记录时为 TokenError。
func (l *lexer) lastTokenType() TokenType {
	if len(l.tokens) > 0 {
		return l.tokens[len(l.tokens)-1].Type
	}
	return TokenError
}

// next 返回当前字符并前进笔指针。
func (l *lexer) next() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	r, size := utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += size
	return r
}

// peek 返回当前字符但不前进。
func (l *lexer) peek() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.pos:])
	return r
}

// emit 将当前 [start, pos) 切片为指定类型的 Token。
func (l *lexer) emit(t TokenType) {
	l.tokens = append(l.tokens, Token{Type: t, Value: l.input[l.start:l.pos]})
}

// skipWhitespace 跳过空白字符。
func (l *lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		r, size := utf8.DecodeRuneInString(l.input[l.pos:])
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			l.pos += size
		} else {
			break
		}
	}
}

// readNumber 读取整数或浮点数。
func (l *lexer) readNumber() {
	for l.pos < len(l.input) && isDigit(l.peek()) {
		l.next()
	}
	if l.peek() == '.' {
		l.next()
		for l.pos < len(l.input) && isDigit(l.peek()) {
			l.next()
		}
	}
	l.emit(TokenNumber)
}

// readIdent 读取标识符或保留字。
func (l *lexer) readIdent() {
	for l.pos < len(l.input) {
		r := l.peek()
		if isIdentContinue(r) || r == '.' {
			l.next()
		} else {
			break
		}
	}
	word := l.input[l.start:l.pos]
	upper := strings.ToUpper(word)
	if kw, ok := keywords[upper]; ok {
		l.tokens = append(l.tokens, Token{Type: kw, Value: word})
	} else {
		l.emit(TokenIdent)
	}
}

// readPathValue 读取紧跟在冒号后的路径值（如 /api/*）。
// 读取到空白字符或已知运算符为止，保留路径形态不分割。
func (l *lexer) readPathValue() {
	for l.pos < len(l.input) {
		r := l.peek()
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			break
		}
		if r == '|' || r == '(' || r == ')' || r == '"' || r == '\'' {
			break
		}
		l.next()
	}
	l.emit(TokenIdent)
}

// readString 读取引号包围的字符串，支持简单转义。
func (l *lexer) readString(quote rune) {
	for {
		if l.pos >= len(l.input) {
			l.emit(TokenString)
			return
		}
		r := l.next()
		if r == quote {
			l.emit(TokenString)
			return
		}
		if r == '\\' && l.pos < len(l.input) {
			l.next()
		}
	}
}

// isIdentStart 判断字符是否可做标识符首字符。
func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

// isIdentContinue 判断字符是否可做标识符后续字符。
func isIdentContinue(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}

// isDigit 判断字符是否为数字（0-9）。
func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}
