package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The test suite is data-driven. Each case is a .vy file paired with a
// .expected file:
//
//	tests/ok/NAME.vy   + tests/ok/NAME.expected    program output
//	tests/err/NAME.vy  + tests/err/NAME.expected   compiler errors
//
// Adding a test means adding two files, never touching this one. To
// regenerate the .expected files after an intentional change:
//
//	go test -run TestVeyl -update
//
// Always read the diff before accepting it - that flag is how a bug
// becomes the expected behaviour.

var update = flag.Bool("update", false, "regenerate the .expected golden files")

// buildCompiler compiles the compiler once and returns its path.
func buildCompiler(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "veyl-under-test")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("building the compiler failed: %v\n%s", err, out)
	}
	return exe
}

func TestVeylOK(t *testing.T) {
	runSuite(t, "../tests/ok", false)
}

// The golden suite runs every program without arguments, so it cannot
// see whether `veyl run app.vy a b` reaches os.args. It did not: the
// driver read the file from args[1] and dropped the rest, silently,
// while an example documented the opposite.
func TestRunForwardsArguments(t *testing.T) {
	veyl := buildCompiler(t)

	src := filepath.Join(t.TempDir(), "args.vy")
	prog := "for a in os.args() {\n    print(a)\n}\n"
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(veyl, "run", src, "one", "two three").CombinedOutput()
	if err != nil {
		t.Fatalf("running failed: %v\n%s", err, out)
	}
	got := strings.ReplaceAll(string(out), "\r\n", "\n")
	if got != "one\ntwo three\n" {
		t.Errorf("os.args produced %q, want %q", got, "one\ntwo three\n")
	}
}

func TestVeylErrors(t *testing.T) {
	runSuite(t, "../tests/err", true)
}

// TestFormatPreservesBehaviour is the test that matters for a
// formatter: it must not change what a program does.
//
// The whole tests/ok tree is copied, every file in it is formatted, and
// each program is run again against the same golden output. Formatting
// is also checked to be idempotent - a second pass must change nothing,
// or the formatter has no fixed point and `veyl fmt` would rewrite
// the file forever.
func TestFormatPreservesBehaviour(t *testing.T) {
	veyl := buildCompiler(t)

	work := t.TempDir()
	if err := copyTree("../tests/ok", work); err != nil {
		t.Fatal(err)
	}

	sources, err := filepath.Glob(filepath.Join(work, "**", "*.vy"))
	if err != nil {
		t.Fatal(err)
	}
	top, _ := filepath.Glob(filepath.Join(work, "*.vy"))
	sources = append(sources, top...)

	for _, src := range sources {
		before, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(veyl, "fmt", src).CombinedOutput(); err != nil {
			t.Fatalf("formatting %s failed: %v\n%s", filepath.Base(src), err, out)
		}
		once, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := exec.Command(veyl, "fmt", src).CombinedOutput(); err != nil {
			t.Fatal(err)
		}
		twice, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if string(once) != string(twice) {
			t.Errorf("%s: formatting is not idempotent", filepath.Base(src))
		}
		_ = before
	}

	cases, err := filepath.Glob(filepath.Join(work, "*.vy"))
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range cases {
		src := src
		name := strings.TrimSuffix(filepath.Base(src), ".vy")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.Command(veyl, "run", src)
			cmd.Stdin = strings.NewReader("")
			raw, runErr := cmd.CombinedOutput()
			if runErr != nil {
				t.Fatalf("formatted program no longer runs: %v\n%s", runErr, raw)
			}
			got := normalize(string(raw), src)

			wantBytes, err := os.ReadFile(filepath.Join("../tests/ok", name+".expected"))
			if err != nil {
				t.Fatal(err)
			}
			if want := normalizeNewlines(string(wantBytes)); got != want {
				t.Errorf("formatting changed the output\n--- want ---\n%s\n--- got ---\n%s", want, got)
			}
		})
	}
}

func copyTree(from, to string) error {
	return filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func runSuite(t *testing.T, dir string, wantFailure bool) {
	veyl := buildCompiler(t)

	cases, err := filepath.Glob(filepath.Join(dir, "*.vy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Skipf("no cases in %s", dir)
	}

	for _, src := range cases {
		src := src
		name := strings.TrimSuffix(filepath.Base(src), ".vy")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.Command(veyl, "run", src)
			cmd.Stdin = strings.NewReader("")
			raw, runErr := cmd.CombinedOutput()
			got := normalize(string(raw), src)

			if wantFailure && runErr == nil {
				t.Fatalf("expected compilation to fail, but it succeeded:\n%s", got)
			}
			if !wantFailure && runErr != nil {
				t.Fatalf("expected success, got %v:\n%s", runErr, got)
			}

			golden := strings.TrimSuffix(src, ".vy") + ".expected"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			wantBytes, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden file (run with -update to create it): %v\n--- got ---\n%s", err, got)
			}
			if want := normalizeNewlines(string(wantBytes)); got != want {
				t.Errorf("output mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
			}
		})
	}
}

