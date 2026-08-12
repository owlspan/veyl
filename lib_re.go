package main

// The re library: regular expressions.
//
// A pattern that does not compile is treated as a mistake in the
// program rather than a runtime condition, so it stops the program with
// a message naming the pattern and what was wrong with it. That is a
// departure from the rest of the library, which returns `T!`, and it is
// deliberate: patterns are almost always literals the author wrote, and
// `must(re.matches("^a", s))` on every call would be noise for a case
// that only fires when the code is wrong. `re.valid()` is there for the
// rare pattern that comes from input.
//
// Compiled patterns are cached, so using one inside a loop does not
// recompile it every iteration.

var reHelperDefs = map[string]helperDef{
	"reCompile": {
		code: `var (
	__reCache = map[string]*regexp.Regexp{}
	__reMutex sync.Mutex
)

func __re(pattern string) *regexp.Regexp {
	__reMutex.Lock()
	defer __reMutex.Unlock()
	if rx, ok := __reCache[pattern]; ok {
		return rx
	}
	rx, err := regexp.Compile(pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "runtime error: %q is not a valid pattern: %v\n", pattern, err)
		os.Exit(1)
	}
	__reCache[pattern] = rx
	return rx
}`,
		imports: []string{"fmt", "os", "regexp", "sync"},
	},

	"reHelpers": {
		code: `func __reFind(pattern string, text string) string {
	return __re(pattern).FindString(text)
}

func __reFindAll(pattern string, text string) []string {
	out := __re(pattern).FindAllString(text, -1)
	if out == nil {
		return []string{}
	}
	return out
}

// The first match's capture groups, without the whole match at index
// zero - that is what the user asked for separately with find().
func __reGroups(pattern string, text string) []string {
	m := __re(pattern).FindStringSubmatch(text)
	if len(m) < 2 {
		return []string{}
	}
	out := make([]string, len(m)-1)
	copy(out, m[1:])
	return out
}

func __reSplit(pattern string, text string) []string {
	out := __re(pattern).Split(text, -1)
	if out == nil {
		return []string{}
	}
	return out
}

func __reCount(pattern string, text string) int {
	return len(__re(pattern).FindAllString(text, -1))
}`,
		deps: []string{"reCompile"},
	},
}

var reBuiltins map[string]builtin

func buildReBuiltins() {
	fn := func(goFn string, params []*Type, ret *Type) builtin {
		return builtin{
			emit: func(a []string) string {
				out := goFn + "("
				for i, arg := range a {
					if i > 0 {
						out += ", "
					}
					out += arg
				}
				return out + ")"
			},
			params: params, ret: ret,
			minArgs: len(params), maxArgs: len(params),
			helpers: []string{"reHelpers"},
		}
	}

	reBuiltins = map[string]builtin{
		"re.matches": {
			emit:   func(a []string) string { return "__re(" + a[0] + ").MatchString(" + a[1] + ")" },
			params: []*Type{Str, Str}, ret: Bool, minArgs: 2, maxArgs: 2,
			helpers: []string{"reCompile"},
		},
		"re.find":    fn("__reFind", []*Type{Str, Str}, Str),
		"re.findAll": fn("__reFindAll", []*Type{Str, Str}, ListOf(Str)),
		"re.groups":  fn("__reGroups", []*Type{Str, Str}, ListOf(Str)),
		"re.split":   fn("__reSplit", []*Type{Str, Str}, ListOf(Str)),
		"re.count":   fn("__reCount", []*Type{Str, Str}, Int),

		"re.replace": {
			emit: func(a []string) string {
				return "__re(" + a[0] + ").ReplaceAllString(" + a[1] + ", " + a[2] + ")"
			},
			params: []*Type{Str, Str, Str}, ret: Str, minArgs: 3, maxArgs: 3,
			helpers: []string{"reCompile"},
		},

		// The one that does not stop the program, for a pattern that came
		// from somewhere other than the source code.
		"re.valid": {
			emit: func(a []string) string {
				return "func() bool { _, err := regexp.Compile(" + a[0] + "); return err == nil }()"
			},
			params: []*Type{Str}, ret: Bool, minArgs: 1, maxArgs: 1,
			imports: []string{"regexp"},
		},

		// Escaping, for building a pattern out of text that should be
		// matched literally.
		"re.escape": {
			emit:   func(a []string) string { return "regexp.QuoteMeta(" + a[0] + ")" },
			params: []*Type{Str}, ret: Str, minArgs: 1, maxArgs: 1,
			imports: []string{"regexp"},
		},
	}
}

func registerRe() {
	buildReBuiltins()
	registerNamespace("re")
	for k, v := range reHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range reBuiltins {
		builtins[k] = v
	}
}
