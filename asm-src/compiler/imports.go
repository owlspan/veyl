package main

// Multi-file programs: `import "helpers.vl"`.
//
// A file import folds another file's declarations into this program
// before anything is checked, so from the checker down there is one
// program and nothing else has to know files exist. That is the same
// thing the Go backend's loader does.
//
// What this does not do is the other half of that loader: importing a
// *package* by bare name. That needs the package registry, which lives
// in src/ and is about where a package is installed rather than about
// the language. A bare-word import is a clean error naming the gap
// instead of a mysterious missing file.
//
// This is duplicated from the Go backend rather than shared, which is
// the same papercut as the resolver: both belong in the front end, and
// neither is there yet.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type importLoader struct {
	seen   map[string]bool // absolute paths already folded in
	stack  []string        // in progress, for cycle detection
	errors []string
}

// loadImports pulls in every file this program imports, and every file
// those import, depth first.
func loadImports(prog *Program, from string) []string {
	abs, err := filepath.Abs(from)
	if err != nil {
		abs = from
	}
	l := &importLoader{seen: map[string]bool{abs: true}}
	l.resolve(prog, abs)
	return l.errors
}

func (l *importLoader) resolve(prog *Program, from string) {
	for _, imp := range prog.Imports {
		if !strings.HasSuffix(imp.Path, ".vl") {
			l.errorAt(imp, "importing a package by name is not on the assembly "+
				"backend yet - import the file, as in: import \"mod/geometry.vl\"")
			continue
		}

		target := imp.Path
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(from), target)
		}
		if abs, err := filepath.Abs(target); err == nil {
			target = abs
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
// rather than silently dropped: a file that runs code on import would
// run it at a moment nobody chose.
func (l *importLoader) load(imp *ImportDecl, target string) (*Program, bool) {
	srcBytes, err := os.ReadFile(target)
	if err != nil {
		// The path as written, not the resolved one. The absolute path
		// is noise, and it would make error output differ between
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
			"%s:%d:%d: an imported file can only declare things - "+
				"move this statement into the program that imports it", name, line, col))
	}

	stampImportedFile(sub, target)
	return sub, true
}

func (l *importLoader) onStack(path string) bool {
	for _, p := range l.stack {
		if p == path {
			return true
		}
	}
	return false
}

func (l *importLoader) errorAt(n Node, format string, args ...any) {
	line, col := n.Pos()
	file := "?"
	if imp, ok := n.(*ImportDecl); ok {
		file = imp.File
	}
	l.errors = append(l.errors,
		fmt.Sprintf("%s:%d:%d: %s", file, line, col, fmt.Sprintf(format, args...)))
}

// stampImportedFile records which file every declaration came from,
// using the absolute path as the identity. That is what lets `pub` be
// enforced across files that happen to share a base name.
func stampImportedFile(prog *Program, abs string) {
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
