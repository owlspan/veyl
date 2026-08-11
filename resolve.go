package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// varInfo tracks one declared name within a scope.
type varInfo struct {
	isConst bool
	decl    *LetStmt // nil for parameters
	used    bool
}

// Resolver walks the AST before codegen and answers the questions that
// codegen shouldn't have to: does this name exist, is it a constant,
// is it ever read, does this function actually return.
type Resolver struct {
	file     string
	funcs    map[string]*FnDecl
	structs  map[string]*StructDecl
	methods  map[string]map[string]*FnDecl // struct name -> method name -> decl
	scopes   []map[string]*varInfo
	curFn    *FnDecl
	curFile  string // the file whose code is being walked, for pub checks
	mainFile string
	locals   map[string]bool // names the program body declares with `let`
	inGlobal bool            // walking a global's initialiser
	loops    int             // nesting depth, so break/continue can be validated
	Errors   []string

	// Warnings are things worth saying that are not worth refusing to
	// compile over. They are reported and then ignored.
	Warnings []string
}

func (r *Resolver) warnAt(n Node, format string, args ...any) {
	line, col := n.Pos()
	r.Warnings = append(r.Warnings,
		fmt.Sprintf("%s:%d:%d: %s", r.file, line, col, fmt.Sprintf(format, args...)))
}

// visible reports whether a declaration in declFile may be used from
// the file currently being walked. Same file, always. Different file,
// only if it was marked pub.
func (r *Resolver) visible(declFile string, pub bool) bool {
	return pub || declFile == "" || declFile == r.curFile
}

func NewResolver(file string) *Resolver {
	return &Resolver{
		file:    file,
		funcs:   map[string]*FnDecl{},
		structs: map[string]*StructDecl{},
		methods: map[string]map[string]*FnDecl{},
	}
}

func (r *Resolver) errorAt(n Node, format string, args ...any) {
	line, col := n.Pos()
	r.Errors = append(r.Errors,
		fmt.Sprintf("%s:%d:%d: %s", r.file, line, col, fmt.Sprintf(format, args...)))
}

// ---- scope handling ----

func (r *Resolver) push() { r.scopes = append(r.scopes, map[string]*varInfo{}) }

func (r *Resolver) pop() {
	top := r.scopes[len(r.scopes)-1]
	// Hand usage information back to the AST so codegen knows whether
	// it needs to emit a `_ = x` discard for Go's unused-variable rule.
	for _, info := range top {
		if info.decl == nil {
			continue
		}
		info.decl.Used = info.used
		// A name that is written and never read is usually a typo or a
		// leftover. Go refuses to compile these; Quartz allows them and
		// says so, because stopping a half-written program from running
		// is worse than the mistake.
		if !info.used && !info.decl.Global {
			r.warnAt(info.decl, "%q is declared but never used", info.decl.Name)
		}
	}
	r.scopes = r.scopes[:len(r.scopes)-1]
}

func (r *Resolver) declare(name string, info *varInfo, n Node) {
	top := r.scopes[len(r.scopes)-1]
	if _, exists := top[name]; exists {
		r.errorAt(n, "%q is already declared in this scope", name)
		return
	}
	top[name] = info
}

// lookup searches innermost scope outward.
func (r *Resolver) lookup(name string) *varInfo {
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if info, ok := r.scopes[i][name]; ok {
			return info
		}
	}
	return nil
}

// ---- entry point ----