// normalize strips everything machine-specific from compiler output so
// the golden files are portable: absolute paths, the temp build
// directory, and Windows line endings.
func normalize(s, src string) string {
	s = normalizeNewlines(s)
	abs, err := filepath.Abs(src)
	if err == nil {
		s = strings.ReplaceAll(s, abs, filepath.Base(src))
	}
	s = strings.ReplaceAll(s, src, filepath.Base(src))
	return s
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// A runtime failure used to reach the terminal as a Go panic: a
// goroutine dump, hex offsets, and Go's vocabulary rather than Veyl's.
// The //line directives mean the stack already carries .vy paths, so it
// can be filtered into a traceback naming the program's own lines.
func TestRuntimeErrorsAreVeylShaped(t *testing.T) {
	veyl := buildCompiler(t)
	dir := t.TempDir()

	src := filepath.Join(dir, "boom.vy")
	prog := `fn inner(n: int) -> int {
    return 100 / n
}

fn outer() -> int {
    return inner(0)
}

print(outer())
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(veyl, "run", src).CombinedOutput()
	if err == nil {
		t.Fatal("dividing by zero should fail")
	}
	got := strings.ReplaceAll(string(out), "\r\n", "\n")

	// Go's wording, translated.
	if !strings.Contains(got, "error: divided by zero") {
		t.Errorf("missing the explained message, got:\n%s", got)
	}
	// Innermost frame first, then the caller: a real traceback.
	if !strings.Contains(got, "at boom.vy:2") || !strings.Contains(got, "at boom.vy:6") {
		t.Errorf("missing the Veyl traceback, got:\n%s", got)
	}
	// None of Go's internals should survive.
	for _, leak := range []string{"goroutine", "runtime.", "0x"} {
		if strings.Contains(got, leak) {
			t.Errorf("Go internals leaked (%q) into:\n%s", leak, got)
		}
	}

	// The full Go stack stays available for debugging the compiler.
	cmd := exec.Command(veyl, "run", src)
	cmd.Env = append(os.Environ(), "VEYL_TRACE=1")
	traced, _ := cmd.CombinedOutput()
	if !strings.Contains(string(traced), "goroutine") {
		t.Errorf("VEYL_TRACE=1 should show the Go stack, got:\n%s", traced)
	}
}

// The console rebuilds and reruns the whole session on every line, so
// the two things that can go wrong are showing output twice and letting
// a line that does not compile into the session.
func TestConsole(t *testing.T) {
	veyl := buildCompiler(t)

	feed := func(t *testing.T, input string) string {
		t.Helper()
		cmd := exec.Command(veyl, "console")
		cmd.Stdin = strings.NewReader(input)
		// No console attached, so colour is off and the output is plain.
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("console failed: %v\n%s", err, out)
		}
		return strings.ReplaceAll(string(out), "\r\n", "\n")
	}

	t.Run("evaluates and remembers", func(t *testing.T) {
		got := feed(t, "1 + 1\nlet name = \"veyl\"\n\"hi, {name}\"\n:quit\n")
		for _, want := range []string{"2", "hi, veyl"} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
	})

	t.Run("output is not repeated", func(t *testing.T) {
		// Rerunning the session reprints everything; only the new part
		// should reach the screen.
		got := feed(t, "print(\"once\")\nprint(\"twice\")\n:quit\n")
		if n := strings.Count(got, "once"); n != 1 {
			t.Errorf("printed \"once\" %d times, want 1:\n%s", n, got)
		}
	})

	t.Run("warnings stay out of the way", func(t *testing.T) {
		// A variable declared now and used on the next line would warn
		// on every rebuild.
		got := feed(t, "let unused = 5\n:quit\n")
		if strings.Contains(got, "never used") {
			t.Errorf("a warning leaked into the console:\n%s", got)
		}
	})

	t.Run("a bad line is refused, not kept", func(t *testing.T) {
		got := feed(t, "let a = 5\nnope()\na * 2\n:list\n:quit\n")
		if !strings.Contains(got, `undefined function "nope"`) {
			t.Errorf("expected the error to be reported:\n%s", got)
		}
		if !strings.Contains(got, "10") {
			t.Errorf("the session should still work after a bad line:\n%s", got)
		}
		if strings.Contains(got, "nope()") {
			t.Errorf("the bad line was kept in the session:\n%s", got)
		}
	})

	t.Run("a Go backend rejection is also refused", func(t *testing.T) {
		// `1 / 0` passes Veyl's checker and is caught by Go. Keeping
		// it would leave a session that can never compile again.
		got := feed(t, "let a = 5\n1/0\na * 2\n:quit\n")
		if !strings.Contains(got, "division by zero") {
			t.Errorf("expected the rejection to be reported:\n%s", got)
		}
		if !strings.Contains(got, "10") {
			t.Errorf("the session should still work afterwards:\n%s", got)
		}
		if strings.Contains(got, "session.vy") || strings.Contains(got, "Temp") {
			t.Errorf("the temporary path leaked into the message:\n%s", got)
		}
	})

	t.Run("brackets hold the prompt open", func(t *testing.T) {
		got := feed(t, "fn twice(n: int) -> int {\n    return n * 2\n}\ntwice(21)\n:quit\n")
		if !strings.Contains(got, "42") {
			t.Errorf("a multi-line function was not accepted:\n%s", got)
		}
	})
}
