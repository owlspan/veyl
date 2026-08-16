package main

// The intermediate representation, and the lowering from AST into it.
//
// Three-address code over an unlimited supply of virtual registers. Every
// instruction reads two operands and writes one, which is the shape most
// machine instructions already have, so instruction selection stays a
// local decision rather than a tree walk.
//
// Nothing in this file knows what an x86 register is. That separation is
// the whole point: the register allocator and the emitter are what change
// when a second architecture arrives, and lowering is not.
//
// Deliberately not SSA. SSA earns its keep when there are optimisation
// passes to run, and there are none yet. Adding it later means inserting
// a pass between here and the emitter, not rewriting either.

import (
	"fmt"
	"strconv"
	"strings"
)

// A vty is the type information this backend tracks.
//
// The real type checker lives in ../../src/compiler/check.go and is not
// shared yet, so this is not type checking - a program that is wrong
// will still compile to something. Run it through the Go backend first.
//
// What this does buy is knowing which of print, concat or compare to
// emit, which is not optional: `print` of a comparison has to write
// "true" and `a + b` on two strings has to concatenate rather than add.
type vty int

const (
	vVoid vty = iota
	vInt
	vBool
	vStr
)

func (t vty) String() string {
	switch t {
	case vInt:
		return "int"
	case vBool:
		return "bool"
	case vStr:
		return "str"
	}
	return "void"
}

// typeOfName maps a written type annotation onto what this backend
// tracks. Anything else is reported where it is used.
func typeOfName(s string) (vty, bool) {
	switch s {
	case "int":
		return vInt, true
	case "bool":
		return vBool, true
	case "str":
		return vStr, true
	case "":
		return vVoid, true
	}
	return vVoid, false
}

// A Reg is a virtual register. There are as many as the program needs;
// mapping them onto the sixteen real ones is the allocator's problem.
type Reg int

// NoReg marks an instruction that writes nothing, or an absent operand.
const NoReg Reg = -1

type Op int

const (
	OpConst Op = iota // Dst = Imm
	OpStr             // Dst = address of string constant Imm
	OpLoad            // Dst = slot[Imm]
	OpStore           // slot[Imm] = A
	OpAdd             // Dst = A + B
	OpSub             // Dst = A - B
	OpMul             // Dst = A * B
	OpDiv             // Dst = A / B   (truncating, as the language specifies)
	OpMod             // Dst = A % B
	OpNeg             // Dst = -A

	// Bitwise. All int-only, matching the language.
	OpBAnd // Dst = A & B
	OpBOr  // Dst = A | B
	OpBXor // Dst = A ^ B
	OpBNot // Dst = ~A
	OpShl  // Dst = A << B
	OpShr  // Dst = A >> B

	// Comparisons produce 0 or 1, so a bool is just an int and needs no
	// separate representation. Branching then only needs one conditional
	// jump form rather than one per comparison.
	OpEq
	OpNe
	OpLt
	OpLe
	OpGt
	OpGe

	OpNot // Dst = !A

	// Control flow. Labels are integer targets rather than pointers, so
	// the byte writer can backpatch them the same way later without a
	// different IR.
	OpLabel   // Imm names this point
	OpJump    // goto Imm
	OpJumpIf  // if A != 0 goto Imm
	OpJumpNot // if A == 0 goto Imm

	// Functions.
	OpParam // Dst = incoming parameter number Imm
	OpCall  // Dst = Sym(Args...)
	OpRet   // return A, or nothing when A is NoReg

	// Anything needing the C runtime goes through a named helper, which
	// keeps the emitter free of open-coded sequences that the byte
	// writer would have to reproduce exactly.
	OpPrintInt
	OpPrintBool
	OpPrintStr
	OpConcat // Dst = A ++ B
	OpStrEq  // Dst = A == B, as strings
	OpStrLen // Dst = len(A)
	OpIntToStr
	OpBoolToStr
)

var opNames = [...]string{
	"const", "str", "load", "store", "add", "sub", "mul", "div", "mod", "neg",
	"band", "bor", "bxor", "bnot", "shl", "shr",
	"eq", "ne", "lt", "le", "gt", "ge", "not",
	"label", "jump", "jumpif", "jumpnot",
	"param", "call", "ret",
	"printint", "printbool", "printstr",
	"concat", "streq", "strlen", "inttostr", "booltostr",
}

func (o Op) String() string {
	if int(o) < len(opNames) {
		return opNames[o]
	}
	return "op?"
}

type Instr struct {
	Op      Op
	Dst     Reg
	A, B    Reg
	Imm     int64
	Args    []Reg  // OpCall only
	Sym     string // OpCall only: the function being called
	Comment string // the Veyl this came from, carried into the .s file
}