func (r *Resolver) Resolve(p *Program) {
	r.mainFile = p.MainFile
	r.curFile = r.mainFile

	// Names the program body declares. A global cannot use one, and
	// saying why beats "undefined variable".
	r.locals = map[string]bool{}
	for _, s := range p.Main {
		if l, ok := s.(*LetStmt); ok {
			r.locals[l.Name] = true
		}
	}

	// Scope 0 holds the globals — top-level consts, from every file.
	// Functions sit on top of it, which is what makes a global visible
	// inside one where a top-level `let` is not.
	r.inGlobal = true
	r.push()
	for _, g := range p.Globals {
		r.curFile = g.File
		r.expr(g.Value)
		r.declare(g.Name, &varInfo{isConst: true, decl: g, used: true}, g)
	}
	r.inGlobal = false
	r.curFile = r.mainFile

	// Pass 0: struct declarations, so a field or parameter may name any
	// struct regardless of the order they appear in.
	for _, d := range p.Structs {
		if prev, dup := r.structs[d.Name]; dup {
			line, _ := prev.Pos()
			r.errorAt(d, "struct %q is already defined on line %d (in %s)",
				d.Name, line, filepath.Base(prev.File))
			continue
		}
		seen := map[string]bool{}
		for _, f := range d.Fields {
			if seen[f.Name] {
				r.errorAt(f, "struct %q already has a field called %q", d.Name, f.Name)
			}
			seen[f.Name] = true
		}
		r.structs[d.Name] = d
	}

	// Pass 1: collect every signature first, so functions can call each
	// other (and themselves) regardless of declaration order.
	for _, f := range p.Funcs {
		if f.Recv != "" {
			r.declareMethod(f)
			continue
		}
		if _, isBuiltin := builtins[f.Name]; isBuiltin {
			r.errorAt(f, "%q is a builtin and cannot be redefined", f.Name)
			continue
		}
		if _, isStruct := r.structs[f.Name]; isStruct {
			r.errorAt(f, "%q is already a struct, so it cannot also be a function", f.Name)
			continue
		}
		if prev, dup := r.funcs[f.Name]; dup {
			line, _ := prev.Pos()
			r.errorAt(f, "function %q is already defined on line %d (in %s)",
				f.Name, line, filepath.Base(prev.File))
			continue
		}
		r.funcs[f.Name] = f
	}

	// Pass 2: walk each body, remembering which file it came from so a
	// reference to something private to another file can be caught.
	for _, f := range p.Funcs {
		r.curFile = f.File
		r.resolveFn(f)
	}

	// Pass 3: top-level statements become main().
	r.curFn = nil
	r.curFile = r.mainFile
	r.push()
	r.walkStmts(p.Main)
	r.pop()
	r.pop() // globals
}

func (r *Resolver) declareMethod(f *FnDecl) {
	if _, known := r.structs[f.Recv]; !known {
		r.errorAt(f, "there is no struct called %q to add methods to", f.Recv)
		return
	}
	if r.methods[f.Recv] == nil {
		r.methods[f.Recv] = map[string]*FnDecl{}
	}
	if prev, dup := r.methods[f.Recv][f.Name]; dup {
		line, _ := prev.Pos()
		r.errorAt(f, "%s already has a method called %q, defined on line %d",
			f.Recv, f.Name, line)
		return
	}
	// A method and a field cannot share a name, or `u.total` would be
	// ambiguous between reading a field and forgetting to call a method.
	for _, fld := range r.structs[f.Recv].Fields {
		if fld.Name == f.Name {
			r.errorAt(f, "%s already has a field called %q, so it cannot also have a method with that name",
				f.Recv, f.Name)
			return
		}
	}
	r.methods[f.Recv][f.Name] = f
}

func (r *Resolver) resolveFn(f *FnDecl) {
	prev := r.curFn
	prevLoops := r.loops
	r.curFn = f
	r.loops = 0
	r.push()

	for _, prm := range f.Params {
		// Parameters are exempt from the unused check; Go allows them.
		r.declare(prm.Name, &varInfo{used: true}, prm)
	}
	if f.Recv != "" && !hasSelf(f) {
		r.errorAt(f, "a method needs 'self' as its first parameter, as in: fn %s(self)", f.Name)
	}

	r.walkStmts(f.Body.Stmts)

	if f.Ret != "" && !blockReturns(f.Body) {
		if f.Name == "" {
			r.errorAt(f, "this function literal must return a value of type %s on every path", f.Ret)
		} else {
			r.errorAt(f, "function %q must return a value of type %s on every path", f.Name, f.Ret)
		}
	}

	r.pop()
	r.curFn = prev
	r.loops = prevLoops
}

// ---- statements ----

