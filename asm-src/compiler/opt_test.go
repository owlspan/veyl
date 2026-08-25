package main

import (
	"math"
	"testing"
)

// mkfn builds a one-block function for the opt tests.
func mkfn(code []Instr, nregs int) *Func {
	return &Func{Name: "t", NRegs: nregs, Code: code}
}

func TestFoldArith(t *testing.T) {
	fn := mkfn([]Instr{
		{Op: OpConst, Dst: 0, Imm: 2},
		{Op: OpConst, Dst: 1, Imm: 3},
		{Op: OpAdd, Dst: 2, A: 0, B: 1},
		{Op: OpStore, A: 2, Imm: 0},
	}, 3)
	fold(&Module{}, fn)

	got := fn.Code[2]
	if got.Op != OpConst || got.Imm != 5 {
		t.Fatalf("add of two consts did not fold: %+v", got)
	}
}

func TestFoldChains(t *testing.T) {
	// (2 + 3) * 4 as three ops folds to one const within the sweeps.
	fn := mkfn([]Instr{
		{Op: OpConst, Dst: 0, Imm: 2},
		{Op: OpConst, Dst: 1, Imm: 3},
		{Op: OpAdd, Dst: 2, A: 0, B: 1},
		{Op: OpConst, Dst: 3, Imm: 4},
		{Op: OpMul, Dst: 4, A: 2, B: 3},
		{Op: OpStore, A: 4, Imm: 0},
	}, 5)
	fold(&Module{}, fn)

	if got := fn.Code[4]; got.Op != OpConst || got.Imm != 20 {
		t.Fatalf("chained fold wrong: %+v", got)
	}
}

func TestFoldDivGuards(t *testing.T) {
	const min64 = math.MinInt64

	fn := mkfn([]Instr{
		{Op: OpConst, Dst: 0, Imm: 7},
		{Op: OpConst, Dst: 1, Imm: 2},
		{Op: OpDiv, Dst: 2, A: 0, B: 1},
	}, 3)
	fold(&Module{}, fn)
	if got := fn.Code[2]; got.Op != OpConst || got.Imm != 3 {
		t.Fatalf("7/2 did not fold: %+v", got)
	}

	// x / 0 must stay a runtime event.
	fn = mkfn([]Instr{
		{Op: OpConst, Dst: 0, Imm: 7},
		{Op: OpConst, Dst: 1, Imm: 0},
		{Op: OpDiv, Dst: 2, A: 0, B: 1},
	}, 3)
	fold(&Module{}, fn)
	if got := fn.Code[2]; got.Op != OpDiv {
		t.Fatalf("div by zero folded: %+v", got)
	}

	// MinInt / -1 traps on the hardware; folding would remove the trap.
	fn = mkfn([]Instr{
		{Op: OpConst, Dst: 0, Imm: min64},
		{Op: OpConst, Dst: 1, Imm: -1},
		{Op: OpDiv, Dst: 2, A: 0, B: 1},
	}, 3)
	fold(&Module{}, fn)
	if got := fn.Code[2]; got.Op != OpDiv {
		t.Fatalf("MinInt/-1 folded: %+v", got)
	}
}

func TestFoldShiftMask(t *testing.T) {
	// The hardware masks the count with 63, so shifting by 64 leaves
	// the value alone rather than producing zero.
	fn := mkfn([]Instr{
		{Op: OpConst, Dst: 0, Imm: 1},
		{Op: OpConst, Dst: 1, Imm: 64},
		{Op: OpShl, Dst: 2, A: 0, B: 1},
	}, 3)
	fold(&Module{}, fn)
	if got := fn.Code[2]; got.Op != OpConst || got.Imm != 1 {
		t.Fatalf("shift by 64 did not follow the hardware mask: %+v", got)
	}
}

func TestFoldBranchKillsFallthrough(t *testing.T) {
	// cond is 1, so the folded unconditional jump makes everything
	// between it and its target unreachable. The label survives.
	fn := mkfn([]Instr{
		{Op: OpConst, Dst: 0, Imm: 1},
		{Op: OpJumpIf, A: 0, Imm: 0},
		{Op: OpConst, Dst: 1, Imm: 99},
		{Op: OpLabel, Imm: 0},
		{Op: OpRet, A: NoReg},
	}, 2)
	fold(&Module{}, fn)

	if got := fn.Code[1]; got.Op != OpJump {
		t.Fatalf("jumpif on known one did not become a jump: %+v", got)
	}
	for _, in := range fn.Code {
		if in.Op == OpConst && in.Imm == 99 {
			t.Fatal("block under the taken branch survived")
		}
	}
	if len(fn.Code) != 4 {
		t.Fatalf("wrong shape after fold: %+v", fn.Code)
	}
}

func TestFoldZeroCondFallsThrough(t *testing.T) {
	// cond is 0: the branch never takes, the branch is deleted, and the
	// code after it still runs. Deleting the branch must NOT delete the
	// fallthrough path; that is the path execution takes.
	fn := mkfn([]Instr{
		{Op: OpConst, Dst: 0, Imm: 0},
		{Op: OpJumpIf, A: 0, Imm: 0},
		{Op: OpConst, Dst: 1, Imm: 7},
		{Op: OpStore, A: 1, Imm: 0},
	}, 2)
	fold(&Module{}, fn)

	found := false
	for _, in := range fn.Code {
		if in.Op == OpConst && in.Imm == 7 {
			found = true
		}
		if in.Op == OpJumpIf || in.Op == OpJump {
			t.Fatal("a branch survived on a known-zero condition")
		}
	}
	if !found {
		t.Fatal("fallthrough path was deleted with the branch")
	}
}

