package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// qzTypes maps Quartz type names to their Go equivalents.
var qzTypes = map[string]string{
	"int":   "int",
	"float": "float64",
	"str":   "string",
	"bool":  "bool",
}

func goType(qz string) string {
	if g, ok := qzTypes[qz]; ok {
		return g
	}
	return "any"
}

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
}

var builtins map[string]builtin

// runtime helpers, injected into the generated program on demand.
// Top-level Go declarations are order-independent, so emission order
// doesn't matter — but dependencies must be listed so they come along.
type helperDef struct {
	code    string
	imports []string
	deps    []string
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
			emit:    func(a []string) string { return "fmt.Println(" + join(a) + ")" },
			imports: []string{"fmt"}, minArgs: 0, maxArgs: -1,
		},
		"write": {
			emit:    func(a []string) string { return "fmt.Print(" + join(a) + ")" },
			imports: []string{"fmt"}, minArgs: 0, maxArgs: -1,
		},
		"input": {
			emit: func(a []string) string {
				if len(a) == 0 {
					return `__input("")`
				}
				return "__input(" + a[0] + ")"
			},
			helpers: []string{"input"}, minArgs: 0, maxArgs: 1,
		},
		"pause": {
			emit:    func(a []string) string { return "__pause()" },
			helpers: []string{"pause"}, minArgs: 0, maxArgs: 0,
		},
		"toInt": {
			emit: func(a []string) string {
				if len(a) == 1 {
					return "__toInt(" + a[0] + ", 0)"
				}
				return "__toInt(" + a[0] + ", " + a[1] + ")"
			},
			helpers: []string{"toInt"}, minArgs: 1, maxArgs: 2,
		},
		"toFloat": {
			emit: func(a []string) string {
				if len(a) == 1 {
					return "__toFloat(" + a[0] + ", 0)"
				}
				return "__toFloat(" + a[0] + ", " + a[1] + ")"
			},
			helpers: []string{"toFloat"}, minArgs: 1, maxArgs: 2,
		},
		"isInt": {
			emit:    func(a []string) string { return "__isInt(" + a[0] + ")" },
			helpers: []string{"isInt"}, minArgs: 1, maxArgs: 1,
		},
		"isFloat": {
			emit:    func(a []string) string { return "__isFloat(" + a[0] + ")" },
			helpers: []string{"isFloat"}, minArgs: 1, maxArgs: 1,
		},
		"str": {
			emit:    func(a []string) string { return `fmt.Sprintf("%v", ` + a[0] + ")" },
			imports: []string{"fmt"}, minArgs: 1, maxArgs: 1,
		},
		"len": {
			emit:    func(a []string) string { return "len(" + a[0] + ")" },
			minArgs: 1, maxArgs: 1,
		},
		"min": {
			emit:    func(a []string) string { return "min(" + join(a) + ")" },
			minArgs: 2, maxArgs: -1,
		},
		"max": {
			emit:    func(a []string) string { return "max(" + join(a) + ")" },
			minArgs: 2, maxArgs: -1,
		},
		"sleep": {
			emit: func(a []string) string {
				return "time.Sleep(time.Duration(" + a[0] + ") * time.Millisecond)"
			},
			imports: []string{"time"}, minArgs: 1, maxArgs: 1,
		},
		"exit": {
			emit:    func(a []string) string { return "os.Exit(" + a[0] + ")" },
			imports: []string{"os"}, minArgs: 1, maxArgs: 1,
		},
	}

	registerStdlib()
	registerWindowsRuntime()
}

// ---- the generator ----

type Codegen struct {
	body    strings.Builder
	indent  int
	srcPath string // absolute path, used in //line directives
	target  string // GOOS the program is being built for
	tmp     int    // counter for generated temporary names
	imports map[string]bool
	helpers map[string]bool
	Errors  []string
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
	for _, i := range def.imports {
		c.imports[i] = true
	}
	for _, d := range def.deps {
		c.addHelper(d)
	}
}

