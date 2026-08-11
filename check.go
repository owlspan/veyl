package main

import "fmt"

// Checker is the fourth pipeline stage, between resolve and codegen. The
// resolver answers "does this name exist"; the checker answers "does this
// expression make sense", and records the answer on the AST so codegen
// can emit explicit Go types instead of leaning on Go's inference.
//
// Like every other stage it accumulates errors rather than aborting, and
// every error names Quartz types (str, float) and never Go ones.
type Checker struct {
	file   string
	funcs  map[string]*FnDecl
	scopes []map[string]*Type
	curFn  *FnDecl
	Errors []string
}

func NewChecker(file string) *Checker {
	return &Checker{file: file, funcs: map[string]*FnDecl{}}
}

func (c *Checker) errorAt(n Node, format string, args ...any) {
	line, col := n.Pos()
	c.Errors = append(c.Errors,
		fmt.Sprintf("%s:%d:%d: %s", c.file, line, col, fmt.Sprintf(format, args...)))
}

// ---- scopes ----

func (c *Checker) push() { c.scopes = append(c.scopes, map[string]*Type{}) }
func (c *Checker) pop()  { c.scopes = c.scopes[:len(c.scopes)-1] }

func (c *Checker) define(name string, t *Type) {
	c.scopes[len(c.scopes)-1][name] = t
}

func (c *Checker) lookup(name string) *Type {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if t, ok := c.scopes[i][name]; ok {
			return t
		}
	}
	return nil
}

// resolveAnnotation turns a written `: type` into a Type, reporting a
// clear error if it names something that does not exist.
func (c *Checker) resolveAnnotation(text string, n Node) *Type {
	if text == "" {
		return nil
	}
	if t := ParseType(text); t != nil {
		return t
	}
	c.errorAt(n, "unknown type %q", text)
	return Unknown
}

// ---- entry point ----

func (c *Checker) Check(p *Program) {
	// Pass 1: resolve every signature before checking any body, so calls
	// to functions declared later in the file type-check correctly.
	for _, f := range p.Funcs {
		for i := range f.Params {
			prm := &f.Params[i]
			prm.T = c.resolveAnnotation(prm.Type, prm)
			if prm.T == nil {
				prm.T = Unknown
			}
		}
		if f.Ret == "" {
			f.RetT = Void
		} else {
			f.RetT = c.resolveAnnotation(f.Ret, f)
		}
		c.funcs[f.Name] = f
	}

	// Pass 2: each function body, in its own scope.
	for _, f := range p.Funcs {
		c.checkFn(f)
	}

	// Pass 3: the top-level statements, which become main().
	c.curFn = nil
	c.push()
	for _, s := range p.Main {
		c.stmt(s)
	}
	c.pop()
}

func (c *Checker) checkFn(f *FnDecl) {
	prev := c.curFn
	c.curFn = f
	c.push()
	for _, prm := range f.Params {
		c.define(prm.Name, prm.T)
	}
	for _, s := range f.Body.Stmts {
		c.stmt(s)
	}
	c.pop()
	c.curFn = prev
}

// ---- statements ----

func (c *Checker) stmt(s Stmt) {
	switch st := s.(type) {

	case *LetStmt:
		valT := c.expr(st.Value)
		annot := c.resolveAnnotation(st.Type, st)

		switch {
		case annot == nil:
			// No annotation: infer, but reject types that carry no value.
			if valT.Kind == KVoid {
				c.errorAt(st, "cannot assign the result of a call that returns nothing to %q", st.Name)
				valT = Unknown
			}
			st.T = valT

		case annot.Accepts(valT), isUntypedInt(st.Value) && annot.Kind == KFloat:
			// The second case is Go's untyped-constant rule: a plain
			// integer literal is happy to become a float.
			st.T = annot

		default:
			c.errorAt(st, "%q is declared as %s but the value is %s", st.Name, annot, valT)
			st.T = annot
		}
		c.define(st.Name, st.T)

	case *AssignStmt:
		valT := c.expr(st.Value)
		want := c.lookup(st.Name)
		if want == nil {
			return // the resolver already reported the undefined name
		}
		if st.Op != ASSIGN {
			c.checkCompound(st, want, valT)
			return
		}
		if !want.Accepts(valT) && !(isUntypedInt(st.Value) && want.Kind == KFloat) {
			c.errorAt(st, "cannot assign %s to %q, which is %s", valT, st.Name, want)
		}

	case *ExprStmt:
		c.expr(st.X)

	case *IfStmt:
		c.condition(st.Cond, "an if")
		c.block(st.Then)
		if st.Else != nil {
			c.stmt(st.Else)
		}

	case *WhileStmt:
		c.condition(st.Cond, "a while")
		c.block(st.Body)

	case *ForStmt:
		startT := c.expr(st.Start)
		endT := c.expr(st.End)
		for _, pair := range []struct {
			t *Type
			n Node
		}{{startT, st.Start}, {endT, st.End}} {
			if !pair.t.IsUnknown() && pair.t.Kind != KInt {
				c.errorAt(pair.n, "a for-loop range must be int, got %s", pair.t)
			}
		}
		if st.Step != nil {
			if stepT := c.expr(st.Step); !stepT.IsUnknown() && stepT.Kind != KInt {
				c.errorAt(st.Step, "a for-loop step must be int, got %s", stepT)
			}
		}
		c.push()
		c.define(st.Var, Int)
		for _, s := range st.Body.Stmts {
			c.stmt(s)
		}
		c.pop()

	case *ReturnStmt:
		if c.curFn == nil {
			return // the resolver already reported this
		}
		want := c.curFn.RetT
		if st.Value == nil {
			return // resolver already checked that a value is present when needed
		}
		got := c.expr(st.Value)
		if want.Kind == KVoid {
			return // resolver already reported the mismatch
		}
		if !want.Accepts(got) && !(isUntypedInt(st.Value) && want.Kind == KFloat) {
			c.errorAt(st, "function %q returns %s but this returns %s",
				c.curFn.Name, want, got)
		}

	case *Block:
		c.block(st)
	}
}

