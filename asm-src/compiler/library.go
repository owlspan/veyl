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

	// The namespaced library. A dotted name is looked up here exactly
	// like a plain one - the checker flattens `time.now` to that string
	// before asking - so a namespace is a naming convention rather than
	// a scope, the same as on the Go backend.
	//
	// Everything else the Go backend puts under os, time, mem, json, re,
	// hash, csv, net, http and task is absent, and absent here means a
	// type error naming the function rather than something the lowerer
	// discovers later.
	"time.now":   {Ret: Int},
	"os.env.get": {Params: []*Type{Str}, Ret: Str},
	"os.env.has": {Params: []*Type{Str}, Ret: Bool},
}

func (asmLibrary) Signature(name string) (front.Signature, bool) {
	// The polymorphic ones. A fixed Params/Ret cannot say "any T!" or
	// "a str or a list", so each carries its own check, the same way the
	// Go backend's overloaded builtins do.
	switch name {
	case "len":
		return front.Signature{Check: checkLen}, true
	case "push":
		return front.Signature{Check: checkPush}, true

	// The error type. These mirror the Go backend's declarations exactly,
	// because a program that type-checks on one backend and not on the
	// other would be a language with two definitions.
	case "fail":
		return front.Signature{Params: []*Type{Str}, Ret: ErrLitT}, true
	case "ok":
		return front.Signature{Check: checkOk}, true
	case "isOk":
		return front.Signature{Check: checkIsOk}, true
	case "failed":
		return front.Signature{Check: checkFailed}, true
	case "errorOf":
		return front.Signature{Check: checkErrorOf}, true
	case "must":
		return front.Signature{Check: checkMust, WantsTarget: true, HintFor: hintResult}, true
	case "valueOr":
		return front.Signature{Check: checkValueOr, WantsTarget: true, HintFor: hintValueOr}, true
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
	case KStr, KList, KMap:
		return Int
	}
	c.ErrorAt(x, "len needs a str, a list or a map, got %s", args[0])
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

// wantResult is the shared front half of every builtin that takes a
// single T! and does something with it. It returns the inner T, or
// Unknown once it has reported why there is not one.
func wantResult(c *Checker, x *Call, args []*Type, name string) *Type {
	if len(args) != 1 {
		c.ErrorAt(x, "%s takes 1 argument, got %d", name, len(args))
		return Unknown
	}
	if args[0].IsUnknown() {
		return Unknown
	}
	if !args[0].IsResult() {
		c.ErrorAt(x, "%s needs a T!, got %s", name, args[0])
		return Unknown
	}
	return args[0].Elem
}

// ok() is the counterpart to fail() for an action that produces no
// value: `return ok()` in a function declared void!.
func checkOk(c *Checker, x *Call, args []*Type) *Type {
	if len(args) != 0 {
		c.ErrorAt(x, "ok takes no arguments, got %d", len(args))
	}
	return ResultOf(Void)
}

func checkIsOk(c *Checker, x *Call, args []*Type) *Type {
	wantResult(c, x, args, "isOk")
	return Bool
}

func checkFailed(c *Checker, x *Call, args []*Type) *Type {
	wantResult(c, x, args, "failed")
	return Bool
}

func checkErrorOf(c *Checker, x *Call, args []*Type) *Type {
	wantResult(c, x, args, "errorOf")
	return Str
}

func checkMust(c *Checker, x *Call, args []*Type) *Type {
	return wantResult(c, x, args, "must")
}

func checkValueOr(c *Checker, x *Call, args []*Type) *Type {
	if len(args) != 2 {
		c.ErrorAt(x, "valueOr takes 2 arguments, got %d", len(args))
		return Unknown
	}
	inner := wantResult(c, x, args[:1], "valueOr")
	if inner.IsUnknown() || args[1].IsUnknown() {
		return inner
	}
	if !args[1].Equal(inner) {
		c.ErrorAt(x, "valueOr needs a %s to fall back on, got %s", inner, args[1])
	}
	return inner
}

// hintResult carries the type the context wants inwards, so that the
// argument of must() knows which result it is meant to produce.
func hintResult(x *Call, known []*Type, i int) *Type {
	if i == 0 && x.Want != nil && !x.Want.IsUnknown() {
		return ResultOf(x.Want)
	}
	return nil
}

// hintValueOr does the same, and additionally tells the fallback which
// type it has to be once the result argument is known.
func hintValueOr(x *Call, known []*Type, i int) *Type {
	if i == 0 && x.Want != nil && !x.Want.IsUnknown() {
		return ResultOf(x.Want)
	}
	if i == 1 && len(known) == 1 && known[0].IsResult() {
		return known[0].Elem
	}
	return nil
}
