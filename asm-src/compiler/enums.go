package main

// Enums: a fixed set of named variants.
//
//	enum State { Idle, Running, Done }
//	let s = State.Running
//
// A value is its variant's position, an int, so comparing two is one
// integer compare and a match on one is a chain of them. What makes it
// an enum rather than an int is the checker - no arithmetic, no mixing
// with ints or with another enum - and the name its vty carries here,
// which is how printing knows to write "Running" rather than 1.

// enumVariants holds the program's enums while it is being lowered.
// vtyOf is a plain function of a checked type, and a type annotation
// such as `s: State` reaches it as a name the parser could not tell
// from a struct's, so this is where it looks.
var enumVariants = map[string][]string{}

func vEnumOf(name string) vty { return vty{k: kInt, name: name} }

func isEnum(t vty) bool { return t.k == kInt && t.name != "" && !t.res && !t.null }

// enumNamed is the enum an expression names as a type, as in the State
// of State.Idle, or "" when it is a variable.
func (l *lowerer) enumNamed(e Expr) string {
	id, ok := e.(*Ident)
	if !ok {
		return ""
	}
	if _, local := l.lookup(id.Name); local {
		return ""
	}
	if _, ok := enumVariants[id.Name]; ok {
		return id.Name
	}
	return ""
}

// enumValue lowers State.Running.
func (l *lowerer) enumValue(x *Field, name string) Reg {
	for i, v := range enumVariants[name] {
		if v == x.Name {
			d := l.constant(int64(i))
			l.regTy[d] = vEnumOf(name)
			return d
		}
	}
	l.errorAt(x, "%s has no variant %q", name, x.Name)
	return l.junk()
}

// enumName is the name of the variant a value holds, as a string.
func (l *lowerer) enumName(v Reg) Reg {
	variants := enumVariants[l.regTy[v].name]
	out := l.temp(vStr)
	l.emit(Instr{Op: OpStore, A: l.strLit("?"), Dst: NoReg, Imm: out})
	done := l.newLabel()
	for i, name := range variants {
		next := l.newLabel()
		l.emit(Instr{Op: OpJumpNot, A: l.compare(OpEq, v, l.constant(int64(i))), Dst: NoReg, Imm: next})
		l.emit(Instr{Op: OpStore, A: l.strLit(name), Dst: NoReg, Imm: out})
		l.emit(Instr{Op: OpJump, A: NoReg, Dst: NoReg, Imm: done})
		l.mark(next)
	}
	l.mark(done)
	return l.load(out, vStr)
}
