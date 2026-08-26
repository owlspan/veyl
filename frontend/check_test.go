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
