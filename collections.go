package main

import "fmt"

// The list and map half of the standard library.
//
// These builtins are polymorphic over the element type, which a fixed
// params/ret signature cannot express, so most of them supply their own
// `check` and `emitT` instead. The generated Go leans on generics, so
// there is exactly one helper per operation rather than one per type.

var collectionHelperDefs = map[string]helperDef{
	// A Quartz program should never show the user a Go panic and a stack
	// trace through generated code they did not write. Out-of-range
	// access exits with a plain sentence instead.
	"qzBounds": {
		code: `func __bounds(i int, n int) {
	fmt.Fprintf(os.Stderr, "runtime error: index %d is out of range for a list of length %d\n", i, n)
	os.Exit(1)
}`,
		imports: []string{"fmt", "os"},
	},
	"listGet": {
		code: `func __listGet[T any](xs []T, i int) T {
	if i < 0 || i >= len(xs) {
		__bounds(i, len(xs))
	}
	return xs[i]
}`,
		deps: []string{"qzBounds"},
	},
	// __listAt is __listGet that hands back the element's address rather
	// than a copy, so a method called on xs[i] can change the element in
	// place while still being bounds-checked.
	// Boxing a value into a nullable. Go cannot take the address of an
	// arbitrary expression, but it can of a parameter.
	"ptr": {
		code: `func __ptr[T any](v T) *T { return &v }`,
	},

	// The result type. A failure carries a reason rather than just a
	// flag, because "it failed" on its own is never enough to act on.
	"result": {
		code: `type __Res[T any] struct {
	v T
	e string
}

func __ok[T any](v T) __Res[T]        { return __Res[T]{v: v} }
func __fail[T any](e string) __Res[T] { return __Res[T]{e: e} }
func __isOk[T any](r __Res[T]) bool   { return r.e == "" }
func __errOf[T any](r __Res[T]) string { return r.e }

func __valueOr[T any](r __Res[T], alt T) T {
	if r.e != "" {
		return alt
	}
	return r.v
}

func __must[T any](r __Res[T]) T {
	if r.e != "" {
		fmt.Fprintln(os.Stderr, "runtime error: "+r.e)
		os.Exit(1)
	}
	return r.v
}`,
		imports: []string{"fmt", "os"},
	},
	"listAt": {
		code: `func __listAt[T any](xs []T, i int) *T {
	if i < 0 || i >= len(xs) {
		__bounds(i, len(xs))
	}
	return &xs[i]
}`,
		deps: []string{"qzBounds"},
	},
	"listSet": {
		// Assigning through a slice header mutates the shared backing
		// array, so this needs no pointer.
		code: `func __listSet[T any](xs []T, i int, v T) {
	if i < 0 || i >= len(xs) {
		__bounds(i, len(xs))
	}
	xs[i] = v
}`,
		deps: []string{"qzBounds"},
	},
	// push, pop and clear change the slice header itself, so they take a
	// pointer to it. Codegen passes &xs, which is why their first
	// argument must be assignable.
	"listPush": {
		code: `func __push[T any](xs *[]T, vs ...T) {
	*xs = append(*xs, vs...)
}`,
	},
	"listPop": {
		code: `func __pop[T any](xs *[]T) T {
	if len(*xs) == 0 {
		fmt.Fprintln(os.Stderr, "runtime error: pop from an empty list")
		os.Exit(1)
	}
	last := (*xs)[len(*xs)-1]
	*xs = (*xs)[:len(*xs)-1]
	return last
}`,
		imports: []string{"fmt", "os"},
	},
	"listClear": {
		code: `func __clearList[T any](xs *[]T) {
	*xs = (*xs)[:0]
}`,
	},
	"listInsert": {
		code: `func __insert[T any](xs *[]T, i int, v T) {
	if i < 0 || i > len(*xs) {
		__bounds(i, len(*xs))
	}
	*xs = append(*xs, v)
	copy((*xs)[i+1:], (*xs)[i:])
	(*xs)[i] = v
}`,
		deps: []string{"qzBounds"},
	},
	"listRemove": {
		code: `func __removeAt[T any](xs *[]T, i int) T {
	if i < 0 || i >= len(*xs) {
		__bounds(i, len(*xs))
	}
	v := (*xs)[i]
	*xs = append((*xs)[:i], (*xs)[i+1:]...)
	return v
}`,
		deps: []string{"qzBounds"},
	},
	"listSlice": {
		// Indexes are clamped rather than fatal, matching substr(), and a
		// copy is returned so the result does not alias the original.
		code: `func __slice[T any](xs []T, start int, end int) []T {
	if start < 0 {
		start = 0
	}
	if end > len(xs) {
		end = len(xs)
	}
	if start >= end {
		return []T{}
	}
	out := make([]T, end-start)
	copy(out, xs[start:end])
	return out
}`,
	},
	"listFirst": {
		code: `func __first[T any](xs []T) T {
	if len(xs) == 0 {
		fmt.Fprintln(os.Stderr, "runtime error: first() on an empty list")
		os.Exit(1)
	}
	return xs[0]
}`,
		imports: []string{"fmt", "os"},
	},
	"listLast": {
		code: `func __last[T any](xs []T) T {
	if len(xs) == 0 {
		fmt.Fprintln(os.Stderr, "runtime error: last() on an empty list")
		os.Exit(1)
	}
	return xs[len(xs)-1]
}`,
		imports: []string{"fmt", "os"},
	},
	"listReverse": {
		code: `func __reverse[T any](xs []T) []T {
	out := make([]T, len(xs))
	for i, v := range xs {
		out[len(xs)-1-i] = v
	}
	return out
}`,
	},
	"listSort": {
		code: `func __sorted[T cmp.Ordered](xs []T) []T {
	out := make([]T, len(xs))
	copy(out, xs)
	slices.Sort(out)
	return out
}`,
		imports: []string{"cmp", "slices"},
	},
	"listJoin": {
		code: `func __join[T any](xs []T, sep string) string {
	parts := make([]string, len(xs))
	for i, v := range xs {
		parts[i] = fmt.Sprintf("%v", v)
	}
	return strings.Join(parts, sep)
}`,
		imports: []string{"fmt", "strings"},
	},
	"listSum": {
		code: `func __sum[T int | float64](xs []T) T {
	var total T
	for _, v := range xs {
		total += v
	}
	return total
}`,
	},
	"mapKeys": {
		// Sorted, for the same reason map iteration is sorted: a program
		// whose output changes between runs is a bad first experience.
		code: `func __keys[K cmp.Ordered, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}`,
		imports: []string{"cmp", "slices"},
	},
	"mapFind": {
		code: `func __find[K comparable, V any](m map[K]V, k K) *V {
	v, ok := m[k]
	if !ok {
		return nil
	}
	return &v
}`,
	},
	"mapValues": {
		code: `func __values[K cmp.Ordered, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, k := range __keys(m) {
		out = append(out, m[k])
	}
	return out
}`,
		deps: []string{"mapKeys"},
	},
	// show renders a value the way Quartz spells it, not the way Go does:
	// [1, 2, 3] rather than [1 2 3], and {"a": 1} rather than map[a:1].
	// A printed value should be something the user could paste back into
	// their program.
	//
	// Reflection rather than generics, because nesting has to recurse
	// through element types that are not known statically at this depth.
	// It only ever runs on a value being printed, so the cost is
	// irrelevant.
	"show": {
		// Everything walks reflect.Value rather than `any`, because
		// Quartz field names are lower case and therefore unexported in
		// the generated Go. reflect refuses .Interface() on an unexported
		// field, but reading the Value itself is fine, and fmt prints a
		// reflect.Value as the value it holds.
		code: `func __show(v any) string { return __showV(reflect.ValueOf(v)) }

func __showV(rv reflect.Value) string {
	if !rv.IsValid() {
		return "nil"
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		parts := make([]string, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			parts[i] = __showInner(rv.Index(i))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case reflect.Map:
		keys := rv.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].Kind() == reflect.Int {
				return keys[i].Int() < keys[j].Int()
			}
			return keys[i].String() < keys[j].String()
		})
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = __showInner(k) + ": " + __showInner(rv.MapIndex(k))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case reflect.Struct:
		t := rv.Type()
		parts := make([]string, rv.NumField())
		for i := 0; i < rv.NumField(); i++ {
			// The json tag holds the name as it was written in Quartz;
			// the Go field name carries a prefix to make it exported.
			name := t.Field(i).Tag.Get("json")
			if name == "" {
				name = t.Field(i).Name
			}
			parts[i] = name + ": " + __showInner(rv.Field(i))
		}
		return t.Name() + "{" + strings.Join(parts, ", ") + "}"
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return "nil"
		}
		return __showV(rv.Elem())
	}
	return fmt.Sprintf("%v", rv)
}

// __showInner quotes strings, so a list of words reads as ["a", "b"]
// rather than [a, b] and an empty string is visible.
func __showInner(rv reflect.Value) string {
	if rv.Kind() == reflect.String {
		return strconv.Quote(rv.String())
	}
	return __showV(rv)
}`,
		imports: []string{"fmt", "reflect", "sort", "strconv", "strings"},
	},

	"chars": {
		code: `func __chars(s string) []string {
	rs := []rune(s)
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}`,
	},
}

