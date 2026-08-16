package main

// Differential tests: every program in examples/ is compiled by both
// backends and the output compared byte for byte.
//
// This is the safety net for the whole assembly backend. There is no
// expected-output file to maintain, because the Go backend in ../../src
// is the definition of what Veyl means - if the two disagree, the new
// backend is wrong, and the test says so without anyone having to
// predict the right answer in advance.
//
// It earned its place immediately. The first program ever compiled here
// printed the correct numbers with the wrong line endings, because the C
// runtime translates \n to \r\n on stdout and Go does not. Reading the
// terminal, the two looked identical. Comparing bytes, they were not.
//
// Skipped when the Go backend has not been built, so a clone of this
// folder alone still runs its other tests.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func goBackend(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "src", "veyl.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Skip("the Go backend is not built; run `go build -o veyl.exe ./compiler` in ../../src")
	}
	return p
}

func asmBackend(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "veylasm.exe")
	build := exec.Command("go", "build", "-o", out, ".")
	if outp, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building veylasm failed: %v\n%s", err, outp)
	}
	return out
}

// runBounded compiles and runs one program under a deadline.
//
// A miscompile does not always produce wrong output. Lowering `i += 1`
// as `i = 1` made every counting loop run forever, which produces no
// output at all - so without a deadline the test suite hangs instead of
// failing, and a hang tells you nothing about which program broke.
func runBounded(t *testing.T, backend, src string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, backend, "run", src).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("%s did not finish within 30s, which usually means "+
			"a loop was miscompiled into one that never ends", filepath.Base(backend))
	}
	return out, err
}

func TestBackendsAgree(t *testing.T) {
	veyl := goBackend(t)
	veylasm := asmBackend(t)

	programs, err := filepath.Glob(filepath.Join("..", "examples", "*.vy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(programs) == 0 {
		t.Fatal("no example programs found to compare")
	}

	for _, src := range programs {
		src := src
		t.Run(filepath.Base(src), func(t *testing.T) {
			t.Parallel()

			wantOut, wantErr := runBounded(t, veyl, src)
			if wantErr != nil {
				t.Fatalf("the Go backend could not run this program, so there is "+
					"nothing to compare against: %v\n%s", wantErr, wantOut)
			}

			gotOut, gotErr := runBounded(t, veylasm, src)
			if gotErr != nil {
				t.Fatalf("the assembly backend failed: %v\n%s", gotErr, gotOut)
			}

			if string(gotOut) != string(wantOut) {
				t.Errorf("the backends disagree.\n"+
					"--- go backend  (%d bytes) ---\n%q\n"+
					"--- asm backend (%d bytes) ---\n%q",
					len(wantOut), wantOut, len(gotOut), gotOut)
			}
		})
	}
}

// TestUnsupportedIsAnError checks that the subset boundary is a clean
// compile error rather than wrong output. A backend that silently
// mis-compiles what it does not understand is worse than one that
// refuses, and this is the whole reason the subset is safe to ship.
func TestUnsupportedIsAnError(t *testing.T) {
	veylasm := asmBackend(t)
	dir := t.TempDir()

	// Keep this list honest as the subset grows. `while` and comparisons
	// were here until branches landed, and this test is what said so:
	// they started compiling, the assertion that they could not failed,
	// and the boundary moved on purpose rather than by drift.
	cases := map[string]string{
		"float":        "let x = 1.5\nprint(x)\n",
		"unknown fn":   "print(sqrt(4))\n",
		"undefined":    "print(nope)\n",
		"list":         "let xs = [1, 2]\nprint(xs[0])\n",
		"map":          "let m = {1: 2}\nprint(m[1])\n",
		"struct":       "struct P {\n x: int\n}\nlet p = P{x: 1}\nprint(p.x)\n",
		"arity":        "fn f(a: int) -> int { return a }\nprint(f(1, 2))\n",
		"bad annot":    "let x: int = \"hi\"\nprint(x)\n",
		"str minus":    "print(\"a\" - \"b\")\n",
		"out of scope": "if true {\n let inner = 1\n}\nprint(inner)\n",
	}

	for name, src := range cases {
		name, src := name, src
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// One file per case: these subtests run in parallel, and a
			// shared path would have them overwriting each other's source
			// and passing for the wrong reason.
			p := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".vy")
			if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(veylasm, "run", p).CombinedOutput()
			if err == nil {
				t.Fatalf("expected a compile error, but it compiled and printed:\n%s", out)
			}
		})
	}
}
