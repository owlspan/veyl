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
	"math"
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
type vkind int

const (
	kVoid vkind = iota
	kInt
	kFloat
	kBool
	kStr
	kList
	kMap
)

// A vty is a kind plus, for a list, the kind of its elements, and for a
// map, the kinds of its keys and values. One level only: [][]int needs
// elem to be a vty rather than a vkind, and nothing is served by paying
// for that before lists themselves work.
type vty struct {
	k    vkind
	elem vkind // list: the element kind. map: the value kind.
	key  vkind // map only: the key kind.
}

var (
	vVoid  = vty{k: kVoid}
	vInt   = vty{k: kInt}
	vFloat = vty{k: kFloat}
	vBool  = vty{k: kBool}
	vStr   = vty{k: kStr}
)

func vListOf(e vkind) vty     { return vty{k: kList, elem: e} }
func vMapOf(kk, vk vkind) vty { return vty{k: kMap, elem: vk, key: kk} }

// elemType is the type of what comes out of indexing this list or map.
func (t vty) elemType() vty { return vty{k: t.elem} }

// keyType is the type of a map's keys.
func (t vty) keyType() vty { return vty{k: t.key} }

func (t vty) String() string {
	switch t.k {
	case kInt:
		return "int"
	case kFloat:
		return "float"
	case kBool:
		return "bool"
	case kStr:
		return "str"
	case kList:
		return "[]" + vty{k: t.elem}.String()
	case kMap:
		return "{" + vty{k: t.key}.String() + ": " + vty{k: t.elem}.String() + "}"
	}
	return "void"
}

// typeOfName maps a written type annotation onto what this backend
// tracks. Anything else is reported where it is used.
func typeOfName(s string) (vty, bool) {
	switch s {
	case "int":
		return vInt, true
	case "float":
		return vFloat, true
	case "bool":
		return vBool, true
	case "str":
		return vStr, true
	case "":
		return vVoid, true
	}
	if strings.HasPrefix(s, "[]") {
		e, ok := typeOfName(strings.TrimSpace(s[2:]))
		if !ok || e.k == kVoid || e.k == kList {
			return vVoid, false
		}
		return vListOf(e.k), true
	}
	// {K: V}. The colon is found rather than split on, because neither a
	// key nor a value type contains one at this depth - nested maps are
	// rejected below along with every other compound.
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		body := s[1 : len(s)-1]
		i := strings.Index(body, ":")
		if i < 0 {
			return vVoid, false
		}
		kt, kok := typeOfName(strings.TrimSpace(body[:i]))
		vt, vok := typeOfName(strings.TrimSpace(body[i+1:]))
		if !kok || !vok {
			return vVoid, false
		}
		if kt.k != kInt && kt.k != kStr {
			return vVoid, false
		}
		if vt.k == kVoid || vt.k == kList || vt.k == kMap {
			return vVoid, false
		}
		return vMapOf(kt.k, vt.k), true
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

	// Floats. A separate set rather than overloading the integer ops,
	// because they run on a different register file (xmm0-15) and the
	// emitter has to know which one at the point it picks an
	// instruction - regTy is a lowering-time concept, gone by then.
	OpFConst // Dst = the float constant pool entry Imm
	OpFAdd
	OpFSub
	OpFMul
	OpFDiv
	OpFNeg
	OpFEq
	OpFNe
	OpFLt
	OpFLe
	OpFGt
	OpFGe
	OpIntToFloat // Dst = float(A), exact
	OpFloatToInt // Dst = int(A), truncated toward zero
	OpSqrt       // Dst = sqrt(A)
	OpFMod       // Dst = fmod(A, B), the C library's floating remainder

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
	OpPrintFloat
	OpPrintBool
	OpPrintStr
	OpConcat // Dst = A ++ B
	OpStrEq  // Dst = A == B, as strings
	OpStrLen // Dst = len(A)
	OpIntToStr
	OpFloatToStr
	OpBoolToStr

	// Raw memory. These exist so that lists can be built in the IR
	// rather than as hand-written assembly: a bounds check becomes an
	// ordinary compare and branch, and the byte writer inherits all of
	// it without a second implementation. They are also the ops that
	// `unsafe` and manual memory will eventually be written in terms of.
	OpAlloc     // Dst = allocate A bytes
	OpIndexAddr // Dst = A + B*8
	OpLoadMem   // Dst = qword at [A + Imm]
	OpStoreMem  // qword at [A + Imm] = B

	// Unbuffered output, for building up a line without a newline after
	// each piece. print() is these plus a newline.
	OpWriteStr
	OpWriteInt
	OpWriteFloat
	OpBoundsFail // A is the index, B the length; does not return

	// Byte-level memory, for strings. A Veyl string is a pointer to
	// NUL-terminated bytes, so anything that inspects or builds one
	// needs to read and write single bytes rather than words.
	OpLoadByte  // Dst = zero-extended byte at [A + B]
	OpStoreByte // byte at [A + B] = low byte of the value in Imm's slot

	// The first of the namespaced library functions. It is its own op
	// rather than a general foreign call because the general one has to
	// settle argument classification and shadow space first, and time()
	// takes a null pointer and returns an integer - the one shape that
	// needs none of that.
	OpTimeNow // Dst = time(NULL)

	// Three-way string comparison, negative, zero or positive like the
	// strcmp it calls. Needed to keep a map sorted by string key; the
	// existing streq only answers equality.
	OpStrCmp // Dst = strcmp(A, B)

	// getenv. Returns the raw pointer, which is NULL when the variable
	// is unset - the null check is done in the IR by whichever builtin
	// asked, because get and has want different answers to it.
	OpGetEnv // Dst = getenv(A)
)

