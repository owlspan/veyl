package main

// Every node knows where it came from. This is what makes error
// messages and //line directives possible.
type Node interface {
	Pos() (line, col int)
}

type pos struct {
	Line int
	Col  int
}

func (p pos) Pos() (int, int) { return p.Line, p.Col }

func at(t Token) pos { return pos{Line: t.Line, Col: t.Col} }

type Expr interface {
	Node
	exprNode()
}

type Stmt interface {
	Node
	stmtNode()
}

// Program is a whole .qz file: zero or more functions, plus the
// top-level statements that become main().
type Program struct {
	Funcs []*FnDecl
	Main  []Stmt
}

// ---- expressions ----

type IntLit struct {
	pos
	Val string // kept as text; Go parses it identically
}

type FloatLit struct {
	pos
	Val string
}

type StrLit struct {
	pos
	Val string // already decoded by the lexer
}

type BoolLit struct {
	pos
	Val bool
}

type Ident struct {
	pos
	Name string
}

type Unary struct {
	pos
	Op Kind // BANG or MINUS
	X  Expr
}

type Binary struct {
	pos
	Op   Kind
	L, R Expr

	// T is the result type, filled in by the checker. Codegen reads it to
	// tell integer division from float division.
	T *Type
}

type Call struct {
	pos
	Callee Expr
	Args   []Expr
}

// InterpPart is one chunk of an interpolated string: either raw text
// (X == nil) or an embedded expression.
type InterpPart struct {
	Lit string
	X   Expr
}

// Interp is a string literal containing {expr} holes.
type Interp struct {
	pos
	Parts []InterpPart
}

func (*IntLit) exprNode()   {}
func (*FloatLit) exprNode() {}
func (*StrLit) exprNode()   {}
func (*BoolLit) exprNode()  {}
func (*Ident) exprNode()    {}
func (*Unary) exprNode()    {}
func (*Binary) exprNode()   {}
func (*Call) exprNode()     {}
func (*Interp) exprNode()   {}

// ---- declarations ----

type Param struct {
	pos
	Name string
	Type string // as written in the source
	T    *Type  // resolved by the checker
}

type FnDecl struct {
	pos
	Name   string
	Params []Param
	Ret    string // "" means the function returns nothing
	RetT   *Type  // resolved by the checker; Void when Ret is ""
	Body   *Block
}

// ---- statements ----

type Block struct {
	pos
	Stmts []Stmt
}

type LetStmt struct {
	pos
	Name  string
	Const bool
	Type  string // as written; "" when inferred
	T     *Type  // resolved by the checker, whether written or inferred
	Value Expr

	// Used is filled in by the resolver. Go rejects unused locals, so
	// codegen emits a `_ = x` discard only when this is false.
	Used bool
}

type AssignStmt struct {
	pos
	Name  string
	Op    Kind // ASSIGN, PLUSEQ, MINUSEQ, STAREQ, SLASHEQ
	Value Expr
}

type ExprStmt struct {
	pos
	X Expr
}

type IfStmt struct {
	pos
	Cond Expr
	Then *Block
	Else Stmt // *Block, *IfStmt, or nil
}

type WhileStmt struct {
	pos
	Cond Expr
	Body *Block
}

type ReturnStmt struct {
	pos
	Value Expr // nil for a bare `return`
}

// ForStmt is `for i in start..end` — plus an optional `step`, and an
// inclusive form written `..=`.
type ForStmt struct {
	pos
	Var       string
	Start     Expr
	End       Expr
	Step      Expr // nil means 1
	Inclusive bool
	Body      *Block
}

type BreakStmt struct{ pos }

type ContinueStmt struct{ pos }

func (*Block) stmtNode()        {}
func (*LetStmt) stmtNode()      {}
func (*AssignStmt) stmtNode()   {}
func (*ExprStmt) stmtNode()     {}
func (*IfStmt) stmtNode()       {}
func (*WhileStmt) stmtNode()    {}
func (*ReturnStmt) stmtNode()   {}
func (*ForStmt) stmtNode()      {}
func (*BreakStmt) stmtNode()    {}
func (*ContinueStmt) stmtNode() {}