// A Func is one lowered function.
type Func struct {
	Name    string
	NParams int
	Ret     vty
	Code    []Instr

	NRegs   int // virtual registers used
	NSlots  int // stack slots for locals and spilled temporaries
	NLabels int

	// MaxCallArgs is the widest call this function makes. The outgoing
	// argument area has to be big enough for it, and is reserved once in
	// the prologue rather than pushed per call, which is what keeps rsp
	// 16-byte aligned without any per-call arithmetic.
	MaxCallArgs int
}

// A Module is the whole program: its functions and its string pool.
type Module struct {
	Funcs   []*Func
	Strings []string
	Helpers map[string]bool // runtime helpers actually used
}

func (m *Module) needs(h string) { m.Helpers[h] = true }

// intern adds a string constant to the pool, reusing an existing entry
// when the same text appears twice.
func (m *Module) intern(s string) int64 {
	for i, existing := range m.Strings {
		if existing == s {
			return int64(i)
		}
	}
	m.Strings = append(m.Strings, s)
	return int64(len(m.Strings) - 1)
}

// ---- function signatures ----

// A sig is what the lowerer knows about a function before it lowers a
// call to it. Built from the written annotations rather than from the
// checker, which is not shared yet.
type sig struct {
	params []vty
	ret    vty
}

// ---- lowering ----

type lowerer struct {
	mod  *Module
	fn   *Func
	file string
	errs []string

	// Scopes, innermost last. A block introduces one, so an inner name
	// can shadow an outer one and stops existing at the closing brace.
	scopes []map[string]int64
	slotTy map[int64]vty
	regTy  map[Reg]vty

	loops []loopTarget
	sigs  map[string]sig
}

type loopTarget struct {
	brk  int64
	cont int64
}

// Lower turns the parsed program into a module. It reports its own
// errors rather than aborting, matching the rest of the pipeline: one
// mistake should not hide the next one.
func Lower(p *Program, file string) (*Module, []string) {
	l := &lowerer{
		mod:  &Module{Helpers: map[string]bool{}},
		file: file,
		sigs: map[string]sig{},
	}

	// Signatures first, so a function can call one declared below it.
	// The Go backend guarantees order-independent declaration and this
	// has to match.
	for _, fd := range p.Funcs {
		if fd.Recv != "" {
			l.errorAt(fd, "methods are not on the assembly backend yet")
			continue
		}
		s := sig{}
		ok := true
		for _, pa := range fd.Params {
			t, good := typeOfName(pa.Type)
			if !good || t == vVoid {
				l.errorAt(fd, "parameter %q has type %q, which the assembly backend does not handle yet",
					pa.Name, pa.Type)
				ok = false
				continue
			}
			s.params = append(s.params, t)
		}
		ret, good := typeOfName(fd.Ret)
		if !good {
			l.errorAt(fd, "return type %q is not on the assembly backend yet", fd.Ret)
			ok = false
		}
		s.ret = ret
		if ok {
			l.sigs[fd.Name] = s
		}
	}

	for _, fd := range p.Funcs {
		if fd.Recv != "" {
			continue
		}
		l.function(fd)
	}

	// main last, so it reads as the entry point at the bottom of the
	// listing the way it does in the source.
	l.fn = &Func{Name: "main", Ret: vVoid}
	l.pushScope()
	for _, st := range p.Main {
		l.stmt(st)
	}
	l.popScope()
	l.mod.Funcs = append(l.mod.Funcs, l.fn)

	return l.mod, l.errs
}

func (l *lowerer) function(fd *FnDecl) {
	s, known := l.sigs[fd.Name]
	if !known {
		return // its signature was already rejected
	}

	l.fn = &Func{Name: fd.Name, NParams: len(fd.Params), Ret: s.ret}
	l.slotTy = map[int64]vty{}
	l.regTy = map[Reg]vty{}
	l.pushScope()

	// Parameters arrive in registers and are immediately written to
	// slots, like every other value. The allocator will hoist the ones
	// worth keeping in registers; doing it here would be guessing.
	for i, pa := range fd.Params {
		slot := l.declare(pa.Name, s.params[i])
		d := l.newReg()
		l.regTy[d] = s.params[i]
		l.emit(Instr{Op: OpParam, Dst: d, A: NoReg, B: NoReg, Imm: int64(i),
			Comment: pa.Name})
		l.emit(Instr{Op: OpStore, A: d, Dst: NoReg, Imm: slot})
	}

	for _, st := range fd.Body.Stmts {
		l.stmt(st)
	}
	l.popScope()

	// A function that runs off the end returns zero. The Go backend's
	// return-path checking would have rejected that already for a
	// non-void function, and this backend does not have it yet, so the
	// safe thing is a defined value rather than whatever is in rax.
	z := l.newReg()
	l.emit(Instr{Op: OpConst, Dst: z, A: NoReg, B: NoReg, Imm: 0})
	if s.ret == vVoid {
		l.emit(Instr{Op: OpRet, A: NoReg, Dst: NoReg})
	} else {
		l.emit(Instr{Op: OpRet, A: z, Dst: NoReg})
	}

	l.mod.Funcs = append(l.mod.Funcs, l.fn)
}

