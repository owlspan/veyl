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

// The test suite is data-driven. Each case is a .qz file paired with a
// .expected file:
//
//	tests/ok/NAME.qz   + tests/ok/NAME.expected    program output
//	tests/err/NAME.qz  + tests/err/NAME.expected   compiler errors
//
// Adding a test means adding two files, never touching this one. To
// regenerate the .expected files after an intentional change:
//
//	go test -run TestQuartz -update
//
// Always read the diff before accepting it — that flag is how a bug
// becomes the expected behaviour.

var update = flag.Bool("update", false, "regenerate the .expected golden files")

// buildCompiler compiles the compiler once and returns its path.
func buildCompiler(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "quartz-under-test")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("building the compiler failed: %v\n%s", err, out)
	}
	return exe
}

func TestQuartzOK(t *testing.T) {
	runSuite(t, "tests/ok", false)
}

func TestQuartzErrors(t *testing.T) {
	runSuite(t, "tests/err", true)
}

func runSuite(t *testing.T, dir string, wantFailure bool) {
	quartz := buildCompiler(t)

	cases, err := filepath.Glob(filepath.Join(dir, "*.qz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Skipf("no cases in %s", dir)
	}

	for _, src := range cases {
		src := src
		name := strings.TrimSuffix(filepath.Base(src), ".qz")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.Command(quartz, "run", src)
			cmd.Stdin = strings.NewReader("")
			raw, runErr := cmd.CombinedOutput()
			got := normalize(string(raw), src)

			if wantFailure && runErr == nil {
				t.Fatalf("expected compilation to fail, but it succeeded:\n%s", got)
			}
			if !wantFailure && runErr != nil {
				t.Fatalf("expected success, got %v:\n%s", runErr, got)
			}

			golden := strings.TrimSuffix(src, ".qz") + ".expected"
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
