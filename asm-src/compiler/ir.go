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
)

// A vty is the little type information this backend tracks: enough to
// tell an int from a bool, and no more.
//
// The real type checker lives in ../../src/compiler/check.go and is not
// shared yet, so this is not type checking - a program that is wrong
// will still be compiled to something. Run it through the Go backend
// first. What this does buy is that `print` of a comparison writes
// "true" rather than "1", which is what the language says it does and
// what the differential test caught it not doing.
type vty int

const (
	vInt vty = iota
	vBool
)

// A Reg is a virtual register. There are as many as the program needs;
// mapping them onto the sixteen real ones is the allocator's problem.
type Reg int

// NoReg marks an instruction that writes nothing.
const NoReg Reg = -1

type Op int

const (
	OpConst     Op = iota // Dst = Imm
	OpLoad                // Dst = local[Imm]
	OpStore               // local[Imm] = A
	OpAdd                 // Dst = A + B
	OpSub                 // Dst = A - B
	OpMul                 // Dst = A * B
	OpDiv                 // Dst = A / B   (truncating, as the language specifies)
	OpMod                 // Dst = A % B
	OpNeg                 // Dst = -A
	OpPrintInt            // print A as a number
	OpPrintBool           // print A as true or false

	// Comparisons produce 0 or 1, so a bool is just an int and needs no
	// separate representation. Branching then only needs one conditional
	// jump form rather than one per comparison.
	OpEq // Dst = A == B
	OpNe // Dst = A != B
	OpLt // Dst = A < B
	OpLe // Dst = A <= B
	OpGt // Dst = A > B
	OpGe // Dst = A >= B

	OpNot // Dst = !A

	// Control flow. Labels are indices into the instruction stream,
	// resolved by the emitter into assembler labels. Keeping jumps as
	// integer targets rather than pointers means the byte writer can
	// backpatch them the same way, later, without a different IR.
	OpLabel   // Imm names this point
	OpJump    // goto Imm
	OpJumpIf  // if A != 0 goto Imm
	OpJumpNot // if A == 0 goto Imm
)

var opNames = [...]string{
	"const", "load", "store", "add", "sub", "mul", "div", "mod", "neg",
	"printint", "printbool",
	"eq", "ne", "lt", "le", "gt", "ge", "not",
	"label", "jump", "jumpif", "jumpnot",
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
	Comment string // the Veyl this came from, carried into the .s file
}

// A Func is one lowered function. Only main exists so far.
type Func struct {
	Name    string
	Code    []Instr
	NRegs   int // virtual registers used
	NSlots  int // local variable slots
	NLabels int // labels handed out
}

// ---- lowering ----

type lowerer struct {
	fn     *Func
	locals map[string]int64 // Veyl name -> slot index
	errs   []string
	file   string

	// Loop targets for break and continue, innermost last. A stack
	// rather than a field, because loops nest.
	loops []loopTarget

	// The tracked type of each local, by slot, and of each virtual
	// register. Kept beside the IR rather than inside Instr: the emitter
	// never needs it, only the choice of print does.
	slotTy map[int64]vty
	regTy  map[Reg]vty
}

type loopTarget struct {
	brk  int64 // label after the loop
	cont int64 // label at the top, where the condition is retested
}

// Lower turns the checked program into IR. It reports its own errors
// rather than aborting, matching the rest of the pipeline: one mistake
// should not hide the next one.
func Lower(p *Program, file string) (*Func, []string) {
	l := &lowerer{
		fn:     &Func{Name: "main"},
		locals: map[string]int64{},
		slotTy: map[int64]vty{},
		regTy:  map[Reg]vty{},
		file:   file,
	}
	for _, st := range p.Main {
		l.stmt(st)
	}
	return l.fn, l.errs
}

func (l *lowerer) errorAt(n Node, format string, args ...any) {
	line, col := n.Pos()
	l.errs = append(l.errs, fmt.Sprintf("%s:%d:%d: %s", l.file, line, col,
		fmt.Sprintf(format, args...)))
}