func (r *Resolver) stmt(s Stmt) {
	switch st := s.(type) {

	case *LetStmt:
		r.expr(st.Value) // resolve the value before the name is in scope
		r.declare(st.Name, &varInfo{isConst: st.Const, decl: st}, st)

	case *AssignStmt:
		r.expr(st.Value)
		// Index targets are expressions in their own right: `xs[i] = v`
		// reads both xs and i before it writes.
		switch t := st.Target.(type) {
		case *Index:
			r.expr(t)
		case *Field:
			r.expr(t)
		}
		name := st.TargetName()
		info := r.lookup(name)
		switch {
		case info == nil:
			r.errorAt(st, "undefined variable %q (declare it with 'let %s = ...')", name, name)
		case info.isConst:
			r.errorAt(st, "cannot assign to %q because it was declared const", name)
		default:
			info.used = true
		}

	case *ExprStmt:
		r.expr(st.X)

	case *IfStmt:
		r.expr(st.Cond)
		r.block(st.Then)
		if st.Else != nil {
			r.stmt(st.Else)
		}

	case *WhileStmt:
		r.expr(st.Cond)
		r.loops++
		r.block(st.Body)
		r.loops--

	case *ForStmt:
		if st.Coll != nil {
			r.expr(st.Coll)
		} else {
			r.expr(st.Start)
			r.expr(st.End)
			if st.Step != nil {
				r.expr(st.Step)
			}
		}
		// The loop variable lives in its own scope wrapping the body, so
		// it can shadow an outer name without clashing with it.
		r.push()
		r.declare(st.Var, &varInfo{used: true}, st)
		if st.Var2 != "" {
			r.declare(st.Var2, &varInfo{used: true}, st)
		}
		r.loops++
		r.walkStmts(st.Body.Stmts)
		r.loops--
		r.pop()

	case *MatchStmt:
		r.expr(st.Subject)
		for _, arm := range st.Cases {
			for _, v := range arm.Values {
				r.expr(v)
			}
			r.stmt(arm.Body)
		}
		if st.Else != nil {
			r.stmt(st.Else)
		}

	case *BreakStmt:
		if r.loops == 0 {
			r.errorAt(st, "'break' can only appear inside a loop")
		}

	case *ContinueStmt:
		if r.loops == 0 {
			r.errorAt(st, "'continue' can only appear inside a loop")
		}

	case *ReturnStmt:
		if st.Value != nil {
			r.expr(st.Value)
		}
		switch {
		case r.curFn == nil:
			r.errorAt(st, "'return' can only appear inside a function")
		case r.curFn.Ret == "" && st.Value != nil:
			r.errorAt(st, "function %q has no return type, so 'return' cannot take a value", r.curFn.Name)
		case r.curFn.Ret != "" && st.Value == nil:
			r.errorAt(st, "function %q must return a value of type %s", r.curFn.Name, r.curFn.Ret)
		}

	case *Block:
		r.block(st)
	}
}

func (r *Resolver) block(b *Block) {
	r.push()
	r.walkStmts(b.Stmts)
	r.pop()
}

// walkStmts resolves a run of statements and reports the first one that
// can never run. Only the first: once control has left, everything
// after it is equally unreachable, and ten warnings for one mistake is
// noise.
func (r *Resolver) walkStmts(stmts []Stmt) {
	warned := false
	for i, s := range stmts {
		if !warned && i > 0 && exits(stmts[i-1]) {
			r.warnAt(s, "this can never run — %s above it always leaves", exitWord(stmts[i-1]))
			warned = true
		}
		r.stmt(s)
	}
}

// exits reports whether a statement always transfers control away, so
// nothing after it in the same block can run.
func exits(s Stmt) bool {
	switch st := s.(type) {
	case *ReturnStmt, *BreakStmt, *ContinueStmt:
		return true
	case *Block:
		return len(st.Stmts) > 0 && exits(st.Stmts[len(st.Stmts)-1])
	case *IfStmt:
		if st.Else == nil {
			return false
		}
		then := st.Then
		return len(then.Stmts) > 0 && exits(then.Stmts[len(then.Stmts)-1]) && exits(st.Else)
	case *MatchStmt:
		if st.Else == nil || !exits(st.Else) {
			return false
		}
		for _, arm := range st.Cases {
			if !exits(arm.Body) {
				return false
			}
		}
		return true
	}
	return false
}

func exitWord(s Stmt) string {
	switch s.(type) {
	case *BreakStmt:
		return "the 'break'"
	case *ContinueStmt:
		return "the 'continue'"
	case *ReturnStmt:
		return "the 'return'"
	}
	return "the statement"
}

// ---- expressions ----