// Generate produces a complete Go program from a resolved Quartz AST.
func (c *Codegen) Generate(p *Program) string {
	for _, f := range p.Funcs {
		c.fnDecl(f)
	}

	c.raw("func main() {")
	c.indent = 1
	for _, s := range p.Main {
		c.stmt(s)
	}
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
		b.WriteString(helperDefs[n].code)
		b.WriteString("\n\n")
	}
	return b.String()
}

// ---- emission helpers ----

func (c *Codegen) w(format string, args ...any) {
	c.body.WriteString(strings.Repeat("\t", c.indent))
	fmt.Fprintf(&c.body, format, args...)
	c.body.WriteByte('\n')
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

func (c *Codegen) fnDecl(f *FnDecl) {
	params := make([]string, len(f.Params))
	for i, p := range f.Params {
		params[i] = p.Name + " " + goType(p.Type)
	}

	ret := ""
	if f.Ret != "" {
		ret = " " + goType(f.Ret)
	}

	c.line(f)
	c.raw("func %s(%s)%s {", f.Name, strings.Join(params, ", "), ret)
	c.indent = 1
	for _, s := range f.Body.Stmts {
		c.stmt(s)
	}
	c.indent = 0
	c.raw("}")
	c.raw("")
}

// ---- statements ----

func (c *Codegen) stmt(s Stmt) {
	switch st := s.(type) {

	case *LetStmt:
		c.line(st)
		typ := ""
		if st.Type != "" {
			typ = " " + goType(st.Type)
		}
		c.w("var %s%s = %s", st.Name, typ, c.expr(st.Value))
		// Go rejects unused locals. The resolver already worked out
		// whether this variable is ever read, so the discard is only
		// emitted where it's actually needed.
		if !st.Used {
			c.w("_ = %s", st.Name)
		}

	case *AssignStmt:
		c.line(st)
		c.w("%s %s %s", st.Name, goAssignOp(st.Op), c.expr(st.Value))

	case *ExprStmt:
		c.line(st)
		c.w("%s", c.expr(st.X))

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

func (c *Codegen) blockBody(b *Block) {
	c.indent++
	for _, s := range b.Stmts {
		c.stmt(s)
	}
	c.indent--
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
	}
	return "="
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

	case *Ident:
		if bc, ok := builtinConsts[x.Name]; ok {
			for _, i := range bc.imports {
				c.imports[i] = true
			}
			return bc.code
		}
		return x.Name

	case *Unary:
		op := "-"
		if x.Op == BANG {
			op = "!"
		}
		return fmt.Sprintf("%s(%s)", op, c.expr(x.X))

	case *Binary:
		// Parens preserve the tree's grouping regardless of Go precedence.
		return fmt.Sprintf("(%s %s %s)", c.expr(x.L), goBinOp(x.Op), c.expr(x.R))

	case *Call:
		return c.call(x)

	case *Interp:
		return c.interp(x)
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
	}
	return "?"
}

func (c *Codegen) call(x *Call) string {
	id, ok := x.Callee.(*Ident)
	if !ok {
		return "nil" // the resolver already reported this
	}

	args := make([]string, len(x.Args))
	for i, a := range x.Args {
		args[i] = c.expr(a)
	}

	if b, isBuiltin := builtins[id.Name]; isBuiltin {
		if b.osOnly != "" && b.osOnly != c.target {
			c.errorAt(x, "%s() is only available on %s (building for %s)",
				id.Name, b.osOnly, c.target)
			return "nil"
		}
		c.need(b.imports, b.helpers)
		return b.emit(args)
	}
	// User-defined function; the resolver verified it exists.
	return fmt.Sprintf("%s(%s)", id.Name, strings.Join(args, ", "))
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
		args = append(args, c.expr(p.X))
	}

	if len(args) == 0 {
		return strconv.Quote(format.String())
	}
	return fmt.Sprintf("fmt.Sprintf(%s, %s)",
		strconv.Quote(format.String()), strings.Join(args, ", "))
}
