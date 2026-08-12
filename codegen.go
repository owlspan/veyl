package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ---- the builtin library ----

type builtin struct {
	// emit turns already-generated argument expressions into Go source.
	emit func(args []string) string
	// imports and helpers are pulled into the output only when used.
	imports []string
	helpers []string
	minArgs int
	maxArgs int // -1 means unlimited
	// osOnly restricts a builtin to one target OS; "" means portable.
	osOnly string

	// ---- type signature, read by the checker ----

	// params are the positional parameter types. Entries past minArgs are
	// optional. Numeric accepts int or float; Any accepts anything.
	params []*Type
	// rest types the variadic tail, for builtins with maxArgs == -1.
	rest *Type
	// ret is the return type. Void means the builtin produces no value.
	ret *Type
	// retOf overrides ret for the few polymorphic builtins — min and max
	// return whatever type they were given.
	retOf func(args []*Type) *Type

	// wantsTarget asks the checker to record the expected result type on
	// the call, for builtins whose return type comes from the context
	// rather than their arguments — a decoder, essentially.
	wantsTarget bool

	// check replaces the params/rest/ret machinery entirely for builtins
	// whose rules cannot be written as a fixed signature: `contains`
	// means one thing for a str and another for a list. It reports its
	// own errors and returns the result type.
	check func(c *Checker, x *Call, args []*Type) *Type
	// emitT is emit with the argument types available, for the same
	// builtins. When set, it takes precedence over emit.
	emitT func(c *Codegen, x *Call, args []string) string

	// hintFor supplies the expected type of argument i, given the call
	// and the types of the arguments before it. Builtins with a custom
	// check have no fixed params for the checker to read, so without
	// this an empty literal passed to one — valueOr(load(), []) — has
	// nothing to infer from. It also carries an annotation inwards
	// through a wrapper: in `let p: Point! = must(json.decode(t))` the
	// decoder learns what to build from must's own expected type.
	// Return nil to leave an argument unhinted.
	hintFor func(x *Call, known []*Type, i int) *Type
}

// showAll builds an emitter that renders every collection argument in
// Quartz's notation before handing them to a Go print function.
func showAll(goFn string) func(*Codegen, *Call, []string) string {
	return func(c *Codegen, x *Call, a []string) string {
		out := make([]string, len(a))
		for i := range a {
			var t *Type
			if i < len(x.ArgT) {
				t = x.ArgT[i]
			}
			out[i] = c.show(t, a[i])
		}
		return goFn + "(" + strings.Join(out, ", ") + ")"
	}
}

// sameAsFirst is the return rule for min/max: the result is whatever
// type the arguments were.
func sameAsFirst(args []*Type) *Type {
	if len(args) == 0 {
		return Unknown
	}
	first := args[0]
	for _, a := range args[1:] {
		if !a.Equal(first) {
			return Unknown
		}
	}
	return first
}

var builtins map[string]builtin

// runtime helpers, injected into the generated program on demand.
// Top-level Go declarations are order-independent, so emission order
// doesn't matter — but dependencies must be listed so they come along.
type helperDef struct {
	code    string
	imports []string
	deps    []string

	// winCode replaces code when building for Windows, and winImports
	// replaces imports along with it. A generated program is a single
	// file, so it cannot use build tags: a helper needing a syscall
	// that exists on one platform only has to be swapped out here.
	// Leaving winCode empty means the portable body is used everywhere.
	winCode    string
	winImports []string
}

// forTarget picks the platform-specific body, when there is one.
func (d helperDef) forTarget(goos string) (string, []string) {
	if goos == "windows" && d.winCode != "" {
		return d.winCode, d.winImports
	}
	return d.code, d.imports
}

var helperDefs = map[string]helperDef{
	"stdin": {
		code:    "var __stdin = bufio.NewReader(os.Stdin)",
		imports: []string{"bufio", "os"},
	},
	"input": {
		code: `func __input(prompt string) string {
	if prompt != "" {
		fmt.Print(prompt)
	}
	line, _ := __stdin.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}`,
		imports: []string{"fmt", "strings"},
		deps:    []string{"stdin"},
	},
	"pause": {
		code: `func __pause() {
	fmt.Print("Press Enter to continue...")
	__stdin.ReadString('\n')
}`,
		imports: []string{"fmt"},
		deps:    []string{"stdin"},
	},
	"toInt": {
		code: `func __toInt(s string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fallback
	}
	return n
}`,
		imports: []string{"strconv", "strings"},
	},
	"toFloat": {
		code: `func __toFloat(s string, fallback float64) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return fallback
	}
	return f
}`,
		imports: []string{"strconv", "strings"},
	},
	"isInt": {
		code: `func __isInt(s string) bool {
	_, err := strconv.Atoi(strings.TrimSpace(s))
	return err == nil
}`,
		imports: []string{"strconv", "strings"},
	},
	"isFloat": {
		code: `func __isFloat(s string) bool {
	_, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return err == nil
}`,
		imports: []string{"strconv", "strings"},
	},
}

