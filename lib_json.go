package main

// The json library.
//
// Two ways to use it, because two things are actually wanted:
//
//   - Whole-value conversion. json.encode turns any Quartz value into
//     text, and json.decode turns text back into a declared type. The
//     type comes from the annotation on the binding, since Quartz has no
//     type arguments:
//
//         let p: Point = json.decode(text)
//
//   - Poking at one field without declaring anything, for the common
//     case of pulling a single value out of an API response:
//
//         let name = json.get(body, "user.name")
//
// Struct fields carry a json tag holding the name as written in Quartz,
// so encoding round-trips through the spelling the user chose rather
// than the exported one the Go backend needs.

var jsonHelperDefs = map[string]helperDef{
	"jsonEncode": {
		code: `func __jsonEncode(v any, indent bool) string {
	var b []byte
	var err error
	if indent {
		b, err = json.MarshalIndent(v, "", "  ")
	} else {
		b, err = json.Marshal(v)
	}
	if err != nil {
		__fatal("encode", fmt.Sprintf("%T", v), err)
	}
	return string(b)
}`,
		imports: []string{"encoding/json", "fmt"},
		deps:    []string{"qzFatal"},
	},

	"jsonDecode": {
		// Fatal on malformed input, matching os.file.read. decodeOr is
		// the variant for input that is expected to be untrustworthy.
		code: `func __jsonDecode[T any](s string) T {
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		__fatal("decode", __jsonSnippet(s), err)
	}
	return v
}

func __jsonDecodeOr[T any](s string, fallback T) T {
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return fallback
	}
	return v
}

// A parse error should quote the offending text, but not all of it —
// a 40kB response in an error message helps nobody.
func __jsonSnippet(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}`,
		imports: []string{"encoding/json"},
		deps:    []string{"qzFatal"},
	},

	"jsonPath": {
		// Walks a dotted path, stepping into objects by key and arrays by
		// number, so "users.0.name" works.
		code: `func __jsonWalk(text string, path string) (any, bool) {
	var root any
	if err := json.Unmarshal([]byte(text), &root); err != nil {
		return nil, false
	}
	cur := root
	if path == "" {
		return cur, true
	}
	for _, step := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[step]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(step)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			cur = node[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

func __jsonGet(text string, path string) string {
	v, ok := __jsonWalk(text, path)
	if !ok || v == nil {
		return ""
	}
	if s, isStr := v.(string); isStr {
		return s
	}
	// Anything that is not a string comes back as the JSON that
	// produced it, so nested objects survive being fetched.
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func __jsonNum(text string, path string) float64 {
	v, ok := __jsonWalk(text, path)
	if !ok {
		return 0
	}
	if f, isNum := v.(float64); isNum {
		return f
	}
	return 0
}

func __jsonBool(text string, path string) bool {
	v, ok := __jsonWalk(text, path)
	if !ok {
		return false
	}
	b, isBool := v.(bool)
	return isBool && b
}

func __jsonHas(text string, path string) bool {
	_, ok := __jsonWalk(text, path)
	return ok
}

func __jsonKeys(text string, path string) []string {
	v, ok := __jsonWalk(text, path)
	if !ok {
		return []string{}
	}
	obj, isObj := v.(map[string]any)
	if !isObj {
		return []string{}
	}
	out := make([]string, 0, len(obj))
	for k := range obj {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func __jsonLen(text string, path string) int {
	v, ok := __jsonWalk(text, path)
	if !ok {
		return 0
	}
	switch node := v.(type) {
	case []any:
		return len(node)
	case map[string]any:
		return len(node)
	case string:
		return len(node)
	}
	return 0
}`,
		imports: []string{"encoding/json", "sort", "strconv", "strings"},
	},
}

var jsonBuiltins map[string]builtin

func buildJsonBuiltins() {
	// path-based readers all have the same shape.
	reader := func(goFn string, ret *Type) builtin {
		return builtin{
			emit: func(a []string) string {
				path := `""`
				if len(a) == 2 {
					path = a[1]
				}
				return goFn + "(" + a[0] + ", " + path + ")"
			},
			params: []*Type{Str, Str}, ret: ret,
			minArgs: 1, maxArgs: 2,
			helpers: []string{"jsonPath"},
		}
	}

	jsonBuiltins = map[string]builtin{
		"json.encode": {
			emit:   func(a []string) string { return "__jsonEncode(" + a[0] + ", false)" },
			params: []*Type{Any}, ret: Str, minArgs: 1, maxArgs: 1,
			helpers: []string{"jsonEncode"},
		},
		"json.pretty": {
			emit:   func(a []string) string { return "__jsonEncode(" + a[0] + ", true)" },
			params: []*Type{Any}, ret: Str, minArgs: 1, maxArgs: 1,
			helpers: []string{"jsonEncode"},
		},

		// decode takes its result type from the binding it is assigned to.
		"json.decode": {
			minArgs: 1, maxArgs: 1,
			wantsTarget: true,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				matches(c, x, 0, Str, "json.decode")
				if x.Want == nil || x.Want.IsUnknown() {
					c.errorAt(x, "json.decode needs to know what to decode into — "+
						"annotate the variable, as in: let p: Point = json.decode(text)")
					return Unknown
				}
				return x.Want
			},
			helpers: []string{"jsonDecode"},
			emitT: func(c *Codegen, x *Call, a []string) string {
				return "__jsonDecode[" + x.Want.Go() + "](" + a[0] + ")"
			},
		},
		"json.decodeOr": {
			minArgs: 2, maxArgs: 2,
			check: func(c *Checker, x *Call, args []*Type) *Type {
				matches(c, x, 0, Str, "json.decodeOr")
				if len(args) < 2 || args[1].IsUnknown() {
					return Unknown
				}
				// The fallback is the shape, so no annotation is needed.
				return args[1]
			},
			helpers: []string{"jsonDecode"},
			emitT: func(c *Codegen, x *Call, a []string) string {
				return "__jsonDecodeOr(" + a[0] + ", " + a[1] + ")"
			},
		},

		// ---- reading one value without declaring a type ----

		"json.get":   reader("__jsonGet", Str),
		"json.num":   reader("__jsonNum", Float),
		"json.bool":  reader("__jsonBool", Bool),
		"json.has":   reader("__jsonHas", Bool),
		"json.keys":  reader("__jsonKeys", ListOf(Str)),
		"json.count": reader("__jsonLen", Int),

		"json.int": {
			emit: func(a []string) string {
				path := `""`
				if len(a) == 2 {
					path = a[1]
				}
				return "int(__jsonNum(" + a[0] + ", " + path + "))"
			},
			params: []*Type{Str, Str}, ret: Int,
			minArgs: 1, maxArgs: 2,
			helpers: []string{"jsonPath"},
		},

		"json.valid": {
			emit: func(a []string) string {
				return "json.Valid([]byte(" + a[0] + "))"
			},
			params: []*Type{Str}, ret: Bool, minArgs: 1, maxArgs: 1,
			imports: []string{"encoding/json"},
		},
	}
}

func registerJson() {
	buildJsonBuiltins()
	registerNamespace("json")
	for k, v := range jsonHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range jsonBuiltins {
		builtins[k] = v
	}
}
