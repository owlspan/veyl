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

// A Reg is a virtual register. There are as many as the program needs;
// mapping them onto the sixteen real ones is the allocator's problem.
type Reg int

// NoReg marks an instruction that writes nothing.
const NoReg Reg = -1

type Op int

const (
	OpConst    Op = iota // Dst = Imm
	OpLoad               // Dst = local[Imm]
	OpStore              // local[Imm] = A
	OpAdd                // Dst = A + B
	OpSub                // Dst = A - B
	OpMul                // Dst = A * B
	OpDiv                // Dst = A / B   (truncating, as the language specifies)
	OpMod                // Dst = A % B
	OpNeg                // Dst = -A
	OpPrintInt           // print A
)

var opNames = [...]string{
	"const", "load", "store", "add", "sub", "mul", "div", "mod", "neg", "printint",
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
	Name   string
	Code   []Instr
	NRegs  int // virtual registers used
	NSlots int // local variable slots
}

// ---- lowering ----

type lowerer struct {
	fn     *Func
	locals map[string]int64 // Veyl name -> slot index
	errs   []string
	file   string
}

// Lower turns the checked program into IR. It reports its own errors
// rather than aborting, matching the rest of the pipeline: one mistake
// should not hide the next one.
func Lower(p *Program, file string) (*Func, []string) {
	l := &lowerer{
		fn:     &Func{Name: "main"},
		locals: map[string]int64{},
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
		l.emit(Instr{Op: OpStore, A: v, Dst: NoReg, Imm: l.slot(st.Name),
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
		v := l.expr(st.Value)
		l.emit(Instr{Op: OpStore, A: v, Dst: NoReg, Imm: l.slot(id.Name),
			Comment: id.Name + " ="})

	case *ExprStmt:
		call, ok := st.X.(*Call)
		if !ok {
			l.errorAt(st, "this statement does nothing")
			return
		}
		l.call(call)

	default:
		l.errorAt(s.(Node), "the assembly backend does not handle this statement yet")
	}
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
	l.emit(Instr{Op: OpPrintInt, A: a, Dst: NoReg, Comment: "print"})
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
		l.emit(Instr{Op: OpLoad, Dst: d, A: NoReg, B: NoReg, Imm: s, Comment: x.Name})
		return d

	case *Unary:
		if x.Op != MINUS {
			l.errorAt(x, "the assembly backend does not handle this operator yet")
			return l.newReg()
		}
		a := l.expr(x.X)
		d := l.newReg()
		l.emit(Instr{Op: OpNeg, Dst: d, A: a, B: NoReg})
		return d

	case *Binary:
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
		default:
			l.errorAt(x, "the assembly backend does not handle %s yet", x.Op)
			return l.newReg()
		}
		a := l.expr(x.L)
		b := l.expr(x.R)
		d := l.newReg()
		l.emit(Instr{Op: op, Dst: d, A: a, B: b})
		return d

	default:
		l.errorAt(e.(Node), "the assembly backend does not handle this expression yet")
		return l.newReg()
	}
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
		case OpNeg, OpPrintInt:
			line += fmt.Sprintf(" v%d", in.A)
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
