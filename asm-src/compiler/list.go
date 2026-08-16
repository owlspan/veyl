package main

// Lists, built out of the raw memory ops rather than hand-written
// assembly.
//
// A list value is a pointer to a three-word header:
//
//	[ptr+0]   length
//	[ptr+8]   capacity
//	[ptr+16]  pointer to the elements
//
// The header is allocated separately from the elements so that growing
// the list replaces the element block without the header pointer
// changing. If the elements lived inline, every push that grew the list
// would invalidate every copy of the list value that anyone held, which
// is the kind of bug that shows up months later in one program.
//
// Every element is eight bytes, whether it is an int, a bool or a
// pointer to a string. That uniformity is what lets one implementation
// serve every element type without generics or monomorphisation.
//
// None of this is ever freed. A list that goes out of scope leaks its
// header and its elements. That is the state of a backend with no
// collector, and it is the single largest thing between here and the Go
// backend.

const (
	listLenOff  = 0
	listCapOff  = 8
	listDataOff = 16
	listHeader  = 24
	wordSize    = 8
)

// initialCap is what an empty list grows to on its first push. Four is
// small enough not to waste much on the many short lists a program
// makes, and large enough that a handful of pushes do not each realloc.
const initialCap = 4

// newList allocates a header and an element block, and returns the
// header pointer.
func (l *lowerer) newList(t vty, capacity int64) Reg {
	if capacity < 1 {
		capacity = 1
	}

	hdr := l.alloc(l.constant(listHeader))
	l.regTy[hdr] = t

	data := l.alloc(l.constant(capacity * wordSize))

	l.emit(Instr{Op: OpStoreMem, A: hdr, B: l.constant(0), Imm: listLenOff})
	l.emit(Instr{Op: OpStoreMem, A: hdr, B: l.constant(capacity), Imm: listCapOff})
	l.emit(Instr{Op: OpStoreMem, A: hdr, B: data, Imm: listDataOff})
	return hdr
}

func (l *lowerer) constant(n int64) Reg {
	d := l.newReg()
	l.regTy[d] = vInt
	l.emit(Instr{Op: OpConst, Dst: d, A: NoReg, B: NoReg, Imm: n})
	return d
}

func (l *lowerer) alloc(bytes Reg) Reg {
	l.mod.needs("alloc")
	d := l.newReg()
	l.regTy[d] = vInt
	l.emit(Instr{Op: OpAlloc, Dst: d, A: bytes, B: NoReg})
	return d
}

// field reads one word out of the header.
func (l *lowerer) field(list Reg, off int64, t vty) Reg {
	d := l.newReg()
	l.regTy[d] = t
	l.emit(Instr{Op: OpLoadMem, Dst: d, A: list, B: NoReg, Imm: off})
	return d
}

func (l *lowerer) arith(op Op, a, b Reg) Reg {
	d := l.newReg()
	l.regTy[d] = vInt
	l.emit(Instr{Op: op, Dst: d, A: a, B: b})
	return d
}

func (l *lowerer) compare(op Op, a, b Reg) Reg {
	d := l.newReg()
	l.regTy[d] = vBool
	l.emit(Instr{Op: op, Dst: d, A: a, B: b})
	return d
}