func init() {
	// Declared in init() because some entries reference helpers, and Go
	// won't allow a package-level cycle between the two tables.
	join := func(args []string) string { return strings.Join(args, ", ") }

	builtins = map[string]builtin{
		"print": {
			emitT:   showAll("fmt.Println"),
			imports: []string{"fmt"}, minArgs: 0, maxArgs: -1,
			rest: Any, ret: Void,
		},
		"write": {
			emitT:   showAll("fmt.Print"),
			imports: []string{"fmt"}, minArgs: 0, maxArgs: -1,
			rest: Any, ret: Void,
		},
		"input": {
			emit: func(a []string) string {
				if len(a) == 0 {
					return `__input("")`
				}
				return "__input(" + a[0] + ")"
			},
			helpers: []string{"input"}, minArgs: 0, maxArgs: 1,
			params: []*Type{Str}, ret: Str,
		},
		"pause": {
			emit:    func(a []string) string { return "__pause()" },
			helpers: []string{"pause"}, minArgs: 0, maxArgs: 0,
			ret: Void,
		},
		"toInt": {
			emit: func(a []string) string {
				if len(a) == 1 {
					return "__toInt(" + a[0] + ", 0)"
				}
				return "__toInt(" + a[0] + ", " + a[1] + ")"
			},
			helpers: []string{"toInt"}, minArgs: 1, maxArgs: 2,
			params: []*Type{Str, Int}, ret: Int,
		},
		"toFloat": {
			emit: func(a []string) string {
				if len(a) == 1 {
					return "__toFloat(" + a[0] + ", 0)"
				}
				return "__toFloat(" + a[0] + ", " + a[1] + ")"
			},
			helpers: []string{"toFloat"}, minArgs: 1, maxArgs: 2,
			params: []*Type{Str, Float}, ret: Float,
		},
		"isInt": {
			emit:    func(a []string) string { return "__isInt(" + a[0] + ")" },
			helpers: []string{"isInt"}, minArgs: 1, maxArgs: 1,
			params: []*Type{Str}, ret: Bool,
		},
		"isFloat": {
			emit:    func(a []string) string { return "__isFloat(" + a[0] + ")" },
			helpers: []string{"isFloat"}, minArgs: 1, maxArgs: 1,
			params: []*Type{Str}, ret: Bool,
		},
		"str": {
			emitT: func(c *Codegen, x *Call, a []string) string {
				var t *Type
				if len(x.ArgT) > 0 {
					t = x.ArgT[0]
				}
				if t.NeedsShow() {
					return c.show(t, a[0])
				}
				return `fmt.Sprintf("%v", ` + a[0] + ")"
			},
			imports: []string{"fmt"}, minArgs: 1, maxArgs: 1,
			params: []*Type{Any}, ret: Str,
		},
		"len": {
			emit:    func(a []string) string { return "len(" + a[0] + ")" },
			minArgs: 1, maxArgs: 1,
			params: []*Type{Any}, ret: Int,
		},
		"min": {
			emit:    func(a []string) string { return "min(" + join(a) + ")" },
			minArgs: 2, maxArgs: -1,
			rest: Numeric, retOf: sameAsFirst,
		},
		"max": {
			emit:    func(a []string) string { return "max(" + join(a) + ")" },
			minArgs: 2, maxArgs: -1,
			rest: Numeric, retOf: sameAsFirst,
		},
		"sleep": {
			emit: func(a []string) string {
				return "time.Sleep(time.Duration(" + a[0] + ") * time.Millisecond)"
			},
			imports: []string{"time"}, minArgs: 1, maxArgs: 1,
			params: []*Type{Int}, ret: Void,
		},
		"exit": {
			emit:    func(a []string) string { return "os.Exit(" + a[0] + ")" },
			imports: []string{"os"}, minArgs: 1, maxArgs: 1,
			params: []*Type{Int}, ret: Void,
		},
	}

	registerStdlib()
	registerCollections()
	registerOs()
	registerNet()
	registerTime()
	registerJson()
	registerTask()
	registerRe()
	registerData()
	registerExtra()
	registerMore()
	registerWindowsRuntime()
}

// ---- the generator ----

type Codegen struct {
	body     strings.Builder
	indent   int
	srcPath  string          // absolute path, used in //line directives
	target   string          // GOOS the program is being built for
	funcs    map[string]bool // qualified names of user functions
	curPkg   string          // namespace of the code being emitted
	tmp      int             // counter for generated temporary names
	pending  []string        // statements hoisted out of the expression being built
	curFnRet *Type           // return type of the function being emitted, for `?`
	imports  map[string]bool
	helpers  map[string]bool
	Errors   []string
}

func NewCodegen(srcPath, target string) *Codegen {
	return &Codegen{
		srcPath: srcPath,
		target:  target,
		imports: map[string]bool{},
		helpers: map[string]bool{},
	}
}

func (c *Codegen) errorAt(n Node, format string, args ...any) {
	line, col := n.Pos()
	c.Errors = append(c.Errors,
		fmt.Sprintf("%s:%d:%d: %s", c.srcPath, line, col, fmt.Sprintf(format, args...)))
}

func (c *Codegen) need(imports, helpers []string) {
	for _, i := range imports {
		c.imports[i] = true
	}
	for _, h := range helpers {
		c.addHelper(h)
	}
}