// ---- scopes ----

func (l *lowerer) pushScope() {
	l.scopes = append(l.scopes, map[string]int64{})
	if l.slotTy == nil {
		l.slotTy = map[int64]vty{}
		l.regTy = map[Reg]vty{}
	}
}

func (l *lowerer) popScope() { l.scopes = l.scopes[:len(l.scopes)-1] }

// declare puts a name in the innermost scope with a fresh slot. Slots
// are never reused across scopes: the frame is a few bytes larger and
// nothing can alias.
func (l *lowerer) declare(name string, t vty) int64 {
	slot := int64(l.fn.NSlots)
	l.fn.NSlots++
	l.scopes[len(l.scopes)-1][name] = slot
	l.slotTy[slot] = t
	return slot
}

func (l *lowerer) lookup(name string) (int64, bool) {
	for i := len(l.scopes) - 1; i >= 0; i-- {
		if slot, ok := l.scopes[i][name]; ok {
			return slot, true
		}
	}
	return 0, false
}

// temp makes an anonymous slot, for values that two branches write.
func (l *lowerer) temp(t vty) int64 {
	slot := int64(l.fn.NSlots)
	l.fn.NSlots++
	l.slotTy[slot] = t
	return slot
}

func (l *lowerer) errorAt(n Node, format string, args ...any) {
	line, col := n.Pos()
	l.errs = append(l.errs, fmt.Sprintf("%s:%d:%d: %s", l.file, line, col,
		fmt.Sprintf(format, args...)))
}

func (l *lowerer) newReg() Reg {
	r := Reg(l.fn.NRegs)
	l.fn.NRegs++
	return r
}

func (l *lowerer) emit(in Instr) { l.fn.Code = append(l.fn.Code, in) }

func (l *lowerer) newLabel() int64 {
	n := int64(l.fn.NLabels)
	l.fn.NLabels++
	return n
}

func (l *lowerer) mark(label int64) {
	l.emit(Instr{Op: OpLabel, A: NoReg, Dst: NoReg, Imm: label})
}

// ---- statements ----

func (l *lowerer) stmt(s Stmt) {
	switch st := s.(type) {
	case *LetStmt:
		v := l.expr(st.Value)
		t := l.regTy[v]
		if st.Type != "" {
			declared, ok := typeOfName(st.Type)
			if !ok {
				l.errorAt(st, "type %q is not on the assembly backend yet", st.Type)
				return
			}
			if declared != t {
				l.errorAt(st, "%s was declared %s but the value is %s",
					st.Name, declared, t)
			}
			t = declared
		}
		slot := l.declare(st.Name, t)
		l.emit(Instr{Op: OpStore, A: v, Dst: NoReg, Imm: slot,
			Comment: "let " + st.Name})

	case *AssignStmt:
		l.assign(st)

	case *ExprStmt:
		call, ok := st.X.(*Call)
		if !ok {
			l.errorAt(st, "this statement does nothing")
			return
		}
		l.call(call)

	case *Block:
		l.pushScope()
		for _, inner := range st.Stmts {
			l.stmt(inner)
		}
		l.popScope()

	case *IfStmt:
		cond := l.expr(st.Cond)
		if st.Else == nil {
			done := l.newLabel()
			l.emit(Instr{Op: OpJumpNot, A: cond, Dst: NoReg, Imm: done, Comment: "if"})
			l.stmt(st.Then)
			l.mark(done)
			return
		}
		otherwise := l.newLabel()
		done := l.newLabel()
		l.emit(Instr{Op: OpJumpNot, A: cond, Dst: NoReg, Imm: otherwise, Comment: "if"})
		l.stmt(st.Then)
		l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: done})
		l.mark(otherwise)
		l.stmt(st.Else)
		l.mark(done)

	case *WhileStmt:
		top := l.newLabel()
		done := l.newLabel()
		l.mark(top)
		cond := l.expr(st.Cond)
		l.emit(Instr{Op: OpJumpNot, A: cond, Dst: NoReg, Imm: done, Comment: "while"})
		l.loops = append(l.loops, loopTarget{brk: done, cont: top})
		l.stmt(st.Body)
		l.loops = l.loops[:len(l.loops)-1]
		l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: top})
		l.mark(done)

	case *ForStmt:
		l.forRange(st)

	case *ReturnStmt:
		if st.Value == nil {
			l.emit(Instr{Op: OpRet, A: NoReg, Dst: NoReg, Comment: "return"})
			return
		}
		v := l.expr(st.Value)
		l.emit(Instr{Op: OpRet, A: v, Dst: NoReg, Comment: "return"})

	case *BreakStmt:
		if len(l.loops) == 0 {
			l.errorAt(st, "break outside a loop")
			return
		}
		l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg,
			Imm: l.loops[len(l.loops)-1].brk, Comment: "break"})

	case *ContinueStmt:
		if len(l.loops) == 0 {
			l.errorAt(st, "continue outside a loop")
			return
		}
		l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg,
			Imm: l.loops[len(l.loops)-1].cont, Comment: "continue"})

	default:
		l.errorAt(s.(Node), "the assembly backend does not handle this statement yet")
	}
}

