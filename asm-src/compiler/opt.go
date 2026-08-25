package main

// The IR optimizer. Each pass is a function over one Func, checked by
// its own environment variable so a suspect pass can be turned off
// without touching the rest:
//
//	VEYL_NOFOLD     constant and branch folding
//
// More passes land behind their own switches as they are written.
//
// VEYL_NOOPT=1 turns every pass off and leaves the output identical to
// what the unoptimized compiler emits.

import (
	"math"
	"os"
)

func envOff(name string) bool { return os.Getenv(name) == "1" }

// Optimize rewrites every function of the module in place. It runs
// after Lower has reported no errors, so the IR it sees always came
// from a program that checked clean.
func Optimize(m *Module) {
	if envOff("VEYL_NOOPT") {
		return
	}
	for _, fn := range m.Funcs {
		if !envOff("VEYL_NOFOLD") {
			fold(m, fn)
		}
		if !envOff("VEYL_NODCE") {
			dce(fn)
		}
	}
}

// ---- constant and branch folding ----

// fold evaluates operations whose operands are constants, folds
// branches on a known condition, and then drops everything no path
// reaches.
//
// Virtual registers are single assignment, so a constant definition
// holds at every mention of its register no matter how control flow
// runs between them. No dominance analysis is needed.
//
// Division folds only when the divisor is a nonzero constant other
// than minus one over the most negative value: those two trap at
// runtime, and a folded program must keep trapping exactly there.
func fold(m *Module, fn *Func) {
	for round := 0; round < 4; round++ {
		iconst := map[Reg]int64{}
		fconst := map[Reg]float64{}
		for _, in := range fn.Code {
			switch in.Op {
			case OpConst:
				iconst[in.Dst] = in.Imm
			case OpFConst:
				if int(in.Imm) < len(m.Floats) {
					fconst[in.Dst] = m.Floats[in.Imm]
				}
			}
		}

		changed := false
		out := make([]Instr, 0, len(fn.Code))
		for _, in := range fn.Code {
			if newin, ok := foldInstr(m, in, iconst, fconst); ok {
				in = newin
				changed = true
				switch in.Op {
				case OpConst:
					iconst[in.Dst] = in.Imm
				case OpFConst:
					fconst[in.Dst] = m.Floats[in.Imm]
				}
			}
			switch in.Op {
			case OpJumpIf:
				if c, ok := iconst[in.A]; ok {
					changed = true
					if c != 0 {
						in = Instr{Op: OpJump, Imm: in.Imm}
					} else {
						continue // known true: never taken
					}
				}
			case OpJumpNot:
				if c, ok := iconst[in.A]; ok {
					changed = true
					if c == 0 {
						in = Instr{Op: OpJump, Imm: in.Imm}
					} else {
						continue // known false: never taken
					}
				}
			}
			out = append(out, in)
		}
		fn.Code = out
		if !changed {
			break
		}
	}
	dropUnreachable(fn)
}

// foldInstr rewrites one instruction to a constant when it can.
// Integer arithmetic uses Go's own semantics: signed overflow wraps,
// shifts mask their count with 63 to match what the hardware does.
// Float results are pooled through internFloat like any literal, but
// folding stops for NaN, infinity and zero results: the first two have
// bit patterns the hardware would not produce, and internFloat cannot
// tell negative from positive zero because Go's == says they are equal.
func foldInstr(m *Module, in Instr, ic map[Reg]int64, fc map[Reg]float64) (Instr, bool) {
	a, aok := ic[in.A]
	b, bok := ic[in.B]

	toInt := func(v int64) (Instr, bool) {
		return Instr{Op: OpConst, Dst: in.Dst, Imm: v}, true
	}

	switch in.Op {
	case OpNeg:
		if aok {
			return toInt(-a)
		}
	case OpBNot:
		if aok {
			return toInt(^a)
		}
	case OpNot:
		if aok {
			if a == 0 {
				return toInt(1)
			}
			return toInt(0)
		}
	case OpAdd:
		if aok && bok {
			return toInt(a + b)
		}
	case OpSub:
		if aok && bok {
			return toInt(a - b)
		}
	case OpMul:
		if aok && bok {
			return toInt(a * b)
		}
	case OpDiv:
		if aok && bok && b != 0 && !(a == math.MinInt64 && b == -1) {
			return toInt(a / b)
		}
	case OpMod:
		if aok && bok && b != 0 && !(a == math.MinInt64 && b == -1) {
			return toInt(a % b)
		}
	case OpBAnd:
		if aok && bok {
			return toInt(a & b)
		}
	case OpBOr:
		if aok && bok {
			return toInt(a | b)
		}
	case OpBXor:
		if aok && bok {
			return toInt(a ^ b)
		}
	case OpShl:
		if aok && bok {
			return toInt(a << (uint64(b) & 63))
		}
	case OpShr:
		if aok && bok {
			return toInt(a >> (uint64(b) & 63)) // arithmetic, like sar
		}
	case OpEq:
		if aok && bok {
			if a == b {
				return toInt(1)
			}
			return toInt(0)
		}
	case OpNe:
		if aok && bok {
			if a != b {
				return toInt(1)
			}
			return toInt(0)
		}
	case OpLt:
		if aok && bok {
			if a < b {
				return toInt(1)
			}
			return toInt(0)
		}
	case OpLe:
		if aok && bok {
			if a <= b {
				return toInt(1)
			}
			return toInt(0)
		}
	case OpGt:
		if aok && bok {
			if a > b {
				return toInt(1)
			}
			return toInt(0)
		}
	case OpGe:
		if aok && bok {
			if a >= b {
				return toInt(1)
			}
			return toInt(0)
		}
	case OpIntToFloat:
		if aok {
			return Instr{Op: OpFConst, Dst: in.Dst,
				Imm: m.internFloat(float64(a))}, true
		}
	case OpFAdd:
		if fa, faok := fc[in.A]; faok {
			if fb, fbok := fc[in.B]; fbok {
				if r := fa + fb; !isNaNInf(r) && r != 0 {
					return Instr{Op: OpFConst, Dst: in.Dst,
						Imm: m.internFloat(r)}, true
				}
			}
		}
	case OpFSub:
		if fa, faok := fc[in.A]; faok {
			if fb, fbok := fc[in.B]; fbok {
				if r := fa - fb; !isNaNInf(r) && r != 0 {
					return Instr{Op: OpFConst, Dst: in.Dst,
						Imm: m.internFloat(r)}, true
				}
			}
		}
	case OpFMul:
		if fa, faok := fc[in.A]; faok {
			if fb, fbok := fc[in.B]; fbok {
				if r := fa * fb; !isNaNInf(r) && r != 0 {
					return Instr{Op: OpFConst, Dst: in.Dst,
						Imm: m.internFloat(r)}, true
				}
			}
		}
	}
	return Instr{}, false
}

