package main

import "strings"

// The rand, stats, term and log libraries.
//
// These are the things a program reaches for constantly and that
// Quartz previously made you write yourself: pick a random element,
// average a list, print something in colour, log a line with a
// timestamp. Python ships all of them and it is a large part of why
// small Python programs are short.

var extraHelperDefs = map[string]helperDef{

	// A constraint covering both numeric types, so one generic helper
	// serves []int and []float instead of two near-identical ones.
	"numeric": {
		code: `type __Num interface{ ~int | ~float64 }

func __toFloats[T __Num](xs []T) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = float64(x)
	}
	return out
}`,
	},

	"randSource": {
		// math/rand/v2 is seeded from the OS, so programs differ run to
		// run without anyone remembering to seed. rand.seed exists for
		// when a fixed sequence is wanted, and only then.
		code: `// math/rand rather than math/rand/v2: v2 needs Go 1.22, and the
// generated program declares go 1.21 so that anyone on the older
// toolchain can still build what Quartz produces.
var __rng = rand.New(rand.NewSource(time.Now().UnixNano()))

func __seed(n int) {
	__rng = rand.New(rand.NewSource(int64(n)))
}

func __randInt(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + __rng.Intn(hi-lo+1)
}

func __randHex(n int) string {
	if n <= 0 {
		return ""
	}
	const digits = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[__rng.Intn(16)]
	}
	return string(b)
}

// A version 4 UUID: 122 random bits, with the version and variant
// fields set as the spec requires.
func __uuid() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(__rng.Intn(256))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func __pick[T any](xs []T) T {
	var zero T
	if len(xs) == 0 {
		return zero
	}
	return xs[__rng.Intn(len(xs))]
}

// Shuffle returns a new list. Quartz assignment copies, so a function
// that reordered its argument in place would surprise people.
func __shuffle[T any](xs []T) []T {
	out := append([]T(nil), xs...)
	__rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func __sample[T any](xs []T, n int) []T {
	if n <= 0 || len(xs) == 0 {
		return []T{}
	}
	if n > len(xs) {
		n = len(xs)
	}
	return __shuffle(xs)[:n]
}`,
		imports: []string{"fmt", "math/rand", "time"},
	},

	"stats": {
		code: `func __mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	total := 0.0
	for _, x := range xs {
		total += x
	}
	return total / float64(len(xs))
}

func __median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

// The sample variance, dividing by n-1. That is what you want when the
// numbers are measurements rather than the whole population, which is
// the common case and the one people get wrong.
func __variance(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := __mean(xs)
	total := 0.0
	for _, x := range xs {
		total += (x - m) * (x - m)
	}
	return total / float64(len(xs)-1)
}

func __stdev(xs []float64) float64 { return math.Sqrt(__variance(xs)) }

// Linear interpolation between the two neighbouring ranks, which is
// what spreadsheets and numpy both do by default.
func __percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if p <= 0 {
		return s[0]
	}
	if p >= 100 {
		return s[len(s)-1]
	}
	pos := (p / 100) * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return s[lo]
	}
	return s[lo] + (pos-float64(lo))*(s[hi]-s[lo])
}`,
		imports: []string{"math", "sort"},
	},

	// Colour. The portable body assumes a terminal that understands
	// ANSI, which every Linux and macOS terminal and Windows Terminal
	// does. Classic Windows conhost does not, until asked.
	"term": {
		code: `func __style(code, s string) string {
	if !__termColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

var __termColor = __termColorEnabled()

// NO_COLOR is honoured because a program whose output is being piped
// into a file should not fill it with escape sequences.
func __termColorEnabled() bool {
	if _, off := os.LookupEnv("NO_COLOR"); off {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func __termBar(done, total, width int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	filled := done * width / total
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}`,
		imports: []string{"os", "strings"},

		// On Windows the escape sequences are inert until the console
		// is switched into virtual-terminal mode, so without this the
		// colours arrive as visible garbage like <-[31m. Turning it on
		// needs kernel32, which does not exist to link against
		// anywhere else, hence the separate body.
		winCode: `var __kernel32Term = syscall.NewLazyDLL("kernel32.dll")

func init() {
	// ENABLE_VIRTUAL_TERMINAL_PROCESSING on the console output handle.
	// If any of this fails the console is not a console -- output is
	// redirected -- and __termColorEnabled has already said no.
	const enableVT = 0x0004
	get := __kernel32Term.NewProc("GetConsoleMode")
	set := __kernel32Term.NewProc("SetConsoleMode")
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return
	}
	var mode uint32
	if r, _, _ := get.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return
	}
	set.Call(uintptr(h), uintptr(mode|enableVT))
}

func __style(code, s string) string {
	if !__termColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

var __termColor = __termColorEnabled()

func __termColorEnabled() bool {
	if _, off := os.LookupEnv("NO_COLOR"); off {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func __termBar(done, total, width int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	filled := done * width / total
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}`,
		winImports: []string{"os", "strings", "syscall", "unsafe"},
	},

	"log": {
		code: `func __logLine(level, msg string) {
	fmt.Fprintf(os.Stderr, "%s %-5s %s\n",
		time.Now().Format("15:04:05"), level, msg)
}`,
		imports: []string{"fmt", "os", "time"},
	},
}

var extraBuiltins map[string]builtin