// ---- helpers for writing polymorphic builtins ----

// wantList reports an error unless the argument is a list, and returns
// its element type.
func wantList(c *Checker, x *Call, i int, args []*Type, fn string) *Type {
	if i >= len(args) {
		return Unknown
	}
	t := args[i]
	if t.IsUnknown() {
		return Unknown
	}
	if t.Kind != KList {
		c.errorAt(x.Args[i], "%s expects a list, got %s", fn, t)
		return Unknown
	}
	return t.Elem
}

// wantResult reports an error unless the first argument is a result,
// and returns the type it carries.
func wantResult(c *Checker, x *Call, args []*Type, fn string) *Type {
	if len(args) == 0 || args[0].IsUnknown() {
		return Unknown
	}
	if !args[0].IsResult() {
		c.errorAt(x.Args[0], "%s expects a value that can fail, and %s cannot", fn, args[0])
		return Unknown
	}
	return args[0].Elem
}

// wantAssignable reports an error unless the argument is something that
// can be written to. push, pop and clear replace the slice header, so
// they need a variable or an element, not a temporary.
func wantAssignable(c *Checker, x *Call, i int, fn string) {
	if i >= len(x.Args) {
		return
	}
	switch x.Args[i].(type) {
	case *Ident, *Index, *Field:
	default:
		c.errorAt(x.Args[i], "%s changes the list, so its first argument must be a variable, a field, or an element", fn)
	}
}

