package main

import "strings"

// builtinConst is a name that acts like a value, not a function.
type builtinConst struct {
	code    string
	imports []string
	typ     *Type
}

var builtinConsts = map[string]builtinConst{
	"PI":  {code: "math.Pi", imports: []string{"math"}, typ: Float},
	"E":   {code: "math.E", imports: []string{"math"}, typ: Float},
	"INF": {code: "math.Inf(1)", imports: []string{"math"}, typ: Float},
	"NAN": {code: "math.NaN()", imports: []string{"math"}, typ: Float},
}

// f wraps an argument in float64(). Math builtins declare their
// parameters as Numeric, meaning they genuinely accept int or float, so
// this conversion is real work rather than a workaround: float64 to
// float64 is a no-op in Go, and int to float64 is the widening the
// signature promised.
func f(arg string) string { return "float64(" + arg + ")" }

// numParams builds a parameter list of n Numeric slots.
func numParams(n int) []*Type {
	ps := make([]*Type, n)
	for i := range ps {
		ps[i] = Numeric
	}
	return ps
}

// strParams builds a parameter list of n Str slots.
func strParams(n int) []*Type {
	ps := make([]*Type, n)
	for i := range ps {
		ps[i] = Str
	}
	return ps
}

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
		params: numParams(n), ret: Float,
	}
}

// mathInt is like mathFn but truncates the result back to an int, which
// is what people actually want from floor, ceil and round.
func mathInt(goFn string) builtin {
	return builtin{
		emit:    func(a []string) string { return "int(" + goFn + "(" + f(a[0]) + "))" },
		imports: []string{"math"},
		minArgs: 1, maxArgs: 1,
		params: numParams(1), ret: Int,
	}
}

// strFn builds a builtin that forwards directly to a strings function.
func strFn(goFn string, n int, ret *Type) builtin {
	return builtin{
		emit: func(a []string) string {
			return goFn + "(" + strings.Join(a, ", ") + ")"
		},
		imports: []string{"strings"},
		minArgs: n, maxArgs: n,
		params: strParams(n), ret: ret,
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
	// Index-based string helpers clamp instead of panicking. Veyl has
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
			params: numParams(3), ret: Float,
		},
		"sign": {
			emit:    func(a []string) string { return "__sign(" + f(a[0]) + ")" },
			helpers: []string{"sign"}, minArgs: 1, maxArgs: 1,
			params: numParams(1), ret: Int,
		},
		"isNan": {
			emit:    func(a []string) string { return "math.IsNaN(" + f(a[0]) + ")" },
			imports: []string{"math"}, minArgs: 1, maxArgs: 1,
			params: numParams(1), ret: Bool,
		},

		// ---- conversion ----
		// int() goes through math.Trunc so that a literal like int(3.7)
		// doesn't hit Go's "constant truncated" error.
		"int": {
			emit:    func(a []string) string { return "int(math.Trunc(" + f(a[0]) + "))" },
			imports: []string{"math"}, minArgs: 1, maxArgs: 1,
			params: numParams(1), ret: Int,
		},
		"float": {
			emit:    func(a []string) string { return f(a[0]) },
			minArgs: 1, maxArgs: 1,
			params: numParams(1), ret: Float,
		},
		// `/` between two ints is integer division, so divf exists to get
		// a fractional result out of them: divf(7, 2) is 3.5.
		"divf": {
			emit:    func(a []string) string { return "(" + f(a[0]) + " / " + f(a[1]) + ")" },
			minArgs: 2, maxArgs: 2,
			params: numParams(2), ret: Float,
		},

		// ---- randomness ----
		"random": {
			emit:    func(a []string) string { return "rand.Float64()" },
			imports: []string{"math/rand"}, minArgs: 0, maxArgs: 0,
			ret: Float,
		},
		"randomInt": {
			emit:    func(a []string) string { return "__randomInt(" + a[0] + ", " + a[1] + ")" },
			helpers: []string{"randomInt"}, minArgs: 2, maxArgs: 2,
			params: []*Type{Int, Int}, ret: Int,
		},

		// ---- strings ----
		"upper":      strFn("strings.ToUpper", 1, Str),
		"lower":      strFn("strings.ToLower", 1, Str),
		"trim":       strFn("strings.TrimSpace", 1, Str),
		"contains":   strFn("strings.Contains", 2, Bool),
		"startsWith": strFn("strings.HasPrefix", 2, Bool),
		"endsWith":   strFn("strings.HasSuffix", 2, Bool),
		"indexOf":    strFn("strings.Index", 2, Int),
		"count":      strFn("strings.Count", 2, Int),
		"repeat": {
			emit:    func(a []string) string { return "strings.Repeat(" + a[0] + ", " + a[1] + ")" },
			imports: []string{"strings"}, minArgs: 2, maxArgs: 2,
			params: []*Type{Str, Int}, ret: Str,
		},
		"replace": {
			emit: func(a []string) string {
				return "strings.ReplaceAll(" + a[0] + ", " + a[1] + ", " + a[2] + ")"
			},
			imports: []string{"strings"}, minArgs: 3, maxArgs: 3,
			params: strParams(3), ret: Str,
		},
		"charAt": {
			emit:    func(a []string) string { return "__charAt(" + a[0] + ", " + a[1] + ")" },
			helpers: []string{"charAt"}, minArgs: 2, maxArgs: 2,
			params: []*Type{Str, Int}, ret: Str,
		},
		"substr": {
			emit: func(a []string) string {
				return "__substr(" + a[0] + ", " + a[1] + ", " + a[2] + ")"
			},
			helpers: []string{"substr"}, minArgs: 3, maxArgs: 3,
			params: []*Type{Str, Int, Int}, ret: Str,
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
			params: []*Type{Str, Int, Str}, ret: Str,
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
			params: []*Type{Str, Int, Str}, ret: Str,
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
