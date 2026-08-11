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

// Version is stamped into `quartz version`. Bump it with the tag.
const Version = "0.14"

const usage = `Quartz ` + Version + ` — a small language that compiles to native executables

usage:
  quartz run    <file.qz>    compile and run
  quartz build  <file.qz>    compile to an executable next to the source
  quartz fmt    <file.qz>    reformat the file in place
  quartz emit   <file.qz>    print the generated Go
  quartz tokens <file.qz>    print the token stream
  quartz builtins            list every builtin, for editor tooling
  quartz version             print the version
  quartz help                print this

  quartz <file.qz>           same as 'run'

environment:
  QUARTZ_TARGET=windows      cross-compile for another OS
  QUARTZ_QUIET=1             suppress warnings

Go must be installed: Quartz hands its generated code to the Go
toolchain. Language reference: SYNTAX.md
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(64)
	}

	// Anything after the .qz file belongs to the program being run, not
	// to the compiler, so `quartz run app.qz --verbose` passes
	// --verbose through to os.args rather than trying to interpret it.
	cmd, path := "run", ""
	var progArgs []string
	switch args[0] {
	case "run", "build", "emit", "tokens", "fmt":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "quartz: %s needs a file\n", args[0])
			os.Exit(64)
		}
		cmd, path, progArgs = args[0], args[1], args[2:]
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	case "-v", "--version", "version":
		fmt.Printf("quartz %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
		return
	case "builtins":
		// Exists so editor tooling can be generated from the compiler
		// rather than transcribed from it and left to drift.
		printBuiltins()
		return
	default:
		path, progArgs = args[0], args[1:]
	}

	if err := run(cmd, path, progArgs); err != nil {
		fmt.Fprintf(os.Stderr, "quartz: %v\n", err)
		os.Exit(1)
	}
}

func run(cmd, path string, progArgs []string) error {
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

	// ---- format ----
	// Formatting happens before anything else, because it only needs the
	// file to lex, not to make sense. Reformatting a program you are
	// halfway through writing is exactly when you want it most.
	if cmd == "fmt" {
		return formatFile(abs, name, src)
	}

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
	stampFile(prog, abs)
	prog.MainFile = abs

	// ---- load imports ----
	loader := &loader{seen: map[string]bool{abs: true}}
	loader.resolve(prog, abs)
	if err := reportErrors(loader.errors); err != nil {
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

	// Warnings are held back until the program is known to compile.
	// Stacking "unused variable" on top of a dozen type errors buries
	// the thing that actually needs fixing.
	reportWarnings(rs.Warnings)

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
	exe := exec.Command(outPath, progArgs...)
	exe.Stdin, exe.Stdout, exe.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := exe.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	return nil
}

// printBuiltins lists every builtin name, one per line, bare names
// first and then the dotted library paths.
func printBuiltins() {
	var bare, dotted []string
	for name := range builtins {
		if strings.Contains(name, ".") {
			dotted = append(dotted, name)
		} else {
			bare = append(bare, name)
		}
	}
	for name := range builtinConsts {
		bare = append(bare, name)
	}
	sort.Strings(bare)
	sort.Strings(dotted)
	for _, n := range append(bare, dotted...) {
		fmt.Println(n)
	}
}

// formatFile rewrites a file in place, or leaves it alone and says why.
// Writing only when something changed keeps timestamps stable, which
// matters to anything watching the file.
func formatFile(abs, name, src string) error {
	out, ok := Format(name, src)
	if !ok {
		return fmt.Errorf("%s does not lex cleanly, so it was left alone — "+
			"fix the syntax error first, then format", name)
	}
	if out == src {
		fmt.Printf("%s is already formatted\n", name)
		return nil
	}
	if err := os.WriteFile(abs, []byte(out), 0o644); err != nil {
		return err
	}
	fmt.Printf("formatted %s\n", name)
	return nil
}

// ---- modules ----
//
// An import loads another .qz file and folds its declarations into the
// same program. There is no module registry, no search path and no
// package naming: a path is a path, relative to the file that wrote it.
//
// Imported files may only declare things. A statement at the top level
// of an imported file would have to run at some point, and there is no
// module-initialisation order worth inventing, so it is refused.

type loader struct {
	seen   map[string]bool // absolute paths already loaded
	stack  []string        // in-progress, for cycle detection
	errors []string
}

func (l *loader) resolve(prog *Program, from string) {
	for _, imp := range prog.Imports {
		target := imp.Path
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(from), target)
		}
		target, err := filepath.Abs(target)
		if err != nil {
			l.errorAt(imp, "cannot resolve %q: %v", imp.Path, err)
			continue
		}
		if !strings.HasSuffix(target, ".qz") {
			l.errorAt(imp, "an import must name a .qz file, got %q", imp.Path)
			continue
		}
		if l.onStack(target) {
			l.errorAt(imp, "import cycle: %s imports itself, directly or indirectly",
				filepath.Base(target))
			continue
		}
		if l.seen[target] {
			continue // already folded in; importing twice is harmless
		}
		l.seen[target] = true

		sub, ok := l.load(imp, target)
		if !ok {
			continue
		}

		l.stack = append(l.stack, target)
		l.resolve(sub, target)
		l.stack = l.stack[:len(l.stack)-1]

		prog.Structs = append(prog.Structs, sub.Structs...)
		prog.Funcs = append(prog.Funcs, sub.Funcs...)
		prog.Globals = append(prog.Globals, sub.Globals...)
	}
}

// load parses one imported file. Its top-level statements are refused
// rather than silently dropped.
func (l *loader) load(imp *ImportDecl, target string) (*Program, bool) {
	srcBytes, err := os.ReadFile(target)
	if err != nil {
		// The path as written, not the resolved one. The absolute path
		// is noise here, and it would make error output differ between
		// machines for no reason.
		if os.IsNotExist(err) {
			l.errorAt(imp, "cannot find %q next to %s", imp.Path, filepath.Base(imp.File))
		} else {
			l.errorAt(imp, "cannot read %q", imp.Path)
		}
		return nil, false
	}

	name := filepath.Base(target)
	lx := NewLexer(name, string(srcBytes))
	toks := lx.Scan()
	l.errors = append(l.errors, lx.Errors...)

	ps := NewParser(name, toks)
	sub := ps.ParseProgram()
	l.errors = append(l.errors, ps.Errors...)

	for _, s := range sub.Main {
		line, col := s.Pos()
		l.errors = append(l.errors, fmt.Sprintf(
			"%s:%d:%d: an imported file can only declare things — "+
				"move this statement into the program that imports it", name, line, col))
	}

	stampFile(sub, target)
	return sub, true
}

func (l *loader) onStack(path string) bool {
	for _, p := range l.stack {
		if p == path {
			return true
		}
	}
	return false
}

func (l *loader) errorAt(n Node, format string, args ...any) {
	line, col := n.Pos()
	file := "?"
	if imp, ok := n.(*ImportDecl); ok {
		file = imp.File
	}
	l.errors = append(l.errors,
		fmt.Sprintf("%s:%d:%d: %s", file, line, col, fmt.Sprintf(format, args...)))
}

// stampFile records which file every declaration came from, using the
// absolute path as the identity. That is what lets `pub` be enforced
// across files that happen to share a base name.
func stampFile(prog *Program, abs string) {
	for _, d := range prog.Structs {
		d.File = abs
	}
	for _, f := range prog.Funcs {
		f.File = abs
	}
	for _, g := range prog.Globals {
		g.File = abs
	}
}

// reportWarnings prints things worth saying that are not worth
// stopping for. Set QUARTZ_QUIET to silence them — useful when a
// warning is known and the noise is in the way.
func reportWarnings(warnings []string) {
	if len(warnings) == 0 || os.Getenv("QUARTZ_QUIET") != "" {
		return
	}
	sortByPosition(warnings)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
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