func (c *Codegen) addHelper(name string) {
	if c.helpers[name] {
		return
	}
	c.helpers[name] = true
	def := helperDefs[name]
	_, imports := def.forTarget(c.target)
	for _, i := range imports {
		c.imports[i] = true
	}
	for _, d := range def.deps {
		c.addHelper(d)
	}
}

// Generate produces a complete Go program from a resolved Quartz AST.
func (c *Codegen) Generate(p *Program) string {
	// Which user functions exist, by qualified name, so a bare call
	// inside a package can be told from one in the program body.
	c.funcs = map[string]bool{}
	for _, f := range p.Funcs {
		if f.Recv == "" {
			c.funcs[qual(f.Pkg, f.Name)] = true
		}
	}

	for _, d := range p.Structs {
		c.structDecl(d)
	}
	// Globals become package-level vars, which is what lets a function
	// see them. Go works out the initialisation order from the
	// dependencies between them, so declaration order does not matter
	// here any more than it does for functions.
	for _, g := range p.Globals {
		c.line(g)
		c.raw("var %s %s = %s", g.Name, g.T.Go(), c.expr(g.Value))
		c.flushGlobalPending()
	}
	if len(p.Globals) > 0 {
		c.raw("")
	}
	for _, f := range p.Funcs {
		c.fnDecl(f)
	}

	c.raw("func main() {")
	c.indent = 1
	// Every program gets the crash handler. A Go panic reaching the
	// terminal shows a goroutine dump and Go's own vocabulary, neither
	// of which means anything to someone writing Quartz.
	c.need(nil, []string{"crash"})
	c.raw("	defer __crash()")
	c.stmts(p.Main)
	c.indent = 0
	c.raw("}")

	// The header is assembled last because imports and helpers are only
	// known after the whole body has been walked.
	var out strings.Builder
	out.WriteString("package main\n\n")
	out.WriteString(c.importBlock())
	out.WriteString(c.helperBlock())
	out.WriteString(c.body.String())
	return out.String()
}

func (c *Codegen) importBlock() string {
	if len(c.imports) == 0 {
		return ""
	}
	names := make([]string, 0, len(c.imports))
	for n := range c.imports {
		names = append(names, n)
	}
	sort.Strings(names)

	if len(names) == 1 {
		return fmt.Sprintf("import %q\n\n", names[0])
	}
	var b strings.Builder
	b.WriteString("import (\n")
	for _, n := range names {
		fmt.Fprintf(&b, "\t%q\n", n)
	}
	b.WriteString(")\n\n")
	return b.String()
}

