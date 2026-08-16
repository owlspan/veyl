package main

// The driver for the assembly backend.
//
//     veylasm run   f.vy    compile and run
//     veylasm build f.vy    write an executable next to the source
//     veylasm asm   f.vy    print the generated assembly
//     veylasm ir    f.vy    print the intermediate representation
//
// `asm` and `ir` are the debugging tools, and they matter more here than
// `veyl emit` does on the Go backend: when a register allocator produces
// something wrong, reading the instruction stream is the only way to see
// it. They exist from the first commit for that reason.
//
// Assembling and linking currently shell out to the MinGW toolchain,
// exactly as the Go backend shells out to `go build`. That dependency is
// temporary and is the thing this backend exists to remove: replacing
// x64.go with a byte writer and a PE emitter drops it, and nothing above
// x64.go has to change when that happens.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const Version = "0.1.0-slice"

const usage = `veylasm ` + Version + ` - the Veyl assembly backend

usage:
  veylasm run   <file.vy>    compile and run
  veylasm build <file.vy>    compile to an executable next to the source
  veylasm asm   <file.vy>    print the generated assembly
  veylasm ir    <file.vy>    print the intermediate representation
  veylasm version            print the version

This backend handles a subset of Veyl: integer arithmetic, let, plain
assignment, and print. Everything else is a clear compile error rather
than wrong output. The Go backend in ../src remains the complete one.
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		os.Exit(2)
	}

	cmd := args[0]
	if cmd == "version" {
		fmt.Printf("veylasm %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
		return
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Print(usage)
		return
	}
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "veylasm: %s needs a file\n", cmd)
		os.Exit(2)
	}

	path, err := filepath.Abs(args[1])
	if err != nil {
		fail("%v", err)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		fail("cannot read %s: %v", args[1], err)
	}

	mod := compile(string(source), path)

	switch cmd {
	case "ir":
		fmt.Print(mod)
	case "asm":
		fmt.Print(Emit(mod))
	case "build":
		out := strings.TrimSuffix(path, filepath.Ext(path)) + ".exe"
		buildExe(mod, out)
		fmt.Printf("wrote %s\n", out)
	case "run":
		tmp, err := os.MkdirTemp("", "veylasm-*")
		if err != nil {
			fail("%v", err)
		}
		defer os.RemoveAll(tmp)
		out := filepath.Join(tmp, "prog.exe")
		buildExe(mod, out)
		run := exec.Command(out)
		run.Stdout, run.Stderr, run.Stdin = os.Stdout, os.Stderr, os.Stdin
		if err := run.Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				os.Exit(exit.ExitCode())
			}
			fail("%v", err)
		}
	default:
		fmt.Fprintf(os.Stderr, "veylasm: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

// compile runs the front end and lowers to IR, reporting every error it
// found in source order rather than only the first.
func compile(source, path string) *Module {
	lex := NewLexer(path, source)
	tokens := lex.Scan()

	p := NewParser(path, tokens)
	prog := p.ParseProgram()

	if errs := append(append([]string{}, lex.Errors...), p.Errors...); len(errs) > 0 {
		report(errs)
	}

	mod, lowerErrs := Lower(prog, path)
	if len(lowerErrs) > 0 {
		report(lowerErrs)
	}
	return mod
}

func report(errs []string) {
	sortByPosition(errs)
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, e)
	}
	fmt.Fprintf(os.Stderr, "veylasm: %d error(s)\n", len(errs))
	os.Exit(1)
}

// buildExe writes the assembly, then assembles and links it.
func buildExe(mod *Module, out string) {
	tmp, err := os.MkdirTemp("", "veylasm-build-*")
	if err != nil {
		fail("%v", err)
	}
	defer os.RemoveAll(tmp)

	asmPath := filepath.Join(tmp, "prog.s")
	objPath := filepath.Join(tmp, "prog.o")

	if err := os.WriteFile(asmPath, []byte(Emit(mod)), 0o644); err != nil {
		fail("%v", err)
	}

	as, cc := findToolchain()

	if outp, err := exec.Command(as, asmPath, "-o", objPath).CombinedOutput(); err != nil {
		fail("the assembler rejected the generated code. This is a compiler "+
			"bug, not a mistake in your program.\n%s\n%s", err, outp)
	}
	if outp, err := exec.Command(cc, objPath, "-o", out).CombinedOutput(); err != nil {
		fail("linking failed.\n%s\n%s", err, outp)
	}
}

// findToolchain locates the assembler and linker. MinGW is not usually
// on PATH on a Windows machine that has it, so the known install
// locations are checked too, the same way findGo does on the Go backend.
func findToolchain() (as string, cc string) {
	if env := os.Getenv("VEYL_MINGW"); env != "" {
		return filepath.Join(env, "as.exe"), filepath.Join(env, "gcc.exe")
	}

	if a, err := exec.LookPath("as"); err == nil {
		if c, err := exec.LookPath("gcc"); err == nil {
			return a, c
		}
	}

	for _, dir := range []string{
		`C:\msys64\mingw64\bin`,
		`C:\msys64\ucrt64\bin`,
		`C:\mingw64\bin`,
		`C:\MinGW\bin`,
	} {
		a := filepath.Join(dir, "as.exe")
		c := filepath.Join(dir, "gcc.exe")
		if exists(a) && exists(c) {
			return a, c
		}
	}

	fail("cannot find an assembler. veylasm needs MinGW's `as` and `gcc`.\n" +
		"Install MSYS2 and its mingw-w64 toolchain, or set VEYL_MINGW to the\n" +
		"folder holding as.exe and gcc.exe.")
	return "", ""
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "veylasm: "+format+"\n", args...)
	os.Exit(1)
}

// sortByPosition puts errors in source order. Windows paths carry a
// colon of their own, so the line and column are taken from the end
// rather than by splitting on the first colon.
func sortByPosition(errs []string) {
	key := func(s string) (int, int) {
		parts := strings.Split(s, ":")
		if len(parts) < 3 {
			return 0, 0
		}
		col, err1 := strconv.Atoi(strings.TrimSpace(parts[len(parts)-2]))
		line, err2 := strconv.Atoi(strings.TrimSpace(parts[len(parts)-3]))
		if err1 != nil || err2 != nil {
			return 0, 0
		}
		return line, col
	}
	sort.SliceStable(errs, func(i, j int) bool {
		li, ci := key(errs[i])
		lj, cj := key(errs[j])
		if li != lj {
			return li < lj
		}
		return ci < cj
	})
}
