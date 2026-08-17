package main

// The string library, lowered in the IR out of byte loads and stores.
//
// A Veyl string here is a pointer to NUL-terminated bytes, so everything
// below is a loop over an index with a bounds condition of "not the
// terminator". Writing these as IR rather than as assembly helpers means
// the byte writer inherits them, and means each one is readable as
// ordinary control flow rather than as a page of instructions.
//
// Every function that returns a string allocates a new one. None of them
// free anything.

// strAlloc reserves n+1 bytes and returns the pointer, typed as a
// string. The extra byte is the terminator, which every caller has to
// write itself.
func (l *lowerer) strAlloc(n Reg) Reg {
	buf := l.allocObj(l.arith(OpAdd, n, l.constant(1)), tagBytes)
	l.regTy[buf] = vStr
	return buf
}

func (l *lowerer) strLen(s Reg) Reg {
	l.mod.needs("strlen")
	d := l.newReg()
	l.regTy[d] = vInt
	l.emit(Instr{Op: OpStrLen, Dst: d, A: s, B: NoReg})
	return d
}

func (l *lowerer) loadByte(base, off Reg) Reg {
	d := l.newReg()
	l.regTy[d] = vInt
	l.emit(Instr{Op: OpLoadByte, Dst: d, A: base, B: off})
	return d
}

// storeByte writes one byte. The value travels through a slot because
// the instruction already spends both register fields on the base and
// the offset.
func (l *lowerer) storeByte(base, off, val Reg) {
	slot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: val, Dst: NoReg, Imm: slot})
	l.emit(Instr{Op: OpStoreByte, A: base, B: off, Imm: slot})
}

// mapBytes copies s into a fresh buffer, passing each byte through
// transform. It is the shape of upper, lower and anything else that
// rewrites a string character by character.
func (l *lowerer) mapBytes(s Reg, transform func(b Reg) Reg) Reg {
	n := l.strLen(s)
	out := l.strAlloc(n)

	srcSlot := l.temp(vStr)
	l.emit(Instr{Op: OpStore, A: s, Dst: NoReg, Imm: srcSlot})
	dstSlot := l.temp(vStr)
	l.emit(Instr{Op: OpStore, A: out, Dst: NoReg, Imm: dstSlot})
	nSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: n, Dst: NoReg, Imm: nSlot})

	iSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: l.constant(0), Dst: NoReg, Imm: iSlot})

	top := l.newLabel()
	done := l.newLabel()
	l.mark(top)

	i := l.load(iSlot, vInt)
	more := l.compare(OpLt, i, l.load(nSlot, vInt))
	l.emit(Instr{Op: OpJumpNot, A: more, Dst: NoReg, Imm: done})

	b := l.loadByte(l.load(srcSlot, vStr), i)
	l.storeByte(l.load(dstSlot, vStr), i, transform(b))

	l.emit(Instr{Op: OpStore, A: l.arith(OpAdd, i, l.constant(1)), Dst: NoReg, Imm: iSlot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: top})
	l.mark(done)

	// The terminator, at the length rather than at wherever the loop
	// happened to stop.
	l.storeByte(l.load(dstSlot, vStr), l.load(nSlot, vInt), l.constant(0))
	return l.load(dstSlot, vStr)
}

func (l *lowerer) load(slot int64, t vty) Reg {
	d := l.newReg()
	l.regTy[d] = t
	l.emit(Instr{Op: OpLoad, Dst: d, A: NoReg, B: NoReg, Imm: slot})
	return d
}

// shiftIf adds delta to b when b is within [lo, hi]. That is the whole
// of ASCII case conversion, and it deliberately leaves anything outside
// that range alone rather than corrupting UTF-8 continuation bytes.
func (l *lowerer) shiftIf(b Reg, lo, hi, delta int64) Reg {
	slot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: b, Dst: NoReg, Imm: slot})

	skip := l.newLabel()
	tooLow := l.compare(OpLt, b, l.constant(lo))
	l.emit(Instr{Op: OpJumpIf, A: tooLow, Dst: NoReg, Imm: skip})
	tooHigh := l.compare(OpGt, b, l.constant(hi))
	l.emit(Instr{Op: OpJumpIf, A: tooHigh, Dst: NoReg, Imm: skip})
	l.emit(Instr{Op: OpStore, A: l.arith(OpAdd, b, l.constant(delta)),
		Dst: NoReg, Imm: slot})
	l.mark(skip)

	return l.load(slot, vInt)
}