// listPush appends one element, growing the element block when it is
// full. Growth doubles, so a list built by repeated pushes costs
// amortised constant time rather than quadratic copying.
func (l *lowerer) listPush(list, val Reg) {
	length := l.field(list, listLenOff, vInt)
	capacity := l.field(list, listCapOff, vInt)

	ready := l.newLabel()
	room := l.compare(OpLt, length, capacity)
	l.emit(Instr{Op: OpJumpIf, A: room, Dst: NoReg, Imm: ready, Comment: "push: room?"})

	// Growing. The new capacity goes through a slot because two branches
	// decide it, which is exactly what a virtual register cannot express
	// while every one of them has a single definition site.
	capSlot := l.temp(vInt)
	doubled := l.arith(OpMul, capacity, l.constant(2))
	l.emit(Instr{Op: OpStore, A: doubled, Dst: NoReg, Imm: capSlot})

	haveCap := l.newLabel()
	big := l.compare(OpGe, doubled, l.constant(initialCap))
	l.emit(Instr{Op: OpJumpIf, A: big, Dst: NoReg, Imm: haveCap})
	l.emit(Instr{Op: OpStore, A: l.constant(initialCap), Dst: NoReg, Imm: capSlot})
	l.mark(haveCap)

	newCap := l.newReg()
	l.regTy[newCap] = vInt
	l.emit(Instr{Op: OpLoad, Dst: newCap, A: NoReg, B: NoReg, Imm: capSlot})

	fresh := l.alloc(l.arith(OpMul, newCap, l.constant(wordSize)))
	old := l.field(list, listDataOff, vInt)

	// Copy the old elements one word at a time. memcpy would be faster
	// and would mean another external symbol; this is a loop the byte
	// writer already knows how to emit.
	iSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: l.constant(0), Dst: NoReg, Imm: iSlot})
	top := l.newLabel()
	copied := l.newLabel()
	l.mark(top)
	i := l.newReg()
	l.regTy[i] = vInt
	l.emit(Instr{Op: OpLoad, Dst: i, A: NoReg, B: NoReg, Imm: iSlot})
	more := l.compare(OpLt, i, length)
	l.emit(Instr{Op: OpJumpNot, A: more, Dst: NoReg, Imm: copied})

	from := l.newReg()
	l.regTy[from] = vInt
	l.emit(Instr{Op: OpIndexAddr, Dst: from, A: old, B: i})
	v := l.newReg()
	l.regTy[v] = vInt
	l.emit(Instr{Op: OpLoadMem, Dst: v, A: from, B: NoReg, Imm: 0})
	to := l.newReg()
	l.regTy[to] = vInt
	l.emit(Instr{Op: OpIndexAddr, Dst: to, A: fresh, B: i})
	l.emit(Instr{Op: OpStoreMem, A: to, B: v, Imm: 0})

	l.emit(Instr{Op: OpStore, A: l.arith(OpAdd, i, l.constant(1)), Dst: NoReg, Imm: iSlot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: top})
	l.mark(copied)

	l.emit(Instr{Op: OpStoreMem, A: list, B: newCap, Imm: listCapOff})
	l.emit(Instr{Op: OpStoreMem, A: list, B: fresh, Imm: listDataOff})

	l.mark(ready)

	// The length and the data pointer are re-read here rather than
	// reused from above: the growth path may have replaced both.
	at := l.field(list, listLenOff, vInt)
	data := l.field(list, listDataOff, vInt)
	addr := l.newReg()
	l.regTy[addr] = vInt
	l.emit(Instr{Op: OpIndexAddr, Dst: addr, A: data, B: at})
	l.emit(Instr{Op: OpStoreMem, A: addr, B: val, Imm: 0})
	l.emit(Instr{Op: OpStoreMem, A: list, B: l.arith(OpAdd, at, l.constant(1)),
		Imm: listLenOff})
}

// elemAddr bounds-checks an index and returns the address of that
// element. Out of range exits with the same sentence the Go backend
// prints, because the differential test compares stderr too.
func (l *lowerer) elemAddr(list, idx Reg) Reg {
	length := l.field(list, listLenOff, vInt)

	ok := l.newLabel()
	bad := l.newLabel()

	negative := l.compare(OpLt, idx, l.constant(0))
	l.emit(Instr{Op: OpJumpIf, A: negative, Dst: NoReg, Imm: bad, Comment: "bounds"})
	tooBig := l.compare(OpGe, idx, length)
	l.emit(Instr{Op: OpJumpIf, A: tooBig, Dst: NoReg, Imm: bad})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: ok})

	l.mark(bad)
	l.mod.needs("bounds")
	l.emit(Instr{Op: OpBoundsFail, A: idx, B: length, Dst: NoReg})

	l.mark(ok)
	data := l.field(list, listDataOff, vInt)
	addr := l.newReg()
	l.regTy[addr] = vInt
	l.emit(Instr{Op: OpIndexAddr, Dst: addr, A: data, B: idx})
	return addr
}

func (l *lowerer) listGet(list, idx Reg, elem vty) Reg {
	addr := l.elemAddr(list, idx)
	d := l.newReg()
	l.regTy[d] = elem
	l.emit(Instr{Op: OpLoadMem, Dst: d, A: addr, B: NoReg, Imm: 0})
	return d
}

func (l *lowerer) listSet(list, idx, val Reg) {
	addr := l.elemAddr(list, idx)
	l.emit(Instr{Op: OpStoreMem, A: addr, B: val, Imm: 0})
}

