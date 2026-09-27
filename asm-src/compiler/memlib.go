package main

// Raw memory: native blocks outside the collector, and reads and writes
// of every width at an address.
//
// An address is an int, the same as a ptr on an extern declaration, so
// pointer arithmetic is integer arithmetic and an address can be stored,
// compared and printed like any other number. Nothing here checks that
// an address is valid - reading one that is not stops the program the
// way it would in C.

// The width each read and write names.
var memKinds = map[string]int64{
	"mem.readU8": memU8, "mem.readI8": memI8,
	"mem.readU16": memU16, "mem.readI16": memI16,
	"mem.readU32": memU32, "mem.readI32": memI32,
	"mem.readI64": memI64,
	"mem.readF32": memF32, "mem.readF64": memF64,
	"mem.write8": memU8, "mem.write16": memU16,
	"mem.write32": memU32, "mem.write64": memI64,
	"mem.writeF32": memF32, "mem.writeF64": memF64,
}

// HEAP_ZERO_MEMORY: a block from mem.alloc starts out as zeros, so an
// extern struct laid over it reads as zero until written.
const heapZeroMemory = 8

func (l *lowerer) rawMemBuiltin(c *Call, name string) (Reg, bool) {
	arity := func(n int) bool {
		if len(c.Args) != n {
			l.errorAt(c, "%s takes %d argument(s), got %d", name, n, len(c.Args))
			return false
		}
		return true
	}

	if kind, ok := memKinds[name]; ok {
		if name[4] == 'r' { // a read
			if !arity(1) {
				return l.junk(), true
			}
			return l.loadWidth(l.intArg(c, 0), kind), true
		}
		if !arity(2) {
			return l.junk(), true
		}
		addr := l.intArg(c, 0)
		var v Reg
		if kind == memF32 || kind == memF64 {
			v = l.numeric(c.Args[1])
		} else {
			v = l.intArg(c, 1)
		}
		l.storeWidth(addr, v, kind)
		return l.void(), true
	}

	switch name {
	case "mem.alloc":
		if !arity(1) {
			return l.junk(), true
		}
		return l.heapCall("HeapAlloc", []Reg{l.constant(heapZeroMemory), l.intArg(c, 0)}), true

	case "mem.resize":
		// A null address is a fresh block, as realloc treats it, since
		// HeapReAlloc itself refuses one.
		if !arity(2) {
			return l.junk(), true
		}
		p, n := l.intArg(c, 0), l.intArg(c, 1)
		out := l.temp(vInt)
		fresh, done := l.newLabel(), l.newLabel()
		l.emit(Instr{Op: OpJumpIf, A: l.compare(OpEq, p, l.constant(0)), Dst: NoReg, Imm: fresh})
		l.emit(Instr{Op: OpStore, Dst: NoReg,
			A:   l.heapCall("HeapReAlloc", []Reg{l.constant(heapZeroMemory), p, n}),
			Imm: out})
		l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: done})
		l.mark(fresh)
		l.emit(Instr{Op: OpStore, Dst: NoReg,
			A:   l.heapCall("HeapAlloc", []Reg{l.constant(heapZeroMemory), n}),
			Imm: out})
		l.mark(done)
		return l.load(out, vInt), true

	case "mem.free":
		// Freeing 0 does nothing, like free(NULL).
		if !arity(1) {
			return l.junk(), true
		}
		p := l.intArg(c, 0)
		skip := l.newLabel()
		l.emit(Instr{Op: OpJumpIf, A: l.compare(OpEq, p, l.constant(0)), Dst: NoReg, Imm: skip})
		l.heapCall("HeapFree", []Reg{l.constant(0), p})
		l.mark(skip)
		return l.void(), true

	case "mem.copy":
		// memmove, so the two ranges may overlap.
		if !arity(3) {
			return l.junk(), true
		}
		l.ccall("memmove", []Reg{l.intArg(c, 0), l.intArg(c, 1), l.intArg(c, 2)},
			[]vty{vInt, vInt, vInt}, vInt, false, false)
		return l.void(), true

	case "mem.fill":
		if !arity(3) {
			return l.junk(), true
		}
		l.ccall("memset", []Reg{l.intArg(c, 0), l.intArg(c, 1), l.intArg(c, 2)},
			[]vty{vInt, vInt, vInt}, vInt, false, false)
		return l.void(), true

	case "mem.str":
		// The NUL-terminated text at an address, copied. A null address
		// reads as "" rather than faulting.
		if !arity(1) {
			return l.junk(), true
		}
		p := l.intArg(c, 0)
		out := l.temp(vStr)
		l.emit(Instr{Op: OpStore, A: l.emptyStr(), Dst: NoReg, Imm: out})
		skip := l.newLabel()
		l.emit(Instr{Op: OpJumpIf, A: l.compare(OpEq, p, l.constant(0)), Dst: NoReg, Imm: skip})
		n := l.ccall("strlen", []Reg{p}, []vty{vInt}, vInt, false, false)
		l.emit(Instr{Op: OpStore, A: l.strFromMem(p, n), Dst: NoReg, Imm: out})
		l.mark(skip)
		return l.load(out, vStr), true

	case "mem.strN":
		// At most n bytes, stopping early at a NUL: a fixed char[n]
		// field that fills its space has no terminator.
		if !arity(2) {
			return l.junk(), true
		}
		p, limit := l.intArg(c, 0), l.intArg(c, 1)
		out := l.temp(vInt)
		l.emit(Instr{Op: OpStore, A: limit, Dst: NoReg, Imm: out})
		zero := l.ccall("memchr", []Reg{p, l.constant(0), limit},
			[]vty{vInt, vInt, vInt}, vInt, false, false)
		skip := l.newLabel()
		l.emit(Instr{Op: OpJumpIf, A: l.compare(OpEq, zero, l.constant(0)), Dst: NoReg, Imm: skip})
		l.emit(Instr{Op: OpStore, A: l.arith(OpSub, zero, p), Dst: NoReg, Imm: out})
		l.mark(skip)
		return l.strFromMem(p, l.load(out, vInt)), true

	case "mem.protect":
		// VirtualProtect, with the protection spelled the way people
		// say it. Whole pages change: the range is widened to them.
		if !arity(3) {
			return l.junk(), true
		}
		p, n := l.intArg(c, 0), l.intArg(c, 1)
		mode := l.expr(c.Args[2])
		if l.regTy[mode].k != kStr {
			l.errorAt(c.Args[2], "mem.protect expects a mode such as \"rw\", got %s", l.regTy[mode])
			return l.junk(), true
		}
		flags := l.temp(vInt)
		l.emit(Instr{Op: OpStore, A: l.constant(0), Dst: NoReg, Imm: flags})
		for _, m := range []struct {
			text string
			page int64
		}{{"", 1}, {"r", 2}, {"rw", 4}, {"x", 0x10}, {"rx", 0x20}, {"rwx", 0x40}} {
			next := l.newLabel()
			l.emit(Instr{Op: OpJumpNot, A: l.strEq(mode, l.strLit(m.text)), Dst: NoReg, Imm: next})
			l.emit(Instr{Op: OpStore, A: l.constant(m.page), Dst: NoReg, Imm: flags})
			l.mark(next)
		}
		out := l.temp(vBool)
		l.emit(Instr{Op: OpStore, A: l.boolConst(false), Dst: NoReg, Imm: out})
		bad := l.newLabel()
		l.emit(Instr{Op: OpJumpIf, A: l.compare(OpEq, l.load(flags, vInt), l.constant(0)),
			Dst: NoReg, Imm: bad})
		old := l.ptrSlot()
		ok := l.ccall("VirtualProtect", []Reg{p, n, l.load(flags, vInt), old},
			[]vty{vInt, vInt, vInt, vInt}, vInt, true, false)
		l.emit(Instr{Op: OpStore, A: l.compare(OpNe, ok, l.constant(0)), Dst: NoReg, Imm: out})
		l.mark(bad)
		return l.load(out, vBool), true

	case "mem.bytes":
		if !arity(2) {
			return l.junk(), true
		}
		return l.bytesFromMem(l.intArg(c, 0), l.intArg(c, 1)), true

	case "mem.addr":
		// Where a bytes, a str or an extern struct lives. A bytes or a
		// str allocated by Veyl stays put for as long as something
		// still refers to it.
		if !arity(1) {
			return l.junk(), true
		}
		v := l.expr(c.Args[0])
		switch {
		case l.regTy[v].k == kBytes, l.regTy[v].k == kStr, l.isView(l.regTy[v]):
		default:
			l.errorAt(c.Args[0], "mem.addr expects bytes, a str or an extern struct, got %s", l.regTy[v])
			return l.junk(), true
		}
		d := l.newReg()
		l.regTy[d] = vInt
		l.emit(Instr{Op: OpAdd, Dst: d, A: v, B: l.constant(0)})
		return d, true
	}
	return NoReg, false
}

// heapCall calls one of the Heap functions on the process heap, which
// every one of them takes first.
func (l *lowerer) heapCall(fn string, rest []Reg) Reg {
	heap := l.ccall("GetProcessHeap", nil, nil, vInt, false, false)
	args := append([]Reg{heap}, rest...)
	types := make([]vty, len(args))
	for i := range types {
		types[i] = vInt
	}
	ret32 := fn == "HeapFree" // a BOOL; the other two return a pointer
	return l.ccall(fn, args, types, vInt, ret32, false)
}

// loadWidth reads a value of the given width at an address.
func (l *lowerer) loadWidth(addr Reg, kind int64) Reg {
	d := l.newReg()
	l.regTy[d] = vInt
	if kind == memF32 || kind == memF64 {
		l.regTy[d] = vFloat
	}
	l.emit(Instr{Op: OpPeek, Dst: d, A: addr, B: NoReg, Imm: kind})
	return d
}

// storeWidth writes one.
func (l *lowerer) storeWidth(addr, v Reg, kind int64) {
	l.emit(Instr{Op: OpPoke, Dst: NoReg, A: addr, B: v, Imm: kind})
}
