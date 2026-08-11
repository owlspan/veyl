package main

import (
	"fmt"
	"sort"
	"strings"
)

// Checker is the fourth pipeline stage, between resolve and codegen. The
// resolver answers "does this name exist"; the checker answers "does this
// expression make sense", and records the answer on the AST so codegen
// can emit explicit Go types instead of leaning on Go's inference.
//
// Like every other stage it accumulates errors rather than aborting, and
// every error names Quartz types (str, float) and never Go ones.
type Checker struct {
	file     string
	funcs    map[string]*FnDecl
	structs  map[string]*StructDecl
	methods  map[string]map[string]*FnDecl // struct name -> method name -> decl
	scopes   []map[string]*Type
	narrowed []map[string]bool // names proved non-nil, innermost last
	curFn    *FnDecl
	Errors   []string
}

func NewChecker(file string) *Checker {
	return &Checker{
		file:    file,
		funcs:   map[string]*FnDecl{},
		structs: map[string]*StructDecl{},
		methods: map[string]map[string]*FnDecl{},
	}
}

// fieldType looks up one field of a struct.
func (c *Checker) fieldType(structName, field string) (*Type, bool) {
	d, ok := c.structs[structName]
	if !ok {
		return nil, false
	}
	for _, f := range d.Fields {
		if f.Name == field {
			return f.T, true
		}
	}
	return nil, false
}