func (c *Codegen) helperBlock() string {
	if len(c.helpers) == 0 {
		return ""
	}
	names := make([]string, 0, len(c.helpers))
	for n := range c.helpers {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, n := range names {
		code, _ := helperDefs[n].forTarget(c.target)
		b.WriteString(code)
		b.WriteString("\n\n")
	}
	return b.String()
}

// ---- emission helpers ----

// w writes one statement, first emitting anything that `?` hoisted out
// of the expression being written.
//
// Go's early return cannot live inside an expression, so `load(p)?`
// becomes a temporary and an `if` placed before the statement that used
// it. Expressions are generated before w is called, so by the time we
// get here the pending lines are known.
func (c *Codegen) w(format string, args ...any) {
	c.flushPending()
	c.body.WriteString(strings.Repeat("\t", c.indent))
	fmt.Fprintf(&c.body, format, args...)
	c.body.WriteByte('\n')
}

// flushGlobalPending discards anything an initialiser tried to hoist.
// Only `?` hoists, and the resolver already rejects it outside a
// function — there is nothing at package level to return from. This
// just makes sure a slip cannot leak into the next declaration.
func (c *Codegen) flushGlobalPending() { c.pending = nil }

func (c *Codegen) flushPending() {
	if len(c.pending) == 0 {
		return
	}
	lines := c.pending
	c.pending = nil
	for _, line := range lines {
		c.body.WriteString(strings.Repeat("\t", c.indent))
		c.body.WriteString(line)
		c.body.WriteByte('\n')
	}
}

// tryExpr lowers `expr?`. The value is bound to a temporary, the
// failure path returns early, and the expression evaluates to the
// unwrapped value.
func (c *Codegen) tryExpr(x *Try) string {
	c.need(nil, []string{"result"})
	inner := c.expr(x.X)

	c.tmp++
	tmp := fmt.Sprintf("__try%d", c.tmp)

	failType := "any"
	if c.curFnRet != nil && c.curFnRet.IsResult() {
		failType = c.curFnRet.Elem.Go()
	}

	c.pending = append(c.pending,
		fmt.Sprintf("%s := %s", tmp, inner),
		fmt.Sprintf("if %s.e != \"\" { return __fail[%s](%s.e) }", tmp, failType, tmp),
	)
	return tmp + ".v"
}

func (c *Codegen) raw(format string, args ...any) {
	fmt.Fprintf(&c.body, format, args...)
	c.body.WriteByte('\n')
}

// line emits a //line directive so errors from the Go backend and any
// debugger point at the .qz source. It must start at column 1.
func (c *Codegen) line(n Node) {
	l, _ := n.Pos()
	fmt.Fprintf(&c.body, "//line %s:%d\n", c.srcPath, l)
}

// ---- declarations ----

// goField mangles a Quartz field name into an exported Go one.
//
// Quartz field names are lower case, which in Go means unexported, and
// unexported fields are invisible to encoding/json and unreadable
// through reflect.Interface(). Prefixing a capital X fixes both. The
// mapping is injective — the original name is preserved exactly after
// the prefix — so `x` and `X` stay distinct fields.
//
// The json tag carries the name the user actually wrote, so encoding
// round-trips through the spelling they expect.
func goField(name string) string { return "X" + name }

func (c *Codegen) structDecl(d *StructDecl) {
	c.line(d)
	c.raw("type %s struct {", d.Name)
	for _, f := range d.Fields {
		c.raw("\t%s %s `json:%q`", goField(f.Name), f.T.Go(), f.Name)
	}
	c.raw("}")
	c.raw("")
}

func (c *Codegen) fnDecl(f *FnDecl) {
	// A method takes a pointer receiver so it can change the struct it is
	// called on. Assignment still copies — `let b = a` gives two
	// independent values — so this only affects methods, and it is what
	// makes an ordinary bump()/add() method possible at all.
	recv := ""
	params := f.Params
	if f.Recv != "" && len(params) > 0 {
		recv = fmt.Sprintf("(self *%s) ", f.Recv)
		params = params[1:]
	}

	goParams := make([]string, len(params))
	for i, p := range params {
		goParams[i] = p.Name + " " + p.T.Go()
	}

	ret := ""
	if f.RetT != nil && f.RetT.Kind != KVoid {
		ret = " " + f.RetT.Go()
	}

	c.line(f)
	c.raw("func %s%s(%s)%s {", recv, goIdent(qual(f.Pkg, f.Name)), strings.Join(goParams, ", "), ret)
	prevPkg := c.curPkg
	c.curPkg = f.Pkg
	prevRet := c.curFnRet
	c.curFnRet = f.RetT
	c.indent = 1
	c.stmts(f.Body.Stmts)
	c.indent = 0
	c.curFnRet = prevRet
	c.curPkg = prevPkg
	c.raw("}")
	c.raw("")
}

// ---- statements ----

func (c *Codegen) stmt(s Stmt) {
	switch st := s.(type) {

	case *LetStmt:
		c.line(st)
		// The checker resolved a type for every binding, written or
		// inferred, so the Go type is always stated explicitly. That keeps
		// Go's inference from quietly disagreeing with Quartz's.
		c.w("var %s %s = %s", st.Name, st.T.Go(), c.expr(st.Value))
		// Go rejects unused locals. The resolver already worked out
		// whether this variable is ever read, so the discard is only
		// emitted where it's actually needed.
		if !st.Used {
			c.w("_ = %s", st.Name)
		}

	case *AssignStmt:
		c.line(st)
		// A list element is written through a bounds-checked helper, so
		// `xs[99] = v` reports a Quartz error rather than panicking. Maps
		// and plain variables assign directly.
		if idx, ok := st.Target.(*Index); ok && idx.T != nil && idx.T.Kind == KList {
			value := c.expr(st.Value)
			if st.Op != ASSIGN {
				value = fmt.Sprintf("(__listGet(%s, %s) %s %s)",
					c.expr(idx.X), c.expr(idx.Idx), strings.TrimSuffix(goAssignOp(st.Op), "="), value)
			}
			c.need(nil, []string{"listSet"})
			c.w("__listSet(%s, %s, %s)", c.lvalue(idx.X), c.expr(idx.Idx), value)
			return
		}
		c.w("%s %s %s", c.lvalue(st.Target), goAssignOp(st.Op), c.expr(st.Value))

	case *ExprStmt:
		c.line(st)
		code := c.expr(st.X)
		// Go accepts only a call as a bare statement, but plenty of
		// builtins compile to something else — `(os.Remove(p) == nil)`,
		// or a func literal. Discarding the result makes any of them a
		// legal statement. Void calls are left alone so the common case
		// stays readable.
		if t := staticType(st.X); t != nil && t.Kind != KVoid {
			c.w("_ = %s", code)
			return
		}
		c.w("%s", code)

	case *ReturnStmt:
		c.line(st)
		if st.Value == nil {
			c.w("return")
		} else {
			c.w("return %s", c.expr(st.Value))
		}

	case *IfStmt:
		c.line(st)
		c.w("if %s {", c.expr(st.Cond))
		c.blockBody(st.Then)
		if st.Else == nil {
			c.w("}")
			return
		}
		switch e := st.Else.(type) {
		case *Block:
			c.w("} else {")
			c.blockBody(e)
			c.w("}")
		case *IfStmt:
			c.w("} else {")
			c.indent++
			c.stmt(e)
			c.indent--
			c.w("}")
		}

	case *WhileStmt:
		c.line(st)
		c.w("for %s {", c.expr(st.Cond))
		c.blockBody(st.Body)
		c.w("}")

	case *ForStmt:
		c.forStmt(st)

	case *MatchStmt:
		c.line(st)
		// Go's switch does not fall through, which is the behaviour
		// Quartz wants, so this is a direct translation.
		c.w("switch %s {", c.expr(st.Subject))
		for _, arm := range st.Cases {
			values := make([]string, len(arm.Values))
			for i, v := range arm.Values {
				values[i] = c.expr(v)
			}
			c.w("case %s:", strings.Join(values, ", "))
			c.armBody(arm.Body)
		}
		if st.Else != nil {
			c.w("default:")
			c.armBody(st.Else)
		}
		c.w("}")

	case *BreakStmt:
		c.line(st)
		c.w("break")

	case *ContinueStmt:
		c.line(st)
		c.w("continue")

	case *Block:
		c.w("{")
		c.blockBody(st)
		c.w("}")
	}
}

// forStmt emits a counted loop. With no explicit step it produces the
// plain idiomatic Go form. With a step it wraps the bounds in temporaries
// and tests both directions, so a negative step counts downward and the
// bounds are evaluated exactly once.
func (c *Codegen) forStmt(st *ForStmt) {
	if st.Coll != nil {
		c.forEach(st)
		return
	}
	cmp := "<"
	if st.Inclusive {
		cmp = "<="
	}

	c.line(st)

	if st.Step == nil {
		c.w("for %s := %s; %s %s %s; %s++ {",
			st.Var, c.expr(st.Start), st.Var, cmp, c.expr(st.End), st.Var)
		c.blockBody(st.Body)
		c.w("}")
		return
	}

	down := ">"
	if st.Inclusive {
		down = ">="
	}

	c.tmp++
	lo := fmt.Sprintf("__lo%d", c.tmp)
	hi := fmt.Sprintf("__hi%d", c.tmp)
	step := fmt.Sprintf("__st%d", c.tmp)

	c.w("{")
	c.indent++
	c.w("%s := %s", lo, c.expr(st.Start))
	c.w("%s := %s", hi, c.expr(st.End))
	c.w("%s := %s", step, c.expr(st.Step))
	c.w("for %s := %s; (%s > 0 && %s %s %s) || (%s < 0 && %s %s %s); %s += %s {",
		st.Var, lo,
		step, st.Var, cmp, hi,
		step, st.Var, down, hi,
		st.Var, step)
	c.blockBody(st.Body)
	c.w("}")
	c.indent--
	c.w("}")
}

// forEach emits `for x in collection`.
//
// Map iteration is emitted in sorted key order rather than Go's
// randomised order. Random ordering is a genuine source of confusion,
// and a language aimed at beginners should not hand them a loop whose
// output changes between runs. The cost is a sort per loop.
func (c *Codegen) forEach(st *ForStmt) {
	c.line(st)
	coll := c.expr(st.Coll)

	if st.CollT != nil && st.CollT.Kind == KMap {
		c.tmp++
		keys := fmt.Sprintf("__keys%d", c.tmp)
		m := fmt.Sprintf("__m%d", c.tmp)

		c.imports["sort"] = true
		c.w("{")
		c.indent++
		c.w("%s := %s", m, coll)
		c.w("%s := make([]%s, 0, len(%s))", keys, st.CollT.Key.Go(), m)
		c.w("for __k := range %s {", m)
		c.w("\t%s = append(%s, __k)", keys, keys)
		c.w("}")
		c.w("sort.Slice(%s, func(i, j int) bool { return %s[i] < %s[j] })", keys, keys, keys)
		c.w("for _, %s := range %s {", st.Var, keys)
		c.indent++
		c.w("%s := %s[%s]", st.Var2, m, st.Var)
		c.w("_, _ = %s, %s", st.Var, st.Var2)
		c.indent--
		c.blockBody(st.Body)
		c.w("}")
		c.indent--
		c.w("}")
		return
	}

	// A list. One name binds the element; two bind index and element.
	if st.Var2 != "" {
		c.w("for %s, %s := range %s {", st.Var, st.Var2, coll)
		c.indent++
		c.w("_, _ = %s, %s", st.Var, st.Var2)
		c.indent--
	} else {
		c.w("for _, %s := range %s {", st.Var, coll)
		c.indent++
		c.w("_ = %s", st.Var)
		c.indent--
	}
	c.blockBody(st.Body)
	c.w("}")
}

// armBody emits a match arm's statement, indented under its case label.
// A block arm is flattened rather than wrapped in braces, since the
// case already scopes it.
func (c *Codegen) armBody(s Stmt) {
	c.indent++
	if b, isBlock := s.(*Block); isBlock {
		c.stmts(b.Stmts)
	} else if s != nil {
		c.stmt(s)
	}
	c.indent--
}

func (c *Codegen) blockBody(b *Block) {
	c.indent++
	c.stmts(b.Stmts)
	c.indent--
}

// stmts emits a run of statements, stopping after one that always
// transfers control away. Emitting the rest would be pointless, and
// worse than pointless: Go decides whether a function returns by
// looking at its final statement, so a trailing unreachable line makes
// the backend reject a function that is perfectly fine. The resolver
// has already warned about the dead code.
func (c *Codegen) stmts(list []Stmt) {
	for _, s := range list {
		c.stmt(s)
		if exits(s) {
			return
		}
	}
}

// needsDeepEqual reports whether == on this type has to compare
// contents rather than let Go do it. Go cannot compare slices or maps
// at all, and compares a struct field by field only if every field is
// itself comparable — which stops being true the moment one holds a
// list.
func needsDeepEqual(t *Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case KList, KMap, KStruct:
		return true
	case KNullable, KResult:
		// Always, regardless of what is inside. A nullable is a pointer,
		// so Go's == compares addresses: two ?int both holding 5 would
		// come out unequal, which is not what anyone means.
		return true
	}
	return false
}

