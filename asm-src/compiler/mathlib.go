package main

// The math library.
//
// Almost none of it is here. sin, cos, tan, asin, acos, atan, atan2,
// exp, log, log2, log10, pow, cbrt and hypot are all written in Veyl, in
// prelude_math.go, and this file only routes a call to them - see
// prelude.go for why calling msvcrt instead would be a different
// function rather than a different implementation.
//
// What is left here is the handful that are one comparison or one call
// and would be sillier as Veyl source than as five lines of IR.

func (l *lowerer) mathBuiltin(c *Call, name string) (Reg, bool) {
	arity := func(n int) bool {
		if len(c.Args) != n {
			l.errorAt(c, "%s takes %d argument(s), got %d", name, n, len(c.Args))
			return false
		}
		return true
	}

	// Everything transcendental is a call into the prelude, which is
	// Veyl source compiled by this compiler - see prelude.go. Routing it
	// here rather than renaming the callee earlier keeps the error a
	// user sees naming `sin` rather than `__vy_sin`.
	if fn, ok := preludeOf[name]; ok {
		sig, declared := l.sigs[fn]
		if !declared {
			l.errorAt(c, "the prelude did not supply %s", fn)
			return l.junk(), true
		}
		if !arity(len(sig.params)) {
			return l.junk(), true
		}
		args := make([]Reg, len(c.Args))
		for i := range c.Args {
			args[i] = l.numeric(c.Args[i])
		}
		if len(args) > l.fn.MaxCallArgs {
			l.fn.MaxCallArgs = len(args)
		}
		d := l.newReg()
		l.regTy[d] = sig.ret
		l.emit(Instr{Op: OpCall, Dst: d, A: NoReg, B: NoReg, Args: args,
			ArgTypes: sig.params, RetType: sig.ret, Sym: fn, Comment: name + "()"})
		return d, true
	}

	switch name {
	case "__abort":
		if !arity(1) {
			return l.junk(), true
		}
		l.mod.needs("must")
		reason := l.expr(c.Args[0])
		if l.regTy[reason].k != kStr {
			l.errorAt(c.Args[0], "__abort expects a str")
			return l.junk(), true
		}
		l.emit(Instr{Op: OpMustFail, A: reason, Dst: NoReg, B: NoReg,
			Comment: "__abort()"})
		// Nothing runs after it, but the lowerer still needs a register
		// to hand back to whatever asked for the value.
		return l.floatConst(0), true

	case "__bits", "__frombits":
		if !arity(1) {
			return l.junk(), true
		}
		// A bitcast, and it needs no instruction at all. Every value
		// here lives in a stack slot and moves through rax as a raw
		// word, whatever its type - so storing a float and loading the
		// same slot as an int already is the reinterpretation, and the
		// only thing that changes is what the lowerer believes about the
		// register it hands back.
		want, from := vInt, kFloat
		if name == "__frombits" {
			want, from = vFloat, kInt
		}
		v := l.expr(c.Args[0])
		if l.regTy[v].k != from {
			l.errorAt(c.Args[0], "%s expects a %s", name, vty{k: from})
			return l.junk(), true
		}
		slot := l.temp(l.regTy[v])
		l.emit(Instr{Op: OpStore, A: v, Dst: NoReg, Imm: slot})
		d := l.newReg()
		l.regTy[d] = want
		l.emit(Instr{Op: OpLoad, Dst: d, A: NoReg, B: NoReg, Imm: slot})
		return d, true

	case "sign":
		if !arity(1) {
			return l.junk(), true
		}
		x := l.numeric(c.Args[0])
		zero := l.floatConst(0)
		pos := l.pick(l.compare(OpFGt, x, zero), l.intConst(1), l.intConst(0), vInt)
		return l.pick(l.compare(OpFLt, x, zero), l.intConst(-1), pos, vInt), true

	case "clamp":
		if !arity(3) {
			return l.junk(), true
		}
		x := l.numeric(c.Args[0])
		lo, hi := l.numeric(c.Args[1]), l.numeric(c.Args[2])
		// Written in the Go backend's order: below the floor first, then
		// above the ceiling, so clamp(x, 5, 1) gives 1 on both.
		x = l.pick(l.compare(OpFLt, x, lo), lo, x, vFloat)
		return l.pick(l.compare(OpFGt, x, hi), hi, x, vFloat), true

	case "isNan":
		if !arity(1) {
			return l.junk(), true
		}
		// A NaN is the only value that is not equal to itself, and
		// comisd reports an unordered pair as not-equal, so this is the
		// one NaN question the comparison hardware answers correctly.
		// It is also why NAN itself is not a constant here: every other
		// comparison gets an unordered pair wrong.
		x := l.numeric(c.Args[0])
		eq := l.compare(OpFEq, x, x)
		return l.pick(eq, l.intConst(0), l.intConst(1), vBool), true

	case "exit":
		if !arity(1) {
			return l.junk(), true
		}
		code := l.expr(c.Args[0])
		if l.regTy[code].k != kInt {
			l.errorAt(c.Args[0], "exit expects an int, got %s", l.regTy[code])
			return l.junk(), true
		}
		// Flushed first: exit through msvcrt does run the stream
		// teardown, but print here writes through printf and a runtime
		// abort writes through _write, and only an explicit flush keeps
		// those two in the order the program wrote them.
		l.ccall("fflush", []Reg{l.intConst(0)}, []vty{vInt}, vty{k: kVoid}, false, false)
		l.ccall("exit", []Reg{code}, []vty{vInt}, vty{k: kVoid}, false, false)
		return l.junk(), true

	case "sleep":
		if !arity(1) {
			return l.junk(), true
		}
		ms := l.numeric(c.Args[0])
		// Veyl sleeps in seconds and Sleep takes milliseconds.
		thousand := l.floatConst(1000)
		scaled := l.newReg()
		l.regTy[scaled] = vFloat
		l.emit(Instr{Op: OpFMul, Dst: scaled, A: ms, B: thousand})
		whole := l.newReg()
		l.regTy[whole] = vInt
		l.emit(Instr{Op: OpFloatToInt, Dst: whole, A: scaled, B: NoReg})
		l.ccall("Sleep", []Reg{whole}, []vty{vInt}, vty{k: kVoid}, false, false)
		return l.junk(), true
	}

	return NoReg, false
}

// intConst is an integer literal as a register.
func (l *lowerer) intConst(n int64) Reg {
	d := l.newReg()
	l.regTy[d] = vInt
	l.emit(Instr{Op: OpConst, Dst: d, A: NoReg, B: NoReg, Imm: n})
	return d
}

// slotAddr is the address of a frame slot, for a C function that writes
// through a pointer.
func (l *lowerer) slotAddr(slot int64) Reg {
	d := l.newReg()
	l.regTy[d] = vInt
	l.emit(Instr{Op: OpSlotAddr, Dst: d, A: NoReg, B: NoReg, Imm: slot})
	return d
}
