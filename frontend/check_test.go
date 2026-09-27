package frontend

import (
	"strings"
	"testing"
)

// checked compiles a snippet through lexer, parser and checker,
// returning every diagnostic. EmptyLibrary keeps the sources free of
// builtins, so what a test asserts is never about one backend's set.
func checked(t *testing.T, src string) []string {
	t.Helper()
	lx := NewLexer("t.vl", src)
	ps := NewParser("t.vl", lx.Scan())
	prog := ps.ParseProgram()
	ck := NewChecker("t.vl", EmptyLibrary{})
	ck.Check(prog)
	return append(append(lx.Errors, ps.Errors...), ck.Errors...)
}

func TestMainIsImplicit(t *testing.T) {
	errs := checked(t, `
fn main() {
    let a = 1
}
`)
	if len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), "already called main") {
		t.Errorf("declaring fn main must be refused with a pointed message, got %v", errs)
	}

	// Script style is how programs are written; it stays clean.
	if errs := checked(t, "let n = 1 + 2"); len(errs) != 0 {
		t.Errorf("top-level statements rejected: %v", errs)
	}

	// A method named main belongs to its struct, not to the program.
	errs = checked(t, `
struct Box { n: int }
impl Box {
    fn main(self) -> int { return self.n }
}
let b = Box { n: 3 }
let r = b.main()
`)
	for _, e := range errs {
		if strings.Contains(e, "already called main") {
			t.Errorf("a method named main was mistaken for the entry point: %s", e)
		}
	}
}

func TestConstAssignment(t *testing.T) {
	errs := strings.Join(checked(t, `
const LIMIT = 3
LIMIT = 4
fn f() {
    const inner = 1
    inner += 1
}
let fine = 1
fine = 2
`), "\n")
	for _, name := range []string{"LIMIT", "inner"} {
		if !strings.Contains(errs, `cannot assign to "`+name+`" because it was declared const`) {
			t.Errorf("assigning to const %s was not refused: %q", name, errs)
		}
	}
	if strings.Contains(errs, `"fine"`) {
		t.Errorf("a let was treated as const: %q", errs)
	}
}

func TestVarGlobal(t *testing.T) {
	if errs := checked(t, `
var score = 0
fn add(n: int) {
    score += n
}
add(2)
var var = 1
`); len(errs) > 0 {
		t.Errorf("a var global was refused: %v", errs)
	}
}

func TestPubAcrossFiles(t *testing.T) {
	lx := NewLexer("main.vl", "print(secret())\nprint(hidden)\nlet h = Box{n: 1}\nprint(open())\n")
	prog := NewParser("main.vl", lx.Scan()).ParseProgram()
	lib := NewParser("lib.vl", NewLexer("lib.vl",
		"fn secret() -> int { return 1 }\nconst hidden = 2\nstruct Box { n: int }\npub fn open() -> int { return 3 }\n").Scan()).ParseProgram()
	for _, f := range lib.Funcs {
		f.File = "lib.vl"
	}
	for _, g := range lib.Globals {
		g.File = "lib.vl"
	}
	for _, s := range lib.Structs {
		s.File = "lib.vl"
	}
	prog.Funcs = append(prog.Funcs, lib.Funcs...)
	prog.Globals = append(prog.Globals, lib.Globals...)
	prog.Structs = append(prog.Structs, lib.Structs...)
	ck := NewChecker("main.vl", EmptyLibrary{})
	ck.Check(prog)
	errs := strings.Join(ck.Errors, "\n")
	for _, want := range []string{`"secret" is private to lib.vl`, `"hidden" is private to lib.vl`,
		`struct "Box" is private to lib.vl`} {
		if !strings.Contains(errs, want) {
			t.Errorf("want %q, got %q", want, errs)
		}
	}
	if strings.Contains(errs, `"open"`) {
		t.Errorf("a pub function was refused: %q", errs)
	}
}
