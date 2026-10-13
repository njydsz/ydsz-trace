package dsl

// Expr 表示 DSL 语法树节点。所有 AST 节点实现此接口。
type Expr interface {
	exprNode()
}

// AndExpr 逻辑与表达式。
type AndExpr struct {
	Left, Right Expr
}

// OrExpr 逻辑或表达式。
type OrExpr struct {
	Left, Right Expr
}

// NotExpr 逻辑非表达式。
type NotExpr struct {
	Inner Expr
}

// FieldEq 字段等值匹配，如 level:ERROR。
type FieldEq struct {
	Field, Value string
}

// FieldLike 字段通配/模糊匹配，如 path:/api/*。
type FieldLike struct {
	Field, Value string
}

// FieldCmp 字段数值比较，如 duration:>100。
type FieldCmp struct {
	Field string
	Op    string // ">", ">=", "<", "<="
	Value float64
}

// Keyword 裸词全文匹配。
type Keyword struct {
	Value string
}

// Group 括号分组。
type Group struct {
	Inner Expr
}

func (AndExpr) exprNode()  {}
func (OrExpr) exprNode()   {}
func (NotExpr) exprNode()  {}
func (FieldEq) exprNode()  {}
func (FieldLike) exprNode() {}
func (FieldCmp) exprNode() {}
func (Keyword) exprNode()  {}
func (Group) exprNode()    {}