func (l *lowerer) assign(st *AssignStmt) {
	id, ok := st.Target.(*Ident)
	if !ok {
		l.errorAt(st, "the assembly backend can only assign to a plain name so far")
		return
	}
	slot, known := l.lookup(id.Name)
	if !known {
		l.errorAt(st, "undefined variable %q", id.Name)
		return
	}
	v := l.expr(st.Value)

	// The compound forms read the target first. Ignoring st.Op here
	// silently turned `i += 1` into `i = 1`, which made every loop that
	// counted upwards run forever - a miscompile that produces no output
	// rather than wrong output, so nothing catches it except a deadline.
	if st.Op != ASSIGN {
		var op Op
		switch st.Op {
		case PLUSEQ:
			op = OpAdd
			if l.slotTy[slot] == vStr {
				op = OpConcat
				l.mod.needs("concat")
			}
		case MINUSEQ:
			op = OpSub
		case STAREQ:
			op = OpMul
		case SLASHEQ:
			op = OpDiv
		case PERCENTEQ:
			op = OpMod
		case AMPEQ:
			op = OpBAnd
		case PIPEEQ:
			op = OpBOr
		case CARETEQ:
			op = OpBXor
		case SHLEQ:
			op = OpShl
		case SHREQ:
			op = OpShr
		default:
			l.errorAt(st, "the assembly backend does not handle %s yet", st.Op)
			return
		}
		cur := l.newReg()
		l.regTy[cur] = l.slotTy[slot]
		l.emit(Instr{Op: OpLoad, Dst: cur, A: NoReg, B: NoReg, Imm: slot,
			Comment: id.Name})
		d := l.newReg()
		l.regTy[d] = l.slotTy[slot]
		l.emit(Instr{Op: op, Dst: d, A: cur, B: v})
		v = d
	}

	l.emit(Instr{Op: OpStore, A: v, Dst: NoReg, Imm: slot,
		Comment: id.Name + " " + st.Op.String()})
}

