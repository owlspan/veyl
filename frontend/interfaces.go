package frontend

// Interfaces: a set of methods, and any struct that has them.
//
//	interface Shape {
//	    fn area(self) -> float
//	    fn name(self) -> str
//	}
//
//	let shapes: []Shape = [Square{side: 2.0}, Circle{r: 1.0}]
//	for s in shapes {
//	    print("{s.name()}: {s.area()}")
//	}
//
// A struct is a Shape by having the methods, with the same parameter
// and result types; nothing says so on the struct. That is Go's rule,
// and it lets a struct written before the interface satisfy it.
//
// The whole program is compiled at once, so every struct ever used as a
// Shape is known by the end of checking. That makes an interface a data
// enum whose variants are those structs: a Shape is which one it is and
// the struct itself. Converting a Square to a Shape builds the variant,
// wherever a Shape is wanted - an argument, a let with a type, a list
// element, a return, a push. Each interface method is a method on that
// enum, written here once the variants are all known: an if on the tag
// for each struct, calling the struct's own method on the value inside.
//
// The value inside is a copy, as any struct is when it is stored, and a
// method that changes self changes that copy - the one the interface
// holds - so it persists from one call through the interface to the
// next.
//
// A match on an interface value names the structs it could be, as in
// Shape.Square(sq); no arm is required for each, since the set is only
// the structs this program happens to convert.

import (
	"strconv"
	"strings"
)

// ifaceValueField is the one field each struct's variant has.
const ifaceValueField = "value"

// declareInterface makes the enum an interface is kept in, with no
// variants yet. Its methods come next, in declareIfaceMethods, and
// their bodies last, in writeDispatch.
func (c *Checker) declareInterface(d *InterfaceDecl) {
	if prev, dup := c.ifaces[d.Name]; dup {
		if prev.File != d.File || prev.Span != d.Span {
			c.ErrorAt(d, "interface %s is declared twice", d.Name)
		}
		return
	}
	if _, clash := c.structs[d.Name]; clash {
		c.ErrorAt(d, "%s is already a struct", d.Name)
		return
	}
	if _, clash := c.enums[d.Name]; clash {
		c.ErrorAt(d, "%s is already an enum", d.Name)
		return
	}
	c.ifaces[d.Name] = d
	e := &EnumDecl{Span: d.Span, Name: d.Name, Pub: d.Pub, File: d.File, Data: true, Interface: true}
	c.enums[d.Name] = e
	c.declareData(e)
}

// declareIfaceMethods resolves an interface's method signatures, once
// every interface's type exists, since one may take or return another.
func (c *Checker) declareIfaceMethods(d *InterfaceDecl) {
	if c.ifaces[d.Name] != d {
		return // a duplicate, already reported
	}
	seen := map[string]bool{}
	for _, m := range d.Methods {
		if seen[m.Name] {
			c.ErrorAt(m, "%s lists %s twice", d.Name, m.Name)
			continue
		}
		seen[m.Name] = true
		stub := &FnDecl{Span: m.Span, Name: m.Name, Recv: d.Name, Params: append([]Param(nil), m.Params...),
			Ret: m.Ret, Pub: d.Pub, File: d.File, Instance: true}
		c.resolveSig(stub)
		m.RetT = stub.RetT
		for i := range m.Params {
			m.Params[i].T = stub.Params[i].T
		}
		if c.methods[d.Name] == nil {
			c.methods[d.Name] = map[string]*FnDecl{}
		}
		c.methods[d.Name][m.Name] = stub
		c.prog.Funcs = append(c.prog.Funcs, stub)
		c.dispatch = append(c.dispatch, stub)
	}
}

// ifaceOf is the interface a type is, or nil.
func (c *Checker) ifaceOf(t *Type) *InterfaceDecl {
	if t == nil || t.Kind != KStruct {
		return nil
	}
	return c.ifaces[t.Name]
}

// missingMethod says why a struct is not an interface, or "" when it is.
func (c *Checker) missingMethod(structName string, d *InterfaceDecl) string {
	for _, m := range d.Methods {
		sm, ok := c.methods[structName][m.Name]
		if !ok {
			return "it has no method " + m.Name
		}
		same := len(sm.Params) == len(m.Params) && sm.RetT.Equal(m.RetT)
		for i := 1; same && i < len(m.Params); i++ {
			same = sm.Params[i].T.Equal(m.Params[i].T)
		}
		if !same {
			return m.Name + " is " + methodShape(sm) + " where " + d.Name + " needs " + methodShape(m)
		}
	}
	return ""
}

