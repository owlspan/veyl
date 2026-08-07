package main

import "fmt"

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
	file   string
	funcs  map[string]*FnDecl
	scopes []map[string]*varInfo
	curFn  *FnDecl
	Errors []string
}

func NewResolver(file string) *Resolver {
	return &Resolver{file: file, funcs: map[string]*FnDecl{}}
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
	// Pass 1: collect every signature first, so functions can call each
	// other (and themselves) regardless of declaration order.
	for _, f := range p.Funcs {
		if _, isBuiltin := builtins[f.Name]; isBuiltin {
			r.errorAt(f, "%q is a builtin and cannot be redefined", f.Name)
			continue
		}
		if prev, dup := r.funcs[f.Name]; dup {
			line, _ := prev.Pos()
			r.errorAt(f, "function %q is already defined on line %d", f.Name, line)
			continue
		}
		if f.Ret != "" {
			r.checkType(f.Ret, f)
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

func (r *Resolver) resolveFn(f *FnDecl) {
	prev := r.curFn
	r.curFn = f
	r.push()

	for _, prm := range f.Params {
		if prm.Type != "" {
			r.checkType(prm.Type, prm)
		}
		// Parameters are exempt from the unused check; Go allows them.
		r.declare(prm.Name, &varInfo{used: true}, prm)
	}

	for _, s := range f.Body.Stmts {
		r.stmt(s)
	}

	if f.Ret != "" && !blockReturns(f.Body) {
		r.errorAt(f, "function %q must return a value of type %s on every path", f.Name, f.Ret)
	}

	r.pop()
	r.curFn = prev
}

func (r *Resolver) checkType(name string, n Node) {
	if _, ok := qzTypes[name]; !ok {
		r.errorAt(n, "unknown type %q (Quartz has int, float, str, bool)", name)
	}
}

// ---- statements ----

func (r *Resolver) stmt(s Stmt) {
	switch st := s.(type) {

	case *LetStmt:
		r.expr(st.Value) // resolve the value before the name is in scope
		if st.Type != "" {
			r.checkType(st.Type, st)
		}
		r.declare(st.Name, &varInfo{isConst: st.Const, decl: st}, st)

	case *AssignStmt:
		r.expr(st.Value)
		info := r.lookup(st.Name)
		switch {
		case info == nil:
			r.errorAt(st, "undefined variable %q (declare it with 'let %s = ...')", st.Name, st.Name)
		case info.isConst:
			r.errorAt(st, "cannot assign to %q because it was declared const", st.Name)
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
		r.block(st.Body)

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

	case *Call:
		for _, a := range x.Args {
			r.expr(a)
		}
		r.call(x)
	}
}

func (r *Resolver) call(x *Call) {
	id, ok := x.Callee.(*Ident)
	if !ok {
		r.errorAt(x, "this expression is not a function")
		return
	}

	if b, isBuiltin := builtins[id.Name]; isBuiltin {
		r.checkArity(x, id.Name, len(x.Args), b.minArgs, b.maxArgs)
		return
	}

	f, isUser := r.funcs[id.Name]
	if !isUser {
		r.errorAt(x, "undefined function %q", id.Name)
		return
	}
	r.checkArity(x, id.Name, len(x.Args), len(f.Params), len(f.Params))
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
