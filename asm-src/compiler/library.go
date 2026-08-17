package main

// The assembly backend's builtin table, presented to the shared type
// checker.
//
// The checker in ../../frontend is the same code the Go backend runs. It
// asks a Library what a builtin looks like, and this is that Library.
// Where the Go backend declares 302 builtins backed by Go's standard
// library, this declares the ones written by hand here.
//
// That is the point of the seam. A program calling `http.get` on this
// backend is now an ordinary type error with a file, line and column,
// reported alongside every other error in the program, instead of
// something the lowerer discovers halfway through and reports in its own
// vocabulary. The subset stops being a property of what happens to be
// implemented and becomes something the compiler states up front.

import front "veylfront"

type asmLibrary struct{}

// sigs is the whole standard library of this backend. Keeping it as data
// rather than as a switch means the checker and the lowerer cannot
// disagree about what exists.
var sigs = map[string]front.Signature{
	// Output. print is variadic in neither backend, but it accepts any
	// single value and decides how to render it from the type.
	"print": {Params: []*Type{Any}, Ret: Void},
	"write": {Params: []*Type{Any}, Ret: Void},

	// Conversion.
	"str": {Params: []*Type{Any}, Ret: Str},

	// Numbers.
	"abs": {Params: []*Type{Int}, Ret: Int},
	// Numeric accepts int or float and always yields a float, matching
	// the Go backend: divf(7, 2) is 3.5 however its arguments were
	// written.
	"int":   {Params: []*Type{Numeric}, Ret: Int},
	"float": {Params: []*Type{Numeric}, Ret: Float},
	"divf":  {Params: []*Type{Numeric, Numeric}, Ret: Float},
	"sqrt":  {Params: []*Type{Numeric}, Ret: Float},
	"mod":   {Params: []*Type{Numeric, Numeric}, Ret: Float},
	"min":   {Params: []*Type{Int, Int}, Ret: Int},
	"max":   {Params: []*Type{Int, Int}, Ret: Int},

	// Strings.
	"upper":      {Params: []*Type{Str}, Ret: Str},
	"lower":      {Params: []*Type{Str}, Ret: Str},
	"substr":     {Params: []*Type{Str, Int, Int}, Ret: Str},
	"charAt":     {Params: []*Type{Str, Int}, Ret: Str},
	"indexOf":    {Params: []*Type{Str, Str}, Ret: Int},
	"contains":   {Params: []*Type{Str, Str}, Ret: Bool},
	"startsWith": {Params: []*Type{Str, Str}, Ret: Bool},
	"endsWith":   {Params: []*Type{Str, Str}, Ret: Bool},
	"repeat":     {Params: []*Type{Str, Int}, Ret: Str},
}

func (asmLibrary) Signature(name string) (front.Signature, bool) {
	// len and push are polymorphic over strings and lists, which a fixed
	// signature cannot describe, so they carry a custom check the same
	// way the Go backend's overloaded builtins do.
	switch name {
	case "len":
		return front.Signature{Check: checkLen}, true
	case "push":
		return front.Signature{Check: checkPush}, true
	}
	s, ok := sigs[name]
	return s, ok
}

// ConstType reports the float constants this backend can honour.
//
// NAN and INF are deliberately absent, for two different reasons, and
// both are real gaps rather than oversights.
//
// NAN: the float comparisons here lower to comisd, which reports an
// unordered pair as both below and equal. A NaN would therefore compare
// wrong rather than fail loudly, which is the worse of the two.
//
// INF: this build links the legacy msvcrt, whose printf writes infinity
// as "1.#INF" where Go writes "+Inf". Printing one would silently
// disagree with the Go backend, and the whole guarantee of this backend
// is that when both compile a program they produce the same bytes.
//
// Leaving both out keeps each a compile error naming what is missing.
func (asmLibrary) ConstType(name string) (*Type, bool) {
	switch name {
	case "PI", "E":
		return Float, true
	}
	return nil, false
}

func checkLen(c *Checker, x *Call, args []*Type) *Type {
	if len(args) != 1 {
		c.ErrorAt(x, "len takes 1 argument, got %d", len(args))
		return Unknown
	}
	switch args[0].Kind {
	case KStr, KList:
		return Int
	}
	c.ErrorAt(x, "len needs a str or a list, got %s", args[0])
	return Unknown
}

func checkPush(c *Checker, x *Call, args []*Type) *Type {
	if len(args) != 2 {
		c.ErrorAt(x, "push takes 2 arguments, got %d", len(args))
		return Unknown
	}
	if args[0].Kind != KList {
		c.ErrorAt(x, "push needs a list, got %s", args[0])
		return Unknown
	}
	if !args[1].Equal(args[0].Elem) {
		c.ErrorAt(x, "cannot push %s into %s", args[1], args[0])
		return Unknown
	}
	return Void
}
