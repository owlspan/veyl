package frontend

// Default parameter values.
//
//	fn greet(name: str, greeting: str = "hello") -> str { ... }
//	greet("ada")               // greeting is "hello"
//	greet("ada", "welcome")
//
// A call that leaves trailing arguments off gets a copy of each one's
// default, put into the call as if written there, so nothing after the
// checker knows defaults exist. That is why a default has to be a
// constant - a number, a string, true or false, nil, or an enum's
// variant: each call gets a fresh copy of it, and a constant is the
// same wherever it is copied to. Once one parameter has a default,
// every one after it needs one too, so which argument is which never
// depends on how many there are.

// checkDefaults validates a function's defaults once its parameter
// types are known.
func (c *Checker) checkDefaults(f *FnDecl) {
	seen := false
	for i := range f.Params {
		p := &f.Params[i]
		if p.Default == nil {
			if seen && p.Name != "self" {
				c.ErrorAt(p, "%s needs a default too: every parameter after one with a default has one", p.Name)
			}
			continue
		}
		seen = true
		if f.Extern {
			c.ErrorAt(p, "an extern function's parameters cannot have defaults")
			continue
		}
		if !constantExpr(p.Default) {
			c.ErrorAt(p.Default, "a default is a constant: a number, a string, true, false, nil or an enum value")
			continue
		}
		got := c.exprWant(copyConstant(p.Default, p.Span), p.T)
		probe := copyConstant(p.Default, p.Span)
		if !c.coerce(&probe, p.T, got) {
			c.ErrorAt(p.Default, "%s is %s, but its default is %s", p.Name, p.T, got)
		}
	}
}

func constantExpr(e Expr) bool {
	switch x := e.(type) {
	case *IntLit, *FloatLit, *StrLit, *BoolLit, *NilLit:
		return true
	case *Unary:
		switch x.X.(type) {
		case *IntLit, *FloatLit:
			return x.Op == MINUS
		}
	case *Field:
		_, ok := x.X.(*Ident)
		return ok
	}
	return false
}

// copyConstant is a fresh copy of a default, placed at the call.
func copyConstant(e Expr, at Span) Expr {
	switch x := e.(type) {
	case *IntLit:
		return &IntLit{Span: at, Val: x.Val}
	case *FloatLit:
		return &FloatLit{Span: at, Val: x.Val}
	case *StrLit:
		return &StrLit{Span: at, Val: x.Val}
	case *BoolLit:
		return &BoolLit{Span: at, Val: x.Val}
	case *NilLit:
		return &NilLit{Span: at}
	case *Unary:
		return &Unary{Span: at, Op: x.Op, X: copyConstant(x.X, at)}
	case *Field:
		id := x.X.(*Ident)
		return &Field{Span: at, X: &Ident{Span: at, Name: id.Name}, Name: x.Name}
	}
	return e
}

// fillDefaults adds the defaults a call left off, checked, to its
// arguments. params are the ones the call's arguments line up with.
func (c *Checker) fillDefaults(x *Call, params []Param, args []*Type) []*Type {
	line, col := x.Pos()
	at := Span{Line: line, Col: col}
	for i := len(x.Args); i < len(params); i++ {
		d := params[i].Default
		if d == nil || !constantExpr(d) {
			break
		}
		arg := copyConstant(d, at)
		t := c.exprWant(arg, params[i].T)
		x.Args = append(x.Args, arg)
		args = append(args, t)
	}
	x.ArgT = args
	return args
}
