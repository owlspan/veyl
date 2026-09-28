package main

// defer.
//
//	fn save(path: str) {
//	    let m = lockFor(path)
//	    thread.lock(m)
//	    defer thread.unlock(m)
//	    ...
//	}
//
// A deferred statement runs when the block it is written in is left,
// however it is left: its last statement, a return, or a break or
// continue out of a loop inside the function. Several run in the
// opposite order to how they were written. It is Zig's and Swift's
// defer, tied to the block rather than to the function as Go's is,
// which is what makes `defer delete(xs)` inside a loop body free each
// trip's list on that trip.
//
// It is done entirely while lowering. Each block keeps the statements
// it has deferred so far, and each way out lowers them again, in place:
// the end of the block runs its own, a return runs every open block's,
// and a break or continue runs those opened inside the loop. Only what
// has been reached is deferred - one in an if that was not taken is
// never recorded on that path, because the recording happens where the
// defer is lowered, not where it is written.
//
// The statement is evaluated when it runs, not when the defer is
// reached: `defer print(x)` prints x as it is on the way out.

// stmtList lowers a block's statements, then what it deferred.
func (l *lowerer) stmtList(list []Stmt) {
	l.defers = append(l.defers, nil)
	for _, st := range list {
		l.stmt(st)
	}
	top := l.defers[len(l.defers)-1]
	l.defers = l.defers[:len(l.defers)-1]
	for i := len(top) - 1; i >= 0; i-- {
		l.stmt(top[i])
	}
}

// runDefers lowers every deferred statement in blocks opened after the
// first `from`, innermost block first and latest first within one, for
// a return or a jump out of them. The blocks stay open: other paths
// through them still need their statements.
func (l *lowerer) runDefers(from int) {
	if !l.hasDefers(from) {
		return
	}
	saved := l.defers
	for b := len(saved) - 1; b >= from; b-- {
		// While a block's statements run, only the blocks outside it
		// are open, so nothing inside them sees itself as pending.
		l.defers = saved[:b]
		for i := len(saved[b]) - 1; i >= 0; i-- {
			l.stmt(saved[b][i])
		}
	}
	l.defers = saved
}

func (l *lowerer) hasDefers(from int) bool {
	for b := from; b < len(l.defers); b++ {
		if len(l.defers[b]) > 0 {
			return true
		}
	}
	return false
}