// printList writes a list the way the Go backend does: [1, 2, 3], with
// strings quoted. Built from the write ops rather than a runtime helper,
// so it costs no assembly of its own.
func (l *lowerer) printList(list Reg, t vty) {
	l.mod.needs("write")
	l.writeLit("[")

	length := l.field(list, listLenOff, vInt)
	iSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: l.constant(0), Dst: NoReg, Imm: iSlot})

	top := l.newLabel()
	done := l.newLabel()
	l.mark(top)

	i := l.newReg()
	l.regTy[i] = vInt
	l.emit(Instr{Op: OpLoad, Dst: i, A: NoReg, B: NoReg, Imm: iSlot})
	more := l.compare(OpLt, i, length)
	l.emit(Instr{Op: OpJumpNot, A: more, Dst: NoReg, Imm: done})

	noComma := l.newLabel()
	firstOne := l.compare(OpEq, i, l.constant(0))
	l.emit(Instr{Op: OpJumpIf, A: firstOne, Dst: NoReg, Imm: noComma})
	l.writeLit(", ")
	l.mark(noComma)

	addr := l.newReg()
	l.regTy[addr] = vInt
	data := l.field(list, listDataOff, vInt)
	l.emit(Instr{Op: OpIndexAddr, Dst: addr, A: data, B: i})
	v := l.newReg()
	l.regTy[v] = t.elemType()
	l.emit(Instr{Op: OpLoadMem, Dst: v, A: addr, B: NoReg, Imm: 0})

	switch t.elem {
	case kStr:
		l.writeLit("\"")
		l.emit(Instr{Op: OpWriteStr, A: v, Dst: NoReg})
		l.writeLit("\"")
	case kBool:
		l.mod.needs("booltostr")
		s := l.newReg()
		l.regTy[s] = vStr
		l.emit(Instr{Op: OpBoolToStr, Dst: s, A: v, B: NoReg})
		l.emit(Instr{Op: OpWriteStr, A: s, Dst: NoReg})
	default:
		l.emit(Instr{Op: OpWriteInt, A: v, Dst: NoReg})
	}

	l.emit(Instr{Op: OpStore, A: l.arith(OpAdd, i, l.constant(1)), Dst: NoReg, Imm: iSlot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: top})
	l.mark(done)

	l.writeLit("]\n")
}

func (l *lowerer) writeLit(s string) {
	r := l.newReg()
	l.regTy[r] = vStr
	l.emit(Instr{Op: OpStr, Dst: r, A: NoReg, B: NoReg, Imm: l.mod.intern(s)})
	l.emit(Instr{Op: OpWriteStr, A: r, Dst: NoReg})
}

// forList lowers `for x in xs`.
//
// The length is read once, before the first iteration, matching Go's
// range: pushing onto the list inside the loop does not make it run
// longer. The list itself is held in a slot so that the loop reads the
// same list every time even if the variable it came from is reassigned.
func (l *lowerer) forList(st *ForStmt) {
	coll := l.expr(st.Coll)
	t := l.regTy[coll]
	if t.k != kList {
		l.errorAt(st, "only a list can be iterated on this backend so far")
		return
	}

	l.pushScope()

	listSlot := l.temp(t)
	l.emit(Instr{Op: OpStore, A: coll, Dst: NoReg, Imm: listSlot, Comment: "for ... in"})

	held := l.newReg()
	l.regTy[held] = t
	l.emit(Instr{Op: OpLoad, Dst: held, A: NoReg, B: NoReg, Imm: listSlot})
	length := l.field(held, listLenOff, vInt)
	lenSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: length, Dst: NoReg, Imm: lenSlot})

	iSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: l.constant(0), Dst: NoReg, Imm: iSlot})

	varSlot := l.declare(st.Var, t.elemType())

	top := l.newLabel()
	cont := l.newLabel()
	done := l.newLabel()
	l.mark(top)

	i := l.newReg()
	l.regTy[i] = vInt
	l.emit(Instr{Op: OpLoad, Dst: i, A: NoReg, B: NoReg, Imm: iSlot})
	n := l.newReg()
	l.regTy[n] = vInt
	l.emit(Instr{Op: OpLoad, Dst: n, A: NoReg, B: NoReg, Imm: lenSlot})
	more := l.compare(OpLt, i, n)
	l.emit(Instr{Op: OpJumpNot, A: more, Dst: NoReg, Imm: done})

	list := l.newReg()
	l.regTy[list] = t
	l.emit(Instr{Op: OpLoad, Dst: list, A: NoReg, B: NoReg, Imm: listSlot})
	l.emit(Instr{Op: OpStore, A: l.listGet(list, i, t.elemType()), Dst: NoReg,
		Imm: varSlot, Comment: st.Var})

	l.loops = append(l.loops, loopTarget{brk: done, cont: cont})
	l.stmt(st.Body)
	l.loops = l.loops[:len(l.loops)-1]

	// continue goes to the increment, not the test, or the index never
	// advances and the loop never ends.
	l.mark(cont)
	cur := l.newReg()
	l.regTy[cur] = vInt
	l.emit(Instr{Op: OpLoad, Dst: cur, A: NoReg, B: NoReg, Imm: iSlot})
	l.emit(Instr{Op: OpStore, A: l.arith(OpAdd, cur, l.constant(1)), Dst: NoReg, Imm: iSlot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: top})

	l.mark(done)
	l.popScope()
}
