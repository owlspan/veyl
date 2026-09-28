package main

// The crash handler.
//
//	crash: access violation - an address that is not valid was read or written
//	    in parse
//
// A Veyl runtime error explains itself (see where.go). A crash is
// different: a bad address handed to mem.*, a native function writing
// where it should not, a stack that ran out. Windows would end the
// program with no word at all. Instead an unhandled-exception filter,
// installed as main starts, says what happened and which of the
// program's functions it happened in, after flushing what the program
// had printed so far, and the program ends as before.
//
// Which function comes from comparing the faulting address with where
// each one starts. The functions are laid out in the order they are
// emitted, each start is a rip-relative lea, so nothing here needs an
// absolute address or a table to relocate, and a label after the last
// one tells an address in the program from one in a DLL.

const crashSym = "crashhandler"

// Exception codes worth a name.
var crashNames = []struct {
	code int64
	text string
}{
	{0xC0000005, "access violation - an address that is not valid was read or written"},
	{0xC00000FD, "stack overflow - probably a function that calls itself without end"},
	{0xC0000094, "integer division by zero"},
	{0xC000001D, "illegal instruction"},
	{0xC0000096, "privileged instruction"},
	{0x80000003, "breakpoint"},
}

// installCrashHandler is the first thing main does.
func (l *lowerer) installCrashHandler() {
	if lowerForDLL {
		return // a guest does not take over its host's crashes
	}
	l.mod.needs("crash")
	l.ccall("SetUnhandledExceptionFilter", []Reg{l.symAddr(fnSym(crashSym))},
		[]vty{vInt}, vInt, false, false)
	// A stack overflow runs the handler on what is left of the stack
	// that overflowed, which is almost nothing; this keeps 64 KB back
	// for it on the main thread.
	guarantee := l.ptrSlot()
	l.emit(Instr{Op: OpStoreMem, A: guarantee, B: l.constant(64 << 10), Imm: 0})
	l.ccall("SetThreadStackGuarantee", []Reg{guarantee}, []vty{vInt}, vInt, true, false)
}

// buildCrashHandler writes the filter, once every function it might
// have to name exists.
func (l *lowerer) buildCrashHandler() {
	if !l.mod.Helpers["crash"] {
		return
	}
	funcs := append([]*Func(nil), l.mod.Funcs...)
	l.helperFunc(crashSym, []vty{vInt}, vInt, func(a []Reg) {
		ptrs := a[0]
		rec := l.peekWord(ptrs, 0)
		ctx := l.peekWord(ptrs, wordSize)
		code := l.loadWidth(rec, memU32)
		codeSlot := l.temp(vInt)
		l.emit(Instr{Op: OpStore, A: code, Dst: NoReg, Imm: codeSlot})
		rip := l.peekWord(ctx, 0xF8) // CONTEXT.Rip
		ripSlot := l.temp(vInt)
		l.emit(Instr{Op: OpStore, A: rip, Dst: NoReg, Imm: ripSlot})

		l.ccall("fflush", []Reg{l.constant(0)}, []vty{vInt}, vInt, true, false)

		what := l.temp(vStr)
		l.emit(Instr{Op: OpStore, A: l.strLit("an exception Windows raised"), Dst: NoReg, Imm: what})
		for _, c := range crashNames {
			next := l.newLabel()
			l.emit(Instr{Op: OpJumpNot, A: l.compare(OpEq, l.load(codeSlot, vInt), l.constant(c.code)),
				Dst: NoReg, Imm: next})
			l.emit(Instr{Op: OpStore, A: l.strLit(c.text), Dst: NoReg, Imm: what})
			l.mark(next)
		}

		where := l.temp(vStr)
		l.emit(Instr{Op: OpStore, A: l.strLit("the runtime"), Dst: NoReg, Imm: where})
		for _, f := range funcs {
			sym := "main"
			name := "the top level"
			if f.Name != "main" {
				sym, name = fnSym(f.Name), f.Name
			}
			next := l.newLabel()
			l.emit(Instr{Op: OpJumpNot, A: l.compare(OpGe, l.load(ripSlot, vInt), l.symAddr(sym)),
				Dst: NoReg, Imm: next})
			l.emit(Instr{Op: OpStore, A: l.strLit(name), Dst: NoReg, Imm: where})
			l.mark(next)
		}
		native := l.newLabel()
		l.emit(Instr{Op: OpJumpNot, A: l.compare(OpGe, l.load(ripSlot, vInt), l.symAddr("__vy_text_end")),
			Dst: NoReg, Imm: native})
		l.emit(Instr{Op: OpStore, A: l.strLit("native code outside the program"), Dst: NoReg, Imm: where})
		l.mark(native)

		l.mod.needs("concat")
		msg := l.load(what, vStr)
		for _, part := range []Reg{l.strLit("\n    in "), l.load(where, vStr), l.strLit("\n")} {
			d := l.newReg()
			l.regTy[d] = vStr
			l.emit(Instr{Op: OpConcat, Dst: d, A: msg, B: part})
			msg = d
		}
		head := l.strLit("crash: ")
		full := l.newReg()
		l.regTy[full] = vStr
		l.emit(Instr{Op: OpConcat, Dst: full, A: head, B: msg})
		n := l.ccall("strlen", []Reg{full}, []vty{vInt}, vInt, false, false)
		l.ccall("_write", []Reg{l.constant(2), full, n}, []vty{vInt, vInt, vInt}, vInt, true, false)
		// EXCEPTION_EXECUTE_HANDLER: end the process, with the exception
		// code as its exit code, as an unhandled crash always has.
		l.emit(Instr{Op: OpRet, A: l.constant(1), Dst: NoReg})
	})
}
