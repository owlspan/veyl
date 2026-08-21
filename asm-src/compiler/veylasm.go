package main

// The driver for the assembly backend.
//
//     veylasm run   f.vl    compile and run
//     veylasm build f.vl    write an executable next to the source
//     veylasm asm   f.vl    print the generated assembly
//     veylasm ir    f.vl    print the intermediate representation
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
  veylasm run   <file.vl>    compile and run
  veylasm build <file.vl>    compile to an executable next to the source
  veylasm asm   <file.vl>    print the generated assembly
  veylasm ir    <file.vl>    print the intermediate representation
  veylasm version            print the version

This backend handles a subset of Veyl: integers, floats, bools, strings,
lists, let, assignment, control flow, functions, and print. Everything
else is a clear compile error rather than wrong output. The Go backend
in ../src remains the complete one.
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
	// `veylasm f.vl` means `veylasm run f.vl`, matching the Go backend.
	// A first argument ending in .vl can only be a file, since no command
	// does, so this cannot shadow one.
	if strings.HasSuffix(cmd, ".vl") {
		args = append([]string{"run"}, args...)
		cmd = "run"
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

	// Imported files are folded in before anything is checked, so from
	// here down there is one program and nothing else has to know that
	// more than one file was involved.
	stampImportedFile(prog, path)
	if errs := loadImports(prog, path); len(errs) > 0 {
		report(errs)
	}

	// The shared type checker, with this backend's own library. A
	// builtin this backend does not have is now an ordinary type error
	// with a position, reported alongside everything else, rather than
	// something the lowerer stumbles into later.
	ck := NewChecker(path, asmLibrary{})
	ck.Check(prog)
	if len(ck.Errors) > 0 {
		report(ck.Errors)
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

	as, cc, binDir := findToolchain()

	if outp, err := toolchainCmd(binDir, as, asmPath, "-o", objPath).CombinedOutput(); err != nil {
		fail("the assembler rejected the generated code. This is a compiler "+
			"bug, not a mistake in your program.\n%s\n%s", err, outp)
	}
	if outp, err := toolchainCmd(binDir, cc, objPath, "-o", out).CombinedOutput(); err != nil {
		fail("linking failed.\n%s\n%s", err, outp)
	}
}

// toolchainCmd runs a MinGW tool with MinGW's bin directory on PATH.
//
// Finding gcc.exe by absolute path is not enough. gcc spawns collect2,
// which spawns ld, and ld lives in a different directory from the DLLs
// it needs. With MinGW off PATH - the normal state of a Windows machine
// that has it - ld cannot load them and exits 53, and the only thing
// that reaches the surface is "ld returned 53 exit status".
//
// This cost a real debugging session because it depends on the shell:
// Git Bash happens to carry compatible DLLs on PATH, so the same command
// worked there and failed from cmd.
//
// The Go backend has the same shape of problem and answers it the same
// way, by setting GOROOT for its child. Nothing is added to this
// process's own PATH, so a developer's own toolchain stays unshadowed.
func toolchainCmd(binDir, exe string, args ...string) *exec.Cmd {
	cmd := exec.Command(exe, args...)
	if binDir == "" {
		return cmd
	}
	env := os.Environ()
	for i, kv := range env {
		if len(kv) >= 5 && strings.EqualFold(kv[:5], "PATH=") {
			env[i] = "PATH=" + binDir + string(os.PathListSeparator) + kv[5:]
			cmd.Env = env
			return cmd
		}
	}
	cmd.Env = append(env, "PATH="+binDir)
	return cmd
}

// findToolchain locates the assembler and linker. MinGW is not usually
// on PATH on a Windows machine that has it, so the known install
// locations are checked too, the same way findGo does on the Go backend.
func findToolchain() (as string, cc string, binDir string) {
	if env := os.Getenv("VEYL_MINGW"); env != "" {
		return filepath.Join(env, "as.exe"), filepath.Join(env, "gcc.exe"), env
	}

	if a, err := exec.LookPath("as"); err == nil {
		if c, err := exec.LookPath("gcc"); err == nil {
			return a, c, filepath.Dir(c)
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
			return a, c, dir
		}
	}

	fail("cannot find an assembler. veylasm needs MinGW's `as` and `gcc`.\n" +
		"Install MSYS2 and its mingw-w64 toolchain, or set VEYL_MINGW to the\n" +
		"folder holding as.exe and gcc.exe.")
	return "", "", ""
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