func (r *Resolver) expr(e Expr) {
	switch x := e.(type) {

	case *Ident:
		info := r.lookup(x.Name)
		if info == nil {
			if _, isConst := builtinConsts[x.Name]; isConst {
				return
			}
			// A declared function used as a value. That is allowed now:
			// `let double = twice` hands the function around.
			if f, isFn := r.funcs[x.Name]; isFn {
				if !r.visible(f.File, f.Pub) {
					r.errorAt(x, "%q is private to %s — mark it 'pub fn %s' to use it from another file",
						x.Name, filepath.Base(f.File), x.Name)
				}
				return
			}
			if _, isBuiltin := builtins[x.Name]; isBuiltin {
				r.errorAt(x, "%q is a builtin and cannot be used as a value — "+
					"wrap it in a function literal, as in: fn(s: str) { %s(s) }", x.Name, x.Name)
				return
			}
			// Two cases where the name does exist, just not from here.
			if r.locals[x.Name] {
				if r.inGlobal {
					r.errorAt(x, "a top-level const is global, so it cannot use %q, "+
						"which belongs to the program body — use 'let' instead of 'const' here", x.Name)
				} else {
					r.errorAt(x, "%q belongs to the program body and is not visible inside a function — "+
						"pass it in as a parameter, or declare it with 'const' to make it global", x.Name)
				}
				return
			}
			r.errorAt(x, "undefined variable %q", x.Name)
			return
		}
		if g := info.decl; g != nil && g.Global && !r.visible(g.File, g.Pub) {
			r.errorAt(x, "%q is private to %s — mark it 'pub const %s' to use it from another file",
				x.Name, filepath.Base(g.File), x.Name)
			return
		}
		info.used = true

	case *Unary:
		r.expr(x.X)

	case *FuncLit:
		// A literal is resolved like any other function body, so it can
		// close over whatever is in scope where it was written.
		r.resolveFn(x.Decl)

	case *Try:
		r.expr(x.X)
		if r.curFn == nil {
			r.errorAt(x, "'?' can only be used inside a function, because it returns from one")
		}

	case *Binary:
		r.expr(x.L)
		r.expr(x.R)

	case *Interp:
		for _, p := range x.Parts {
			if p.X != nil {
				r.expr(p.X)
			}
		}

	case *ListLit:
		for _, el := range x.Elems {
			r.expr(el)
		}

	case *StructLit:
		for _, v := range x.Vals {
			r.expr(v)
		}
		d, known := r.structs[x.Name]
		switch {
		case !known:
			r.errorAt(x, "there is no struct called %q", x.Name)
		case !r.visible(d.File, d.Pub):
			r.errorAt(x, "struct %q is private to %s — mark it 'pub struct %s' to use it from another file",
				x.Name, filepath.Base(d.File), x.Name)
		}

	case *MapLit:
		for i := range x.Keys {
			r.expr(x.Keys[i])
			r.expr(x.Vals[i])
		}

	case *Index:
		r.expr(x.X)
		r.expr(x.Idx)

	case *Field:
		// `a.b` is field access when a is a variable, and a library path
		// otherwise. Variables win, so a local named `os` shadows the
		// library rather than being silently ignored.
		if r.rootsAtVariable(x) {
			r.expr(x.X)
			return
		}
		if name, ok := DottedName(x); ok {
			if _, isBuiltin := builtins[name]; isBuiltin {
				r.errorAt(x, "%s is a function; did you mean %s(...)?", name, name)
				return
			}
			r.errorAt(x, "there is no value called %s%s", name, nearestNamespaceHint(name))
			return
		}
		// Not a plain path — the base is some other expression, so this is
		// field access on whatever it evaluates to.
		r.expr(x.X)

	case *Call:
		for _, a := range x.Args {
			r.expr(a)
		}
		r.call(x)
	}
}

// rootsAtVariable reports whether `a.b.c` is access on a value rather
// than a library path. It is a library path only when the whole chain
// is plain names and the outermost one is not in scope — so a local
// called `os` shadows the library, and anything that is not a bare name
// chain (a literal, a call, an index) is a value by definition.
func (r *Resolver) rootsAtVariable(e Expr) bool {
	for {
		switch x := e.(type) {
		case *Ident:
			return r.lookup(x.Name) != nil
		case *Field:
			e = x.X
		default:
			return true
		}
	}
}