func methodShape(f *FnDecl) string {
	parts := []string{"self"}
	for _, p := range f.Params[1:] {
		parts = append(parts, p.T.String())
	}
	s := "fn(" + strings.Join(parts, ", ") + ")"
	if f.RetT != nil && f.RetT.Kind != KVoid {
		s += " -> " + f.RetT.String()
	}
	return s
}

// ifaceVariant is the variant a struct is in an interface's enum,
// added the first time the struct is converted.
func (c *Checker) ifaceVariant(d *InterfaceDecl, structName string) int {
	e := c.enums[d.Name]
	if i := variantIndex(e, structName); i >= 0 {
		return i
	}
	e.Variants = append(e.Variants, structName)
	e.Payloads = append(e.Payloads, []Param{{Span: d.Span, Name: ifaceValueField, Type: structName}})
	sd := c.structs[d.Name]
	sd.Fields = append(sd.Fields, StructField{Span: d.Span, Name: PayloadField(structName, ifaceValueField),
		Type: structName, T: StructOf(structName)})
	return len(e.Variants) - 1
}

// toInterface converts a struct where an interface is wanted, anywhere
// inside ? and !, by building the struct's variant around it. It
// returns the type the value now has, and false when there was nothing
// to convert. A struct without the methods is reported here, with the
// reason, and comes back Unknown so no second error follows.
func (c *Checker) toInterface(slot *Expr, want, got *Type) (*Type, bool) {
	target := want
	for target != nil && (target.Kind == KNullable || target.Kind == KResult) {
		target = target.Elem
	}
	d := c.ifaceOf(target)
	if d == nil || got == nil || got.Kind != KStruct || got.Name == d.Name || c.ifaces[got.Name] != nil {
		return got, false
	}
	if sd, ok := c.structs[got.Name]; !ok || sd.Enum != nil || sd.Extern {
		return got, false
	}
	if why := c.missingMethod(got.Name, d); why != "" {
		c.ErrorAt(*slot, "%s is not a %s: %s", got.Name, d.Name, why)
		return Unknown, true
	}
	i := c.ifaceVariant(d, got.Name)
	line, col := (*slot).Pos()
	at := Span{Line: line, Col: col}
	t := StructOf(d.Name)
	*slot = &StructLit{Span: at, Name: d.Name, T: t,
		Fields: []string{TagField, PayloadField(got.Name, ifaceValueField)},
		Vals:   []Expr{&IntLit{Span: at, Val: strconv.Itoa(i)}, *slot}}
	return t, true
}

// writeDispatch gives each interface method its body, now that every
// struct converted to the interface is known:
//
//	if self.#tag == 0 { return self.Square.value.area() }
//	if self.#tag == 1 { return self.Circle.value.area() }
//
// Calling on the field itself, not a copy of it, is what lets a method
// that changes self change the value the interface holds.
func (c *Checker) writeDispatch() {
	for _, stub := range c.dispatch {
		e := c.enums[stub.Recv]
		at := stub.Span
		body := &Block{Span: at}
		for i, v := range e.Variants {
			inner := &Field{Span: at, X: &Ident{Span: at, Name: "self"}, Name: PayloadField(v, ifaceValueField)}
			call := &Call{Span: at, Callee: &Field{Span: at, X: inner, Name: stub.Name}}
			for _, p := range stub.Params[1:] {
				call.Args = append(call.Args, &Ident{Span: at, Name: p.Name})
			}
			var then []Stmt
			if stub.RetT == nil || stub.RetT.Kind == KVoid {
				then = []Stmt{&ExprStmt{Span: at, X: call}, &ReturnStmt{Span: at}}
			} else {
				then = []Stmt{&ReturnStmt{Span: at, Value: call}}
			}
			tag := &Field{Span: at, X: &Ident{Span: at, Name: "self"}, Name: TagField}
			body.Stmts = append(body.Stmts, &IfStmt{Span: at,
				Cond: &Binary{Span: at, Op: EQ, L: tag, R: &IntLit{Span: at, Val: strconv.Itoa(i)}},
				Then: &Block{Span: at, Stmts: then}})
		}
		// A value made by nobody - the zero value of a struct field or a
		// list slot - is no struct at all, and calling on it is a bug in
		// the program rather than something to guess a result for.
		body.Stmts = append(body.Stmts, &ExprStmt{Span: at, X: &Call{Span: at,
			Callee: &Ident{Span: at, Name: "__abortStr"},
			Args:   []Expr{&StrLit{Span: at, Val: stub.Recv + "." + stub.Name + " called on an empty " + stub.Recv}}}})
		stub.Body = body
		c.pending = append(c.pending, stub)
	}
	c.dispatch = nil
}
