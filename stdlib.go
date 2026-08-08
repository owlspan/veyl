package main

import "strings"

// builtinConst is a name that acts like a value, not a function.
type builtinConst struct {
	code    string
	imports []string
}

var builtinConsts = map[string]builtinConst{
	"PI":  {code: "math.Pi", imports: []string{"math"}},
	"E":   {code: "math.E", imports: []string{"math"}},
	"INF": {code: "math.Inf(1)", imports: []string{"math"}},
	"NAN": {code: "math.NaN()", imports: []string{"math"}},
}

// f wraps an argument in float64() so math functions accept both int and
// float expressions. Converting a float64 to float64 is a no-op in Go,
// so this is safe in both directions — and it's what lets Quartz have
// numeric builtins before it has a real type checker.
func f(arg string) string { return "float64(" + arg + ")" }

// mathFn builds a builtin that forwards to a math package function,
// converting every argument to float64 first.
func mathFn(goFn string, n int) builtin {
	return builtin{
		emit: func(a []string) string {
			args := make([]string, len(a))
			for i, x := range a {
				args[i] = f(x)
			}
			return goFn + "(" + strings.Join(args, ", ") + ")"
		},
		imports: []string{"math"},
		minArgs: n, maxArgs: n,
	}
}

// mathInt is like mathFn but truncates the result back to an int, which
// is what people actually want from floor, ceil and round.
func mathInt(goFn string) builtin {
	return builtin{
		emit:    func(a []string) string { return "int(" + goFn + "(" + f(a[0]) + "))" },
		imports: []string{"math"},
		minArgs: 1, maxArgs: 1,
	}
}

// strFn builds a builtin that forwards directly to a strings function.
func strFn(goFn string, n int) builtin {
	return builtin{
		emit: func(a []string) string {
			return goFn + "(" + strings.Join(a, ", ") + ")"
		},
		imports: []string{"strings"},
		minArgs: n, maxArgs: n,
	}
}

var stdlibHelperDefs = map[string]helperDef{
	"clamp": {
		code: `func __clamp(x float64, lo float64, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}`,
	},
	"sign": {
		code: `func __sign(x float64) int {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}`,
	},
	"randomInt": {
		code: `func __randomInt(lo int, hi int) int {
	if hi < lo {
		lo, hi = hi, lo
	}
	return lo + rand.Intn(hi-lo+1)
}`,
		imports: []string{"math/rand"},
	},
	// Index-based string helpers clamp instead of panicking. Quartz has
	// no exceptions yet, so a crash would be unrecoverable for the user.
	"charAt": {
		code: `func __charAt(s string, i int) string {
	r := []rune(s)
	if i < 0 || i >= len(r) {
		return ""
	}
	return string(r[i])
}`,
	},
	"substr": {
		code: `func __substr(s string, start int, end int) string {
	r := []rune(s)
	if start < 0 {
		start = 0
	}
	if end > len(r) {
		end = len(r)
	}
	if start >= end {
		return ""
	}
	return string(r[start:end])
}`,
	},
	"padLeft": {
		code: `func __padLeft(s string, width int, fill string) string {
	if fill == "" {
		fill = " "
	}
	for len([]rune(s)) < width {
		s = fill + s
	}
	return s
}`,
	},
	"padRight": {
		code: `func __padRight(s string, width int, fill string) string {
	if fill == "" {
		fill = " "
	}
	for len([]rune(s)) < width {
		s = s + fill
	}
	return s
}`,
	},
}

var stdlibBuiltins map[string]builtin