// fieldNames lists a struct's fields, for error messages.
func (c *Checker) fieldNames(structName string) string {
	d, ok := c.structs[structName]
	if !ok {
		return ""
	}
	names := make([]string, 0, len(d.Fields))
	for _, f := range d.Fields {
		names = append(names, f.Name)
	}
	for m := range c.methods[structName] {
		names = append(names, m+"()")
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (c *Checker) errorAt(n Node, format string, args ...any) {
	line, col := n.Pos()
	c.Errors = append(c.Errors,
		fmt.Sprintf("%s:%d:%d: %s", c.file, line, col, fmt.Sprintf(format, args...)))
}

// ---- scopes ----

func (c *Checker) push() { c.scopes = append(c.scopes, map[string]*Type{}) }
func (c *Checker) pop()  { c.scopes = c.scopes[:len(c.scopes)-1] }

// ---- nil narrowing ----
//
// Inside `if x != nil { ... }`, x is known not to be nil, so it can be
// used as a plain T. The set of names currently proved non-nil is kept
// as a stack of frames matching the block structure, and every Ident
// that reads one is marked so codegen knows to dereference it.
//
// This is deliberately syntactic: it understands `x != nil`, `x == nil`
// and && chains of them, and nothing cleverer. A narrowing that only
// fires in obvious cases is easy to predict; one that sometimes fires
// is worse than none.

func (c *Checker) pushNarrow(names []string) {
	frame := map[string]bool{}
	for _, n := range names {
		frame[n] = true
	}
	c.narrowed = append(c.narrowed, frame)
}

func (c *Checker) popNarrow() { c.narrowed = c.narrowed[:len(c.narrowed)-1] }

func (c *Checker) isNarrowed(name string) bool {
	for i := len(c.narrowed) - 1; i >= 0; i-- {
		if c.narrowed[i][name] {
			return true
		}
	}
	return false
}

// nilChecks collects the names a condition proves non-nil when true,
// and the names it proves non-nil when false.
func (c *Checker) nilChecks(e Expr) (whenTrue, whenFalse []string) {
	b, ok := e.(*Binary)
	if !ok {
		return nil, nil
	}
	switch b.Op {
	case AND:
		// Both sides must hold, so both sides' guarantees apply. Nothing
		// is learned from the false branch: either side could be what
		// failed.
		lt, _ := c.nilChecks(b.L)
		rt, _ := c.nilChecks(b.R)
		return append(lt, rt...), nil
	case OR:
		// Mirror image: the false branch means both sides were false.
		_, lf := c.nilChecks(b.L)
		_, rf := c.nilChecks(b.R)
		return nil, append(lf, rf...)
	case NEQ, EQ:
		name, isNilTest := nilComparison(b)
		if !isNilTest {
			return nil, nil
		}
		t := c.lookup(name)
		if !t.IsNullable() {
			return nil, nil
		}
		if b.Op == NEQ {
			return []string{name}, nil
		}
		return nil, []string{name}
	}
	return nil, nil
}

// nilComparison recognises `x != nil` and `nil != x`.
func nilComparison(b *Binary) (string, bool) {
	if id, ok := b.L.(*Ident); ok {
		if _, isNil := b.R.(*NilLit); isNil {
			return id.Name, true
		}
	}
	if id, ok := b.R.(*Ident); ok {
		if _, isNil := b.L.(*NilLit); isNil {
			return id.Name, true
		}
	}
	return "", false
}

// coerce checks a value against an expected type and, where the value
// is a plain T going into a ?T, rewrites the expression in place to box
// it. Returns false if the types are simply incompatible.
func (c *Checker) coerce(slot *Expr, want *Type, got *Type) bool {
	if want == nil || want.IsUnknown() || got.IsUnknown() {
		return true
	}
	if !want.Accepts(got) && !(isUntypedInt(*slot) && innerScalar(want).Kind == KFloat) {
		return false
	}
	if !want.NeedsWrap(got) {
		return true
	}
	// Wrapping composes: putting an int into a ?int! boxes it into a
	// ?int first, then marks that as a success. Doing only the outer
	// layer would produce an int! where a ?int! was wanted.
	if want.IsResult() {
		c.coerce(slot, want.Elem, got)
	}
	line, col := (*slot).Pos()
	*slot = &Widen{pos: pos{Line: line, Col: col}, X: *slot, T: want}
	return true
}

// innerScalar strips every layer of ? and ! to reach the type actually
// being carried, so an untyped integer literal can still find the float
// inside a ?float!.
func innerScalar(t *Type) *Type {
	for t != nil && (t.Kind == KNullable || t.Kind == KResult) {
		t = t.Elem
	}
	return t
}

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
	t := ParseType(text)
	if t == nil {
		c.errorAt(n, "unknown type %q", text)
		return Unknown
	}
	if bad := c.undeclaredStruct(t); bad != "" {
		c.errorAt(n, "unknown type %q", bad)
		return Unknown
	}
	return t
}

// undeclaredStruct returns the name of the first struct inside a type
// that was never declared, so `[]Widget` reports Widget rather than the
// whole type.
func (c *Checker) undeclaredStruct(t *Type) string {
	if t == nil {
		return ""
	}
	switch t.Kind {
	case KStruct:
		if _, ok := c.structs[t.Name]; !ok {
			return t.Name
		}
	case KList:
		return c.undeclaredStruct(t.Elem)
	case KMap:
		if bad := c.undeclaredStruct(t.Key); bad != "" {
			return bad
		}
		return c.undeclaredStruct(t.Elem)
	}
	return ""
}

// ---- entry point ----

func (c *Checker) Check(p *Program) {
	// Pass 0: register struct names before resolving anything, so a field
	// may refer to a struct declared further down the file — including
	// itself, through a list.
	for _, d := range p.Structs {
		c.structs[d.Name] = d
	}
	for _, d := range p.Structs {
		for i := range d.Fields {
			f := &d.Fields[i]
			f.T = c.resolveAnnotation(f.Type, f)
			if f.T == nil {
				f.T = Unknown
			}
			// A struct cannot contain itself by value: the type would need
			// infinite space. Through a list or map it is fine.
			if f.T.Kind == KStruct && f.T.Name == d.Name {
				c.errorAt(f, "%s cannot contain itself — use []%s if you meant a list of them",
					d.Name, d.Name)
				f.T = Unknown
			}
		}
	}
	for _, f := range p.Funcs {
		if f.Recv == "" {
			continue
		}
		if c.methods[f.Recv] == nil {
			c.methods[f.Recv] = map[string]*FnDecl{}
		}
		c.methods[f.Recv][f.Name] = f
	}

	// Scope 0 is the globals, so a function body can see them.
	c.push()
	for _, g := range p.Globals {
		c.stmt(g)
	}

	// Pass 1: resolve every signature before checking any body, so calls
	// to functions declared later in the file type-check correctly.
	for _, f := range p.Funcs {
		for i := range f.Params {
			prm := &f.Params[i]
			// `self` takes its type from the impl block, not an annotation.
			if i == 0 && f.Recv != "" && prm.Name == "self" {
				prm.T = StructOf(f.Recv)
				continue
			}
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
		if f.Recv == "" {
			c.funcs[f.Name] = f
		}
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
	c.pop() // globals
}

func (c *Checker) checkFn(f *FnDecl) {
	prev := c.curFn
	c.curFn = f
	// A function body sees the globals and its own parameters, and
	// nothing from the implicit main.
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
		annot := c.resolveAnnotation(st.Type, st)
		valT := c.exprWant(st.Value, annot)

		switch {
		case annot == nil:
			// No annotation: infer, but reject types that carry no value.
			switch valT.Kind {
			case KVoid:
				c.errorAt(st, "cannot assign the result of a call that returns nothing to %q", st.Name)
				valT = Unknown
			case KNilLit:
				c.errorAt(st, "cannot tell what %q can hold — annotate it, as in: let %s: ?int = nil",
					st.Name, st.Name)
				valT = Unknown
			}
			st.T = valT

		case c.coerce(&st.Value, annot, valT):
			st.T = annot

		default:
			c.errorAt(st, "%q is declared as %s but the value is %s", st.Name, annot, valT)
			st.T = annot
		}
		c.define(st.Name, st.T)

	case *AssignStmt:
		want := c.expr(st.Target)
		valT := c.exprWant(st.Value, want)
		if want.IsUnknown() {
			return // already reported
		}
		if st.Op != ASSIGN {
			c.checkCompound(st, want, valT)
			return
		}
		if !c.coerce(&st.Value, want, valT) {
			c.errorAt(st, "cannot assign %s to %s, which is %s",
				valT, describeTarget(st.Target), want)
		}

	case *ExprStmt:
		c.expr(st.X)

	case *IfStmt:
		c.condition(st.Cond, "an if")
		whenTrue, whenFalse := c.nilChecks(st.Cond)
		c.pushNarrow(whenTrue)
		c.block(st.Then)
		c.popNarrow()
		if st.Else != nil {
			c.pushNarrow(whenFalse)
			c.stmt(st.Else)
			c.popNarrow()
		}

	case *WhileStmt:
		c.condition(st.Cond, "a while")
		whenTrue, _ := c.nilChecks(st.Cond)
		c.pushNarrow(whenTrue)
		c.block(st.Body)
		c.popNarrow()

	case *ForStmt:
		if st.Coll != nil {
			c.forEach(st)
			return
		}
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
		got := c.exprWant(st.Value, want)
		if want.Kind == KVoid {
			return // resolver already reported the mismatch
		}
		if !c.coerce(&st.Value, want, got) {
			c.errorAt(st, "function %q returns %s but this returns %s",
				c.curFn.Name, want, got)
		}

	case *MatchStmt:
		c.match(st)

	case *Block:
		c.block(st)
	}
}

// match checks that every arm compares against the same type as the
// subject, and that the subject is something comparable at all.
func (c *Checker) match(st *MatchStmt) {
	subj := c.expr(st.Subject)
	if !subj.IsUnknown() && (subj.IsCollection() || subj.Kind == KStruct) {
		c.errorAt(st.Subject, "cannot match on %s — match compares values, so it needs an int, float, str or bool", subj)
		subj = Unknown
	}

	seen := map[string]bool{}
	for _, arm := range st.Cases {
		for _, v := range arm.Values {
			got := c.exprWant(v, subj)
			if !subj.Accepts(got) && !(isUntypedInt(v) && subj.Kind == KFloat) {
				c.errorAt(v, "this match is on %s, but this arm compares against %s", subj, got)
				continue
			}
			// Duplicate constants are dead code, and Go rejects them
			// outright in a switch, so catch them here with a better
			// message than the backend would give.
			if key, isConst := constKey(v); isConst {
				if seen[key] {
					c.errorAt(v, "this value is already handled by an earlier arm")
				}
				seen[key] = true
			}
		}
		c.stmt(arm.Body)
	}
	if st.Else != nil {
		c.stmt(st.Else)
	}
}

// constKey renders a literal arm value so duplicates can be spotted.
// Non-literal arms return false and are not checked.
func constKey(e Expr) (string, bool) {
	switch x := e.(type) {
	case *IntLit:
		return "i" + x.Val, true
	case *FloatLit:
		return "f" + x.Val, true
	case *StrLit:
		return "s" + x.Val, true
	case *BoolLit:
		return fmt.Sprintf("b%t", x.Val), true
	}
	return "", false
}

// forEach checks `for x in list` and `for k, v in map`, binding the
// loop variables to the collection's element types.
func (c *Checker) forEach(st *ForStmt) {
	collT := c.expr(st.Coll)
	st.CollT = collT

	var keyT, valT *Type
	switch {
	case collT.IsUnknown():
		keyT, valT = Unknown, Unknown

	case collT.Kind == KList:
		if st.Var2 != "" {
			// Two names over a list means index and element.
			keyT, valT = Int, collT.Elem
		} else {
			keyT = collT.Elem
		}

	case collT.Kind == KMap:
		if st.Var2 == "" {
			c.errorAt(st, "iterating a map binds two names, as in: for key, value in %s { ... }",
				exprText(st.Coll))
			keyT = collT.Key
		} else {
			keyT, valT = collT.Key, collT.Elem
		}

	case collT.Kind == KStr:
		c.errorAt(st, "cannot iterate a str directly — use chars(...) or split(...)")
		keyT, valT = Unknown, Unknown

	default:
		c.errorAt(st, "cannot iterate %s", collT)
		keyT, valT = Unknown, Unknown
	}

	c.push()
	c.define(st.Var, keyT)
	if st.Var2 != "" {
		c.define(st.Var2, valT)
	}
	for _, s := range st.Body.Stmts {
		c.stmt(s)
	}
	c.pop()
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
	op := compoundOp[st.Op]
	target := describeTarget(st.Target)

	// The bitwise family and %= are int-only, like their binary forms.
	switch op {
	case PERCENT, AMP, PIPE, CARET, SHL, SHR:
		if !want.IsUnknown() && want.Kind != KInt {
			c.errorAt(st, "%s needs an int, but %s is %s", goAssignOp(st.Op), target, want)
		}
		if !got.IsUnknown() && got.Kind != KInt {
			c.errorAt(st, "%s needs an int, got %s", goAssignOp(st.Op), got)
		}
		return
	}

	if op == PLUS && want.Kind == KStr {
		if got.Kind != KStr && !got.IsUnknown() {
			c.errorAt(st, "cannot append %s to %s, which is str", got, target)
		}
		return
	}
	if !want.IsNumeric() && !want.IsUnknown() {
		c.errorAt(st, "%s needs a number, but %s is %s", goAssignOp(st.Op), target, want)
		return
	}
	if !got.IsNumeric() && !got.IsUnknown() {
		c.errorAt(st, "%s needs a number, got %s", goAssignOp(st.Op), got)
		return
	}
	// An untyped integer literal adapts to a float target, as in Go.
	if want.Kind == KFloat && got.Kind == KInt && !isUntypedInt(st.Value) {
		c.errorAt(st, "cannot apply %s with an int to %s, which is float (use float(...))",
			goAssignOp(st.Op), target)
	}
	if want.Kind == KInt && got.Kind == KFloat {
		c.errorAt(st, "cannot apply %s with a float to %s, which is int (use int(...))",
			goAssignOp(st.Op), target)
	}
}

// ---- expressions ----

// exprWant is expr with an expected type. The hint matters only for
// empty collection literals, which carry no element type of their own:
// `let xs: []int = []` is the whole reason this exists.
func (c *Checker) exprWant(e Expr, want *Type) *Type {
	if want == nil || want.IsUnknown() {
		return c.expr(e)
	}
	switch x := e.(type) {
	case *ListLit:
		if want.Kind != KList {
			break
		}
		if len(x.Elems) == 0 {
			x.T = want
			return want
		}
		// With an expected element type, use it rather than inferring
		// from the first element. `let xs: []?int = [1, nil, 3]` needs
		// this — inference would read element one as a plain int and then
		// reject nil.
		return c.listLitAs(x, want)

	case *MapLit:
		if want.Kind != KMap {
			break
		}
		if len(x.Keys) == 0 {
			x.T = want
			return want
		}
		return c.mapLitAs(x, want)
	case *Call:
		// A builtin that decodes into a type learns that type from here.
		if name, ok := DottedName(x.Callee); ok {
			if b, isBuiltin := builtins[name]; isBuiltin && b.wantsTarget {
				x.Want = want
			}
		}
	}
	return c.expr(e)
}

// expr returns the type of an expression, reporting any mismatch inside
// it. It never returns nil: unknown stands in for "already reported".
func (c *Checker) expr(e Expr) *Type {
	switch x := e.(type) {

	case *NilLit:
		return NilLitT

	case *Try:
		return c.try(x)

	case *FuncLit:
		return c.funcLit(x)

	case *Widen:
		// Inserted by this pass; already checked when it was created.
		return x.T

	case *IntLit:
		return Int

	case *FloatLit:
		return Float

	case *StrLit:
		return Str

	case *BoolLit:
		return Bool

	case *Interp:
		for i := range x.Parts {
			if x.Parts[i].X == nil {
				continue
			}
			t := c.expr(x.Parts[i].X)
			x.Parts[i].T = t
			// Interpolation is the one place a value gets used without
			// going through a parameter type, so the result check has to
			// be made here too — otherwise "{load(p)}" prints the wrapper.
			if t.IsResult() {
				c.errorAt(x.Parts[i].X,
					"%s might have failed — unwrap it with '?', must(...) or valueOr(...) before printing it", t)
				x.Parts[i].T = Unknown
			}
		}
		return Str

	case *Ident:
		if t := c.lookup(x.Name); t != nil {
			// Inside a proven `x != nil`, the narrowed binding shadows the
			// nullable one and the use is marked so codegen dereferences.
			if c.isNarrowed(x.Name) {
				x.Narrowed = true
				return t.Unwrap()
			}
			return t
		}
		if bc, ok := builtinConsts[x.Name]; ok {
			return bc.typ
		}
		// A declared function used as a value.
		if f, ok := c.funcs[x.Name]; ok {
			return signatureOf(f)
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
		if x.Op == TILDE {
			if t.Kind != KInt {
				c.errorAt(x, "'~' needs an int, got %s", t)
				return Unknown
			}
			return Int
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

	case *Field:
		return c.field(x)

	case *StructLit:
		return c.structLit(x)

	case *ListLit:
		return c.listLit(x)

	case *MapLit:
		return c.mapLit(x)

	case *Index:
		return c.index(x)

	case *Call:
		return c.call(x)
	}
	return Unknown
}

// field types `user.name`. A dotted library path never reaches here —
// the resolver reports those, because they are only valid as a call.
func (c *Checker) field(x *Field) *Type {
	recv := c.expr(x.X)
	if recv.IsUnknown() {
		return Unknown
	}
	if recv.Kind != KStruct {
		c.errorAt(x, "%s has no fields, so %q cannot be read from it", recv, x.Name)
		return Unknown
	}
	if t, ok := c.fieldType(recv.Name, x.Name); ok {
		return t
	}
	if _, isMethod := c.methods[recv.Name][x.Name]; isMethod {
		c.errorAt(x, "%s is a method on %s; did you mean %s()?", x.Name, recv.Name, x.Name)
		return Unknown
	}
	c.errorAt(x, "%s has no field called %q — it has: %s",
		recv.Name, x.Name, c.fieldNames(recv.Name))
	return Unknown
}

// structLit checks `User{name: "ada"}`. Fields may come in any order,
// and any left out take their zero value — but a name that is not a
// field at all, or given twice, is an error.
func (c *Checker) structLit(x *StructLit) *Type {
	d, ok := c.structs[x.Name]
	if !ok {
		for i := range x.Vals {
			c.expr(x.Vals[i])
		}
		x.T = Unknown
		return Unknown // the resolver already reported it
	}

	seen := map[string]bool{}
	for i, name := range x.Fields {
		want, isField := c.fieldType(x.Name, name)
		if !isField {
			c.expr(x.Vals[i])
			c.errorAt(x.Vals[i], "%s has no field called %q — it has: %s",
				x.Name, name, c.fieldNames(x.Name))
			continue
		}
		if seen[name] {
			c.errorAt(x.Vals[i], "field %q is given twice", name)
		}
		seen[name] = true

		got := c.exprWant(x.Vals[i], want)
		if c.coerce(&x.Vals[i], want, got) {
			continue
		}
		c.errorAt(x.Vals[i], "%s.%s is %s, got %s", x.Name, name, want, got)
	}

	// Missing fields are allowed and zero-filled, which is what makes
	// Point{} and Config{debug: true} both reasonable to write.
	_ = d
	x.T = StructOf(x.Name)
	return x.T
}

// callValue checks a call made through a value rather than a declared
// name, and returns what it produces.
func (c *Checker) callValue(x *Call, fnType *Type, what string) *Type {
	if fnType.IsUnknown() {
		return Unknown
	}
	if !fnType.IsFunc() {
		c.errorAt(x, "%s is %s, which cannot be called", what, fnType)
		return Unknown
	}
	if len(x.Args) != len(fnType.Params) {
		c.errorAt(x, "%s expects %s, got %d",
			what, arityText(len(fnType.Params), len(fnType.Params)), len(x.Args))
	}
	for i := 0; i < len(x.Args) && i < len(fnType.Params); i++ {
		want := fnType.Params[i]
		if c.coerce(&x.Args[i], want, x.ArgT[i]) {
			continue
		}
		c.errorAt(x.Args[i], "%s expects %s for argument %d, got %s",
			what, want, i+1, x.ArgT[i])
	}
	return fnType.Elem
}

// signatureOf builds the function type of a declaration, so it can be
// handed around as a value.
func signatureOf(f *FnDecl) *Type {
	params := make([]*Type, 0, len(f.Params))
	for _, p := range f.Params {
		params = append(params, p.T)
	}
	return FuncOf(params, f.RetT)
}

// funcLit checks an anonymous function and returns its type. The body
// is checked in its own scope, stacked on whatever is visible where the
// literal was written, so it can close over locals.
func (c *Checker) funcLit(x *FuncLit) *Type {
	f := x.Decl
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

	x.T = signatureOf(f)
	return x.T
}

// try checks the postfix `?`. Both sides have to line up: the value has
// to be a result, and the enclosing function has to return one too,
// since that is where the failure goes.
func (c *Checker) try(x *Try) *Type {
	inner := c.expr(x.X)
	if inner.IsUnknown() {
		return Unknown
	}
	if !inner.IsResult() {
		c.errorAt(x, "'?' needs a value that can fail, and %s cannot", inner)
		return Unknown
	}
	x.T = inner.Elem

	if c.curFn == nil {
		return x.T // the resolver already reported the misplacement
	}
	if !c.curFn.RetT.IsResult() {
		c.errorAt(x, "'?' returns the failure from %q, so %q must return a type ending in '!' — it returns %s",
			c.curFn.Name, c.curFn.Name, c.curFn.RetT)
		return x.T
	}
	return x.T
}

// listLitAs checks a list literal against a known list type.
func (c *Checker) listLitAs(x *ListLit, want *Type) *Type {
	ok := true
	for i := range x.Elems {
		got := c.exprWant(x.Elems[i], want.Elem)
		if c.coerce(&x.Elems[i], want.Elem, got) {
			continue
		}
		c.errorAt(x.Elems[i], "this list holds %s, but element %d is %s", want.Elem, i+1, got)
		ok = false
		break
	}
	if !ok {
		x.T = Unknown
		return Unknown
	}
	x.T = want
	return want
}

// mapLitAs checks a map literal against a known map type.
func (c *Checker) mapLitAs(x *MapLit, want *Type) *Type {
	ok := true
	for i := range x.Keys {
		if got := c.expr(x.Keys[i]); !want.Key.Accepts(got) {
			c.errorAt(x.Keys[i], "this map has %s keys, but key %d is %s", want.Key, i+1, got)
			ok = false
			break
		}
		got := c.exprWant(x.Vals[i], want.Elem)
		if c.coerce(&x.Vals[i], want.Elem, got) {
			continue
		}
		c.errorAt(x.Vals[i], "this map holds %s, but value %d is %s", want.Elem, i+1, got)
		ok = false
		break
	}
	if !ok {
		x.T = Unknown
		return Unknown
	}
	x.T = want
	return want
}

// listLit infers a list type from the elements, which must all agree.
func (c *Checker) listLit(x *ListLit) *Type {
	if len(x.Elems) == 0 {
		c.errorAt(x, "cannot tell what kind of list this is — annotate it, as in: let xs: []int = []")
		x.T = Unknown
		return Unknown
	}

	elem := c.expr(x.Elems[0])
	// A list of integer literals mixed with any float becomes a float
	// list, following the same untyped-constant rule as arithmetic.
	if elem.Kind == KInt && isUntypedInt(x.Elems[0]) && c.anyFloat(x.Elems) {
		elem = Float
	}

	for i := range x.Elems {
		got := c.exprWant(x.Elems[i], elem)
		if c.coerce(&x.Elems[i], elem, got) {
			continue
		}
		c.errorAt(x.Elems[i], "this list holds %s, but element %d is %s", elem, i+1, got)
		elem = Unknown
		break
	}

	if elem.IsUnknown() {
		x.T = Unknown
		return Unknown
	}
	x.T = ListOf(elem)
	return x.T
}

// anyFloat reports whether any element is a float, so a list written
// [1, 2.5, 3] becomes []float rather than failing on the first element.
func (c *Checker) anyFloat(elems []Expr) bool {
	for _, el := range elems {
		if _, ok := el.(*FloatLit); ok {
			return true
		}
	}
	return false
}

func (c *Checker) mapLit(x *MapLit) *Type {
	if len(x.Keys) == 0 {
		c.errorAt(x, "cannot tell what kind of map this is — annotate it, as in: let m: {str: int} = {}")
		x.T = Unknown
		return Unknown
	}

	keyT := c.expr(x.Keys[0])
	if keyT.Kind != KStr && keyT.Kind != KInt && !keyT.IsUnknown() {
		c.errorAt(x.Keys[0], "a map key must be str or int, got %s", keyT)
		keyT = Unknown
	}
	valT := c.expr(x.Vals[0])

	for i := range x.Keys {
		if got := c.expr(x.Keys[i]); !keyT.Accepts(got) {
			c.errorAt(x.Keys[i], "this map has %s keys, but key %d is %s", keyT, i+1, got)
			keyT = Unknown
			break
		}
		got := c.exprWant(x.Vals[i], valT)
		if c.coerce(&x.Vals[i], valT, got) {
			continue
		}
		c.errorAt(x.Vals[i], "this map holds %s, but value %d is %s", valT, i+1, got)
		valT = Unknown
		break
	}

	if keyT.IsUnknown() || valT.IsUnknown() {
		x.T = Unknown
		return Unknown
	}
	x.T = MapOf(keyT, valT)
	return x.T
}

func (c *Checker) index(x *Index) *Type {
	collT := c.expr(x.X)
	idxT := c.expr(x.Idx)
	x.T = collT

	switch {
	case collT.IsUnknown():
		return Unknown

	case collT.Kind == KList:
		if !idxT.IsUnknown() && idxT.Kind != KInt {
			c.errorAt(x.Idx, "a list index must be int, got %s", idxT)
		}
		return collT.Elem

	case collT.Kind == KMap:
		if !collT.Key.Accepts(idxT) {
			c.errorAt(x.Idx, "this map has %s keys, got %s", collT.Key, idxT)
		}
		return collT.Elem

	case collT.Kind == KStr:
		c.errorAt(x, "cannot index a str — use charAt(s, i) or substr(s, a, b)")
		return Unknown
	}

	c.errorAt(x, "cannot index %s", collT)
	return Unknown
}

// describeTarget names an assignment target for an error message.
func describeTarget(e Expr) string {
	switch t := e.(type) {
	case *Ident:
		return `"` + t.Name + `"`
	case *Field:
		return `"` + t.Name + `"`
	case *Index:
		return "this element"
	}
	return "this target"
}

// exprText renders an expression compactly, for error messages that
// want to quote the user's own code back at them.
func exprText(e Expr) string {
	switch t := e.(type) {
	case *Ident:
		return t.Name
	case *Index:
		return exprText(t.X) + "[...]"
	case *Call:
		return exprText(t.Callee) + "(...)"
	}
	return "it"
}

func (c *Checker) binary(x *Binary) *Type {
	// `a && b` proves a before b runs, so anything a establishes is
	// available inside b. Without this, `x != nil && x > 3` would reject
	// its own right-hand side.
	if x.Op == AND || x.Op == OR {
		lt := c.expr(x.L)
		whenTrue, whenFalse := c.nilChecks(x.L)
		if x.Op == AND {
			c.pushNarrow(whenTrue)
		} else {
			c.pushNarrow(whenFalse)
		}
		rt := c.expr(x.R)
		c.popNarrow()

		if lt.IsUnknown() || rt.IsUnknown() {
			return Unknown
		}
		if lt.Kind != KBool || rt.Kind != KBool {
			c.errorAt(x, "'%s' needs bool on both sides, got %s and %s",
				goBinOp(x.Op), lt, rt)
			return Unknown
		}
		return Bool
	}

	lt := c.expr(x.L)
	rt := c.expr(x.R)

	if lt.IsUnknown() || rt.IsUnknown() {
		return Unknown
	}

	// A nullable in any operator other than a nil comparison is the
	// mistake this type exists to catch, so name the fix.
	if x.Op != EQ && x.Op != NEQ {
		if bad := firstNullable(lt, rt); bad != nil {
			c.errorAt(x, "%s might be nil — %s", bad, nilAdvice(x))
			return Unknown
		}
	}
	// Same idea for a result: it holds a value only if it did not fail.
	if bad := firstResult(lt, rt); bad != nil {
		c.errorAt(x, "%s might have failed — unwrap it with '?', must(...) or valueOr(...) first", bad)
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

	case EQ, NEQ:
		// Comparing against nil is the whole point of a nullable, and is
		// the one place a nullable may appear without being checked.
		if lt.Kind == KNilLit || rt.Kind == KNilLit {
			other := lt
			if lt.Kind == KNilLit {
				other = rt
			}
			if !other.IsNullable() && other.Kind != KNilLit {
				c.errorAt(x, "%s can never be nil, so this comparison is always %t",
					other, x.Op == NEQ)
				return Bool
			}
			return Bool
		}
		if !lt.Equal(rt) {
			c.errorAt(x, "cannot compare %s with %s", lt, rt)
			return Unknown
		}
		if lt.IsFunc() {
			c.errorAt(x, "cannot compare two functions")
			return Unknown
		}
		// Lists, maps and structs compare by their contents, which is
		// what people mean by == and what the printed form suggests.
		x.OpT = lt
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

	case AMP, PIPE, CARET, SHL, SHR:
		if origL.Kind != KInt || origR.Kind != KInt {
			c.errorAt(x, "'%s' works on ints, got %s and %s%s",
				goBinOp(x.Op), origL, origR, bitwiseHint(x, origL, origR))
			return Unknown
		}
		return Int
	}

	return Unknown
}

func firstNullable(ts ...*Type) *Type {
	for _, t := range ts {
		if t.IsNullable() {
			return t
		}
	}
	return nil
}

func firstResult(ts ...*Type) *Type {
	for _, t := range ts {
		if t.IsResult() {
			return t
		}
	}
	return nil
}

// nilAdvice suggests the fix, naming the variable when there is one to
// name so the suggestion can be pasted as written.
func nilAdvice(x *Binary) string {
	for _, side := range []Expr{x.L, x.R} {
		if id, ok := side.(*Ident); ok {
			return "check it first with 'if " + id.Name + " != nil'"
		}
	}
	return "put it in a variable and check that for nil first"
}

// bitwise operators bind looser than comparison in C, and Quartz copies
// that ladder. `flags & MASK == 0` therefore parses as
// `flags & (MASK == 0)`, which is almost never what was meant. When one
// side turns out to be a bool, say so rather than leaving the reader to
// rediscover a fifty-year-old wart.
func bitwiseHint(x *Binary, lt, rt *Type) string {
	if lt.Kind == KBool || rt.Kind == KBool {
		return " — comparison binds tighter than '" + goBinOp(x.Op) +
			"', so you probably want parentheses"
	}
	return ""
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
	// Arguments are typed with the parameter type as a hint, so an empty
	// collection literal passed straight to a function knows what it is.
	// That means working out the callee first.
	hints := c.paramHints(x)
	dynamic := c.dynamicHint(x)
	args := make([]*Type, len(x.Args))
	for i := range x.Args {
		var hint *Type
		if i < len(hints) {
			hint = hints[i]
		}
		// A builtin may work out an argument's type from the ones before
		// it, which a fixed parameter list cannot express.
		if dynamic != nil {
			if h := dynamic(x, args[:i], i); h != nil {
				hint = h
			}
		}
		args[i] = c.exprWant(x.Args[i], hint)
	}
	x.ArgT = args

	// A method call, if the callee is a field on something that types as
	// a struct. Library paths are not values, so they never get here.
	if fld, isField := x.Callee.(*Field); isField {
		if recv := c.receiverType(fld); recv != nil {
			x.Method = true
			x.T = c.methodCall(x, fld, recv, args)
			return x.T
		}
	}

	name, ok := DottedName(x.Callee)
	if !ok {
		// Calling whatever an expression evaluates to.
		x.T = c.callValue(x, c.expr(x.Callee), "this")
		x.ViaValue = true
		return x.T
	}

	// A variable holding a function shadows a declaration of the same
	// name, which is ordinary scoping.
	if t := c.lookup(name); t != nil {
		x.ViaValue = true
		x.T = c.callValue(x, t, name)
		return x.T
	}

	if b, isBuiltin := builtins[name]; isBuiltin {
		if b.check != nil {
			x.T = b.check(c, x, args)
			return x.T
		}
		x.T = c.checkBuiltin(x, name, b, args)
		return x.T
	}

	f, isUser := c.funcs[name]
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
		if c.coerce(&x.Args[i], want, args[i]) {
			continue
		}
		c.errorAt(x.Args[i], "%s expects %s for %q, got %s",
			name, want, f.Params[i].Name, args[i])
	}
	x.T = f.RetT
	return f.RetT
}

// dynamicHint returns a builtin's hintFor hook, if it has one.
func (c *Checker) dynamicHint(x *Call) func(*Call, []*Type, int) *Type {
	name, ok := DottedName(x.Callee)
	if !ok {
		return nil
	}
	if b, isBuiltin := builtins[name]; isBuiltin {
		return b.hintFor
	}
	return nil
}

// paramHints returns the declared parameter types of whatever this call
// resolves to, so each argument can be checked against the type it is
// going into. Anything unresolvable yields no hints, which is harmless:
// the argument is simply typed on its own.
func (c *Checker) paramHints(x *Call) []*Type {
	if fld, isField := x.Callee.(*Field); isField {
		if recv := c.receiverType(fld); recv != nil {
			if m, ok := c.methods[recv.Name][fld.Name]; ok && len(m.Params) > 0 {
				out := make([]*Type, 0, len(m.Params)-1)
				for _, p := range m.Params[1:] { // skip self
					out = append(out, p.T)
				}
				return out
			}
			return nil
		}
	}
	name, ok := DottedName(x.Callee)
	if !ok {
		return nil
	}
	if b, isBuiltin := builtins[name]; isBuiltin {
		if b.check != nil {
			return nil // a custom checker decides for itself
		}
		hints := make([]*Type, len(x.Args))
		for i := range hints {
			if i < len(b.params) {
				hints[i] = b.params[i]
			} else {
				hints[i] = b.rest
			}
		}
		return hints
	}
	if f, isUser := c.funcs[name]; isUser {
		out := make([]*Type, len(f.Params))
		for i, p := range f.Params {
			out[i] = p.T
		}
		return out
	}
	return nil
}

// receiverType returns the struct type a method is being called on, or
// nil when this is a library path rather than a method call.
func (c *Checker) receiverType(fld *Field) *Type {
	// A plain dotted path that names a builtin is a library call.
	if name, ok := DottedName(fld); ok {
		if _, isBuiltin := builtins[name]; isBuiltin {
			return nil
		}
		// A path rooted at a name that is not a variable is a library
		// path too — a broken one, which the resolver has reported.
		if root, isIdent := rootIdent(fld); isIdent && c.lookup(root) == nil {
			return nil
		}
	}
	t := c.expr(fld.X)
	if t.Kind == KStruct {
		return t
	}
	return nil
}

func rootIdent(e Expr) (string, bool) {
	for {
		switch x := e.(type) {
		case *Ident:
			return x.Name, true
		case *Field:
			e = x.X
		default:
			return "", false
		}
	}
}

func (c *Checker) methodCall(x *Call, fld *Field, recv *Type, args []*Type) *Type {
	m, ok := c.methods[recv.Name][fld.Name]
	if !ok {
		if ft, isField := c.fieldType(recv.Name, fld.Name); isField {
			// A field holding a function is callable, it is just not a
			// method — no receiver is passed.
			if ft.IsFunc() {
				x.Method = false
				x.ViaValue = true
				return c.callValue(x, ft, recv.Name+"."+fld.Name)
			}
			c.errorAt(x, "%s.%s is a field, not a method", recv.Name, fld.Name)
			return Unknown
		}
		c.errorAt(x, "%s has no method called %q — it has: %s",
			recv.Name, fld.Name, c.fieldNames(recv.Name))
		return Unknown
	}

	// Params[0] is self, which the receiver supplies.
	want := m.Params[1:]
	if len(args) != len(want) {
		c.errorAt(x, "%s.%s expects %s, got %d",
			recv.Name, fld.Name, arityText(len(want), len(want)), len(args))
	}
	for i := 0; i < len(args) && i < len(want); i++ {
		if c.coerce(&x.Args[i], want[i].T, args[i]) {
			continue
		}
		c.errorAt(x.Args[i], "%s.%s expects %s for %q, got %s",
			recv.Name, fld.Name, want[i].T, want[i].Name, args[i])
	}
	return m.RetT
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