// staticType reports the checker's verdict for an expression, or nil
// where nothing recorded one.
func staticType(e Expr) *Type {
	switch x := e.(type) {
	case *Call:
		return x.T
	case *Binary:
		return x.T
	case *ListLit:
		return x.T
	case *MapLit:
		return x.T
	}
	return nil
}

// lvalue emits an expression in a position that is written to rather
// than read. It differs from expr in one way: an index is emitted as a
// plain `xs[i]`, never through the bounds-checked read helper, because
// `__listGet(xs, i) = v` is not valid Go.
func (c *Codegen) lvalue(e Expr) string {
	if idx, ok := e.(*Index); ok {
		return fmt.Sprintf("%s[%s]", c.lvalue(idx.X), c.expr(idx.Idx))
	}
	return c.expr(e)
}

// mutate emits a call that needs a pointer to `target` — push, pop and
// friends, which replace a slice header rather than its contents.
//
// A variable or a list element is addressable, so `&xs` works directly.
// A *map* element is not: Go forbids `&m[k]` because a rehash can move
// it. For that case the value is copied out, mutated, and written back,
// with the map and key each evaluated exactly once so a call like
// push(groups[next()], v) does not advance next() twice.
//
// ret is the type the call produces, or nil when it produces nothing.
func (c *Codegen) mutate(target Expr, ret *Type, call func(ref string) string) string {
	idx, isIndex := target.(*Index)
	if !isIndex || idx.T == nil || idx.T.Kind != KMap {
		return call("&" + c.lvalue(target))
	}

	c.tmp++
	m := fmt.Sprintf("__mm%d", c.tmp)
	k := fmt.Sprintf("__mk%d", c.tmp)
	v := fmt.Sprintf("__mv%d", c.tmp)

	var b strings.Builder
	if ret != nil && ret.Kind != KVoid {
		fmt.Fprintf(&b, "func() %s { ", ret.Go())
	} else {
		b.WriteString("func() { ")
	}
	fmt.Fprintf(&b, "%s := %s; %s := %s; %s := %s[%s]; ",
		m, c.lvalue(idx.X), k, c.expr(idx.Idx), v, m, k)

	if ret != nil && ret.Kind != KVoid {
		fmt.Fprintf(&b, "__r := %s; %s[%s] = %s; return __r }()", call("&"+v), m, k, v)
	} else {
		fmt.Fprintf(&b, "%s; %s[%s] = %s }()", call("&"+v), m, k, v)
	}
	return b.String()
}