func buildStdlibBuiltins() {
	stdlibBuiltins = map[string]builtin{
		// ---- powers and roots ----
		"sqrt":  mathFn("math.Sqrt", 1),
		"cbrt":  mathFn("math.Cbrt", 1),
		"pow":   mathFn("math.Pow", 2),
		"exp":   mathFn("math.Exp", 1),
		"hypot": mathFn("math.Hypot", 2),

		// ---- logarithms ----
		"log":   mathFn("math.Log", 1),
		"log2":  mathFn("math.Log2", 1),
		"log10": mathFn("math.Log10", 1),

		// ---- rounding ----
		"floor": mathInt("math.Floor"),
		"ceil":  mathInt("math.Ceil"),
		"round": mathInt("math.Round"),
		"trunc": mathInt("math.Trunc"),

		// ---- trigonometry (radians) ----
		"sin":   mathFn("math.Sin", 1),
		"cos":   mathFn("math.Cos", 1),
		"tan":   mathFn("math.Tan", 1),
		"asin":  mathFn("math.Asin", 1),
		"acos":  mathFn("math.Acos", 1),
		"atan":  mathFn("math.Atan", 1),
		"atan2": mathFn("math.Atan2", 2),

		// ---- misc numeric ----
		"abs": mathFn("math.Abs", 1),
		"mod": mathFn("math.Mod", 2),
		"clamp": {
			emit: func(a []string) string {
				return "__clamp(" + f(a[0]) + ", " + f(a[1]) + ", " + f(a[2]) + ")"
			},
			helpers: []string{"clamp"}, minArgs: 3, maxArgs: 3,
		},
		"sign": {
			emit:    func(a []string) string { return "__sign(" + f(a[0]) + ")" },
			helpers: []string{"sign"}, minArgs: 1, maxArgs: 1,
		},
		"isNan": {
			emit:    func(a []string) string { return "math.IsNaN(" + f(a[0]) + ")" },
			imports: []string{"math"}, minArgs: 1, maxArgs: 1,
		},

		// ---- conversion ----
		// int() goes through math.Trunc so that a literal like int(3.7)
		// doesn't hit Go's "constant truncated" error.
		"int": {
			emit:    func(a []string) string { return "int(math.Trunc(" + f(a[0]) + "))" },
			imports: []string{"math"}, minArgs: 1, maxArgs: 1,
		},
		"float": {
			emit:    func(a []string) string { return f(a[0]) },
			minArgs: 1, maxArgs: 1,
		},
		// Integer division truncates in Go, so divf exists to get a real
		// fractional result out of two ints: divf(7, 2) is 3.5.
		"divf": {
			emit:    func(a []string) string { return "(" + f(a[0]) + " / " + f(a[1]) + ")" },
			minArgs: 2, maxArgs: 2,
		},

		// ---- randomness ----
		"random": {
			emit:    func(a []string) string { return "rand.Float64()" },
			imports: []string{"math/rand"}, minArgs: 0, maxArgs: 0,
		},
		"randomInt": {
			emit:    func(a []string) string { return "__randomInt(" + a[0] + ", " + a[1] + ")" },
			helpers: []string{"randomInt"}, minArgs: 2, maxArgs: 2,
		},

		// ---- strings ----
		"upper":      strFn("strings.ToUpper", 1),
		"lower":      strFn("strings.ToLower", 1),
		"trim":       strFn("strings.TrimSpace", 1),
		"contains":   strFn("strings.Contains", 2),
		"startsWith": strFn("strings.HasPrefix", 2),
		"endsWith":   strFn("strings.HasSuffix", 2),
		"indexOf":    strFn("strings.Index", 2),
		"count":      strFn("strings.Count", 2),
		"repeat": {
			emit:    func(a []string) string { return "strings.Repeat(" + a[0] + ", " + a[1] + ")" },
			imports: []string{"strings"}, minArgs: 2, maxArgs: 2,
		},
		"replace": {
			emit: func(a []string) string {
				return "strings.ReplaceAll(" + a[0] + ", " + a[1] + ", " + a[2] + ")"
			},
			imports: []string{"strings"}, minArgs: 3, maxArgs: 3,
		},
		"charAt": {
			emit:    func(a []string) string { return "__charAt(" + a[0] + ", " + a[1] + ")" },
			helpers: []string{"charAt"}, minArgs: 2, maxArgs: 2,
		},
		"substr": {
			emit: func(a []string) string {
				return "__substr(" + a[0] + ", " + a[1] + ", " + a[2] + ")"
			},
			helpers: []string{"substr"}, minArgs: 3, maxArgs: 3,
		},
		"padLeft": {
			emit: func(a []string) string {
				fill := `" "`
				if len(a) == 3 {
					fill = a[2]
				}
				return "__padLeft(" + a[0] + ", " + a[1] + ", " + fill + ")"
			},
			helpers: []string{"padLeft"}, minArgs: 2, maxArgs: 3,
		},
		"padRight": {
			emit: func(a []string) string {
				fill := `" "`
				if len(a) == 3 {
					fill = a[2]
				}
				return "__padRight(" + a[0] + ", " + a[1] + ", " + fill + ")"
			},
			helpers: []string{"padRight"}, minArgs: 2, maxArgs: 3,
		},
	}
}

// registerStdlib folds the math and string library into the core tables.
func registerStdlib() {
	buildStdlibBuiltins()
	for k, v := range stdlibHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range stdlibBuiltins {
		builtins[k] = v
	}
}
