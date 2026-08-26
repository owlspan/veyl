package main

// The register allocator.
//
// A handful of virtual registers get machine registers instead of frame
// slots, so that arithmetic between two barriers runs in registers
// rather than through memory. Which handful is decided by three rules,
// each of which exists because breaking it breaks something quiet:
//
//   - Only non-pointers. The collector finds roots by scanning stack
//     slots. A pointer parked in a register during an allocation is a
//     word the scan never sees, and the object behind it can be freed
//     while the program still holds it. Types come from the lowerer;
//     anything whose type says it holds a pointer keeps its slot.
//
//   - No interval crosses a barrier: a call, direct or through a
//     closure, or one of the helpers that allocates or can abort. The
//     calling convention lets the callee take every volatile register,
//     and every barrier can allocate, which is rule one all over again.
//     A value whose whole life fits between two barriers needs nothing
//     saved around them; one that lives across does not get a register
//     anywhere.
//
//   - One home per value, and no span crosses a label. The IR gives
//     every virtual register exactly one instruction that writes it,
//     which makes plain first-to-last spans tempting: unlike a frame
//     slot, a register here is written once, so no second write can
//     run ahead of a read. What single assignment does not stop is
//     re-entry: a value defined outside a loop and read inside it dies
//     textually at its last mention, but the back edge comes around
//     and reads it again after some other value has taken the
//     register. Every wrap re-enters through a label, so a span with
//     no label inside it cannot be re-entered either, and within one
//     straight run the linear span is exact.
//
// Floats are not pooled, though xmm4 and xmm5 sit idle for the taking.
// Reading a pooled float sometimes means getting its raw bits into a
// general register - storing it to a slot, passing it beyond the fourth
// argument position - and that move is movq, which the byte writer does
// not encode yet. An integer home works everywhere a slot works, which
// is the property that keeps this pass small.

import "sort"

var raIntRegs = [4]string{"r8", "r9", "r10", "r11"}

// raBarrier reports whether this op calls code the emitter does not see.
// Every one of these can allocate, and every one clobbers the volatile
// registers the pool draws from.
func raBarrier(in *Instr) bool {
	switch in.Op {
	case OpCall, OpCallClosure,
		OpConcat, OpStrEq, OpStrLen, OpIntToStr, OpFloatToStr,
		OpAlloc, OpFMod,
		OpPrintInt, OpPrintFloat, OpPrintStr, OpPrintBool,
		OpWriteStr, OpWriteInt, OpWriteFloat,
		OpBoundsFail, OpMustFail:
		return true
	}
	return false
}

// allocateRegs returns one home per pooled virtual register: a machine
// register for the few that qualify, absence for everything else. A nil
// map means the function was not suited to allocation and every read
// goes to its slot, as it would with the pass off.
func allocateRegs(f *Func) map[Reg]string {
	if len(f.RegTypes) < f.NRegs || f.NRegs == 0 {
		return nil
	}

	// First and last mention of every register, and where the barriers
	// sit. Mentions outside the register range cannot exist, but a
	// defensive bound costs one comparison.
	defOf := make([]int, f.NRegs)
	lastOf := make([]int, f.NRegs)
	barrier := make([]bool, len(f.Code))
	for i := range defOf {
		defOf[i] = -1
	}
	note := func(r Reg, pos int) {
		if r >= 0 && int(r) < f.NRegs {
			if defOf[r] < 0 {
				defOf[r] = pos
			}
			lastOf[r] = pos
		}
	}
	for i := range f.Code {
		in := &f.Code[i]
		if raBarrier(in) {
			barrier[i] = true
		}
		note(in.Dst, i)
		note(in.A, i)
		note(in.B, i)
		for _, a := range in.Args {
			note(a, i)
		}
	}

	type cand struct {
		r     Reg
		def   int
		last  int
		param bool // arrives in a fixed argument register; not worth moving
	}
	var cands []cand
	for r := 0; r < f.NRegs; r++ {
		t := f.RegTypes[r]
		if t.holdsPointer() || t.k == kFloat {
			continue
		}
		def := defOf[r]
		if def < 0 {
			continue
		}
		// A result carried out of a call arrives after every volatile
		// register has been taken; its slot is written by callResult
		// directly and that is where readers should look.
		if barrier[def] {
			continue
		}
		last := lastOf[r]
		crossed := false
		for p := def + 1; p < last; p++ {
			if barrier[p] || f.Code[p].Op == OpLabel {
				crossed = true
				break
			}
		}
		if crossed {
			continue
		}
		cands = append(cands, cand{
			r: Reg(r), def: def, last: last,
			param: f.Code[def].Op == OpParam,
		})
	}
	if len(cands) == 0 {
		return nil
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].def != cands[j].def {
			return cands[i].def < cands[j].def
		}
		return cands[i].r < cands[j].r
	})

	// Linear scan. A register whose holder dies before or at this
	// definition point is free again: both mentions sit in the same
	// instruction, the read happens before the write, and every shape
	// the emitter produces reads its inputs first.
	type held struct {
		until int
		home  string
	}
	var busy []held
	homes := make(map[Reg]string)
next:
	for _, c := range cands {
		if c.param {
			continue // parameters arrive in fixed registers; moving them buys nothing
		}
	free:
		for _, name := range raIntRegs[:] {
			for _, b := range busy {
				if b.home == name && b.until >= c.def {
					continue free
				}
			}
			homes[c.r] = name
			busy = append(busy, held{until: c.last, home: name})
			continue next
		}
		// Nothing free. The value keeps its slot; nothing already
		// holding a register is evicted over it.
	}
	if len(homes) == 0 {
		return nil
	}
	return homes
}
