package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestGolden runs the programs in ../tests and compares what they print
// with the .out file beside each one.
//
// Everything the Go backend can also run belongs in examples/, where the
// differential test holds it to that backend's output instead. This is
// for what it cannot: syntax it never had, like \u{...} escapes, and
// programs that read standard input, which get the .in file beside them
// when there is one.
func TestGolden(t *testing.T) {
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("wine"); err != nil {
			t.Skip("the programs are Windows executables, and there is no wine to run them")
		}
	}
	programs, err := filepath.Glob(filepath.Join("..", "tests", "*.vl"))
	if err != nil || len(programs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	veyl := asmBackend(t)

	for _, src := range programs {
		src := src
		t.Run(filepath.Base(src), func(t *testing.T) {
			t.Parallel()
			base := strings.TrimSuffix(src, ".vl")
			want, err := os.ReadFile(base + ".out")
			if err != nil {
				t.Fatalf("no expected output: %v", err)
			}
			cmd := exec.Command(veyl, "run", src)
			if in, err := os.Open(base + ".in"); err == nil {
				defer in.Close()
				cmd.Stdin = in
			}
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Run(); err != nil {
				t.Fatalf("%v\n%s", err, out.Bytes())
			}
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("output differs\n--- got ---\n%q\n--- want ---\n%q", out.Bytes(), want)
			}
		})
	}
}
