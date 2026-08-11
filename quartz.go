package main

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

const usage = `Quartz compiler

usage:
  quartz run    <file.qz>    compile and run
  quartz build  <file.qz>    compile to an executable next to the source
  quartz emit   <file.qz>    print the generated Go
  quartz tokens <file.qz>    print the token stream

  quartz <file.qz>           same as 'run'
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(64)
	}

	cmd, path := "run", ""
	switch args[0] {
	case "run", "build", "emit", "tokens":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "quartz: %s needs a file\n", args[0])
			os.Exit(64)
		}
		cmd, path = args[0], args[1]
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		path = args[0]
	}

	if err := run(cmd, path); err != nil {
		fmt.Fprintf(os.Stderr, "quartz: %v\n", err)
		os.Exit(1)
	}
}

func run(cmd, path string) error {
	if !strings.HasSuffix(path, ".qz") {
		return fmt.Errorf("%s is not a .qz file", path)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	srcBytes, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	src := string(srcBytes)
	name := filepath.Base(abs)

	// ---- lex ----
	lx := NewLexer(name, src)
	toks := lx.Scan()

	if cmd == "tokens" {
		for _, t := range toks {
			fmt.Println(t)
		}
		return reportErrors(lx.Errors)
	}
	if err := reportErrors(lx.Errors); err != nil {
		return err
	}

	// ---- parse ----
	ps := NewParser(name, toks)
	prog := ps.ParseProgram()
	if err := reportErrors(ps.Errors); err != nil {
		return err
	}

	// ---- resolve ----
	rs := NewResolver(name)
	rs.Resolve(prog)
	if err := reportErrors(rs.Errors); err != nil {
		return err
	}

	// ---- type check ----
	ck := NewChecker(name)
	ck.Check(prog)
	if err := reportErrors(ck.Errors); err != nil {
		return err
	}

	// ---- codegen ----
	target := os.Getenv("QUARTZ_TARGET")
	if target == "" {
		target = runtime.GOOS
	}
	cg := NewCodegen(abs, target)
	goSrc := cg.Generate(prog)
	if err := reportErrors(cg.Errors); err != nil {
		return err
	}

	if cmd == "emit" {
		fmt.Print(goSrc)
		return nil
	}

	// ---- hand off to the Go toolchain ----
	exeName := strings.TrimSuffix(name, ".qz")
	if target == "windows" {
		exeName += ".exe"
	}

	tmp, err := os.MkdirTemp("", "quartz-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte(goSrc), 0o644); err != nil {
		return err
	}
	gomod := "module qzprog\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(gomod), 0o644); err != nil {
		return err
	}

	outPath := filepath.Join(tmp, exeName)
	build := exec.Command("go", "build", "-o", outPath, ".")
	build.Dir = tmp
	if target != runtime.GOOS {
		build.Env = append(os.Environ(), "GOOS="+target)
	}
	build.Stderr = os.Stderr
	build.Stdout = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("the Go backend rejected the generated program (see above). "+
			"This is a compiler bug, not a mistake in %s — the type checker should have "+
			"caught it first. Run 'quartz emit %s' to see what was generated.", path, path)
	}

	if cmd == "build" {
		final := filepath.Join(filepath.Dir(abs), exeName)
		if err := moveFile(outPath, final); err != nil {
			return err
		}
		fmt.Printf("built %s\n", final)
		return nil
	}

	if target != runtime.GOOS {
		return fmt.Errorf("cannot run a %s executable on %s — use 'quartz build' instead", target, runtime.GOOS)
	}

	// cmd == "run"
	exe := exec.Command(outPath)
	exe.Stdin, exe.Stdout, exe.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := exe.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	return nil
}

func reportErrors(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	sortByPosition(errs)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "error: %s\n", e)
	}
	return fmt.Errorf("%d error(s)", len(errs))
}

// sortByPosition orders "file:line:col: msg" strings numerically, so a
// run of errors reads top-to-bottom in source order rather than in
// whatever order the compiler passes happened to find them.
func sortByPosition(errs []string) {
	key := func(s string) (int, int) {
		parts := strings.Split(s, ":")
		if len(parts) < 3 {
			return 1 << 30, 0
		}
		line, err1 := strconv.Atoi(parts[len(parts)-3])
		col, err2 := strconv.Atoi(parts[len(parts)-2])
		if err1 != nil || err2 != nil {
			// Windows paths start with "C:", shifting the field offsets.
			for i := 0; i+2 < len(parts); i++ {
				if l, e1 := strconv.Atoi(parts[i]); e1 == nil {
					if c, e2 := strconv.Atoi(parts[i+1]); e2 == nil {
						return l, c
					}
				}
			}
			return 1 << 30, 0
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

// moveFile falls back to copy+delete because os.Rename fails across
// drives or filesystems (the temp dir is often on a different volume).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return err
	}
	return os.Remove(src)
}