func TestFoldFloatPoolResult(t *testing.T) {
	m := &Module{Floats: []float64{2.5, 4}}
	fn := mkfn([]Instr{
		{Op: OpFConst, Dst: 0, Imm: 0},
		{Op: OpFConst, Dst: 1, Imm: 1},
		{Op: OpFMul, Dst: 2, A: 0, B: 1},
		{Op: OpStore, A: 2, Imm: 0},
	}, 3)
	fold(m, fn)

	got := fn.Code[2]
	if got.Op != OpFConst {
		t.Fatalf("fmul of pooled consts did not fold: %+v", got)
	}
	if m.Floats[got.Imm] != 10 {
		t.Fatalf("folded float value wrong: %v", m.Floats[got.Imm])
	}
}

func TestFoldFloatZeroNotPooled(t *testing.T) {
	// internFloat cannot tell -0.0 from +0.0, so a zero result never
	// folds: the sign would come out of whatever entry was pooled first.
	m := &Module{Floats: []float64{2.5, 2.5}}
	fn := mkfn([]Instr{
		{Op: OpFConst, Dst: 0, Imm: 0},
		{Op: OpFConst, Dst: 1, Imm: 1},
		{Op: OpFSub, Dst: 2, A: 0, B: 1},
		{Op: OpStore, A: 2, Imm: 0},
	}, 3)
	fold(m, fn)

	if got := fn.Code[2]; got.Op != OpFSub {
		t.Fatalf("zero float result folded and lost its sign: %+v", got)
	}
}

func TestDCERemovesUnusedChains(t *testing.T) {
	// The add is unused, so it dies; that leaves const r1 unused, so it
	// dies on the next sweep. r0 survives on the store.
	fn := mkfn([]Instr{
		{Op: OpConst, Dst: 0, Imm: 1},
		{Op: OpConst, Dst: 1, Imm: 2},
		{Op: OpAdd, Dst: 2, A: 0, B: 1},
		{Op: OpStore, A: 0, Imm: 0},
	}, 3)
	dce(fn)

	if len(fn.Code) != 2 {
		t.Fatalf("dead chain survived: %+v", fn.Code)
	}
	if fn.Code[0].Op != OpConst || fn.Code[0].Dst != 0 ||
		fn.Code[1].Op != OpStore || fn.Code[1].A != 0 {
		t.Fatalf("wrong instructions kept: %+v", fn.Code)
	}
}

func TestDCEKeepsTraps(t *testing.T) {
	// Nobody reads the quotient, but x/0 still has to stop the program.
	fn := mkfn([]Instr{
		{Op: OpConst, Dst: 0, Imm: 7},
		{Op: OpConst, Dst: 1, Imm: 0},
		{Op: OpDiv, Dst: 2, A: 0, B: 1},
	}, 3)
	dce(fn)

	if len(fn.Code) != 3 {
		t.Fatalf("dividing by zero was deleted: %+v", fn.Code)
	}
}

func TestDCEKeepsEffects(t *testing.T) {
	// A store stays; a call with an ignored result stays (it may print,
	// or write through a pointer); an unused load goes.
	fn := mkfn([]Instr{
		{Op: OpStr, Dst: 0, Imm: 0},
		{Op: OpStore, A: 0, Imm: 0},
		{Op: OpLoad, Dst: 1, Imm: 0},
		{Op: OpCall, Dst: 2, Sym: "f", Args: []Reg{0}},
	}, 3)
	dce(fn)

	if len(fn.Code) != 3 {
		t.Fatalf("wrong number of instructions kept: %+v", fn.Code)
	}
	sawLoad, sawCall := false, false
	for _, in := range fn.Code {
		switch in.Op {
		case OpLoad:
			sawLoad = true
		case OpCall:
			sawCall = true
		}
	}
	if sawLoad {
		t.Fatal("the dead load should have gone")
	}
	if !sawCall {
		t.Fatal("a call must survive even when its result is ignored")
	}
}

func TestFoldKillSwitches(t *testing.T) {
	build := func() *Module {
		fn := mkfn([]Instr{
			{Op: OpConst, Dst: 0, Imm: 2},
			{Op: OpConst, Dst: 1, Imm: 3},
			{Op: OpAdd, Dst: 2, A: 0, B: 1},
			{Op: OpStore, A: 2, Imm: 0},
		}, 3)
		return &Module{Funcs: []*Func{fn}}
	}

	t.Setenv("VEYL_NOOPT", "1")
	m := build()
	Optimize(m)
	if m.Funcs[0].Code[2].Op != OpAdd {
		t.Fatal("VEYL_NOOPT did not disable folding")
	}

	t.Setenv("VEYL_NOOPT", "")
	t.Setenv("VEYL_NOFOLD", "1")
	m = build()
	Optimize(m)
	if m.Funcs[0].Code[2].Op != OpAdd {
		t.Fatal("VEYL_NOFOLD did not disable folding")
	}
}