// matches checks a value against an element type, allowing the same
// untyped-integer-literal flexibility as everywhere else.
func matches(c *Checker, x *Call, i int, want *Type, fn string) {
	if i >= len(x.ArgT) || want.IsUnknown() {
		return
	}
	got := x.ArgT[i]
	if want.Accepts(got) || (isUntypedInt(x.Args[i]) && want.Kind == KFloat) {
		return
	}
	c.errorAt(x.Args[i], "%s expects %s here, got %s", fn, want, got)
}

var collectionBuiltins map[string]builtin

func buildCollectionBuiltins() {
	collectionBuiltins = map[string]builtin{

		// ---- growing and shrinking ----

		"push": {
			minArgs: 2, maxArgs: -1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				elem := wantList(c, x, 0, args, "push")
				wantAssignable(c, x, 0, "push")
				for i := 1; i < len(x.Args); i++ {
					matches(c, x, i, elem, "push")
				}
				return Void
			},
			helpers: []string{"listPush"},
			emitT: func(c *Codegen, x *Call, a []string) string {
				return c.mutate(x.Args[0], Void, func(ref string) string {
					return "__push(" + ref + ", " + joinFrom(a, 1) + ")"
				})
			},
		},

		"pop": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				wantAssignable(c, x, 0, "pop")
				return wantList(c, x, 0, args, "pop")
			},
			helpers: []string{"listPop"},
			emitT: func(c *Codegen, x *Call, a []string) string {
				return c.mutate(x.Args[0], x.T, func(ref string) string {
					return "__pop(" + ref + ")"
				})
			},
		},

		"insert": {
			minArgs: 3, maxArgs: 3,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				elem := wantList(c, x, 0, args, "insert")
				wantAssignable(c, x, 0, "insert")
				matches(c, x, 1, Int, "insert")
				matches(c, x, 2, elem, "insert")
				return Void
			},
			helpers: []string{"listInsert"},
			emitT: func(c *Codegen, x *Call, a []string) string {
				return c.mutate(x.Args[0], Void, func(ref string) string {
					return "__insert(" + ref + ", " + a[1] + ", " + a[2] + ")"
				})
			},
		},

		"removeAt": {
			minArgs: 2, maxArgs: 2,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				elem := wantList(c, x, 0, args, "removeAt")
				wantAssignable(c, x, 0, "removeAt")
				matches(c, x, 1, Int, "removeAt")
				return elem
			},
			helpers: []string{"listRemove"},
			emitT: func(c *Codegen, x *Call, a []string) string {
				return c.mutate(x.Args[0], x.T, func(ref string) string {
					return "__removeAt(" + ref + ", " + a[1] + ")"
				})
			},
		},

		"clear": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if len(args) == 1 && args[0].Kind == KMap {
					return Void
				}
				wantList(c, x, 0, args, "clear")
				wantAssignable(c, x, 0, "clear")
				return Void
			},
			helpers: []string{"listClear"},
			emitT: func(c *Codegen, x *Call, a []string) string {
				if len(x.ArgT) == 1 && x.ArgT[0].Kind == KMap {
					return "clear(" + a[0] + ")"
				}
				return c.mutate(x.Args[0], Void, func(ref string) string {
					return "__clearList(" + ref + ")"
				})
			},
		},

		// ---- reading ----

		"first": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				return wantList(c, x, 0, args, "first")
			},
			helpers: []string{"listFirst"},
			emit:    func(a []string) string { return "__first(" + a[0] + ")" },
		},

		"last": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				return wantList(c, x, 0, args, "last")
			},
			helpers: []string{"listLast"},
			emit:    func(a []string) string { return "__last(" + a[0] + ")" },
		},

		"slice": {
			minArgs: 3, maxArgs: 3,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				matches(c, x, 1, Int, "slice")
				matches(c, x, 2, Int, "slice")
				if len(args) > 0 && args[0].Kind == KList {
					return args[0]
				}
				wantList(c, x, 0, args, "slice")
				return Unknown
			},
			helpers: []string{"listSlice"},
			emit: func(a []string) string {
				return "__slice(" + a[0] + ", " + a[1] + ", " + a[2] + ")"
			},
		},

		"reverse": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if len(args) > 0 && args[0].Kind == KList {
					return args[0]
				}
				wantList(c, x, 0, args, "reverse")
				return Unknown
			},
			helpers: []string{"listReverse"},
			emit:    func(a []string) string { return "__reverse(" + a[0] + ")" },
		},

		"sort": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				elem := wantList(c, x, 0, args, "sort")
				if elem.IsUnknown() {
					return Unknown
				}
				if !elem.IsNumeric() && elem.Kind != KStr {
					c.errorAt(x.Args[0], "sort needs a list of numbers or strings, got %s", args[0])
					return Unknown
				}
				return args[0]
			},
			helpers: []string{"listSort"},
			emit:    func(a []string) string { return "__sorted(" + a[0] + ")" },
		},

		"sum": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				elem := wantList(c, x, 0, args, "sum")
				if elem.IsUnknown() {
					return Unknown
				}
				if !elem.IsNumeric() {
					c.errorAt(x.Args[0], "sum needs a list of numbers, got %s", args[0])
					return Unknown
				}
				return elem
			},
			helpers: []string{"listSum"},
			emit:    func(a []string) string { return "__sum(" + a[0] + ")" },
		},

		"join": {
			minArgs: 2, maxArgs: 2,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				wantList(c, x, 0, args, "join")
				matches(c, x, 1, Str, "join")
				return Str
			},
			helpers: []string{"listJoin"},
			emit:    func(a []string) string { return "__join(" + a[0] + ", " + a[1] + ")" },
		},

		// ---- results ----

		// fail produces a failure of whatever result type the context
		// wants, the way nil produces a nullable of whatever is wanted.
		"fail": {
			minArgs: 1, maxArgs: 1,
			wantsTarget: true,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				matches(c, x, 0, Str, "fail")
				return ErrLitT
			},
			helpers: []string{"result"},
			emitT: func(c *Codegen, x *Call, a []string) string {
				inner := "any"
				if x.Want != nil && x.Want.IsResult() {
					inner = x.Want.Elem.Go()
				}
				return "__fail[" + inner + "](" + a[0] + ")"
			},
		},

		"isOk": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				wantResult(c, x, args, "isOk")
				return Bool
			},
			helpers: []string{"result"},
			emit:    func(a []string) string { return "__isOk(" + a[0] + ")" },
		},

		"failed": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				wantResult(c, x, args, "failed")
				return Bool
			},
			helpers: []string{"result"},
			emit:    func(a []string) string { return "(!__isOk(" + a[0] + "))" },
		},

		"errorOf": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				wantResult(c, x, args, "errorOf")
				return Str
			},
			helpers: []string{"result"},
			emit:    func(a []string) string { return "__errOf(" + a[0] + ")" },
		},

		"valueOr": {
			minArgs: 2, maxArgs: 2,
			// The fallback has to be the same type the result carries, so
			// `valueOr(load(), [])` can tell what kind of list to build.
			wantsTarget: true,
			hintFor: func(x *Call, known []*Type, i int) *Type {
				// The whole call's expected type tells the argument what
				// kind of result to produce, which is what carries an
				// annotation through into json.decode.
				if i == 0 && x.Want != nil && !x.Want.IsUnknown() {
					return ResultOf(x.Want)
				}
				if i == 1 && len(known) == 1 && known[0].IsResult() {
					return known[0].Elem
				}
				return nil
			},
			check: func(c *Checker, x *Call, args []*Type) *Type {
				inner := wantResult(c, x, args, "valueOr")
				matches(c, x, 1, inner, "valueOr")
				return inner
			},
			helpers: []string{"result"},
			emit:    func(a []string) string { return "__valueOr(" + a[0] + ", " + a[1] + ")" },
		},

		// must is the escape hatch: take the value, or stop the program
		// with the reason. Explicit, unlike a fatal library call.
		"must": {
			minArgs: 1, maxArgs: 1,
			wantsTarget: true,
			hintFor: func(x *Call, known []*Type, i int) *Type {
				if i == 0 && x.Want != nil && !x.Want.IsUnknown() {
					return ResultOf(x.Want)
				}
				return nil
			},
			check: func(c *Checker, x *Call, args []*Type) *Type {
				return wantResult(c, x, args, "must")
			},
			helpers: []string{"result"},
			emit:    func(a []string) string { return "__must(" + a[0] + ")" },
		},

		// ---- maps ----

		"has": {
			minArgs: 2, maxArgs: 2,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if len(args) < 2 || args[0].IsUnknown() {
					return Bool
				}
				if args[0].Kind != KMap {
					c.errorAt(x.Args[0], "has expects a map, got %s", args[0])
					return Bool
				}
				matches(c, x, 1, args[0].Key, "has")
				return Bool
			},
			emitT: func(c *Codegen, x *Call, a []string) string {
				return "func() bool { _, ok := " + a[0] + "[" + a[1] + "]; return ok }()"
			},
		},

		"remove": {
			minArgs: 2, maxArgs: 2,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if len(args) < 2 || args[0].IsUnknown() {
					return Void
				}
				if args[0].Kind != KMap {
					c.errorAt(x.Args[0], "remove expects a map, got %s (use removeAt for a list)", args[0])
					return Void
				}
				matches(c, x, 1, args[0].Key, "remove")
				return Void
			},
			emit: func(a []string) string { return "delete(" + a[0] + ", " + a[1] + ")" },
		},

		// find is the nil-safe counterpart to m[k]. A bare index gives the
		// zero value for a missing key, which is usually what you want and
		// occasionally a silent bug; this returns ?V so the difference
		// between "absent" and "zero" survives.
		"find": {
			minArgs: 2, maxArgs: 2,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if len(args) < 2 || args[0].IsUnknown() {
					return Unknown
				}
				if args[0].Kind != KMap {
					c.errorAt(x.Args[0], "find expects a map, got %s", args[0])
					return Unknown
				}
				matches(c, x, 1, args[0].Key, "find")
				return NullableOf(args[0].Elem)
			},
			helpers: []string{"mapFind"},
			emit:    func(a []string) string { return "__find(" + a[0] + ", " + a[1] + ")" },
		},

		"keys": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if len(args) < 1 || args[0].IsUnknown() {
					return Unknown
				}
				if args[0].Kind != KMap {
					c.errorAt(x.Args[0], "keys expects a map, got %s", args[0])
					return Unknown
				}
				return ListOf(args[0].Key)
			},
			helpers: []string{"mapKeys"},
			emit:    func(a []string) string { return "__keys(" + a[0] + ")" },
		},

		"values": {
			minArgs: 1, maxArgs: 1,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				if len(args) < 1 || args[0].IsUnknown() {
					return Unknown
				}
				if args[0].Kind != KMap {
					c.errorAt(x.Args[0], "values expects a map, got %s", args[0])
					return Unknown
				}
				return ListOf(args[0].Elem)
			},
			helpers: []string{"mapValues"},
			emit:    func(a []string) string { return "__values(" + a[0] + ")" },
		},

		// ---- strings to lists and back ----

		"split": {
			minArgs: 2, maxArgs: 2,
			params: []*Type{Str, Str}, ret: ListOf(Str),
			imports: []string{"strings"},
			emit:    func(a []string) string { return "strings.Split(" + a[0] + ", " + a[1] + ")" },
		},

		"chars": {
			minArgs: 1, maxArgs: 1,
			params: []*Type{Str}, ret: ListOf(Str),
			helpers: []string{"chars"},
			emit:    func(a []string) string { return "__chars(" + a[0] + ")" },
		},

		"lines": {
			minArgs: 1, maxArgs: 1,
			params: []*Type{Str}, ret: ListOf(Str),
			imports: []string{"strings"},
			emit: func(a []string) string {
				return `strings.Split(strings.ReplaceAll(` + a[0] + `, "\r\n", "\n"), "\n")`
			},
		},
	}
}