// substring copies the half-open range [from, to). Both ends are clamped
// into the string rather than reported, matching what the Go backend
// does for slice(), so a program cannot crash on an off-by-one here.
func (l *lowerer) substring(s, from, to Reg) Reg {
	n := l.strLen(s)

	lo := l.clamp(from, l.constant(0), n)
	hi := l.clamp(to, lo, n)

	count := l.arith(OpSub, hi, lo)
	out := l.strAlloc(count)

	srcSlot := l.temp(vStr)
	l.emit(Instr{Op: OpStore, A: s, Dst: NoReg, Imm: srcSlot})
	dstSlot := l.temp(vStr)
	l.emit(Instr{Op: OpStore, A: out, Dst: NoReg, Imm: dstSlot})
	loSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: lo, Dst: NoReg, Imm: loSlot})
	cntSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: count, Dst: NoReg, Imm: cntSlot})

	iSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: l.constant(0), Dst: NoReg, Imm: iSlot})

	top := l.newLabel()
	done := l.newLabel()
	l.mark(top)
	i := l.load(iSlot, vInt)
	more := l.compare(OpLt, i, l.load(cntSlot, vInt))
	l.emit(Instr{Op: OpJumpNot, A: more, Dst: NoReg, Imm: done})

	at := l.arith(OpAdd, i, l.load(loSlot, vInt))
	l.storeByte(l.load(dstSlot, vStr), i, l.loadByte(l.load(srcSlot, vStr), at))

	l.emit(Instr{Op: OpStore, A: l.arith(OpAdd, i, l.constant(1)), Dst: NoReg, Imm: iSlot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: top})
	l.mark(done)

	l.storeByte(l.load(dstSlot, vStr), l.load(cntSlot, vInt), l.constant(0))
	return l.load(dstSlot, vStr)
}

// clamp returns v held between lo and hi.
func (l *lowerer) clamp(v, lo, hi Reg) Reg {
	slot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: v, Dst: NoReg, Imm: slot})

	notLow := l.newLabel()
	below := l.compare(OpLt, v, lo)
	l.emit(Instr{Op: OpJumpNot, A: below, Dst: NoReg, Imm: notLow})
	l.emit(Instr{Op: OpStore, A: lo, Dst: NoReg, Imm: slot})
	l.mark(notLow)

	held := l.load(slot, vInt)
	notHigh := l.newLabel()
	above := l.compare(OpGt, held, hi)
	l.emit(Instr{Op: OpJumpNot, A: above, Dst: NoReg, Imm: notHigh})
	l.emit(Instr{Op: OpStore, A: hi, Dst: NoReg, Imm: slot})
	l.mark(notHigh)

	return l.load(slot, vInt)
}

// indexOfStr finds needle in haystack, or -1. The naive scan is O(n*m)
// and is what the standard library of every language starts with; a
// program that needs better than that needs a different function, not a
// cleverer version of this one.
func (l *lowerer) indexOfStr(hay, needle Reg) Reg {
	hn := l.strLen(hay)
	nn := l.strLen(needle)

	haySlot := l.temp(vStr)
	l.emit(Instr{Op: OpStore, A: hay, Dst: NoReg, Imm: haySlot})
	needleSlot := l.temp(vStr)
	l.emit(Instr{Op: OpStore, A: needle, Dst: NoReg, Imm: needleSlot})
	hnSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: hn, Dst: NoReg, Imm: hnSlot})
	nnSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: nn, Dst: NoReg, Imm: nnSlot})

	resultSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: l.constant(-1), Dst: NoReg, Imm: resultSlot})

	iSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: l.constant(0), Dst: NoReg, Imm: iSlot})

	outer := l.newLabel()
	outerDone := l.newLabel()
	l.mark(outer)

	i := l.load(iSlot, vInt)
	last := l.arith(OpSub, l.load(hnSlot, vInt), l.load(nnSlot, vInt))
	past := l.compare(OpGt, i, last)
	l.emit(Instr{Op: OpJumpIf, A: past, Dst: NoReg, Imm: outerDone})

	jSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: l.constant(0), Dst: NoReg, Imm: jSlot})

	inner := l.newLabel()
	matched := l.newLabel()
	mismatch := l.newLabel()
	l.mark(inner)

	j := l.load(jSlot, vInt)
	moreJ := l.compare(OpLt, j, l.load(nnSlot, vInt))
	l.emit(Instr{Op: OpJumpNot, A: moreJ, Dst: NoReg, Imm: matched})

	a := l.loadByte(l.load(haySlot, vStr), l.arith(OpAdd, i, j))
	b := l.loadByte(l.load(needleSlot, vStr), j)
	same := l.compare(OpEq, a, b)
	l.emit(Instr{Op: OpJumpNot, A: same, Dst: NoReg, Imm: mismatch})

	l.emit(Instr{Op: OpStore, A: l.arith(OpAdd, j, l.constant(1)), Dst: NoReg, Imm: jSlot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: inner})

	l.mark(matched)
	l.emit(Instr{Op: OpStore, A: i, Dst: NoReg, Imm: resultSlot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: outerDone})

	l.mark(mismatch)
	l.emit(Instr{Op: OpStore, A: l.arith(OpAdd, i, l.constant(1)), Dst: NoReg, Imm: iSlot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: outer})

	l.mark(outerDone)
	return l.load(resultSlot, vInt)
}