func goAssignOp(k Kind) string {
	switch k {
	case PLUSEQ:
		return "+="
	case MINUSEQ:
		return "-="
	case STAREQ:
		return "*="
	case SLASHEQ:
		return "/="
	case PERCENTEQ:
		return "%="
	case AMPEQ:
		return "&="
	case PIPEEQ:
		return "|="
	case CARETEQ:
		return "^="
	case SHLEQ:
		return "<<="
	case SHREQ:
		return ">>="
	}
	return "="
}

// compoundOp maps `x op= y` to the binary operator it stands for.
var compoundOp = map[Kind]Kind{
	PLUSEQ:    PLUS,
	MINUSEQ:   MINUS,
	STAREQ:    STAR,
	SLASHEQ:   SLASH,
	PERCENTEQ: PERCENT,
	AMPEQ:     AMP,
	PIPEEQ:    PIPE,
	CARETEQ:   CARET,
	SHLEQ:     SHL,
	SHREQ:     SHR,
}

// ---- expressions ----

func (c *Codegen) expr(e Expr) string {
	switch x := e.(type) {

	case *IntLit:
		return x.Val

	case *FloatLit:
		return x.Val

	case *StrLit:
		return strconv.Quote(x.Val)

	case *BoolLit:
		if x.Val {
			return "true"
		}
		return "false"

	case *NilLit:
		return "nil"

	case *Widen:
		// The type argument is always written out rather than left to Go
		// to infer. Inference fails on an untyped nil, and gets the wrong
		// answer for `let x: ?float = 1`, where the literal is an int
		// until the target says otherwise.
		if x.T.IsResult() {
			c.need(nil, []string{"result"})
			return "__ok[" + x.T.Elem.Go() + "](" + c.expr(x.X) + ")"
		}
		// A pointer to a fresh copy of the value. Go will not let us take
		// the address of an arbitrary expression, so a helper does it.
		c.need(nil, []string{"ptr"})
		return "__ptr[" + x.T.Elem.Go() + "](" + c.expr(x.X) + ")"

	case *Try:
		return c.tryExpr(x)

	case *FuncLit:
		return c.funcLit(x)

	case *Ident:
		if bc, ok := builtinConsts[x.Name]; ok {
			for _, i := range bc.imports {
				c.imports[i] = true
			}
			return bc.code
		}
		// The checker proved this use sits inside a nil check, so the
		// pointer can be read through safely.
		if x.Narrowed {
			return "(*" + x.Name + ")"
		}
		return x.Name

	case *Unary:
		op := "-"
		switch x.Op {
		case BANG:
			op = "!"
		case TILDE:
			op = "^" // Go spells bitwise NOT with a unary caret
		}
		return fmt.Sprintf("%s(%s)", op, c.expr(x.X))

	case *Binary:
		// Go compares slices and maps by identity, or refuses outright.
		// Quartz compares them by contents, matching what == means for
		// everything else in the language.
		if (x.Op == EQ || x.Op == NEQ) && needsDeepEqual(x.OpT) {
			c.need(nil, []string{"deepEqual"})
			call := fmt.Sprintf("__deepEqual(%s, %s)", c.expr(x.L), c.expr(x.R))
			if x.Op == NEQ {
				return "(!" + call + ")"
			}
			return call
		}
		// Parens preserve the tree's grouping regardless of Go precedence.
		return fmt.Sprintf("(%s %s %s)", c.expr(x.L), goBinOp(x.Op), c.expr(x.R))

	case *Call:
		return c.call(x)

	case *Interp:
		return c.interp(x)

	case *ListLit:
		elems := make([]string, len(x.Elems))
		for i, el := range x.Elems {
			elems[i] = c.expr(el)
		}
		return x.T.Go() + "{" + strings.Join(elems, ", ") + "}"

	case *MapLit:
		pairs := make([]string, len(x.Keys))
		for i := range x.Keys {
			pairs[i] = c.expr(x.Keys[i]) + ": " + c.expr(x.Vals[i])
		}
		return x.T.Go() + "{" + strings.Join(pairs, ", ") + "}"

	case *Field:
		return c.expr(x.X) + "." + goField(x.Name)

	case *StructLit:
		pairs := make([]string, len(x.Fields))
		for i, name := range x.Fields {
			pairs[i] = goField(name) + ": " + c.expr(x.Vals[i])
		}
		return x.Name + "{" + strings.Join(pairs, ", ") + "}"

	case *Index:
		// List reads go through a bounds-checked helper so an out-of-range
		// index produces a Quartz-level message instead of a Go panic and
		// a stack trace full of generated code.
		if x.T != nil && x.T.Kind == KList {
			c.need(nil, []string{"listGet"})
			return fmt.Sprintf("__listGet(%s, %s)", c.expr(x.X), c.expr(x.Idx))
		}
		return fmt.Sprintf("%s[%s]", c.expr(x.X), c.expr(x.Idx))
	}
	return "nil"
}