func isNaNInf(v float64) bool {
	return math.IsNaN(v) || math.IsInf(v, 0)
}

// dropUnreachable keeps only the instructions some execution path can
// reach, following the jumps as they now stand. A label nothing jumps
// to any more disappears with its block.
func dropUnreachable(fn *Func) {
	n := len(fn.Code)
	if n == 0 {
		return
	}
	labelAt := map[int64]int{}
	for i, in := range fn.Code {
		if in.Op == OpLabel {
			labelAt[in.Imm] = i
		}
	}

	reach := make([]bool, n)
	work := []int{0}
	for len(work) > 0 {
		i := work[len(work)-1]
		work = work[:len(work)-1]
		for i < n && !reach[i] {
			reach[i] = true
			switch fn.Code[i].Op {
			case OpLabel:
				i++
			case OpJump:
				work = append(work, labelAt[fn.Code[i].Imm])
				i = n
			case OpJumpIf, OpJumpNot:
				work = append(work, labelAt[fn.Code[i].Imm])
				i++
			case OpRet, OpMustFail, OpBoundsFail:
				i = n
			default:
				i++
			}
		}
	}

	code := make([]Instr, 0, n)
	for i, in := range fn.Code {
		if reach[i] {
			code = append(code, in)
		}
	}
	fn.Code = code
}

// ---- dead code elimination ----

// dce removes pure definitions whose result is never used, down to a
// fixed point: killing one definition can make the one feeding it dead.
// Anything that traps, stores, prints, allocates or calls stays even
// with an ignored result, because removing it would change what the
// program does. Division and modulo stay too - they trap on a zero
// divisor whether or not anyone reads the quotient.
func dce(fn *Func) {
	for {
		used := map[Reg]bool{}
		mark := func(r Reg) {
			if r != NoReg {
				used[r] = true
			}
		}
		for _, in := range fn.Code {
			mark(in.A)
			mark(in.B)
			for _, arg := range in.Args {
				mark(arg)
			}
		}

		kept := fn.Code[:0]
		changed := false
		for _, in := range fn.Code {
			if in.Dst != NoReg && !used[in.Dst] && deadOK(in.Op) {
				changed = true
				continue
			}
			kept = append(kept, in)
		}
		fn.Code = kept
		if !changed {
			return
		}
	}
}

// deadOK says whether the operation is safe to delete when its result
// is unused. One list, read as "everything else has an effect".
func deadOK(op Op) bool {
	switch op {
	case OpConst, OpStr, OpLoad, OpFConst,
		OpAdd, OpSub, OpMul, OpNeg,
		OpFAdd, OpFSub, OpFMul, OpFDiv, OpFNeg,
		OpFEq, OpFNe, OpFLt, OpFLe, OpFGt, OpFGe,
		OpIntToFloat, OpFloatToInt, OpSqrt, OpFMod,
		OpBAnd, OpBOr, OpBXor, OpBNot, OpShl, OpShr,
		OpEq, OpNe, OpLt, OpLe, OpGt, OpGe, OpNot,
		OpIndexAddr, OpLoadByte,
		OpStackPtr, OpGlobalAddr, OpSymAddr, OpSlotAddr:
		return true
	}
	return false
}