// forRange lowers the counted form into the same shape as a while loop.
//
// The bound and the step are each evaluated once into a slot, so
// `for i in 1..f()` calls f once rather than every iteration. Whether
// the loop counts up or down decides the comparison, and that has to be
// known here, so the step must be a literal for now: a dynamic one needs
// a runtime sign test that nothing yet writes programs to exercise.
func (l *lowerer) forRange(st *ForStmt) {
	if st.Coll != nil {
		l.errorAt(st, "iterating a collection is not on the assembly backend yet")
		return
	}
	if st.Var2 != "" {
		l.errorAt(st, "the two-variable for form is not on the assembly backend yet")
		return
	}

	step := int64(1)
	if st.Step != nil {
		n, ok := constInt(st.Step)
		if !ok {
			l.errorAt(st, "step must be an integer literal on the assembly backend so far")
			return
		}
		if n == 0 {
			l.errorAt(st, "step cannot be zero")
			return
		}
		step = n
	}

	l.pushScope()

	start := l.expr(st.Start)
	iSlot := l.declare(st.Var, vInt)
	l.emit(Instr{Op: OpStore, A: start, Dst: NoReg, Imm: iSlot,
		Comment: "for " + st.Var})

	end := l.expr(st.End)
	endSlot := l.temp(vInt)
	l.emit(Instr{Op: OpStore, A: end, Dst: NoReg, Imm: endSlot, Comment: "limit"})

	top := l.newLabel()
	cont := l.newLabel()
	done := l.newLabel()
	l.mark(top)

	// Counting up stops at <, counting down at >. Inclusive ranges use
	// the or-equal form.
	var cmp Op
	switch {
	case step > 0 && st.Inclusive:
		cmp = OpLe
	case step > 0:
		cmp = OpLt
	case st.Inclusive:
		cmp = OpGe
	default:
		cmp = OpGt
	}

	iv := l.newReg()
	l.regTy[iv] = vInt
	l.emit(Instr{Op: OpLoad, Dst: iv, A: NoReg, B: NoReg, Imm: iSlot})
	ev := l.newReg()
	l.regTy[ev] = vInt
	l.emit(Instr{Op: OpLoad, Dst: ev, A: NoReg, B: NoReg, Imm: endSlot})
	test := l.newReg()
	l.regTy[test] = vBool
	l.emit(Instr{Op: cmp, Dst: test, A: iv, B: ev})
	l.emit(Instr{Op: OpJumpNot, A: test, Dst: NoReg, Imm: done})

	// continue jumps to the increment, not the test, or the counter
	// never advances and the loop never ends.
	l.loops = append(l.loops, loopTarget{brk: done, cont: cont})
	l.stmt(st.Body)
	l.loops = l.loops[:len(l.loops)-1]

	l.mark(cont)
	cur := l.newReg()
	l.regTy[cur] = vInt
	l.emit(Instr{Op: OpLoad, Dst: cur, A: NoReg, B: NoReg, Imm: iSlot})
	inc := l.newReg()
	l.regTy[inc] = vInt
	l.emit(Instr{Op: OpConst, Dst: inc, A: NoReg, B: NoReg, Imm: step})
	next := l.newReg()
	l.regTy[next] = vInt
	l.emit(Instr{Op: OpAdd, Dst: next, A: cur, B: inc})
	l.emit(Instr{Op: OpStore, A: next, Dst: NoReg, Imm: iSlot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: top})

	l.mark(done)
	l.popScope()
}

// constInt folds the integer expressions the lowerer has to know at
// compile time, which so far is only a loop's step. `step -2` is a unary
// minus around a literal rather than a negative literal, so a bare type
// assertion on *IntLit rejects the one form people actually write.
func constInt(e Expr) (int64, bool) {
	switch x := e.(type) {
	case *IntLit:
		n, err := strconv.ParseInt(strings.ReplaceAll(x.Val, "_", ""), 0, 64)
		return n, err == nil
	case *Unary:
		if x.Op != MINUS {
			return 0, false
		}
		n, ok := constInt(x.X)
		return -n, ok
	}
	return 0, false
}

// ---- calls ----

func (l *lowerer) call(c *Call) Reg {
	name, ok := c.Callee.(*Ident)
	if !ok {
		l.errorAt(c, "the assembly backend can only call a plain name so far")
		return l.junk()
	}

	if s, isUser := l.sigs[name.Name]; isUser {
		if len(c.Args) != len(s.params) {
			l.errorAt(c, "%s takes %d argument(s), got %d",
				name.Name, len(s.params), len(c.Args))
			return l.junk()
		}
		args := make([]Reg, len(c.Args))
		for i, a := range c.Args {
			args[i] = l.expr(a)
		}
		if len(args) > l.fn.MaxCallArgs {
			l.fn.MaxCallArgs = len(args)
		}
		d := l.newReg()
		l.regTy[d] = s.ret
		l.emit(Instr{Op: OpCall, Dst: d, A: NoReg, B: NoReg, Args: args,
			Sym: name.Name, Comment: name.Name + "()"})
		return d
	}

	return l.builtin(c, name.Name)
}

// builtin covers the handful of library functions this backend has. The
// Go backend has 302; the gap is almost entirely things that need a
// heap and a runtime rather than things that need syntax.
func (l *lowerer) builtin(c *Call, name string) Reg {
	arity := func(n int) bool {
		if len(c.Args) != n {
			l.errorAt(c, "%s takes %d argument(s), got %d", name, n, len(c.Args))
			return false
		}
		return true
	}

	switch name {
	case "print":
		if !arity(1) {
			return l.junk()
		}
		a := l.expr(c.Args[0])
		switch l.regTy[a] {
		case vBool:
			l.emit(Instr{Op: OpPrintBool, A: a, Dst: NoReg, Comment: "print"})
		case vStr:
			l.emit(Instr{Op: OpPrintStr, A: a, Dst: NoReg, Comment: "print"})
		default:
			l.emit(Instr{Op: OpPrintInt, A: a, Dst: NoReg, Comment: "print"})
		}
		return l.void()

	case "len":
		if !arity(1) {
			return l.junk()
		}
		a := l.expr(c.Args[0])
		if l.regTy[a] != vStr {
			l.errorAt(c, "len needs a str here; lists are not on this backend yet")
			return l.junk()
		}
		l.mod.needs("strlen")
		d := l.newReg()
		l.regTy[d] = vInt
		l.emit(Instr{Op: OpStrLen, Dst: d, A: a, B: NoReg})
		return d

	case "str":
		if !arity(1) {
			return l.junk()
		}
		return l.toStr(l.expr(c.Args[0]), c)

	case "abs", "min", "max":
		return l.intMath(c, name)
	}

	l.errorAt(c, "%q is not on the assembly backend yet", name)
	return l.junk()
}

