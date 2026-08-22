package main

// The prelude: library functions written in Veyl and compiled by this
// compiler like any other code.
//
// Why this exists. The guarantee of this backend is that a program
// compiled here prints the same bytes as the same program compiled
// through Go. For arithmetic that means agreeing to the last bit,
// because printing goes through the shortest round-tripping decimal and
// a one-bit difference is a visible digit. msvcrt's libm is a perfectly
// good libm and it is not Go's: measured across a hundred and twenty
// values of sin, cos, tan, exp, log, atan, asin, acos and pow, a third
// of them differ in the last place. Calling msvcrt is therefore not an
// implementation of these functions, it is a different function.
//
// So they are implemented, from Go's own algorithms. What this file adds
// is that they are implemented *in Veyl* rather than by hand-writing
// calls into the IR builder. `y = z + z*(zz*((P0*zz+P1)*zz+P2)/D)` is
// one line of Veyl and about twenty lines of emit calls, and the twenty
// lines cannot be read against the algorithm they came from. Writing it
// in the language the compiler already compiles gets the lowering, the
// type checking and the byte writer for nothing.
//
// How it works. The source below is parsed once, and a program that
// mentions one of these builtins has the function it needs - and every
// function that one calls, transitively - folded into it before
// checking, exactly the way an imported file is. Nothing is pulled in
// for a program that does not use it.
//
// Two private builtins exist only for this file: `__bits` and
// `__frombits` reinterpret a float as its IEEE 754 bit pattern and back.
// Go's Frexp, Ldexp, Modf and the special-case tests are all written in
// terms of that, and there is no way to say it in Veyl otherwise. They
// are not in the language and a user program naming one gets the
// ordinary "not on the assembly backend yet" error, because they are
// registered for the prelude's own compilation and nowhere else.

import (
	"strings"
	"sync"
)

// preludeOf maps a builtin to the Veyl function that implements it.
// A builtin absent from here is either lowered directly in the IR or is
// not on this backend at all.
var preludeOf = map[string]string{
	"sin":   "__vy_sin",
	"cos":   "__vy_cos",
	"tan":   "__vy_tan",
	"asin":  "__vy_asin",
	"acos":  "__vy_acos",
	"atan":  "__vy_atan",
	"atan2": "__vy_atan2",
	"exp":   "__vy_exp",
	"log":   "__vy_log",
	"log2":  "__vy_log2",
	"log10": "__vy_log10",
	"pow":   "__vy_pow",
	"cbrt":  "__vy_cbrt",
	"hypot": "__vy_hypot",

	"time.format":  "__vy_timeFormat",
	"time.parse":   "__vy_timeParse",
	"time.date":    "__vy_timeDate",
	"time.clock":   "__vy_timeClock",
	"time.stamp":   "__vy_timeStamp",
	"time.year":    "__vy_timeYear",
	"time.month":   "__vy_timeMonth",
	"time.day":     "__vy_timeDay",
	"time.weekday": "__vy_timeWeekday",
	"time.since":   "__vy_timeSince",
	"floor":        "__vy_floor",
	"ceil":         "__vy_ceil",
	"round":        "__vy_round",
	"trunc":        "__vy_trunc",
}

var (
	preludeOnce sync.Once
	preludeProg *Program
	preludeErrs []string
)

// parsePrelude reads the source once. A syntax error in it is a compiler
// bug rather than anything a user did, so it is reported that way.
func parsePrelude() (*Program, []string) {
	preludeOnce.Do(func() {
		lx := NewLexer("<prelude>", preludeSource)
		toks := lx.Scan()
		ps := NewParser("<prelude>", toks)
		preludeProg = ps.ParseProgram()
		preludeErrs = append(append([]string{}, lx.Errors...), ps.Errors...)
		for _, f := range preludeProg.Funcs {
			f.File = "<prelude>"
		}
	})
	return preludeProg, preludeErrs
}

// addPrelude folds in the prelude functions a program needs.
//
// What a program needs is decided by which identifiers appear in its
// source, not by walking its syntax tree. That over-approximates - a
// program with the word `sin` in a string literal pulls the function in
// - and over-approximating is the safe direction: the cost is a few
// hundred bytes of code nothing calls, where under-approximating is a
// link failure. A twenty-nine-case tree walk that has to be updated
// every time the language grows a node would be the unsafe direction.
//
// The prelude's own internal dependencies are resolved the same way,
// chunk by chunk, which is exact enough because the prelude is this
// compiler's own source and every name in it is deliberate.
func addPrelude(prog *Program, sources []string) []string {
	pre, errs := parsePrelude()
	if len(errs) > 0 {
		return errs
	}

	chunks := preludeChunks()

	wanted := map[string]bool{}
	for _, src := range sources {
		for name, fn := range preludeOf {
			if mentions(src, name) {
				wanted[fn] = true
			}
		}
	}
	if len(wanted) == 0 {
		return nil
	}

	// Transitive closure. A prelude function's dependencies are the
	// other prelude names its own text mentions.
	taken := map[string]bool{}
	var pull func(name string)
	pull = func(name string) {
		if taken[name] {
			return
		}
		body, ok := chunks[name]
		if !ok {
			return
		}
		taken[name] = true
		for other := range chunks {
			if other != name && mentions(body, other) {
				pull(other)
			}
		}
	}
	for name := range wanted {
		pull(name)
	}

	// In the prelude's own order, so two compilations of the same
	// program lay the functions out identically.
	for _, f := range pre.Funcs {
		if taken[f.Name] {
			prog.Funcs = append(prog.Funcs, f)
		}
	}
	return nil
}

// mentions reports whether name appears in src as a whole identifier,
// rather than as part of a longer one. Without the boundary check,
// wanting `log` would drag in every function whose name contains it.
func mentions(src, name string) bool {
	from := 0
	for {
		i := strings.Index(src[from:], name)
		if i < 0 {
			return false
		}
		i += from
		before := byte(' ')
		if i > 0 {
			before = src[i-1]
		}
		after := byte(' ')
		if i+len(name) < len(src) {
			after = src[i+len(name)]
		}
		if !identByte(before) && !identByte(after) {
			return true
		}
		from = i + 1
	}
}

func identByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

var (
	chunkOnce sync.Once
	chunkText map[string]string
)

// preludeChunks splits the source into one entry per function, keyed by
// name. A function starts at a `fn ` in the first column and runs to the
// next one, which is a property of how this file is written rather than
// of the language - the prelude is formatted for this.
func preludeChunks() map[string]string {
	chunkOnce.Do(func() {
		chunkText = map[string]string{}
		lines := strings.Split(preludeSource, "\n")
		name := ""
		var body []string
		flush := func() {
			if name != "" {
				chunkText[name] = strings.Join(body, "\n")
			}
		}
		for _, line := range lines {
			if strings.HasPrefix(line, "fn ") {
				flush()
				rest := line[3:]
				cut := strings.IndexAny(rest, "( ")
				if cut < 0 {
					cut = len(rest)
				}
				name = rest[:cut]
				body = nil
			}
			body = append(body, line)
		}
		flush()
	})
	return chunkText
}

// preludeSource is the Veyl text. It is split into named chunks only so
// that the algorithms can carry the comments explaining where their
// constants came from.
var preludeSource = strings.Join([]string{
	preludeBits,
	preludeLog,
	preludeExp,
	preludeTrig,
	preludeAtan,
	preludePow,
	preludeRoot,
	preludeRound,
	preludeTime,
	preludeTimeHelpers,
}, "\n")
