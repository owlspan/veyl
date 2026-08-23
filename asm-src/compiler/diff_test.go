package main

// Differential tests: every program in examples/ is compiled by both
// backends and the output compared byte for byte.
//
// This is the safety net for the whole compiler. There is no
// expected-output file to maintain, because the old Go backend is the
// definition of what Veyl means - if the two disagree, this one is
// wrong, and the test says so without anyone having to predict the
// right answer in advance.
//
// That backend lives on the veylgo branch now, so running this needs it
// checked out beside this one:
//
//	git worktree add ../veylgo veylgo
//	cd ../veylgo/src && go build -o veyl.exe ./compiler
//
// Without it the differential half skips and everything else runs.
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

// goBackend finds the reference compiler. It is on the veylgo branch,
// so the two places worth looking are a worktree beside this checkout
// and the old in-tree location, which is where it sits for anyone who
// has not split their clone yet. VEYL_REFERENCE overrides both.
func goBackend(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("VEYL_REFERENCE"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		t.Fatalf("VEYL_REFERENCE is set to %s but there is nothing there", p)
	}
	for _, rel := range [][]string{
		{"..", "..", "..", "veylgo", "src", "veyl.exe"},
		{"..", "..", "src", "veyl.exe"},
	} {
		p, err := filepath.Abs(filepath.Join(rel...))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("the reference backend is not built. `git worktree add ../veylgo veylgo` " +
		"then `cd ../veylgo/src && go build -o veyl.exe ./compiler`")
	return ""
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
	// Generous, and it has to be. The deadline is here to turn a
	// miscompiled loop into a failure rather than a hang, and thirty
	// seconds looked like plenty until a machine where `go build` of
	// hello-world takes twenty on its own - every one of these shells
	// out to the Go toolchain, twenty-seven of them at once. Two minutes
	// still catches a loop that never ends.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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

	programs, err := filepath.Glob(filepath.Join("..", "examples", "*.vl"))
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
		"undefined":       "print(nope)\n",
		"bare empty":      "let xs = []\nprint(len(xs))\n",
		"mixed list":      "let xs = [1, \"two\"]\nprint(xs[0])\n",
		"bare empty map":  "let m = {}\nprint(len(m))\n",
		"mixed map":       "let m = {\"a\": 1, \"b\": \"two\"}\nprint(len(m))\n",
		"float key":       "let m = {1.5: 2}\nprint(len(m))\n",
		"wrong key type":  "let m: {str: int} = {\"a\": 1}\nprint(m[1])\n",
		"struct T! field": "struct P {\n x: int!\n}\nlet p = P{}\nprint(1)\n",
		"list of results": "fn f() -> int! {\n return 1\n}\nlet xs = [f()]\nprint(len(xs))\n",
		"arity":           "fn f(a: int) -> int { return a }\nprint(f(1, 2))\n",
		"bad annot":       "let x: int = \"hi\"\nprint(x)\n",
		"str minus":       "print(\"a\" - \"b\")\n",
		"out of scope":    "if true {\n let inner = 1\n}\nprint(inner)\n",
	}

	for name, src := range cases {
		name, src := name, src
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// One file per case: these subtests run in parallel, and a
			// shared path would have them overwriting each other's source
			// and passing for the wrong reason.
			p := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".vl")
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