// intMath covers abs, min and max, which are worth having because they
// need no runtime at all - just a compare and a conditional move.
func (l *lowerer) intMath(c *Call, name string) Reg {
	if name == "abs" {
		if len(c.Args) != 1 {
			l.errorAt(c, "abs takes 1 argument, got %d", len(c.Args))
			return l.junk()
		}
		a := l.expr(c.Args[0])
		neg := l.newReg()
		l.regTy[neg] = vInt
		l.emit(Instr{Op: OpNeg, Dst: neg, A: a, B: NoReg})
		zero := l.newReg()
		l.regTy[zero] = vInt
		l.emit(Instr{Op: OpConst, Dst: zero, A: NoReg, B: NoReg, Imm: 0})
		isNeg := l.newReg()
		l.regTy[isNeg] = vBool
		l.emit(Instr{Op: OpLt, Dst: isNeg, A: a, B: zero})
		return l.pick(isNeg, neg, a, vInt)
	}

	if len(c.Args) != 2 {
		l.errorAt(c, "%s takes 2 arguments, got %d", name, len(c.Args))
		return l.junk()
	}
	a := l.expr(c.Args[0])
	b := l.expr(c.Args[1])
	cond := l.newReg()
	l.regTy[cond] = vBool
	if name == "min" {
		l.emit(Instr{Op: OpLt, Dst: cond, A: a, B: b})
	} else {
		l.emit(Instr{Op: OpGt, Dst: cond, A: a, B: b})
	}
	return l.pick(cond, a, b, vInt)
}

// pick is a branching select: the value of `cond ? whenTrue : whenFalse`.
// It goes through a slot because two branches write it, which is exactly
// the situation the naive allocator cannot express in a register.
func (l *lowerer) pick(cond, whenTrue, whenFalse Reg, t vty) Reg {
	slot := l.temp(t)
	other := l.newLabel()
	done := l.newLabel()
	l.emit(Instr{Op: OpJumpNot, A: cond, Dst: NoReg, Imm: other})
	l.emit(Instr{Op: OpStore, A: whenTrue, Dst: NoReg, Imm: slot})
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: done})
	l.mark(other)
	l.emit(Instr{Op: OpStore, A: whenFalse, Dst: NoReg, Imm: slot})
	l.mark(done)
	d := l.newReg()
	l.regTy[d] = t
	l.emit(Instr{Op: OpLoad, Dst: d, A: NoReg, B: NoReg, Imm: slot})
	return d
}

// toStr converts any tracked type to a string.
func (l *lowerer) toStr(v Reg, at Node) Reg {
	switch l.regTy[v] {
	case vStr:
		return v
	case vBool:
		l.mod.needs("booltostr")
		d := l.newReg()
		l.regTy[d] = vStr
		l.emit(Instr{Op: OpBoolToStr, Dst: d, A: v, B: NoReg})
		return d
	case vInt:
		l.mod.needs("inttostr")
		d := l.newReg()
		l.regTy[d] = vStr
		l.emit(Instr{Op: OpIntToStr, Dst: d, A: v, B: NoReg})
		return d
	}
	l.errorAt(at, "nothing to convert to a string here")
	return l.junk()
}

// junk is a defined-but-meaningless register, returned after an error so
// that lowering can continue and report the next mistake too.
func (l *lowerer) junk() Reg {
	d := l.newReg()
	l.regTy[d] = vInt
	l.emit(Instr{Op: OpConst, Dst: d, A: NoReg, B: NoReg, Imm: 0})
	return d
}

func (l *lowerer) void() Reg {
	d := l.newReg()
	l.regTy[d] = vVoid
	return d
}

// ---- expressions ----

