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

	// ArgT and T are filled in by the checker. Polymorphic builtins —
	// contains, push, join — need the argument types at codegen time to
	// decide what to emit.
	ArgT []*Type
	T    *Type
}

// ListLit is `[1, 2, 3]`. An empty literal carries no element type of
// its own, so it needs an annotation on the binding.
type ListLit struct {
	pos
	Elems []Expr
	T     *Type // the list type, filled in by the checker
}

// MapLit is `{"a": 1, "b": 2}`.
type MapLit struct {
	pos
	Keys []Expr
	Vals []Expr
	T    *Type // the map type, filled in by the checker
}

// Field is `a.b`. Today it only ever forms a dotted builtin path such
// as os.file.read; when structs arrive it becomes field access too.
type Field struct {
	pos
	X    Expr
	Name string
}

// DottedName flattens a chain of Field nodes rooted at an Ident into
// "os.file.read". It reports false for anything else, which is how the
// compiler tells a namespace path from an expression.
func DottedName(e Expr) (string, bool) {
	switch x := e.(type) {
	case *Ident:
		return x.Name, true
	case *Field:
		base, ok := DottedName(x.X)
		if !ok {
			return "", false
		}
		return base + "." + x.Name, true
	}
	return "", false
}

// Index is `xs[i]` for a list or `m[k]` for a map.
type Index struct {
	pos
	X   Expr
	Idx Expr
	T   *Type // the type of X, filled in by the checker
}

// InterpPart is one chunk of an interpolated string: either raw text
// (X == nil) or an embedded expression.
type InterpPart struct {
	Lit string
	X   Expr
	T   *Type // the expression's type, filled in by the checker
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
func (*Field) exprNode()    {}
func (*ListLit) exprNode()  {}
func (*MapLit) exprNode()   {}
func (*Index) exprNode()    {}

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

// AssignStmt covers `x = v`, `x += v`, and `xs[i] = v`. Target is an
// *Ident or an *Index; nothing else is assignable.
type AssignStmt struct {
	pos
	Target Expr
	Op     Kind // ASSIGN, PLUSEQ, MINUSEQ, STAREQ, SLASHEQ
	Value  Expr
}

// TargetName returns the variable at the root of an assignment target,
// which is what the resolver needs for its const and scope checks.
// `xs[0][1] = v` roots at "xs".
func (a *AssignStmt) TargetName() string {
	e := a.Target
	for {
		switch t := e.(type) {
		case *Ident:
			return t.Name
		case *Index:
			e = t.X
		default:
			return ""
		}
	}
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

// ForStmt is either a counted loop over a range —
//
//	for i in start..end step n
//
// — or a loop over a collection:
//
//	for x in list
//	for k, v in map
//
// Coll distinguishes them: nil means the range form.
type ForStmt struct {
	pos
	Var  string
	Var2 string // the value variable in `for k, v in map`; "" otherwise
	Body *Block

	// range form
	Start     Expr
	End       Expr
	Step      Expr // nil means 1
	Inclusive bool

	// collection form
	Coll  Expr
	CollT *Type // the collection's type, filled in by the checker
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