// joinFrom joins arguments from index i onward.
func joinFrom(a []string, i int) string {
	out := ""
	for j := i; j < len(a); j++ {
		if j > i {
			out += ", "
		}
		out += a[j]
	}
	return out
}

// contains and indexOf already exist for strings. Extend them to lists
// rather than inventing a second name for the same idea.
func overloadForCollections() {
	strContains := builtins["contains"]
	builtins["contains"] = builtin{
		minArgs: 2, maxArgs: 2,
		check: func(c *Checker, x *Call, args []*Type) *Type {
			if len(args) < 2 || args[0].IsUnknown() {
				return Bool
			}
			switch args[0].Kind {
			case KStr:
				matches(c, x, 1, Str, "contains")
			case KList:
				matches(c, x, 1, args[0].Elem, "contains")
			case KMap:
				c.errorAt(x.Args[0], "use has(map, key) to test a map")
			default:
				c.errorAt(x.Args[0], "contains expects a str or a list, got %s", args[0])
			}
			return Bool
		},
		imports: strContains.imports,
		emitT: func(c *Codegen, x *Call, a []string) string {
			if len(x.ArgT) > 0 && x.ArgT[0].Kind == KList {
				c.imports["slices"] = true
				return "slices.Contains(" + a[0] + ", " + a[1] + ")"
			}
			c.imports["strings"] = true
			return "strings.Contains(" + a[0] + ", " + a[1] + ")"
		},
	}

	builtins["indexOf"] = builtin{
		minArgs: 2, maxArgs: 2,
		check: func(c *Checker, x *Call, args []*Type) *Type {
			if len(args) < 2 || args[0].IsUnknown() {
				return Int
			}
			switch args[0].Kind {
			case KStr:
				matches(c, x, 1, Str, "indexOf")
			case KList:
				matches(c, x, 1, args[0].Elem, "indexOf")
			default:
				c.errorAt(x.Args[0], "indexOf expects a str or a list, got %s", args[0])
			}
			return Int
		},
		emitT: func(c *Codegen, x *Call, a []string) string {
			if len(x.ArgT) > 0 && x.ArgT[0].Kind == KList {
				c.imports["slices"] = true
				return "slices.Index(" + a[0] + ", " + a[1] + ")"
			}
			c.imports["strings"] = true
			return "strings.Index(" + a[0] + ", " + a[1] + ")"
		},
	}

	// len already accepts anything; make its error message useful when it
	// is handed something with no length.
	prevLen := builtins["len"]
	builtins["len"] = builtin{
		minArgs: 1, maxArgs: 1,
		check: func(c *Checker, x *Call, args []*Type) *Type {
			if len(args) == 1 && !args[0].IsUnknown() {
				switch args[0].Kind {
				case KStr, KList, KMap:
				default:
					c.errorAt(x.Args[0], "len expects a str, list or map, got %s", args[0])
				}
			}
			return Int
		},
		emit: prevLen.emit,
	}
}

// registerCollections folds the list and map library into the core
// tables. Called from codegen's init(), after the stdlib, because it
// deliberately overwrites contains, indexOf and len.
func registerCollections() {
	buildCollectionBuiltins()
	for k, v := range collectionHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range collectionBuiltins {
		if _, taken := builtins[k]; taken {
			panic(fmt.Sprintf("collection builtin %q collides with an existing one", k))
		}
		builtins[k] = v
	}
	overloadForCollections()
}