func (c *Checker) block(b *Block) {
	c.push()
	for _, s := range b.Stmts {
		c.stmt(s)
	}
	c.pop()
}

// condition enforces that if/while take a real bool. This is the check
// that turns `if 5 { }` — legal in C, a silent bug everywhere — into a
// compile error.
func (c *Checker) condition(e Expr, kw string) {
	t := c.expr(e)
	if !t.IsUnknown() && t.Kind != KBool {
		c.errorAt(e, "%s condition must be bool, got %s", kw, t)
	}
}

// checkCompound validates `+=` and friends, which are just the binary
// operator followed by an assignment.
func (c *Checker) checkCompound(st *AssignStmt, want, got *Type) {
	op := map[Kind]Kind{PLUSEQ: PLUS, MINUSEQ: MINUS, STAREQ: STAR, SLASHEQ: SLASH}[st.Op]

	if op == PLUS && want.Kind == KStr {
		if got.Kind != KStr && !got.IsUnknown() {
			c.errorAt(st, "cannot append %s to %q, which is str", got, st.Name)
		}
		return
	}
	if !want.IsNumeric() && !want.IsUnknown() {
		c.errorAt(st, "%s needs a number, but %q is %s", goAssignOp(st.Op), st.Name, want)
		return
	}
	if !got.IsNumeric() && !got.IsUnknown() {
		c.errorAt(st, "%s needs a number, got %s", goAssignOp(st.Op), got)
		return
	}
	// An untyped integer literal adapts to a float target, as in Go.
	if want.Kind == KFloat && got.Kind == KInt && !isUntypedInt(st.Value) {
		c.errorAt(st, "cannot apply %s with an int to %q, which is float (use float(...))",
			goAssignOp(st.Op), st.Name)
	}
	if want.Kind == KInt && got.Kind == KFloat {
		c.errorAt(st, "cannot apply %s with a float to %q, which is int (use int(...))",
			goAssignOp(st.Op), st.Name)
	}
}

// ---- expressions ----

// expr returns the type of an expression, reporting any mismatch inside
// it. It never returns nil: unknown stands in for "already reported".
func (c *Checker) expr(e Expr) *Type {
	switch x := e.(type) {

	case *IntLit:
		return Int

	case *FloatLit:
		return Float

	case *StrLit:
		return Str

	case *BoolLit:
		return Bool

	case *Interp:
		for _, p := range x.Parts {
			if p.X != nil {
				c.expr(p.X)
			}
		}
		return Str

	case *Ident:
		if t := c.lookup(x.Name); t != nil {
			return t
		}
		if bc, ok := builtinConsts[x.Name]; ok {
			return bc.typ
		}
		return Unknown // the resolver reported the undefined name

	case *Unary:
		t := c.expr(x.X)
		if t.IsUnknown() {
			return Unknown
		}
		if x.Op == BANG {
			if t.Kind != KBool {
				c.errorAt(x, "'!' needs a bool, got %s", t)
				return Unknown
			}
			return Bool
		}
		if !t.IsNumeric() {
			c.errorAt(x, "'-' needs a number, got %s", t)
			return Unknown
		}
		return t

	case *Binary:
		t := c.binary(x)
		x.T = t
		return t

	case *Call:
		return c.call(x)
	}
	return Unknown
}

