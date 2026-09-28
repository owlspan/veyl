package main

// Callbacks: a Veyl function handed to native code as a C function
// pointer.
//
//	extern fn EnumWindows(each: fn(ptr, ptr) -> bool, data: ptr) -> bool from "user32"
//
//	fn onWindow(hwnd: int, data: int) -> bool { ... }
//	EnumWindows(onWindow, 0)
//
// A Veyl function already follows the Windows x64 convention - the
// arguments in rcx, rdx, r8, r9 or xmm0 to xmm3 by position, the rest on
// the stack, the result in rax or xmm0, and nothing the caller relies on
// left changed - so native code can call one directly. task.go starts
// threads on Veyl functions for the same reason.
//
// What does not line up is width. A C int or BOOL is 32 bits, and the
// top half of the 64-bit register it arrives in is whatever the caller
// left there, where Veyl reads every int as the whole register. So the
// pointer native code is given is not the function itself but a stub
// that sign-extends each argument the callback type declares as `int`,
// normalises each `bool` to 0 or 1, and jumps to the function. The
// function then returns straight to the native caller.
//
// The spellings follow the rest of the extern boundary: `int` and
// `bool` are the C types, `ptr` is a full 64-bit value, so a handle, a
// pointer or an LPARAM is declared `ptr`. A float is a double.
//
// Only a function declared with fn can be a callback. A closure carries
// its environment in a register native code knows nothing about.

import (
	"fmt"
	"strings"
)

// A widenArg is one callback argument the stub has to fix up.
type widenArg struct {
	pos    int
	isBool bool
}

// A Thunk is one stub: its own symbol, the Veyl function it enters, and
// the arguments to widen on the way.
type Thunk struct {
	Sym    string
	Target string
	Widen  []widenArg
}

// callbackWidening reads a callback type as written, fn(ptr, int) ->
// bool, and lists the arguments that arrive as 32-bit C values. The
// checked type cannot answer this: ptr and int are the same type once
// checked, and the difference is exactly what matters here.
func callbackWidening(text string) []widenArg {
	s := strings.TrimSpace(text)
	if !strings.HasPrefix(s, "fn(") {
		return nil
	}
	depth, end := 0, -1
	for i := 2; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil
	}
	inner := strings.TrimSpace(s[3:end])
	if inner == "" {
		return nil
	}
	var out []widenArg
	for i, part := range strings.Split(inner, ",") {
		switch strings.TrimSpace(part) {
		case "int":
			out = append(out, widenArg{pos: i})
		case "bool":
			out = append(out, widenArg{pos: i, isBool: true})
		}
	}
	return out
}

// callbackArg lowers the argument passed for a function-typed extern
// parameter: the address of a stub for the named function.
func (l *lowerer) callbackArg(e Expr, want vty, widen []widenArg) Reg {
	id, ok := e.(*Ident)
	if ok {
		if _, local := l.lookup(id.Name); local {
			ok = false
		}
	}
	if !ok {
		l.errorAt(e, "a callback has to be a function declared with fn, named directly - "+
			"a closure or a variable holding a function cannot be called from native code")
		return l.junk()
	}
	if _, isFn := l.sigs[id.Name]; !isFn {
		l.errorAt(e, "%s is not a function declared with fn, so it cannot be a callback", id.Name)
		return l.junk()
	}

	shape := ""
	for _, w := range widen {
		kind := "i"
		if w.isBool {
			kind = "b"
		}
		shape += fmt.Sprintf("%s%d", kind, w.pos)
	}
	sym := "__vy_cb_" + sanitizeSym(id.Name) + "_" + shape
	known := false
	for _, t := range l.mod.Thunks {
		if t.Sym == sym {
			known = true
			break
		}
	}
	if !known {
		l.mod.Thunks = append(l.mod.Thunks, Thunk{Sym: sym, Target: fnSym(id.Name), Widen: widen})
	}

	d := l.newReg()
	l.regTy[d] = want
	l.emit(Instr{Op: OpSymAddr, Dst: d, A: NoReg, B: NoReg, Sym: sym, Comment: "callback " + id.Name})
	return d
}

// The registers an argument arrives in, by position, at each width.
var (
	argRegs32 = [4]string{"ecx", "edx", "r8d", "r9d"}
	argRegs8  = [4]string{"cl", "dl", "r8b", "r9b"}
)

// thunk writes one callback stub. At entry the return address is at
// [rsp] and argument i from the fifth on is at [rsp+8+8*i], which is
// where the caller put it and where the Veyl function will look.
func (e *Emitter) thunk(t Thunk) {
	e.b.WriteString("\n" + t.Sym + ":\n")
	for _, w := range t.Widen {
		if w.pos < 4 {
			r64, r32, r8 := argRegs[w.pos], argRegs32[w.pos], argRegs8[w.pos]
			if w.isBool {
				e.line("test %s, %s", r32, r32)
				e.line("setne %s", r8)
				e.line("movzx %s, %s", r32, r8)
			} else {
				e.line("movsxd %s, %s", r64, r32)
			}
			continue
		}
		at := fmt.Sprintf("qword ptr [rsp+%d]", 8+8*w.pos)
		at32 := fmt.Sprintf("dword ptr [rsp+%d]", 8+8*w.pos)
		if w.isBool {
			e.line("mov eax, %s", at32)
			e.line("test eax, eax")
			e.line("setne al")
			e.line("movzx eax, al")
		} else {
			e.line("movsxd rax, %s", at32)
		}
		e.line("mov %s, rax", at)
	}
	e.line("jmp %s", t.Target)
}