// newReg hands out the next virtual register.
func (l *lowerer) newReg() Reg {
	r := Reg(l.fn.NRegs)
	l.fn.NRegs++
	return r
}

func (l *lowerer) emit(in Instr) {
	l.fn.Code = append(l.fn.Code, in)
}

// slot finds or creates the stack slot for a local.
func (l *lowerer) slot(name string) int64 {
	if s, ok := l.locals[name]; ok {
		return s
	}
	s := int64(l.fn.NSlots)
	l.fn.NSlots++
	l.locals[name] = s
	return s
}

func (l *lowerer) stmt(s Stmt) {
	switch st := s.(type) {
	case *LetStmt:
		v := l.expr(st.Value)
		slot := l.slot(st.Name)
		l.slotTy[slot] = l.regTy[v]
		l.emit(Instr{Op: OpStore, A: v, Dst: NoReg, Imm: slot,
			Comment: "let " + st.Name})

	case *AssignStmt:
		id, ok := st.Target.(*Ident)
		if !ok {
			l.errorAt(st, "the assembly backend can only assign to a plain name so far")
			return
		}
		if _, known := l.locals[id.Name]; !known {
			l.errorAt(st, "undefined variable %q", id.Name)
			return
		}
		slot := l.slot(id.Name)
		v := l.expr(st.Value)

		// The compound forms read the target first. Ignoring st.Op here
		// silently turned `i += 1` into `i = 1`, which made every loop
		// that counted upwards run forever - a bug that produces no
		// output at all rather than wrong output, so nothing catches it
		// except waiting.
		if st.Op != ASSIGN {
			var op Op
			switch st.Op {
			case PLUSEQ:
				op = OpAdd
			case MINUSEQ:
				op = OpSub
			case STAREQ:
				op = OpMul
			case SLASHEQ:
				op = OpDiv
			case PERCENTEQ:
				op = OpMod
			default:
				l.errorAt(st, "the assembly backend does not handle %s yet", st.Op)
				return
			}
			cur := l.newReg()
			l.emit(Instr{Op: OpLoad, Dst: cur, A: NoReg, B: NoReg, Imm: slot,
				Comment: id.Name})
			d := l.newReg()
			l.emit(Instr{Op: op, Dst: d, A: cur, B: v})
			v = d
		}

		l.emit(Instr{Op: OpStore, A: v, Dst: NoReg, Imm: slot,
			Comment: id.Name + " " + st.Op.String()})

	case *ExprStmt:
		call, ok := st.X.(*Call)
		if !ok {
			l.errorAt(st, "this statement does nothing")
			return
		}
		l.call(call)

	case *Block:
		// No new scope yet: locals are function-wide, which is enough
		// while there are no shadowing rules to honour. The resolver in
		// ../src is what will eventually decide this, once it is shared.
		for _, inner := range st.Stmts {
			l.stmt(inner)
		}

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

// newLabel reserves a label. mark places it.
func (l *lowerer) newLabel() int64 {
	n := int64(l.fn.NLabels)
	l.fn.NLabels++
	return n
}

func (l *lowerer) mark(label int64) {
	l.emit(Instr{Op: OpLabel, A: NoReg, Dst: NoReg, Imm: label})
}

func (l *lowerer) call(c *Call) {
	name, ok := c.Callee.(*Ident)
	if !ok {
		l.errorAt(c, "the assembly backend can only call a builtin by name so far")
		return
	}
	if name.Name != "print" {
		l.errorAt(c, "%q is not available on the assembly backend yet", name.Name)
		return
	}
	if len(c.Args) != 1 {
		l.errorAt(c, "print takes 1 argument, got %d", len(c.Args))
		return
	}
	a := l.expr(c.Args[0])
	op := OpPrintInt
	if l.regTy[a] == vBool {
		op = OpPrintBool
	}
	l.emit(Instr{Op: op, A: a, Dst: NoReg, Comment: "print"})
}

func (l *lowerer) expr(e Expr) Reg {
	switch x := e.(type) {
	case *IntLit:
		n, err := strconv.ParseInt(x.Val, 0, 64)
		if err != nil {
			l.errorAt(x, "integer literal out of range: %s", x.Val)
			return l.newReg()
		}
		d := l.newReg()
		l.emit(Instr{Op: OpConst, Dst: d, A: NoReg, B: NoReg, Imm: n})
		return d

	case *Ident:
		s, ok := l.locals[x.Name]
		if !ok {
			l.errorAt(x, "undefined variable %q", x.Name)
			return l.newReg()
		}
		d := l.newReg()
		l.regTy[d] = l.slotTy[s]
		l.emit(Instr{Op: OpLoad, Dst: d, A: NoReg, B: NoReg, Imm: s, Comment: x.Name})
		return d

	case *BoolLit:
		// A bool is an int that is 0 or 1, which is what the comparisons
		// already produce. No separate representation needed.
		n := int64(0)
		if x.Val {
			n = 1
		}
		d := l.newReg()
		l.regTy[d] = vBool
		l.emit(Instr{Op: OpConst, Dst: d, A: NoReg, B: NoReg, Imm: n})
		return d

	case *Unary:
		a := l.expr(x.X)
		d := l.newReg()
		switch x.Op {
		case MINUS:
			l.emit(Instr{Op: OpNeg, Dst: d, A: a, B: NoReg})
		case BANG:
			l.regTy[d] = vBool
			l.emit(Instr{Op: OpNot, Dst: d, A: a, B: NoReg})
		default:
			l.errorAt(x, "the assembly backend does not handle this operator yet")
		}
		return d

	case *Binary:
		// && and || short-circuit, so they cannot be lowered as a plain
		// two-operand instruction: the right side must not run when the
		// left already decided the answer.
		if x.Op == AND || x.Op == OR {
			return l.shortCircuit(x)
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
			return l.newReg()
		}
		a := l.expr(x.L)
		b := l.expr(x.R)
		d := l.newReg()
		switch op {
		case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
			l.regTy[d] = vBool
		}
		l.emit(Instr{Op: op, Dst: d, A: a, B: b})
		return d

	default:
		l.errorAt(e.(Node), "the assembly backend does not handle this expression yet")
		return l.newReg()
	}
}

// shortCircuit lowers && and ||.
//
// The result is built in a stack slot rather than a virtual register,
// because two different branches write it and the naive allocator gives
// every virtual register exactly one definition site. A slot has no such
// assumption. When a real allocator arrives this becomes a phi, which is
// the point at which SSA starts paying for itself.
func (l *lowerer) shortCircuit(x *Binary) Reg {
	slot := int64(l.fn.NSlots)
	l.fn.NSlots++
	done := l.newLabel()

	left := l.expr(x.L)
	l.emit(Instr{Op: OpStore, A: left, Dst: NoReg, Imm: slot})

	if x.Op == AND {
		// false and the answer is already false, so skip the right side.
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

// ---- printing, for `veylasm ir` ----

func (f *Func) String() string {
	out := "func " + f.Name + ":\n"
	for _, in := range f.Code {
		line := "    "
		if in.Dst != NoReg {
			line += fmt.Sprintf("v%d = ", in.Dst)
		}
		line += in.Op.String()
		switch in.Op {
		case OpConst:
			line += fmt.Sprintf(" %d", in.Imm)
		case OpLoad:
			line += fmt.Sprintf(" slot%d", in.Imm)
		case OpStore:
			line += fmt.Sprintf(" slot%d, v%d", in.Imm, in.A)
		case OpNeg, OpNot, OpPrintInt, OpPrintBool:
			line += fmt.Sprintf(" v%d", in.A)
		case OpLabel:
			line = fmt.Sprintf("  L%d:", in.Imm)
		case OpJump:
			line += fmt.Sprintf(" L%d", in.Imm)
		case OpJumpIf, OpJumpNot:
			line += fmt.Sprintf(" v%d, L%d", in.A, in.Imm)
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