// stringBuiltin dispatches the ones this backend has. Names and
// behaviour follow ../src exactly, because the differential test
// compares their output.
func (l *lowerer) stringBuiltin(c *Call, name string) (Reg, bool) {
	need := func(n int) bool {
		if len(c.Args) != n {
			l.errorAt(c, "%s takes %d argument(s), got %d", name, n, len(c.Args))
			return false
		}
		return true
	}
	str := func(i int) Reg {
		r := l.expr(c.Args[i])
		if l.regTy[r].k != kStr {
			l.errorAt(c, "%s needs a str for argument %d", name, i+1)
		}
		return r
	}

	switch name {
	case "upper":
		if !need(1) {
			return l.junk(), true
		}
		return l.mapBytes(str(0), func(b Reg) Reg {
			return l.shiftIf(b, 'a', 'z', -32)
		}), true

	case "lower":
		if !need(1) {
			return l.junk(), true
		}
		return l.mapBytes(str(0), func(b Reg) Reg {
			return l.shiftIf(b, 'A', 'Z', 32)
		}), true

	case "substr":
		if !need(3) {
			return l.junk(), true
		}
		s := str(0)
		return l.substring(s, l.expr(c.Args[1]), l.expr(c.Args[2])), true

	case "indexOf":
		if !need(2) {
			return l.junk(), true
		}
		return l.indexOfStr(str(0), str(1)), true

	case "contains":
		if !need(2) {
			return l.junk(), true
		}
		at := l.indexOfStr(str(0), str(1))
		return l.compare(OpGe, at, l.constant(0)), true

	case "startsWith":
		if !need(2) {
			return l.junk(), true
		}
		s, prefix := str(0), str(1)
		n := l.strLen(prefix)
		head := l.substring(s, l.constant(0), n)
		l.mod.needs("streq")
		d := l.newReg()
		l.regTy[d] = vBool
		l.emit(Instr{Op: OpStrEq, Dst: d, A: head, B: prefix})
		return d, true

	case "endsWith":
		if !need(2) {
			return l.junk(), true
		}
		s, suffix := str(0), str(1)
		sn := l.strLen(s)
		fn := l.strLen(suffix)
		tail := l.substring(s, l.arith(OpSub, sn, fn), sn)
		l.mod.needs("streq")
		d := l.newReg()
		l.regTy[d] = vBool
		l.emit(Instr{Op: OpStrEq, Dst: d, A: tail, B: suffix})
		return d, true

	case "charAt":
		if !need(2) {
			return l.junk(), true
		}
		s := str(0)
		at := l.expr(c.Args[1])
		return l.substring(s, at, l.arith(OpAdd, at, l.constant(1))), true

	case "repeat":
		if !need(2) {
			return l.junk(), true
		}
		return l.repeatStr(str(0), l.expr(c.Args[1])), true
	}

	return NoReg, false
}

// repeatStr joins n copies of s. It concatenates in a loop rather than
// sizing the result once, which allocates n times instead of once and is
// the obvious thing to improve when there is a reason to.
func (l *lowerer) repeatStr(s, n Reg) Reg {
	l.mod.needs("concat")

	accSlot := l.temp(vStr)
	empty := l.newReg()
	l.regTy[empty] = vStr
	l.emit(Instr{Op: OpStr, Dst: empty, A: NoReg, B: NoReg, Imm: l.mod.intern("")})
	l.emit(Instr{Op: OpStore, A: empty, Dst: NoReg, Imm: accSlot})

	srcSlot := l.temp(vStr)
	l.emit(Instr{Op: OpStore, A: s, Dst: NoReg, Imm: srcSlot})
	nSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: n, Dst: NoReg, Imm: nSlot})

	iSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: l.constant(0), Dst: NoReg, Imm: iSlot})

	top := l.newLabel()
	done := l.newLabel()
	l.mark(top)
	i := l.load(iSlot, vInt)
	more := l.compare(OpLt, i, l.load(nSlot, vInt))
	l.emit(Instr{Op: OpJumpNot, A: more, Dst: NoReg, Imm: done})

	joined := l.newReg()
	l.regTy[joined] = vStr
	l.emit(Instr{Op: OpConcat, Dst: joined, A: l.load(accSlot, vStr),
		B: l.load(srcSlot, vStr)})
	l.emit(Instr{Op: OpStore, A: joined, Dst: NoReg, Imm: accSlot})

	l.emit(Instr{Op: OpStore, A: l.arith(OpAdd, i, l.constant(1)), Dst: NoReg, Imm: iSlot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: top})
	l.mark(done)

	return l.load(accSlot, vStr)
}
