package dsl

import (
	"testing"
)

// TestLexer_TokenCount 验证基本 Tokenization 产生正确数量的 Token。
func TestLexer_TokenCount(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // 期望的 token 数（含 EOF）
	}{
		{"empty", "", 1},
		{"single_word", "login", 2},
		{"field_eq", "level:ERROR", 4},          // level, :, ERROR, EOF
		{"and_expr", "ERROR AND login", 4},       // ERROR, AND, login, EOF (keyword "login" stays as 1 token)
		{"comparison", "duration:>100", 5},       // duration, :, >, 100, EOF
		{"grouped", "(ERROR OR WARN)", 6},        // (, ERROR, OR, WARN, ), EOF
		{"with_not", "NOT debug", 3},             // NOT, debug, EOF
		{"path_value", "path:/api/*", 4},          // path, :, /api/*, EOF
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenize(tc.input)
			if len(got) != tc.want {
				tokens := make([]string, 0, len(got))
				for _, tok := range got {
					tokens = append(tokens, tokenLabel(tok))
				}
				t.Errorf("tokenize(%q) produced %d tokens = %v, want %d", tc.input, len(got), tokens, tc.want)
			}
		})
	}
}

// TestLexer_Keywords 验证 AND/OR/NOT 被正确识别为关键字 Token。
func TestLexer_Keywords(t *testing.T) {
	tokens := tokenize("x AND y OR z NOT w")
	expected := []TokenType{TokenIdent, TokenAnd, TokenIdent, TokenOr, TokenIdent, TokenNot, TokenIdent, TokenEOF}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(expected))
	}
	for i, exp := range expected {
		if tokens[i].Type != exp {
			t.Errorf("token[%d] type = %v, want %v (value=%q)", i, tokens[i].Type, exp, tokens[i].Value)
		}
	}
}

// TestLexer_Operators 验证比较操作符被正确拆解。
func TestLexer_Operators(t *testing.T) {
	tokens := tokenize("> >= < <=")
	expected := []TokenType{TokenGt, TokenGte, TokenLt, TokenLte, TokenEOF}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(expected))
	}
	for i, exp := range expected {
		if tokens[i].Type != exp {
			t.Errorf("token[%d] = %v, want %v", i, tokens[i].Type, exp)
		}
	}
}

// TestLexer_Parens 验证括号独立为 Token。
func TestLexer_Parens(t *testing.T) {
	tokens := tokenize("(login)")
	expected := []TokenType{TokenLParen, TokenIdent, TokenRParen, TokenEOF}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(expected))
	}
	for i, exp := range expected {
		if tokens[i].Type != exp {
			t.Errorf("token[%d] = %v, want %v", i, tokens[i].Type, exp)
		}
	}
}

// TestLexer_PathValue 验证 /api/* 在冒号后被合并为单个 IdentToken。
func TestLexer_PathValue(t *testing.T) {
	tokens := tokenize("path:/api/*")
	expect := map[int]TokenType{
		0: TokenIdent,  // path
		1: TokenColon,  // :
		2: TokenIdent,  // /api/*
		3: TokenEOF,    // EOF
	}
	if len(tokens) != len(expect) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(expect))
	}
	for i, exp := range expect {
		if tokens[i].Type != exp {
			t.Errorf("token[%d] type=%v value=%q, want %v", i, tokens[i].Type, tokens[i].Value, exp)
		}
	}
	if tokens[2].Value != "/api/*" {
		t.Errorf("path value = %q, want %q", tokens[2].Value, "/api/*")
	}
}

// tokenLabel 返回 Token 类型的简短描述（调试用）。
func tokenLabel(tok Token) string {
	names := map[TokenType]string{
		TokenError:  "ERR",
		TokenEOF:    "EOF",
		TokenIdent:  "IDT",
		TokenNumber: "NUM",
		TokenString: "STR",
		TokenColon:  "COL",
		TokenLParen: "LP",
		TokenRParen: "RP",
		TokenAnd:    "AND",
		TokenOr:     "OR",
		TokenNot:    "NOT",
		TokenPipe:   "PIP",
		TokenStar:   "STR",
		TokenSlash:  "SLH",
		TokenGt:     "GT",
		TokenLt:     "LT",
		TokenGte:    "GTE",
		TokenLte:    "LTE",
	}
	if n, ok := names[tok.Type]; ok {
		return n + "(" + tok.Value + ")"
	}
	return "?"
}