func (l *lowerer) expr(e Expr) Reg {
	switch x := e.(type) {
	case *IntLit:
		n, err := strconv.ParseInt(strings.ReplaceAll(x.Val, "_", ""), 0, 64)
		if err != nil {
			l.errorAt(x, "integer literal out of range: %s", x.Val)
			return l.junk()
		}
		d := l.newReg()
		l.regTy[d] = vInt
		l.emit(Instr{Op: OpConst, Dst: d, A: NoReg, B: NoReg, Imm: n})
		return d

	case *BoolLit:
		n := int64(0)
		if x.Val {
			n = 1
		}
		d := l.newReg()
		l.regTy[d] = vBool
		l.emit(Instr{Op: OpConst, Dst: d, A: NoReg, B: NoReg, Imm: n})
		return d

	case *StrLit:
		d := l.newReg()
		l.regTy[d] = vStr
		l.emit(Instr{Op: OpStr, Dst: d, A: NoReg, B: NoReg,
			Imm: l.mod.intern(x.Val)})
		return d

	case *Interp:
		return l.interp(x)

	case *Ident:
		slot, ok := l.lookup(x.Name)
		if !ok {
			l.errorAt(x, "undefined variable %q", x.Name)
			return l.junk()
		}
		d := l.newReg()
		l.regTy[d] = l.slotTy[slot]
		l.emit(Instr{Op: OpLoad, Dst: d, A: NoReg, B: NoReg, Imm: slot,
			Comment: x.Name})
		return d

	case *Call:
		return l.call(x)

	case *Unary:
		a := l.expr(x.X)
		d := l.newReg()
		switch x.Op {
		case MINUS:
			l.regTy[d] = vInt
			l.emit(Instr{Op: OpNeg, Dst: d, A: a, B: NoReg})
		case BANG:
			l.regTy[d] = vBool
			l.emit(Instr{Op: OpNot, Dst: d, A: a, B: NoReg})
		case TILDE:
			l.regTy[d] = vInt
			l.emit(Instr{Op: OpBNot, Dst: d, A: a, B: NoReg})
		default:
			l.errorAt(x, "the assembly backend does not handle unary %s yet", x.Op)
		}
		return d

	case *Binary:
		return l.binary(x)

	default:
		l.errorAt(e.(Node), "the assembly backend does not handle this expression yet")
		return l.junk()
	}
}

func (l *lowerer) binary(x *Binary) Reg {
	// && and || short-circuit, so they cannot be a plain two-operand
	// instruction: the right side must not run once the left has
	// decided the answer.
	if x.Op == AND || x.Op == OR {
		return l.shortCircuit(x)
	}

	a := l.expr(x.L)
	b := l.expr(x.R)
	at, bt := l.regTy[a], l.regTy[b]

	// Strings overload + and the equality tests, and nothing else.
	if at == vStr || bt == vStr {
		switch x.Op {
		case PLUS:
			if at != vStr || bt != vStr {
				l.errorAt(x, "cannot add %s and %s", at, bt)
				return l.junk()
			}
			l.mod.needs("concat")
			d := l.newReg()
			l.regTy[d] = vStr
			l.emit(Instr{Op: OpConcat, Dst: d, A: a, B: b})
			return d
		case EQ, NEQ:
			l.mod.needs("streq")
			d := l.newReg()
			l.regTy[d] = vBool
			l.emit(Instr{Op: OpStrEq, Dst: d, A: a, B: b})
			if x.Op == NEQ {
				n := l.newReg()
				l.regTy[n] = vBool
				l.emit(Instr{Op: OpNot, Dst: n, A: d, B: NoReg})
				return n
			}
			return d
		default:
			l.errorAt(x, "%s is not defined on strings", x.Op)
			return l.junk()
		}
	}

	var op Op
	switch x.Op {
	case PLUS:
		op = OpAdd
	case MINUS:
		op = OpSub
	case STAR:
		op = OpMul
	case SLASH:
		op = OpDiv
	case PERCENT:
		op = OpMod
	case AMP:
		op = OpBAnd
	case PIPE:
		op = OpBOr
	case CARET:
		op = OpBXor
	case SHL:
		op = OpShl
	case SHR:
		op = OpShr
	case EQ:
		op = OpEq
	case NEQ:
		op = OpNe
	case LT:
		op = OpLt
	case LTE:
		op = OpLe
	case GT:
		op = OpGt
	case GTE:
		op = OpGe
	default:
		l.errorAt(x, "the assembly backend does not handle %s yet", x.Op)
		return l.junk()
	}

	d := l.newReg()
	switch op {
	case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
		l.regTy[d] = vBool
	default:
		l.regTy[d] = vInt
	}
	l.emit(Instr{Op: op, Dst: d, A: a, B: b})
	return d
}

