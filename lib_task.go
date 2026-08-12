package main

// The task library: doing several things at once.
//
// Quartz deliberately does not expose goroutines and channels. It has
// no mutexes, no atomics and no way to talk about ownership, so raw
// shared-memory concurrency would be the one place in the language
// where the compiler stops helping - every other sharp edge here is
// either checked (nil, bounds, types) or removed (map iteration order).
//
// What it exposes instead is structured: work is handed out, results
// come back, and everything has finished by the time the call returns.
// There is no way to leak a task that outlives the statement that
// started it.
//
// The one thing it cannot check is what your function touches. A
// function passed to task.map runs on several threads at once, so it
// should compute a value from its argument rather than change anything
// outside itself. That is documented rather than enforced, and it is
// the honest limit of this design.

var taskHelperDefs = map[string]helperDef{
	"parMap": {
		// Each worker writes to its own index, so the results need no
		// lock and come back in the order they went in - which is what
		// makes this a drop-in replacement for map().
		code: `func __parMap[T any, U any](xs []T, f func(T) U, limit int) []U {
	out := make([]U, len(xs))
	if len(xs) == 0 {
		return out
	}
	if limit <= 0 {
		limit = runtime.NumCPU()
	}
	if limit > len(xs) {
		limit = len(xs)
	}

	work := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < limit; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				out[i] = f(xs[i])
			}
		}()
	}
	for i := range xs {
		work <- i
	}
	close(work)
	wg.Wait()
	return out
}`,
		imports: []string{"runtime", "sync"},
	},

	"parAll": {
		code: `func __parAll(fns []func()) {
	var wg sync.WaitGroup
	for _, f := range fns {
		wg.Add(1)
		go func(f func()) {
			defer wg.Done()
			f()
		}(f)
	}
	wg.Wait()
}`,
		imports: []string{"sync"},
	},
}

var taskBuiltins map[string]builtin

func buildTaskBuiltins() {
	taskBuiltins = map[string]builtin{

		// task.map is map(), run concurrently. Same arguments, same
		// ordered results, so switching between them is a one-word edit.
		"task.map": {
			minArgs: 2, maxArgs: 2,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				elem := wantList(c, x, 0, args, "task.map")
				ret := wantCallback(c, x, 1, args, "task.map", []*Type{elem})
				if elem.IsUnknown() || ret.IsUnknown() {
					return Unknown
				}
				if ret.Kind == KVoid {
					c.errorAt(x.Args[1], "task.map needs a function that returns something - "+
						"use task.each(...) to just do work")
					return Unknown
				}
				return ListOf(ret)
			},
			helpers: []string{"parMap"},
			emit:    func(a []string) string { return "__parMap(" + a[0] + ", " + a[1] + ", 0)" },
		},

		// The same, with a cap on how many run at once. Worth having for
		// work that is rate-limited rather than CPU-bound - twenty
		// concurrent HTTP requests is friendly, two thousand is not.
		"task.mapLimit": {
			minArgs: 3, maxArgs: 3,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				elem := wantList(c, x, 0, args, "task.mapLimit")
				matches(c, x, 1, Int, "task.mapLimit")
				ret := wantCallback(c, x, 2, args, "task.mapLimit", []*Type{elem})
				if elem.IsUnknown() || ret.IsUnknown() {
					return Unknown
				}
				if ret.Kind == KVoid {
					c.errorAt(x.Args[2], "task.mapLimit needs a function that returns something")
					return Unknown
				}
				return ListOf(ret)
			},
			helpers: []string{"parMap"},
			emit: func(a []string) string {
				return "__parMap(" + a[0] + ", " + a[2] + ", " + a[1] + ")"
			},
		},

		"task.each": {
			minArgs: 2, maxArgs: 2,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				elem := wantList(c, x, 0, args, "task.each")
				wantCallback(c, x, 1, args, "task.each", []*Type{elem})
				return Void
			},
			helpers: []string{"parMap"},
			emitT: func(c *Codegen, x *Call, a []string) string {
				// Reuse the mapper by giving the function a result to
				// return, rather than writing a second worker pool.
				elem := "any"
				if len(x.ArgT) > 0 && x.ArgT[0].Kind == KList {
					elem = x.ArgT[0].Elem.Go()
				}
				return "__parMap(" + a[0] + ", func(__v " + elem + ") bool { " +
					a[1] + "(__v); return true }, 0)"
			},
		},

		// task.all runs a list of no-argument functions at once and
		// returns when the last of them is done.
		"task.all": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				elem := wantList(c, x, 0, args, "task.all")
				if elem.IsUnknown() {
					return Void
				}
				if !elem.IsFunc() || len(elem.Params) != 0 || elem.Elem.Kind != KVoid {
					c.errorAt(x.Args[0], "task.all expects a list of fn() taking nothing "+
						"and returning nothing, got []%s", elem)
				}
				return Void
			},
			helpers: []string{"parAll"},
			emit:    func(a []string) string { return "__parAll(" + a[0] + ")" },
		},
	}
}

func registerTask() {
	buildTaskBuiltins()
	registerNamespace("task")
	for k, v := range taskHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range taskBuiltins {
		builtins[k] = v
	}
}