func buildExtraBuiltins() {
	// numbers wraps a []int or []float argument so one generic helper
	// handles both. Anything else is a type error naming the library
	// function rather than the helper underneath it.
	numbers := func(name string, goFn string) builtin {
		return builtin{
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if len(args) > 0 && args[0].Kind == KList &&
					(args[0].Elem.Kind == KInt || args[0].Elem.Kind == KFloat) {
					return Float
				}
				if len(args) > 0 && !args[0].IsUnknown() {
					c.errorAt(x, "%s expects a list of numbers, got %s", name, args[0])
				}
				return Unknown
			},
			emit:    func(a []string) string { return goFn + "(__toFloats(" + a[0] + "))" },
			helpers: []string{"stats", "numeric"},
		}
	}

	// A colour or attribute wrapper: term.red("x") and friends.
	style := func(code string) builtin {
		return builtin{
			emit: func(a []string) string {
				return `__style("` + code + `", ` + a[0] + ")"
			},
			params: []*Type{Str}, ret: Str, minArgs: 1, maxArgs: 1,
			helpers: []string{"term"},
		}
	}

	level := func(name string) builtin {
		return builtin{
			emit:   func(a []string) string { return `__logLine("` + name + `", ` + a[0] + ")" },
			params: []*Type{Str}, ret: Void, minArgs: 1, maxArgs: 1,
			helpers: []string{"log"},
		}
	}

	extraBuiltins = map[string]builtin{

		// ---- rand ----

		"rand.seed": {
			emit:   func(a []string) string { return "__seed(" + a[0] + ")" },
			params: []*Type{Int}, ret: Void, minArgs: 1, maxArgs: 1,
			helpers: []string{"randSource"},
		},
		"rand.int": {
			emit:   func(a []string) string { return "__randInt(" + a[0] + ", " + a[1] + ")" },
			params: []*Type{Int, Int}, ret: Int, minArgs: 2, maxArgs: 2,
			helpers: []string{"randSource"},
		},
		"rand.float": {
			emit:    func(a []string) string { return "__rng.Float64()" },
			ret:     Float,
			helpers: []string{"randSource"},
		},
		"rand.bool": {
			emit:    func(a []string) string { return "(__rng.Intn(2) == 1)" },
			ret:     Bool,
			helpers: []string{"randSource"},
		},
		"rand.hex": {
			emit:   func(a []string) string { return "__randHex(" + a[0] + ")" },
			params: []*Type{Int}, ret: Str, minArgs: 1, maxArgs: 1,
			helpers: []string{"randSource"},
		},
		"rand.uuid": {
			emit:    func(a []string) string { return "__uuid()" },
			ret:     Str,
			helpers: []string{"randSource"},
		},
		"rand.pick": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				return wantList(c, x, 0, args, "rand.pick")
			},
			emit:    func(a []string) string { return "__pick(" + a[0] + ")" },
			helpers: []string{"randSource"},
		},
		"rand.shuffle": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if wantList(c, x, 0, args, "rand.shuffle").IsUnknown() {
					return Unknown
				}
				return args[0]
			},
			emit:    func(a []string) string { return "__shuffle(" + a[0] + ")" },
			helpers: []string{"randSource"},
		},
		"rand.sample": {
			minArgs: 2, maxArgs: 2,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if wantList(c, x, 0, args, "rand.sample").IsUnknown() {
					return Unknown
				}
				if len(args) > 1 && args[1].Kind != KInt {
					c.errorAt(x, "rand.sample expects a count, got %s", args[1])
					return Unknown
				}
				return args[0]
			},
			emit:    func(a []string) string { return "__sample(" + a[0] + ", " + a[1] + ")" },
			helpers: []string{"randSource"},
		},

		// ---- stats ----

		"stats.mean":   numbers("stats.mean", "__mean"),
		"stats.median": numbers("stats.median", "__median"),
		"stats.stdev":  numbers("stats.stdev", "__stdev"),
		"stats.var":    numbers("stats.var", "__variance"),
		"stats.percentile": {
			minArgs: 2, maxArgs: 2,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if len(args) < 2 || args[0].Kind != KList ||
					(args[0].Elem.Kind != KInt && args[0].Elem.Kind != KFloat) {
					c.errorAt(x, "stats.percentile expects a list of numbers and a percentage")
					return Unknown
				}
				if args[1].Kind != KFloat && args[1].Kind != KInt {
					c.errorAt(x, "stats.percentile expects a percentage, got %s", args[1])
					return Unknown
				}
				return Float
			},
			emit: func(a []string) string {
				return "__percentile(__toFloats(" + a[0] + "), float64(" + a[1] + "))"
			},
			helpers: []string{"stats", "numeric"},
		},

		// ---- term ----

		"term.red":       style("31"),
		"term.green":     style("32"),
		"term.yellow":    style("33"),
		"term.blue":      style("34"),
		"term.magenta":   style("35"),
		"term.cyan":      style("36"),
		"term.grey":      style("90"),
		"term.bold":      style("1"),
		"term.dim":       style("2"),
		"term.underline": style("4"),
		"term.invert":    style("7"),
		"term.bar": {
			emit: func(a []string) string {
				return "__termBar(" + strings.Join(a, ", ") + ")"
			},
			params: []*Type{Int, Int, Int}, ret: Str, minArgs: 3, maxArgs: 3,
			helpers: []string{"term"},
		},
		"term.clear": {
			emit:    func(a []string) string { return `fmt.Print("\x1b[2J\x1b[H")` },
			ret:     Void,
			imports: []string{"fmt"},
			helpers: []string{"term"},
		},
		"term.colour": {
			// Whether colour is going to be visible at all, so a program
			// can choose plain output instead of stripping codes later.
			emit:    func(a []string) string { return "__termColor" },
			ret:     Bool,
			helpers: []string{"term"},
		},

		// ---- log ----

		"log.info":  level("info"),
		"log.warn":  level("warn"),
		"log.error": level("error"),
	}
}

func registerExtra() {
	buildExtraBuiltins()
	registerNamespace("rand", "stats", "term", "log")
	for k, v := range extraHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range extraBuiltins {
		builtins[k] = v
	}
}