// shortCircuit lowers && and ||.
//
// The result is built in a stack slot rather than a virtual register,
// because two different branches write it and the naive allocator gives
// every virtual register exactly one definition site. When a real
// allocator arrives this becomes a phi, which is the point at which SSA
// starts paying for itself.
func (l *lowerer) shortCircuit(x *Binary) Reg {
	slot := l.temp(vBool)
	done := l.newLabel()

	left := l.expr(x.L)
	l.emit(Instr{Op: OpStore, A: left, Dst: NoReg, Imm: slot})

	if x.Op == AND {
		l.emit(Instr{Op: OpJumpNot, A: left, Dst: NoReg, Imm: done, Comment: "&&"})
	} else {
		l.emit(Instr{Op: OpJumpIf, A: left, Dst: NoReg, Imm: done, Comment: "||"})
	}

	right := l.expr(x.R)
	l.emit(Instr{Op: OpStore, A: right, Dst: NoReg, Imm: slot})
	l.mark(done)

	d := l.newReg()
	l.regTy[d] = vBool
	l.emit(Instr{Op: OpLoad, Dst: d, A: NoReg, B: NoReg, Imm: slot})
	return d
}

// interp lowers "a{x}b" by converting each hole to a string and
// concatenating left to right.
//
// That allocates once per join, so a long interpolation allocates
// several times and frees none of it. Correct, wasteful, and honest:
// this backend has no collector, which is the single largest thing
// standing between it and the Go backend.
func (l *lowerer) interp(x *Interp) Reg {
	var acc Reg
	first := true

	add := func(r Reg) {
		if first {
			acc = r
			first = false
			return
		}
		l.mod.needs("concat")
		d := l.newReg()
		l.regTy[d] = vStr
		l.emit(Instr{Op: OpConcat, Dst: d, A: acc, B: r})
		acc = d
	}

	for _, part := range x.Parts {
		if part.Lit != "" {
			d := l.newReg()
			l.regTy[d] = vStr
			l.emit(Instr{Op: OpStr, Dst: d, A: NoReg, B: NoReg,
				Imm: l.mod.intern(part.Lit)})
			add(d)
		}
		if part.X != nil {
			add(l.toStr(l.expr(part.X), x))
		}
	}

	if first {
		d := l.newReg()
		l.regTy[d] = vStr
		l.emit(Instr{Op: OpStr, Dst: d, A: NoReg, B: NoReg, Imm: l.mod.intern("")})
		return d
	}
	return acc
}

// ---- printing, for `veylasm ir` ----

func (m *Module) String() string {
	var b strings.Builder
	for i, s := range m.Strings {
		fmt.Fprintf(&b, "str%d = %q\n", i, s)
	}
	if len(m.Strings) > 0 {
		b.WriteString("\n")
	}
	for _, f := range m.Funcs {
		b.WriteString(f.String())
		b.WriteString("\n")
	}
	return b.String()
}

func (f *Func) String() string {
	out := fmt.Sprintf("func %s/%d -> %s:\n", f.Name, f.NParams, f.Ret)
	for _, in := range f.Code {
		line := "    "
		if in.Dst != NoReg && in.Op != OpStore {
			line += fmt.Sprintf("v%d = ", in.Dst)
		}
		line += in.Op.String()
		switch in.Op {
		case OpConst:
			line += fmt.Sprintf(" %d", in.Imm)
		case OpStr:
			line += fmt.Sprintf(" str%d", in.Imm)
		case OpLoad:
			line += fmt.Sprintf(" slot%d", in.Imm)
		case OpStore:
			line += fmt.Sprintf(" slot%d, v%d", in.Imm, in.A)
		case OpParam:
			line += fmt.Sprintf(" %d", in.Imm)
		case OpNeg, OpNot, OpBNot, OpPrintInt, OpPrintBool, OpPrintStr,
			OpStrLen, OpIntToStr, OpBoolToStr:
			line += fmt.Sprintf(" v%d", in.A)
		case OpLabel:
			line = fmt.Sprintf("  L%d:", in.Imm)
		case OpJump:
			line += fmt.Sprintf(" L%d", in.Imm)
		case OpJumpIf, OpJumpNot:
			line += fmt.Sprintf(" v%d, L%d", in.A, in.Imm)
		case OpRet:
			if in.A != NoReg {
				line += fmt.Sprintf(" v%d", in.A)
			}
		case OpCall:
			parts := make([]string, len(in.Args))
			for i, a := range in.Args {
				parts[i] = fmt.Sprintf("v%d", a)
			}
			line += fmt.Sprintf(" %s(%s)", in.Sym, strings.Join(parts, ", "))
		default:
			line += fmt.Sprintf(" v%d, v%d", in.A, in.B)
		}
		if in.Comment != "" {
			line += "    ; " + in.Comment
		}
		out += line + "\n"
	}
	return out
}