func (c *Checker) binary(x *Binary) *Type {
	lt := c.expr(x.L)
	rt := c.expr(x.R)

	if lt.IsUnknown() || rt.IsUnknown() {
		return Unknown
	}

	// Untyped integer literals adapt to a float operand, exactly as Go's
	// untyped constants do. This keeps `radius * 2` working without
	// opening the door to implicit conversion between two variables.
	//
	// The originals are kept for error messages: after adaptation both
	// sides of `7 % 2.0` look like floats, and reporting that would send
	// the reader hunting for a float they never wrote.
	origL, origR := lt, rt
	if lt.Kind == KInt && rt.Kind == KFloat && isUntypedInt(x.L) {
		lt = Float
	}
	if rt.Kind == KInt && lt.Kind == KFloat && isUntypedInt(x.R) {
		rt = Float
	}

	switch x.Op {

	case AND, OR:
		if lt.Kind != KBool || rt.Kind != KBool {
			c.errorAt(x, "'%s' needs bool on both sides, got %s and %s",
				goBinOp(x.Op), lt, rt)
			return Unknown
		}
		return Bool

	case EQ, NEQ:
		if !lt.Equal(rt) {
			c.errorAt(x, "cannot compare %s with %s", lt, rt)
			return Unknown
		}
		if lt.IsCollection() {
			c.errorAt(x, "cannot compare two values of type %s", lt)
			return Unknown
		}
		return Bool

	case LT, LTE, GT, GTE:
		if !lt.Equal(rt) {
			c.errorAt(x, "cannot compare %s with %s", lt, rt)
			return Unknown
		}
		if !lt.IsNumeric() && lt.Kind != KStr {
			c.errorAt(x, "'%s' needs numbers or strings, got %s", goBinOp(x.Op), lt)
			return Unknown
		}
		return Bool

	case PLUS:
		if lt.Kind == KStr && rt.Kind == KStr {
			return Str
		}
		if lt.Kind == KStr || rt.Kind == KStr {
			c.errorAt(x, "cannot add %s and %s (use \"{...}\" interpolation or str(...))", lt, rt)
			return Unknown
		}
		return c.arithmetic(x, lt, rt)

	case MINUS, STAR, SLASH:
		return c.arithmetic(x, lt, rt)

	case PERCENT:
		// Checked against the originals: `%` does no float promotion, so
		// an untyped literal stays an int here.
		if origL.Kind != KInt || origR.Kind != KInt {
			c.errorAt(x, "'%%' needs two ints, got %s and %s (use mod(...) for floats)",
				origL, origR)
			return Unknown
		}
		return Int
	}

	return Unknown
}

// arithmetic enforces the no-implicit-conversion rule for `- * / +`.
func (c *Checker) arithmetic(x *Binary, lt, rt *Type) *Type {
	if !lt.IsNumeric() || !rt.IsNumeric() {
		c.errorAt(x, "'%s' needs numbers, got %s and %s", goBinOp(x.Op), lt, rt)
		return Unknown
	}
	if !lt.Equal(rt) {
		c.errorAt(x, "cannot mix %s and %s in '%s' — convert one with int(...) or float(...)",
			lt, rt, goBinOp(x.Op))
		return Unknown
	}
	return lt
}

// ---- calls ----

func (c *Checker) call(x *Call) *Type {
	id, ok := x.Callee.(*Ident)
	if !ok {
		return Unknown
	}

	args := make([]*Type, len(x.Args))
	for i, a := range x.Args {
		args[i] = c.expr(a)
	}

	if b, isBuiltin := builtins[id.Name]; isBuiltin {
		return c.checkBuiltin(x, id.Name, b, args)
	}

	f, isUser := c.funcs[id.Name]
	if !isUser {
		return Unknown // the resolver reported it
	}
	// Arity is the resolver's job; only check the arguments we have.
	n := len(args)
	if n > len(f.Params) {
		n = len(f.Params)
	}
	for i := 0; i < n; i++ {
		want := f.Params[i].T
		if want.Accepts(args[i]) || (isUntypedInt(x.Args[i]) && want.Kind == KFloat) {
			continue
		}
		c.errorAt(x.Args[i], "%s expects %s for %q, got %s",
			id.Name, want, f.Params[i].Name, args[i])
	}
	return f.RetT
}

func (c *Checker) checkBuiltin(x *Call, name string, b builtin, args []*Type) *Type {
	// A builtin with no declared signature is unchecked. Every builtin
	// should have one; this guard keeps an omission from panicking.
	if b.ret == nil && b.retOf == nil {
		return Unknown
	}

	for i, got := range args {
		want := b.rest
		if i < len(b.params) {
			want = b.params[i]
		}
		if want == nil {
			continue // variadic tail with no declared element type
		}
		if want.Accepts(got) || (isUntypedInt(x.Args[i]) && want.Kind == KFloat) {
			continue
		}
		c.errorAt(x.Args[i], "%s expects %s for argument %d, got %s",
			name, want, i+1, got)
	}

	if b.retOf != nil {
		return b.retOf(args)
	}
	return b.ret
}

// ---- untyped constants ----

// isUntypedInt reports whether an expression is built entirely out of
// integer literals, and so — following Go's untyped-constant rule — can
// stand in for a float without an explicit conversion.
//
// `x * 2` is fine when x is a float. `x * y` is not, when y is an int
// variable. That distinction is the whole reason this function exists.
func isUntypedInt(e Expr) bool {
	switch x := e.(type) {
	case *IntLit:
		return true
	case *Unary:
		return x.Op == MINUS && isUntypedInt(x.X)
	case *Binary:
		switch x.Op {
		case PLUS, MINUS, STAR, SLASH, PERCENT:
			return isUntypedInt(x.L) && isUntypedInt(x.R)
		}
	}
	return false
}
