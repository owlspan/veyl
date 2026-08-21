package main

// Nullables, `?T`.
//
// One word. Zero is nil; anything else is a pointer to a one-word heap
// box holding the value. Same shape as a result, and for the same
// reason: the layout does not depend on what is inside, so `?int` and
// `?str` differ only in how the word inside the box is read back out,
// and a nullable stays one word wherever it is held. That keeps the IR
// three-address and needs nothing of the calling convention.
//
// A str, a list, a map and a struct are already pointers, so a
// null-pointer representation would cost nothing for them. It is not
// worth it: `?int` has no spare value to mean nil, so those would need
// boxing anyway, and two representations means every use site has to ask
// which one it is looking at.
//
// The checker does the hard half. It proves `x != nil` and marks the
// uses inside that block with Ident.Narrowed, so this file never has to
// work out where a dereference is safe - it dereferences exactly the
// idents the checker marked, and nothing else.

// nilValue is the nil of a given nullable type: the zero word.
func (l *lowerer) nilValue(t vty) Reg {
	d := l.constant(0)
	l.regTy[d] = t
	return d
}

// nullBox wraps a value so it can be held as a ?T.
func (l *lowerer) nullBox(v Reg, t vty) Reg {
	box := l.allocObj(l.constant(wordSize), l.nullTag(t))
	l.emit(Instr{Op: OpStoreMem, A: box, B: v, Imm: 0})
	l.regTy[box] = t
	return box
}

// nullTag says whether the boxed word is a pointer, which is what the
// object header records for a collector that does not exist yet.
func (l *lowerer) nullTag(t vty) int64 {
	if t.notNull().holdsPointer() {
		return tagPtrs
	}
	return tagWords
}

// nullValue reads the value out of a non-nil ?T.
//
// Nothing checks the pointer here. It does not have to: the only callers
// are uses the checker marked as narrowed, which means it has already
// proved this cannot be nil. An unmarked use never reaches this, because
// the checker rejects reading a ?T without proving it first.
func (l *lowerer) nullValue(box Reg, t vty) Reg {
	d := l.newReg()
	l.regTy[d] = t.notNull()
	l.emit(Instr{Op: OpLoadMem, Dst: d, A: box, B: NoReg, Imm: 0})
	return d
}

// isNil is `x == nil`, which is the whole of a nullable's runtime.
func (l *lowerer) isNil(v Reg) Reg {
	return l.compare(OpEq, v, l.constant(0))
}

// widenNull boxes a plain T where a ?T was wanted. The checker marks
// every such spot with a Widen, so this is the only place that boxes.
func (l *lowerer) widenNull(x *Widen, t vty) Reg {
	v := l.expr(x.X)
	if l.regTy[v].null && l.regTy[v].k == kVoid {
		// `nil` widened to a ?T is just the zero word - there is
		// nothing to box.
		return l.nilValue(t)
	}
	if t.notNull().k == kFloat && l.regTy[v].k == kInt {
		v = l.toFloat(v)
	}
	return l.nullBox(v, t)
}

// writeNull renders a nullable the way print does: the value, or the
// word nil. The Go backend prints a nil as "nil" rather than Go's
// "<nil>", so this has to as well.
func (l *lowerer) writeNull(n Node, v Reg, t vty) {
	isNilLabel := l.newLabel()
	done := l.newLabel()

	l.emit(Instr{Op: OpJumpIf, A: l.isNil(v), Dst: NoReg, Imm: isNilLabel})
	l.writeValue(n, l.nullValue(v, t), t.notNull())
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: done})

	l.mark(isNilLabel)
	l.writeLit("nil")

	l.mark(done)
}

// strOfNull is writeNull's counterpart for interpolation and str(),
// where a nullable renders unquoted - "printed 7 nil", not
// "printed 7 \"nil\"".
func (l *lowerer) strOfNull(n Node, v Reg, t vty) Reg {
	out := l.temp(vStr)

	isNilLabel := l.newLabel()
	done := l.newLabel()

	l.emit(Instr{Op: OpJumpIf, A: l.isNil(v), Dst: NoReg, Imm: isNilLabel})
	l.emit(Instr{Op: OpStore, A: l.toStr(l.nullValue(v, t), n), Dst: NoReg, Imm: out})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: done})

	l.mark(isNilLabel)
	l.emit(Instr{Op: OpStore, A: l.strLit("nil"), Dst: NoReg, Imm: out})

	l.mark(done)
	return l.load(out, vStr)
}

// findExpr is `find(m, k)`: the value for a key as a ?V, so that absent
// and zero stay distinguishable where a bare index cannot tell them
// apart.
func (l *lowerer) findExpr(c *Call) Reg {
	m := l.expr(c.Args[0])
	t := l.regTy[m]
	if t.k != kMap {
		l.errorAt(c, "find expects a map, got %s", t)
		return l.junk()
	}
	key := l.expr(c.Args[1])
	if l.regTy[key].k != t.key {
		l.errorAt(c, "this map is keyed by %s, but the key is %s", t.keyType(), l.regTy[key])
		return l.junk()
	}

	want := vNullOf(t.elemType())
	out := l.temp(want)

	idxSlot := l.temp(vInt)
	hitSlot := l.temp(vInt)
	l.mapScan(m, key, t, idxSlot, hitSlot)

	miss := l.newLabel()
	done := l.newLabel()
	hit := l.compare(OpNe, l.load(hitSlot, vInt), l.constant(0))
	l.emit(Instr{Op: OpJumpNot, A: hit, Dst: NoReg, Imm: miss})

	l.emit(Instr{Op: OpStore,
		A:   l.nullBox(l.cellAt(m, mapValsOff, l.load(idxSlot, vInt), t.elemType()), want),
		Dst: NoReg, Imm: out})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: done})

	l.mark(miss)
	l.emit(Instr{Op: OpStore, A: l.nilValue(want), Dst: NoReg, Imm: out})

	l.mark(done)
	return l.load(out, want)
}