func goBinOp(k Kind) string {
	switch k {
	case PLUS:
		return "+"
	case MINUS:
		return "-"
	case STAR:
		return "*"
	case SLASH:
		return "/"
	case PERCENT:
		return "%"
	case EQ:
		return "=="
	case NEQ:
		return "!="
	case LT:
		return "<"
	case LTE:
		return "<="
	case GT:
		return ">"
	case GTE:
		return ">="
	case AND:
		return "&&"
	case OR:
		return "||"
	case AMP:
		return "&"
	case PIPE:
		return "|"
	case CARET:
		return "^"
	case SHL:
		return "<<"
	case SHR:
		return ">>"
	}
	return "?"
}

func (c *Codegen) call(x *Call) string {
	args := make([]string, len(x.Args))
	for i, a := range x.Args {
		args[i] = c.expr(a)
	}

	if x.Method {
		return c.methodCall(x, args)
	}

	// A call through a value: a variable holding a function, or a
	// literal applied on the spot. Go spells both the same way, so the
	// callee is just emitted and applied.
	if x.ViaValue {
		return fmt.Sprintf("%s(%s)", c.expr(x.Callee), strings.Join(args, ", "))
	}

	name, ok := DottedName(x.Callee)
	if !ok {
		return "nil" // the resolver already reported this
	}

	if b, isBuiltin := builtins[name]; isBuiltin {
		if b.osOnly != "" && b.osOnly != c.target {
			c.errorAt(x, "%s() is only available on %s (building for %s)",
				name, b.osOnly, c.target)
			return "nil"
		}
		c.need(b.imports, b.helpers)
		if b.emitT != nil {
			return b.emitT(c, x, args)
		}
		return b.emit(args)
	}
	// User-defined function; the resolver verified it exists. A bare
	// name inside a package refers to that package's own function, and
	// a dotted one already carries its namespace.
	if _, ok := c.funcs[name]; !ok {
		if q := qual(c.curPkg, name); c.funcs[q] {
			name = q
		}
	}
	return fmt.Sprintf("%s(%s)", goIdent(name), strings.Join(args, ", "))
}

// show wraps a generated expression in the Quartz-formatting helper when
// its type needs it. Scalars print correctly with %v already, so only
// collections pay for the helper — a program with no lists never pulls
// reflect into its binary.
func (c *Codegen) show(t *Type, code string) string {
	if t == nil || !t.NeedsShow() {
		return code
	}
	c.need(nil, []string{"show"})
	return "__show(" + code + ")"
}