func (r *Resolver) call(x *Call) {
	// A method call: the receiver is a value, not a library name. Whether
	// the method exists depends on the receiver's type, which only the
	// checker knows, so all the resolver does here is walk the receiver.
	if fld, isField := x.Callee.(*Field); isField && r.rootsAtVariable(fld) {
		r.expr(fld.X)
		return
	}

	name, ok := DottedName(x.Callee)
	if !ok {
		// Calling the result of an expression, such as a function
		// literal applied straight away. The checker verifies it.
		r.expr(x.Callee)
		return
	}

	// A variable holding a function shadows anything declared with the
	// same name, which is ordinary scoping. Arity is the checker's job
	// here, since only it knows the signature.
	if info := r.lookup(name); info != nil {
		info.used = true
		return
	}

	if b, isBuiltin := builtins[name]; isBuiltin {
		r.checkArity(x, name, len(x.Args), b.minArgs, b.maxArgs)
		return
	}

	// A dotted name that is not a builtin is a mistake in a library path,
	// so say which part is wrong rather than "undefined function".
	if _, dotted := x.Callee.(*Field); dotted {
		r.errorAt(x, "there is no builtin called %s%s", name, nearestNamespaceHint(name))
		return
	}

	f, isUser := r.funcs[name]
	if !isUser {
		r.errorAt(x, "undefined function %q", name)
		return
	}
	if !r.visible(f.File, f.Pub) {
		r.errorAt(x, "%q is private to %s — mark it 'pub fn %s' to use it from another file",
			name, filepath.Base(f.File), name)
		return
	}
	r.checkArity(x, name, len(x.Args), len(f.Params), len(f.Params))
}

// nearestNamespaceHint suggests what the user may have meant when a
// dotted path does not exist.
//
// It walks the path from the deepest prefix outward, so os.file.slurp
// suggests the other os.file.* names rather than everything in os. A
// misspelt leaf is far more common than a misremembered library, and
// listing the whole library buries the answer.
func nearestNamespaceHint(name string) string {
	parts := strings.Split(name, ".")
	if !namespaces[parts[0]] {
		return fmt.Sprintf(" — %q is not a library (try one of: %s)", parts[0], namespaceList())
	}

	for depth := len(parts) - 1; depth >= 1; depth-- {
		prefix := strings.Join(parts[:depth], ".") + "."
		var near []string
		for candidate := range builtins {
			if strings.HasPrefix(candidate, prefix) {
				near = append(near, candidate)
			}
		}
		if len(near) == 0 {
			continue
		}
		sort.Strings(near)
		suffix := ""
		if len(near) > 6 {
			near, suffix = near[:6], ", ..."
		}
		return " — did you mean one of: " + strings.Join(near, ", ") + suffix
	}
	return ""
}

func (r *Resolver) checkArity(x *Call, name string, got, min, max int) {
	if got < min || (max >= 0 && got > max) {
		r.errorAt(x, "%s expects %s, got %d", name, arityText(min, max), got)
	}
}

func arityText(min, max int) string {
	switch {
	case max < 0 && min == 0:
		return "any number of arguments"
	case max < 0:
		return fmt.Sprintf("at least %d argument(s)", min)
	case min == max && min == 1:
		return "1 argument"
	case min == max:
		return fmt.Sprintf("%d arguments", min)
	default:
		return fmt.Sprintf("%d to %d arguments", min, max)
	}
}

// hasSelf reports whether a method's first parameter is the receiver.
func hasSelf(f *FnDecl) bool {
	return len(f.Params) > 0 && f.Params[0].Name == "self"
}

// ---- return-path analysis ----

// blockReturns reports whether a block is guaranteed to hit a return.
//
// Any statement that always returns settles it, not just the last one:
// if control cannot get past statement three, the function returns no
// matter what statements four and five look like. Anything after is
// unreachable, and walkStmts warns about that separately.
//
// Still conservative about what counts as "always returns" — an
// if/else where both sides do, a match with an else where every arm
// does, and nothing cleverer. A function whose only return is inside a
// loop is rejected, because proving the loop runs is a different job.
func blockReturns(b *Block) bool {
	if b == nil {
		return false
	}
	for _, s := range b.Stmts {
		if stmtReturns(s) {
			return true
		}
	}
	return false
}

func stmtReturns(s Stmt) bool {
	switch st := s.(type) {
	case *ReturnStmt:
		return true
	case *Block:
		return blockReturns(st)
	case *IfStmt:
		if st.Else == nil {
			return false
		}
		return blockReturns(st.Then) && stmtReturns(st.Else)
	case *MatchStmt:
		// Only an `else` arm makes a match exhaustive — without one, a
		// value matching nothing falls straight out of the bottom.
		if st.Else == nil || !stmtReturns(st.Else) {
			return false
		}
		for _, arm := range st.Cases {
			if !stmtReturns(arm.Body) {
				return false
			}
		}
		return true
	}
	return false
}