var opNames = [...]string{
	"const", "str", "load", "store", "add", "sub", "mul", "div", "mod", "neg",
	"fconst", "fadd", "fsub", "fmul", "fdiv", "fneg",
	"feq", "fne", "flt", "fle", "fgt", "fge",
	"inttofloat", "floattoint", "sqrt", "fmod",
	"band", "bor", "bxor", "bnot", "shl", "shr",
	"eq", "ne", "lt", "le", "gt", "ge", "not",
	"label", "jump", "jumpif", "jumpnot",
	"param", "call", "ret",
	"printint", "printfloat", "printbool", "printstr",
	"concat", "streq", "strlen", "inttostr", "floattostr", "booltostr",
	"alloc", "indexaddr", "loadmem", "storemem",
	"writestr", "writeint", "writefloat", "boundsfail",
	"loadbyte", "storebyte",
	"timenow", "strcmp", "getenv",
}

func (o Op) String() string {
	if int(o) < len(opNames) {
		return opNames[o]
	}
	return "op?"
}

type Instr struct {
	Op       Op
	Dst      Reg
	A, B     Reg
	Imm      int64
	Args     []Reg // OpCall only
	ArgTypes []vty // OpCall only, parallel to Args: the emitter needs to
	// know which of the outgoing slots is a float, since
	// the Windows x64 convention routes a float argument
	// to xmm0-3 while an int in the same position goes
	// to rcx/rdx/r8/r9 - the choice depends on the type
	// in that position, not on how many of each kind
	// came before it.
	RetType vty    // OpCall only: whether the result comes back in rax or xmm0
	Sym     string // OpCall only: the function being called
	Comment string // the Veyl this came from, carried into the .s file
}