// funcLit emits an anonymous function. Go closures capture the same way
// Quartz ones do, so the body needs no special handling — but the
// enclosing function's return type has to be saved and restored, or a
// `?` inside the literal would return from the wrong place.
func (c *Codegen) funcLit(x *FuncLit) string {
	f := x.Decl

	params := make([]string, len(f.Params))
	for i, p := range f.Params {
		params[i] = p.Name + " " + p.T.Go()
	}
	ret := ""
	if f.RetT != nil && f.RetT.Kind != KVoid {
		ret = " " + f.RetT.Go()
	}

	// The body is built into a nested generator so its statements do not
	// interleave with whatever expression this literal sits inside.
	inner := &Codegen{
		srcPath: c.srcPath,
		target:  c.target,
		tmp:     c.tmp + 1000,
		imports: c.imports,
		helpers: c.helpers,
		indent:  1,
	}
	inner.curFnRet = f.RetT
	inner.stmts(f.Body.Stmts)
	c.Errors = append(c.Errors, inner.Errors...)

	body := inner.body.String()
	return fmt.Sprintf("func(%s)%s {\n%s}", strings.Join(params, ", "), ret, body)
}

// methodCall emits `receiver.name(args)`.
//
// Methods take a pointer receiver, and Go only takes the address of an
// addressable expression. A variable, a field or a list element is
// addressable; the result of a call is not. For those the value is
// bound to a temporary first, which is exactly what Go itself would do
// if it allowed it.
func (c *Codegen) methodCall(x *Call, args []string) string {
	fld := x.Callee.(*Field)
	joined := strings.Join(args, ", ")

	// A map element cannot be addressed at all, so it is copied out,
	// operated on, and written back — the same dance push() does.
	if base, viaMap := mapElementBase(fld.X); viaMap {
		return c.mutate(base, x.T, func(ref string) string {
			return fmt.Sprintf("(%s)%s.%s(%s)", ref, suffixAfter(fld.X, base), fld.Name, joined)
		})
	}

	if isAddressable(fld.X) {
		return fmt.Sprintf("%s.%s(%s)", c.addressOf(fld.X), fld.Name, joined)
	}

	// Anything else is a temporary — the result of a call, or a literal.
	// There is nothing to mutate, so binding a copy loses nothing.
	c.tmp++
	tmp := fmt.Sprintf("__recv%d", c.tmp)
	ret := ""
	body := fmt.Sprintf("%s.%s(%s)", tmp, fld.Name, joined)
	if x.T != nil && x.T.Kind != KVoid {
		ret = " " + x.T.Go()
		body = "return " + body
	}
	return fmt.Sprintf("func()%s { %s := %s; %s }()", ret, tmp, c.expr(fld.X), body)
}

// addressOf emits an expression Go will let us take the address of.
// It differs from expr for list elements: the read helper returns a
// copy, so the pointer-returning variant is used instead. That keeps
// both the bounds check and the ability to mutate through xs[i].
func (c *Codegen) addressOf(e Expr) string {
	switch x := e.(type) {
	case *Field:
		return c.addressOf(x.X) + "." + goField(x.Name)
	case *Index:
		if x.T != nil && x.T.Kind == KList {
			c.need(nil, []string{"listAt"})
			return fmt.Sprintf("(*__listAt(%s, %s))", c.addressOf(x.X), c.expr(x.Idx))
		}
	}
	return c.expr(e)
}

// isAddressable reports whether Go will let us take the address of an
// expression, which decides whether a pointer-receiver method can be
// called on it directly.
func isAddressable(e Expr) bool {
	switch x := e.(type) {
	case *Ident:
		return true
	case *Field:
		return isAddressable(x.X)
	case *Index:
		return x.T != nil && x.T.Kind == KList && isAddressable(x.X)
	}
	return false
}

// mapElementBase finds the map element an expression is reached
// through, if any: for `byName["a"].origin` it returns `byName["a"]`.
func mapElementBase(e Expr) (Expr, bool) {
	for {
		switch x := e.(type) {
		case *Index:
			if x.T != nil && x.T.Kind == KMap {
				return x, true
			}
			e = x.X
		case *Field:
			e = x.X
		default:
			return nil, false
		}
	}
}

// suffixAfter renders the field path between a base expression and the
// receiver — the ".origin" in byName["a"].origin.method().
func suffixAfter(e Expr, base Expr) string {
	if e == base {
		return ""
	}
	if f, ok := e.(*Field); ok {
		return suffixAfter(f.X, base) + "." + goField(f.Name)
	}
	return ""
}

// interp turns "a {x} b" into fmt.Sprintf("a %v b", x).
func (c *Codegen) interp(x *Interp) string {
	c.imports["fmt"] = true

	var format strings.Builder
	var args []string

	for _, p := range x.Parts {
		if p.X == nil {
			// % introduces a verb in Printf, so literals must double it
			format.WriteString(strings.ReplaceAll(p.Lit, "%", "%%"))
			continue
		}
		format.WriteString("%v")
		args = append(args, c.show(p.T, c.expr(p.X)))
	}

	if len(args) == 0 {
		return strconv.Quote(format.String())
	}
	return fmt.Sprintf("fmt.Sprintf(%s, %s)",
		strconv.Quote(format.String()), strings.Join(args, ", "))
}

// goIdent turns a namespaced Quartz name into a legal Go identifier.
// "greet.hello" cannot be emitted literally — Go would read it as a
// package selector — so the separator becomes a double underscore,
// which no Quartz identifier can contain.
func goIdent(name string) string {
	return strings.ReplaceAll(name, ".", "__")
}
