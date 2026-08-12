package main

import (
	"sort"
	"strings"
)

// The os library: files, directories, paths, environment, time and
// process control, reached through dotted names - os.file.read(...).
//
// Failure is reported one of two ways, and which one is visible from
// the signature:
//
//   - An operation that produces a value returns `T!`, carrying the
//     reason when it fails. `os.file.readOr(p, fallback)` is the
//     variant for when you would rather not deal with it.
//   - An operation that only acts returns a `bool`. There is no unit
//     type to put inside a result, so the reason is lost here - a real
//     gap, and the one thing left to fix in this file.
//
// Nothing in this library kills the program any more. That was the
// stopgap before `T!` existed.

// namespaces are the dotted library roots the compiler knows. Used to
// tell "you misspelled the function" from "that library isn't a thing".
var namespaces = map[string]bool{}

func registerNamespace(names ...string) {
	for _, n := range names {
		namespaces[n] = true
	}
}

func namespaceList() string {
	out := make([]string, 0, len(namespaces))
	for n := range namespaces {
		out = append(out, n)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

var osHelperDefs = map[string]helperDef{
	// One place decides how a failure is worded, so every message in the
	// library reads the same way.
	"qzWhy": {
		code: `func __why(op string, subject string, err error) string {
	return fmt.Sprintf("cannot %s %q: %v", op, subject, err)
}`,
		imports: []string{"fmt"},
	},

	// Kept for the few places that genuinely cannot continue.
	"qzFatal": {
		code: `func __fatal(op string, subject string, err error) {
	fmt.Fprintf(os.Stderr, "runtime error: cannot %s %q: %v\n", op, subject, err)
	os.Exit(1)
}`,
		imports: []string{"fmt", "os"},
	},

	"readFile": {
		code: `func __readFile(path string) __Res[string] {
	b, err := os.ReadFile(path)
	if err != nil {
		return __fail[string](__why("read", path, err))
	}
	return __ok(string(b))
}`,
		imports: []string{"os"},
		deps:    []string{"qzWhy", "result"},
	},
	"readFileOr": {
		code: `func __readFileOr(path string, fallback string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	return string(b)
}`,
		imports: []string{"os"},
	},
	"writeFile": {
		code: `func __writeFile(path string, text string) __Res[__Unit] {
	return __try(os.WriteFile(path, []byte(text), 0o644))
}`,
		imports: []string{"os"},
		deps:    []string{"try"},
	},
	"appendFile": {
		code: `func __appendFile(path string, text string) __Res[__Unit] {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return __fail[__Unit](err.Error())
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return __try(err)
}`,
		imports: []string{"os"},
		deps:    []string{"try"},
	},

	// __try turns Go's error convention into Veyl's. Every fallible
	// action that produces no value goes through it, so the reason a
	// write failed -- permission denied, no such directory, disk full --
	// reaches the program instead of collapsing into false.
	"try": {
		code: `func __try(err error) __Res[__Unit] {
	if err != nil {
		return __fail[__Unit](err.Error())
	}
	return __ok(__unit)
}`,
		deps: []string{"result"},
	},
	"readLines": {
		code: `func __readLines(path string) __Res[[]string] {
	r := __readFile(path)
	if r.e != "" {
		return __fail[[]string](r.e)
	}
	text := strings.ReplaceAll(r.v, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return __ok([]string{})
	}
	return __ok(strings.Split(text, "\n"))
}`,
		imports: []string{"strings"},
		deps:    []string{"readFile", "result"},
	},
	"fileSize": {
		code: `func __fileSize(path string) __Res[int] {
	info, err := os.Stat(path)
	if err != nil {
		return __fail[int](__why("measure", path, err))
	}
	return __ok(int(info.Size()))
}`,
		imports: []string{"os"},
		deps:    []string{"qzWhy", "result"},
	},
	"listDir": {
		code: `func __listDir(path string) __Res[[]string] {
	entries, err := os.ReadDir(path)
	if err != nil {
		return __fail[[]string](__why("list", path, err))
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return __ok(out)
}`,
		imports: []string{"os", "sort"},
		deps:    []string{"qzWhy", "result"},
	},
	"isDir": {
		code: `func __isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}`,
		imports: []string{"os"},
	},
	"runCmd": {
		// Output and exit status are separate calls rather than a pair,
		// since Veyl has no multiple returns yet. Both capture stdout
		// and stderr together, which is what a script usually wants.
		//
		// A non-zero exit is a failure, and the captured output is put in
		// the message: a command that failed has usually already said why.
		code: `func __runCmd(name string, args []string) __Res[string] {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return __fail[string](fmt.Sprintf("cannot run %q: %s", name, detail))
	}
	return __ok(string(out))
}`,
		imports: []string{"fmt", "os/exec", "strings"},
		deps:    []string{"result"},
	},
	"runCode": {
		code: `func __runCode(name string, args []string) int {
	cmd := exec.Command(name, args...)
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return -1
	}
	return 0
}`,
		imports: []string{"os/exec"},
	},
	"osArgs": {
		code: `func __args() []string {
	if len(os.Args) < 2 {
		return []string{}
	}
	out := make([]string, len(os.Args)-1)
	copy(out, os.Args[1:])
	return out
}`,
		imports: []string{"os"},
	},
}

var osBuiltins map[string]builtin

func buildOsBuiltins() {
	// A tiny constructor keeps the table readable: most entries are just
	// "call this helper with these argument types".
	fn := func(goFn string, params []*Type, ret *Type, helper string, imports ...string) builtin {
		return builtin{
			emit: func(a []string) string {
				return goFn + "(" + strings.Join(a, ", ") + ")"
			},
			params: params, ret: ret,
			minArgs: len(params), maxArgs: len(params),
			helpers: []string{helper},
			imports: imports,
		}
	}
	direct := func(goFn string, params []*Type, ret *Type, imports ...string) builtin {
		return builtin{
			emit: func(a []string) string {
				return goFn + "(" + strings.Join(a, ", ") + ")"
			},
			params: params, ret: ret,
			minArgs: len(params), maxArgs: len(params),
			imports: imports,
		}
	}

	osBuiltins = map[string]builtin{

		// ---- files ----

		"os.file.read":   fn("__readFile", []*Type{Str}, ResultOf(Str), "readFile"),
		"os.file.readOr": fn("__readFileOr", []*Type{Str, Str}, Str, "readFileOr"),
		"os.file.lines":  fn("__readLines", []*Type{Str}, ResultOf(ListOf(Str)), "readLines"),
		"os.file.write":  fn("__writeFile", []*Type{Str, Str}, ResultOf(Void), "writeFile"),
		"os.file.append": fn("__appendFile", []*Type{Str, Str}, ResultOf(Void), "appendFile"),
		"os.file.size":   fn("__fileSize", []*Type{Str}, ResultOf(Int), "fileSize"),
		"os.file.exists": {
			emit: func(a []string) string {
				return "func() bool { _, err := os.Stat(" + a[0] + "); return err == nil }()"
			},
			params: []*Type{Str}, ret: Bool, minArgs: 1, maxArgs: 1,
			imports: []string{"os"},
		},
		"os.file.delete": {
			emit:   func(a []string) string { return "__try(os.Remove(" + a[0] + "))" },
			params: []*Type{Str}, ret: ResultOf(Void), minArgs: 1, maxArgs: 1,
			imports: []string{"os"}, helpers: []string{"try"},
		},
		"os.file.rename": {
			emit:   func(a []string) string { return "__try(os.Rename(" + a[0] + ", " + a[1] + "))" },
			params: []*Type{Str, Str}, ret: ResultOf(Void), minArgs: 2, maxArgs: 2,
			imports: []string{"os"}, helpers: []string{"try"},
		},

		// ---- directories ----

		"os.dir.list": fn("__listDir", []*Type{Str}, ResultOf(ListOf(Str)), "listDir"),
		"os.dir.is":   fn("__isDir", []*Type{Str}, Bool, "isDir"),
		"os.dir.make": {
			emit:   func(a []string) string { return "__try(os.MkdirAll(" + a[0] + ", 0o755))" },
			params: []*Type{Str}, ret: ResultOf(Void), minArgs: 1, maxArgs: 1,
			imports: []string{"os"}, helpers: []string{"try"},
		},
		"os.dir.delete": {
			emit:   func(a []string) string { return "__try(os.RemoveAll(" + a[0] + "))" },
			params: []*Type{Str}, ret: ResultOf(Void), minArgs: 1, maxArgs: 1,
			imports: []string{"os"}, helpers: []string{"try"},
		},
		"os.dir.current": {
			emit:    func(a []string) string { return "func() string { d, _ := os.Getwd(); return d }()" },
			ret:     Str,
			imports: []string{"os"},
		},
		"os.dir.change": {
			emit:   func(a []string) string { return "__try(os.Chdir(" + a[0] + "))" },
			params: []*Type{Str}, ret: ResultOf(Void), minArgs: 1, maxArgs: 1,
			imports: []string{"os"}, helpers: []string{"try"},
		},
		"os.dir.temp": {
			emit:    func(a []string) string { return "os.TempDir()" },
			ret:     Str,
			imports: []string{"os"},
		},
		"os.dir.home": {
			emit:    func(a []string) string { return "func() string { d, _ := os.UserHomeDir(); return d }()" },
			ret:     Str,
			imports: []string{"os"},
		},

		// ---- paths ----
		// Pure string manipulation: these never touch the filesystem.

		"os.path.join": {
			emit:    func(a []string) string { return "filepath.Join(" + strings.Join(a, ", ") + ")" },
			rest:    Str,
			ret:     Str,
			minArgs: 1, maxArgs: -1,
			imports: []string{"path/filepath"},
		},
		"os.path.base":  direct("filepath.Base", []*Type{Str}, Str, "path/filepath"),
		"os.path.dir":   direct("filepath.Dir", []*Type{Str}, Str, "path/filepath"),
		"os.path.ext":   direct("filepath.Ext", []*Type{Str}, Str, "path/filepath"),
		"os.path.clean": direct("filepath.Clean", []*Type{Str}, Str, "path/filepath"),
		"os.path.absolute": {
			emit: func(a []string) string {
				return "func() string { p, err := filepath.Abs(" + a[0] + "); if err != nil { return " + a[0] + " }; return p }()"
			},
			params: []*Type{Str}, ret: Str, minArgs: 1, maxArgs: 1,
			imports: []string{"path/filepath"},
		},

		// ---- environment and process ----

		"os.env.get": {
			emit:   func(a []string) string { return "os.Getenv(" + a[0] + ")" },
			params: []*Type{Str}, ret: Str, minArgs: 1, maxArgs: 1,
			imports: []string{"os"},
		},
		"os.env.set": {
			emit:   func(a []string) string { return "__try(os.Setenv(" + a[0] + ", " + a[1] + "))" },
			params: []*Type{Str, Str}, ret: ResultOf(Void), minArgs: 2, maxArgs: 2,
			imports: []string{"os"}, helpers: []string{"try"},
		},
		"os.env.has": {
			emit: func(a []string) string {
				return "func() bool { _, ok := os.LookupEnv(" + a[0] + "); return ok }()"
			},
			params: []*Type{Str}, ret: Bool, minArgs: 1, maxArgs: 1,
			imports: []string{"os"},
		},

		"os.args": {
			emit:    func(a []string) string { return "__args()" },
			ret:     ListOf(Str),
			helpers: []string{"osArgs"},
		},
		"os.name": {
			emit:    func(a []string) string { return "runtime.GOOS" },
			ret:     Str,
			imports: []string{"runtime"},
		},
		"os.arch": {
			emit:    func(a []string) string { return "runtime.GOARCH" },
			ret:     Str,
			imports: []string{"runtime"},
		},
		"os.cpus": {
			emit:    func(a []string) string { return "runtime.NumCPU()" },
			ret:     Int,
			imports: []string{"runtime"},
		},
		"os.pid": {
			emit:    func(a []string) string { return "os.Getpid()" },
			ret:     Int,
			imports: []string{"os"},
		},
		"os.hostname": {
			emit:    func(a []string) string { return "func() string { h, _ := os.Hostname(); return h }()" },
			ret:     Str,
			imports: []string{"os"},
		},

		"os.run": {
			emit: func(a []string) string {
				return "__runCmd(" + a[0] + ", " + a[1] + ")"
			},
			params: []*Type{Str, ListOf(Str)}, ret: ResultOf(Str),
			minArgs: 2, maxArgs: 2,
			helpers: []string{"runCmd"},
		},
		"os.runCode": {
			emit: func(a []string) string {
				return "__runCode(" + a[0] + ", " + a[1] + ")"
			},
			params: []*Type{Str, ListOf(Str)}, ret: Int,
			minArgs: 2, maxArgs: 2,
			helpers: []string{"runCode"},
		},
	}

	// The user's own shorthand reads verb-first. Both spellings resolve
	// to the same builtin, so os.read.file and os.file.read are the same
	// call; the noun-first form is the documented one because it groups.
	for alias, canonical := range map[string]string{
		"os.read.file":   "os.file.read",
		"os.write.file":  "os.file.write",
		"os.append.file": "os.file.append",
		"os.delete.file": "os.file.delete",
		"os.list.dir":    "os.dir.list",
		"os.make.dir":    "os.dir.make",
	} {
		osBuiltins[alias] = osBuiltins[canonical]
	}
}

func registerOs() {
	buildOsBuiltins()
	registerNamespace("os")
	for k, v := range osHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range osBuiltins {
		builtins[k] = v
	}
}