// A Func is one lowered function.
type Func struct {
	Name       string
	NParams    int
	ParamTypes []vty // parallel to the incoming OpParam instructions;
	// the emitter needs this for the same reason OpCall
	// carries ArgTypes - which physical register an
	// incoming argument arrives in depends on its type.
	Ret  vty
	Code []Instr

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
	Floats  []float64
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

// internFloat is intern's counterpart for float literals, so the same
// bit pattern is not written into .rdata twice.
func (m *Module) internFloat(v float64) int64 {
	for i, existing := range m.Floats {
		if existing == v {
			return int64(i)
		}
	}
	m.Floats = append(m.Floats, v)
	return int64(len(m.Floats) - 1)
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

	// hint is the type the surrounding context expects. An empty list
	// literal has no element to infer from, so `let xs: []int = []` can
	// only work if the annotation reaches the literal.
	hint vty
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
			if !good || t.k == kVoid {
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
	// Globals are lowered as the opening statements of main. That is
	// enough while nothing outside main can see them; a function
	// referring to one will need them in static storage instead.
	for _, g := range p.Globals {
		l.stmt(g)
	}
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

	l.fn = &Func{Name: fd.Name, NParams: len(fd.Params), ParamTypes: s.params, Ret: s.ret}
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
	// safe thing is a defined value rather than whatever happens to be
	// in rax or xmm0. The zero has to be the right kind of zero: a float
	// return reads xmm0, so an int OpConst left there would be garbage.
	switch s.ret.k {
	case kVoid:
		l.emit(Instr{Op: OpRet, A: NoReg, Dst: NoReg})
	case kFloat:
		z := l.newReg()
		l.regTy[z] = vFloat
		l.emit(Instr{Op: OpFConst, Dst: z, A: NoReg, B: NoReg, Imm: l.mod.internFloat(0)})
		l.emit(Instr{Op: OpRet, A: z, Dst: NoReg})
	default:
		z := l.newReg()
		l.regTy[z] = vInt
		l.emit(Instr{Op: OpConst, Dst: z, A: NoReg, B: NoReg, Imm: 0})
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
		saved := l.hint
		if st.Type != "" {
			if want, ok := typeOfName(st.Type); ok {
				l.hint = want
			}
		}
		v := l.expr(st.Value)
		l.hint = saved

		t := l.regTy[v]
		if st.Type != "" {
			declared, ok := typeOfName(st.Type)
			if !ok {
				l.errorAt(st, "type %q is not on the assembly backend yet", st.Type)
				return
			}
			// The checker already accepted an untyped int literal going
			// into a float slot - Go's own untyped-constant rule, which
			// Veyl copies - so that combination is promoted rather than
			// rejected here.
			if declared != t {
				if declared.k == kFloat && t.k == kInt {
					v = l.toFloat(v)
				} else {
					l.errorAt(st, "%s was declared %s but the value is %s",
						st.Name, declared, t)
				}
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

	case *MatchStmt:
		l.match(st)

	case *ReturnStmt:
		if st.Value == nil {
			l.emit(Instr{Op: OpRet, A: NoReg, Dst: NoReg, Comment: "return"})
			return
		}
		v := l.expr(st.Value)
		if l.fn.Ret.k == kFloat && l.regTy[v].k == kInt {
			v = l.toFloat(v)
		}
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
	if idx, isIndex := st.Target.(*Index); isIndex {
		if st.Op != ASSIGN {
			l.errorAt(st, "compound assignment into a list is not on this backend yet")
			return
		}
		coll := l.expr(idx.X)
		t := l.regTy[coll]
		switch t.k {
		case kList:
			l.listSet(coll, l.expr(idx.Idx), l.expr(st.Value))
			return
		case kMap:
			key := l.expr(idx.Idx)
			if l.regTy[key].k != t.key {
				l.errorAt(st, "this map is keyed by %s, but the index is %s", t.keyType(), l.regTy[key])
				return
			}
			val := l.expr(st.Value)
			if l.regTy[val].k != t.elem {
				l.errorAt(st, "this map holds %s, but the value is %s", t.elemType(), l.regTy[val])
				return
			}
			l.mapSet(coll, key, val, t)
			return
		}
		l.errorAt(st, "only a list or a map can be indexed on this backend so far")
		return
	}

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
	target := l.slotTy[slot]

	// Plain assignment into a float variable accepts the same untyped
	// int literal promotion `let` does; the checker already allowed it.
	if st.Op == ASSIGN && target.k == kFloat && l.regTy[v].k == kInt {
		v = l.toFloat(v)
	}

	// The compound forms read the target first. Ignoring st.Op here
	// silently turned `i += 1` into `i = 1`, which made every loop that
	// counted upwards run forever - a miscompile that produces no output
	// rather than wrong output, so nothing catches it except a deadline.
	if st.Op != ASSIGN {
		isFloat := target.k == kFloat
		if isFloat && l.regTy[v].k == kInt {
			v = l.toFloat(v)
		}

		var op Op
		switch st.Op {
		case PLUSEQ:
			switch {
			case target.k == kStr:
				op = OpConcat
				l.mod.needs("concat")
			case isFloat:
				op = OpFAdd
			default:
				op = OpAdd
			}
		case MINUSEQ:
			op = pickOp(isFloat, OpFSub, OpSub)
		case STAREQ:
			op = pickOp(isFloat, OpFMul, OpMul)
		case SLASHEQ:
			op = pickOp(isFloat, OpFDiv, OpDiv)
		case PERCENTEQ:
			if isFloat {
				l.errorAt(st, "%%= needs two ints - use mod(...) for floats")
				return
			}
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
		l.regTy[cur] = target
		l.emit(Instr{Op: OpLoad, Dst: cur, A: NoReg, B: NoReg, Imm: slot,
			Comment: id.Name})
		d := l.newReg()
		l.regTy[d] = target
		l.emit(Instr{Op: op, Dst: d, A: cur, B: v})
		v = d
	}

	l.emit(Instr{Op: OpStore, A: v, Dst: NoReg, Imm: slot,
		Comment: id.Name + " " + st.Op.String()})
}

// pickOp chooses between a float and an int op, the same choice
// binary() makes for the infix forms. Named apart from the lowerer's
// own pick (a branch-select over two values) even though Go would not
// object to the clash, because a reader should not have to check which
// one a call site means.
func pickOp(float bool, f, i Op) Op {
	if float {
		return f
	}
	return i
}

// toFloat converts an int-typed register to a float-typed one. Every
// site that promotes an untyped int literal - let, assignment, return,
// a call argument, a binary operand - goes through this, so there is
// one place that knows what "int becomes float" means at the IR level.
func (l *lowerer) toFloat(v Reg) Reg {
	d := l.newReg()
	l.regTy[d] = vFloat
	l.emit(Instr{Op: OpIntToFloat, Dst: d, A: v, B: NoReg})
	return d
}

// forRange lowers the counted form into the same shape as a while loop.
//
// The bound and the step are each evaluated once into a slot, so
// `for i in 1..f()` calls f once rather than every iteration. Whether
// the loop counts up or down decides the comparison, and that has to be
// known here, so the step must be a literal for now: a dynamic one needs
// a runtime sign test that nothing yet writes programs to exercise.
func (l *lowerer) forRange(st *ForStmt) {
	if st.Var2 != "" {
		l.errorAt(st, "the two-variable for form is not on the assembly backend yet")
		return
	}
	if st.Coll != nil {
		l.forList(st)
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
	// DottedName flattens both a plain identifier and a chain of field
	// accesses, so `print` and `time.now` arrive here the same way. The
	// checker has already refused any dotted name the library does not
	// declare, which is why nothing below needs to guess whether a
	// namespace exists.
	name, ok := DottedName(c.Callee)
	if !ok {
		l.errorAt(c, "the assembly backend cannot call this expression yet")
		return l.junk()
	}

	if s, isUser := l.sigs[name]; isUser {
		if len(c.Args) != len(s.params) {
			l.errorAt(c, "%s takes %d argument(s), got %d",
				name, len(s.params), len(c.Args))
			return l.junk()
		}
		args := make([]Reg, len(c.Args))
		for i, a := range c.Args {
			v := l.expr(a)
			if s.params[i].k == kFloat && l.regTy[v].k == kInt {
				v = l.toFloat(v)
			}
			args[i] = v
		}
		if len(args) > l.fn.MaxCallArgs {
			l.fn.MaxCallArgs = len(args)
		}
		d := l.newReg()
		l.regTy[d] = s.ret
		l.emit(Instr{Op: OpCall, Dst: d, A: NoReg, B: NoReg, Args: args,
			ArgTypes: s.params, RetType: s.ret, Sym: name, Comment: name + "()"})
		return d
	}

	return l.builtin(c, name)
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
	case "os.env.get":
		if !arity(1) {
			return l.junk()
		}
		p := l.getenv(c.Args[0])
		// getenv gives NULL for an unset variable, and the Go backend
		// gives "", so the null becomes the empty string here rather than
		// a pointer nothing downstream would survive dereferencing.
		out := l.temp(vStr)
		l.emit(Instr{Op: OpStore, A: l.emptyStr(), Dst: NoReg, Imm: out})
		skip := l.newLabel()
		set := l.compare(OpEq, p, l.constant(0))
		l.emit(Instr{Op: OpJumpIf, A: set, Dst: NoReg, Imm: skip})
		l.emit(Instr{Op: OpStore, A: p, Dst: NoReg, Imm: out})
		l.mark(skip)
		d := l.newReg()
		l.regTy[d] = vStr
		l.emit(Instr{Op: OpLoad, Dst: d, A: NoReg, B: NoReg, Imm: out})
		return d

	case "os.env.has":
		if !arity(1) {
			return l.junk()
		}
		return l.compare(OpNe, l.getenv(c.Args[0]), l.constant(0))

	case "time.now":
		if !arity(0) {
			return l.junk()
		}
		l.mod.needs("timenow")
		d := l.newReg()
		l.regTy[d] = vInt
		l.emit(Instr{Op: OpTimeNow, Dst: d, A: NoReg, B: NoReg, Comment: "time.now()"})
		return d

	case "print":
		if !arity(1) {
			return l.junk()
		}
		a := l.expr(c.Args[0])
		switch l.regTy[a].k {
		case kList:
			l.printList(a, l.regTy[a])
		case kMap:
			l.printMap(a, l.regTy[a])
		case kFloat:
			l.mod.needs("floattostr")
			l.emit(Instr{Op: OpPrintFloat, A: a, Dst: NoReg, Comment: "print"})
		case kBool:
			l.emit(Instr{Op: OpPrintBool, A: a, Dst: NoReg, Comment: "print"})
		case kStr:
			l.emit(Instr{Op: OpPrintStr, A: a, Dst: NoReg, Comment: "print"})
		default:
			l.emit(Instr{Op: OpPrintInt, A: a, Dst: NoReg, Comment: "print"})
		}
		return l.void()

	case "write":
		if !arity(1) {
			return l.junk()
		}
		a := l.expr(c.Args[0])
		switch l.regTy[a].k {
		case kStr:
			l.emit(Instr{Op: OpWriteStr, A: a, Dst: NoReg, Comment: "write"})
		case kFloat:
			l.mod.needs("floattostr")
			l.emit(Instr{Op: OpWriteFloat, A: a, Dst: NoReg, Comment: "write"})
		case kBool:
			l.mod.needs("booltostr")
			d := l.newReg()
			l.regTy[d] = vStr
			l.emit(Instr{Op: OpBoolToStr, Dst: d, A: a, B: NoReg})
			l.emit(Instr{Op: OpWriteStr, A: d, Dst: NoReg})
		default:
			l.emit(Instr{Op: OpWriteInt, A: a, Dst: NoReg, Comment: "write"})
		}
		return l.void()

	case "len":
		if !arity(1) {
			return l.junk()
		}
		a := l.expr(c.Args[0])
		switch l.regTy[a].k {
		case kList:
			return l.field(a, listLenOff, vInt)
		case kMap:
			return l.field(a, mapLenOff, vInt)
		case kStr:
			l.mod.needs("strlen")
			d := l.newReg()
			l.regTy[d] = vInt
			l.emit(Instr{Op: OpStrLen, Dst: d, A: a, B: NoReg})
			return d
		}
		l.errorAt(c, "len needs a str, a list or a map")
		return l.junk()

	case "push":
		if !arity(2) {
			return l.junk()
		}
		list := l.expr(c.Args[0])
		if l.regTy[list].k != kList {
			l.errorAt(c, "push needs a list")
			return l.junk()
		}
		v := l.expr(c.Args[1])
		if l.regTy[v].k != l.regTy[list].elem {
			l.errorAt(c, "cannot push %s into %s", l.regTy[v], l.regTy[list])
			return l.junk()
		}
		l.listPush(list, v)
		return l.void()

	case "str":
		if !arity(1) {
			return l.junk()
		}
		return l.toStr(l.expr(c.Args[0]), c)

	case "abs", "min", "max":
		return l.intMath(c, name)

	case "int":
		if !arity(1) {
			return l.junk()
		}
		v := l.expr(c.Args[0])
		if l.regTy[v].k == kFloat {
			d := l.newReg()
			l.regTy[d] = vInt
			l.emit(Instr{Op: OpFloatToInt, Dst: d, A: v, B: NoReg})
			return d
		}
		return v

	case "float":
		if !arity(1) {
			return l.junk()
		}
		v := l.expr(c.Args[0])
		if l.regTy[v].k == kInt {
			return l.toFloat(v)
		}
		return v

	case "divf":
		if !arity(2) {
			return l.junk()
		}
		a, b := l.numeric(c.Args[0]), l.numeric(c.Args[1])
		d := l.newReg()
		l.regTy[d] = vFloat
		l.emit(Instr{Op: OpFDiv, Dst: d, A: a, B: b})
		return d

	case "sqrt":
		if !arity(1) {
			return l.junk()
		}
		a := l.numeric(c.Args[0])
		d := l.newReg()
		l.regTy[d] = vFloat
		l.emit(Instr{Op: OpSqrt, Dst: d, A: a, B: NoReg})
		return d

	case "mod":
		if !arity(2) {
			return l.junk()
		}
		a, b := l.numeric(c.Args[0]), l.numeric(c.Args[1])
		l.mod.needs("fmod")
		d := l.newReg()
		l.regTy[d] = vFloat
		l.emit(Instr{Op: OpFMod, Dst: d, A: a, B: b})
		return d
	}

	if r, handled := l.stringBuiltin(c, name); handled {
		return r
	}

	l.errorAt(c, "%q is not on the assembly backend yet", name)
	return l.junk()
}

// numeric lowers an argument that the Go backend accepts as int or
// float (the "Numeric" parameter kind) and always hands back a float,
// promoting an int the same way an untyped literal promotes elsewhere.
// divf, sqrt and mod all specify this: divf(7, 2) is 3.5 whichever of
// its arguments were written as int literals.
func (l *lowerer) numeric(e Expr) Reg {
	v := l.expr(e)
	if l.regTy[v].k == kInt {
		return l.toFloat(v)
	}
	return v
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
	switch l.regTy[v].k {
	case kStr:
		return v
	case kBool:
		l.mod.needs("booltostr")
		d := l.newReg()
		l.regTy[d] = vStr
		l.emit(Instr{Op: OpBoolToStr, Dst: d, A: v, B: NoReg})
		return d
	case kInt:
		l.mod.needs("inttostr")
		d := l.newReg()
		l.regTy[d] = vStr
		l.emit(Instr{Op: OpIntToStr, Dst: d, A: v, B: NoReg})
		return d
	case kFloat:
		l.mod.needs("floattostr")
		d := l.newReg()
		l.regTy[d] = vStr
		l.emit(Instr{Op: OpFloatToStr, Dst: d, A: v, B: NoReg})
		return d
	}
	l.errorAt(at, "nothing to convert to a string here")
	return l.junk()
}

// floatConsts are the builtin float constants. NAN is absent on
// purpose; see the note in library.go's ConstType.
var floatConsts = map[string]float64{
	"PI": math.Pi,
	"E":  math.E,
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

	case *FloatLit:
		v, err := strconv.ParseFloat(strings.ReplaceAll(x.Val, "_", ""), 64)
		if err != nil {
			l.errorAt(x, "float literal out of range: %s", x.Val)
			return l.junk()
		}
		d := l.newReg()
		l.regTy[d] = vFloat
		l.emit(Instr{Op: OpFConst, Dst: d, A: NoReg, B: NoReg, Imm: l.mod.internFloat(v)})
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

	case *ListLit:
		return l.listLit(x)

	case *MapLit:
		return l.mapLit(x)

	case *Index:
		coll := l.expr(x.X)
		t := l.regTy[coll]
		switch t.k {
		case kList:
			return l.listGet(coll, l.expr(x.Idx), t.elemType())
		case kMap:
			key := l.expr(x.Idx)
			if l.regTy[key].k != t.key {
				l.errorAt(x, "this map is keyed by %s, but the index is %s", t.keyType(), l.regTy[key])
				return l.junk()
			}
			return l.mapGet(coll, key, t)
		}
		l.errorAt(x, "only a list or a map can be indexed on this backend so far")
		return l.junk()

	case *Ident:
		slot, ok := l.lookup(x.Name)
		if !ok {
			// The float constants are not variables, so they are not in
			// any scope; they lower straight to a pool entry.
			if v, isConst := floatConsts[x.Name]; isConst {
				d := l.newReg()
				l.regTy[d] = vFloat
				l.emit(Instr{Op: OpFConst, Dst: d, A: NoReg, B: NoReg,
					Imm: l.mod.internFloat(v), Comment: x.Name})
				return d
			}
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
			if l.regTy[a].k == kFloat {
				l.regTy[d] = vFloat
				l.emit(Instr{Op: OpFNeg, Dst: d, A: a, B: NoReg})
			} else {
				l.regTy[d] = vInt
				l.emit(Instr{Op: OpNeg, Dst: d, A: a, B: NoReg})
			}
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
	if at.k == kStr || bt.k == kStr {
		switch x.Op {
		case PLUS:
			if at.k != kStr || bt.k != kStr {
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

	// Numbers. An untyped int literal on one side of a float operand was
	// already accepted by the checker - Go's own untyped-constant rule,
	// which Veyl copies for exactly this case - so by the time lowering
	// sees a kind mismatch here it can only be that, and can safely
	// promote rather than error a second time.
	if at.k == kFloat || bt.k == kFloat {
		if at.k == kInt {
			a = l.toFloat(a)
		}
		if bt.k == kInt {
			b = l.toFloat(b)
		}
		if l.regTy[a].k != kFloat || l.regTy[b].k != kFloat {
			l.errorAt(x, "'%s' needs numbers, got %s and %s", OpText(x.Op), at, bt)
			return l.junk()
		}

		var fop Op
		switch x.Op {
		case PLUS:
			fop = OpFAdd
		case MINUS:
			fop = OpFSub
		case STAR:
			fop = OpFMul
		case SLASH:
			fop = OpFDiv
		case EQ:
			fop = OpFEq
		case NEQ:
			fop = OpFNe
		case LT:
			fop = OpFLt
		case LTE:
			fop = OpFLe
		case GT:
			fop = OpFGt
		case GTE:
			fop = OpFGe
		case PERCENT:
			l.errorAt(x, "'%%' needs two ints - use mod(...) for floats")
			return l.junk()
		default:
			l.errorAt(x, "'%s' is not defined on floats", OpText(x.Op))
			return l.junk()
		}
		d := l.newReg()
		switch fop {
		case OpFEq, OpFNe, OpFLt, OpFLe, OpFGt, OpFGe:
			l.regTy[d] = vBool
		default:
			l.regTy[d] = vFloat
		}
		l.emit(Instr{Op: fop, Dst: d, A: a, B: b})
		return d
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

// listLit builds a list from its elements. The element type comes from
// the first element, which is what the Go backend's inference does for
// a literal with no annotation to guide it.
func (l *lowerer) listLit(x *ListLit) Reg {
	if len(x.Elems) == 0 {
		if l.hint.k != kList {
			l.errorAt(x, "an empty list needs an annotation saying what it holds, as in let xs: []int = []")
			return l.junk()
		}
		list := l.newList(l.hint, 0)
		l.regTy[list] = l.hint
		return list
	}

	vals := make([]Reg, len(x.Elems))
	for i, e := range x.Elems {
		vals[i] = l.expr(e)
	}

	elem := l.regTy[vals[0]]
	if elem.k == kList || elem.k == kVoid {
		l.errorAt(x, "a list of %s is not on the assembly backend yet", elem)
		return l.junk()
	}
	for i, v := range vals {
		if l.regTy[v].k != elem.k {
			l.errorAt(x.Elems[i], "this list holds %s, but element %d is %s",
				elem, i, l.regTy[v])
			return l.junk()
		}
	}

	t := vListOf(elem.k)
	list := l.newList(t, int64(len(vals)))
	for _, v := range vals {
		l.listPush(list, v)
	}
	l.regTy[list] = t
	return list
}

// getenv lowers one environment lookup to the raw pointer, NULL and all.
func (l *lowerer) getenv(arg Expr) Reg {
	l.mod.needs("getenv")
	name := l.expr(arg)
	d := l.newReg()
	l.regTy[d] = vStr
	l.emit(Instr{Op: OpGetEnv, Dst: d, A: name, B: NoReg, Comment: "os.env"})
	return d
}

// mapLit builds a map from its entries. The key and value types come
// from the first entry, or from the annotation when there is no first
// entry - the same inference the Go backend does, and the same reason
// an empty literal on its own cannot be typed.
//
// Entries are inserted through mapSet rather than written straight into
// the blocks, so a literal that lists its keys out of order still ends
// up sorted, and a literal that repeats a key keeps the last value.
// Both match the Go backend.
func (l *lowerer) mapLit(x *MapLit) Reg {
	if len(x.Keys) == 0 {
		if l.hint.k != kMap {
			l.errorAt(x, "cannot tell what kind of map this is - annotate it, as in: let m: {str: int} = {}")
			return l.junk()
		}
		return l.newMap(l.hint, 0)
	}

	keys := make([]Reg, len(x.Keys))
	vals := make([]Reg, len(x.Vals))
	for i := range x.Keys {
		keys[i] = l.expr(x.Keys[i])
		vals[i] = l.expr(x.Vals[i])
	}

	kk := l.regTy[keys[0]]
	vk := l.regTy[vals[0]]
	if kk.k != kInt && kk.k != kStr {
		l.errorAt(x, "a map keyed by %s is not on the assembly backend yet", kk)
		return l.junk()
	}
	if vk.k == kList || vk.k == kMap || vk.k == kVoid {
		l.errorAt(x, "a map of %s is not on the assembly backend yet", vk)
		return l.junk()
	}
	for i := range keys {
		if l.regTy[keys[i]].k != kk.k {
			l.errorAt(x.Keys[i], "this map is keyed by %s, but key %d is %s", kk, i, l.regTy[keys[i]])
			return l.junk()
		}
		if l.regTy[vals[i]].k != vk.k {
			l.errorAt(x.Vals[i], "this map holds %s, but value %d is %s", vk, i, l.regTy[vals[i]])
			return l.junk()
		}
	}

	t := vMapOf(kk.k, vk.k)
	m := l.newMap(t, int64(len(keys)))
	for i := range keys {
		l.mapSet(m, keys[i], vals[i], t)
	}
	l.regTy[m] = t
	return m
}

// match lowers a multi-way branch into a chain of comparisons.
//
// The subject is evaluated once into a slot and compared against each
// arm's values in turn. Arms do not fall through, so every one ends with
// a jump past the rest. A jump table would be faster for a dense set of
// integers and is the obvious later optimisation; a chain is correct for
// every case including strings, which is what matters now.
func (l *lowerer) match(st *MatchStmt) {
	subject := l.expr(st.Subject)
	t := l.regTy[subject]
	if t.k == kList {
		l.errorAt(st, "a list cannot be matched on, because it has no single value to compare")
		return
	}

	slot := l.temp(t)
	l.emit(Instr{Op: OpStore, A: subject, Dst: NoReg, Imm: slot, Comment: "match"})

	done := l.newLabel()
	bodies := make([]int64, len(st.Cases))
	for i := range st.Cases {
		bodies[i] = l.newLabel()
	}

	for i, arm := range st.Cases {
		for _, want := range arm.Values {
			held := l.newReg()
			l.regTy[held] = t
			l.emit(Instr{Op: OpLoad, Dst: held, A: NoReg, B: NoReg, Imm: slot})
			v := l.expr(want)
			if l.regTy[v].k != t.k {
				l.errorAt(want, "this arm compares %s against a %s subject",
					l.regTy[v], t)
				continue
			}
			hit := l.newReg()
			l.regTy[hit] = vBool
			if t.k == kStr {
				l.mod.needs("streq")
				l.emit(Instr{Op: OpStrEq, Dst: hit, A: held, B: v})
			} else {
				l.emit(Instr{Op: OpEq, Dst: hit, A: held, B: v})
			}
			l.emit(Instr{Op: OpJumpIf, A: hit, Dst: NoReg, Imm: bodies[i]})
		}
	}

	// Nothing matched. The else arm runs here, or control falls past.
	if st.Else != nil {
		l.stmt(st.Else)
	}
	l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: done})

	for i, arm := range st.Cases {
		l.mark(bodies[i])
		l.stmt(arm.Body)
		l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: done})
	}

	l.mark(done)
}
