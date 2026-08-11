package main

import (
	"fmt"
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
	file    string
	funcs   map[string]*FnDecl
	structs map[string]*StructDecl
	methods map[string]map[string]*FnDecl // struct name -> method name -> decl
	scopes  []map[string]*varInfo
	curFn   *FnDecl
	loops   int // nesting depth, so break/continue can be validated
	Errors  []string
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
		if info.decl != nil {
			info.decl.Used = info.used
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
	// Pass 0: struct declarations, so a field or parameter may name any
	// struct regardless of the order they appear in.
	for _, d := range p.Structs {
		if prev, dup := r.structs[d.Name]; dup {
			line, _ := prev.Pos()
			r.errorAt(d, "struct %q is already defined on line %d", d.Name, line)
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
			r.errorAt(f, "function %q is already defined on line %d", f.Name, line)
			continue
		}
		r.funcs[f.Name] = f
	}

	// Pass 2: walk each body.
	for _, f := range p.Funcs {
		r.resolveFn(f)
	}

	// Pass 3: top-level statements become main().
	r.curFn = nil
	r.push()
	for _, s := range p.Main {
		r.stmt(s)
	}
	r.pop()
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

	for _, s := range f.Body.Stmts {
		r.stmt(s)
	}

	if f.Ret != "" && !blockReturns(f.Body) {
		r.errorAt(f, "function %q must return a value of type %s on every path", f.Name, f.Ret)
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
		for _, s := range st.Body.Stmts {
			r.stmt(s)
		}
		r.loops--
		r.pop()

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
	for _, s := range b.Stmts {
		r.stmt(s)
	}
	r.pop()
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
			if _, isFn := r.funcs[x.Name]; isFn {
				r.errorAt(x, "%q is a function; did you mean %s(...)?", x.Name, x.Name)
				return
			}
			r.errorAt(x, "undefined variable %q", x.Name)
			return
		}
		info.used = true

	case *Unary:
		r.expr(x.X)

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
		if _, known := r.structs[x.Name]; !known {
			r.errorAt(x, "there is no struct called %q", x.Name)
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
		r.errorAt(x, "this expression is not a function")
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
// Deliberately conservative: it only understands trailing returns and
// if/else where both sides return. That's enough to catch the common
// mistake without producing false positives.
func blockReturns(b *Block) bool {
	if b == nil || len(b.Stmts) == 0 {
		return false
	}
	return stmtReturns(b.Stmts[len(b.Stmts)-1])
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
	}
	return false
}
