package frontend

import "testing"

// Type parsing has more edge cases than whole-program tests reach
// comfortably, and getting one wrong is quiet: `fn(str) -> int!` once
// parsed as a result carrying a function rather than a function
// returning a result, and nothing failed until a program used it.

func TestParseType(t *testing.T) {
	cases := []struct {
		src  string
		want string // what String() should give back
	}{
		{"int", "int"},
		{"float", "float"},
		{"str", "str"},
		{"bool", "bool"},

		{"[]int", "[]int"},
		{"[][]str", "[][]str"},
		{"{str: int}", "{str: int}"},
		{"{str: []int}", "{str: []int}"},
		{"{int: {str: bool}}", "{int: {str: bool}}"},

		{"?int", "?int"},
		{"?[]str", "?[]str"},
		{"[]?int", "[]?int"},
		{"??int", "?int"}, // there is only one nil

		{"int!", "int!"},
		{"[]str!", "[]str!"},
		{"?int!", "?int!"}, // a result carrying a nullable

		{"fn()", "fn()"},
		{"fn(int)", "fn(int)"},
		{"fn(int, str)", "fn(int, str)"},
		{"fn(int) -> bool", "fn(int) -> bool"},
		{"fn([]int, {str: int}) -> ?str", "fn([]int, {str: int}) -> ?str"},

		// The precedence that was wrong: the ! belongs to the return
		// type, not to the whole function.
		{"fn(str) -> int!", "fn(str) -> int!"},
		{"fn() -> []str!", "fn() -> []str!"},

		{"Point", "Point"}, // an undeclared struct; the checker judges it
	}

	for _, c := range cases {
		got := ParseType(c.src)
		if got == nil {
			t.Errorf("ParseType(%q) returned nil, want %s", c.src, c.want)
			continue
		}
		if got.String() != c.want {
			t.Errorf("ParseType(%q).String() = %q, want %q", c.src, got.String(), c.want)
		}
	}
}

func TestParseTypeShape(t *testing.T) {
	// String() alone cannot prove the tree is right, so check the two
	// that a wrong precedence would still print correctly.
	fnRes := ParseType("fn(str) -> int!")
	if !fnRes.IsFunc() {
		t.Fatalf("fn(str) -> int! is %s, want a function", fnRes)
	}
	if !fnRes.Elem.IsResult() {
		t.Errorf("its return type is %s, want a result", fnRes.Elem)
	}

	nullRes := ParseType("?int!")
	if !nullRes.IsResult() {
		t.Fatalf("?int! is %s, want a result", nullRes)
	}
	if !nullRes.Elem.IsNullable() {
		t.Errorf("it carries %s, want a nullable", nullRes.Elem)
	}
}

func TestParseTypeRejects(t *testing.T) {
	bad := []string{
		"",
		"[]",
		"?",
		"{int}",
		"{bool: int}", // only str and int may be keys
		"{: int}",
		"fn(",
		"fn(int",
		"fn(int) ->",
		"fn(int) => bool",
		"[]{bool: int}",
	}
	for _, src := range bad {
		if got := ParseType(src); got != nil {
			t.Errorf("ParseType(%q) = %s, want nil", src, got)
		}
	}
}

func TestTypeEqual(t *testing.T) {
	same := [][2]string{
		{"int", "int"},
		{"[]int", "[]int"},
		{"{str: []int}", "{str: []int}"},
		{"?int", "?int"},
		{"int!", "int!"},
		{"fn(int) -> str", "fn(int) -> str"},
	}
	for _, pair := range same {
		if !ParseType(pair[0]).Equal(ParseType(pair[1])) {
			t.Errorf("%s should equal %s", pair[0], pair[1])
		}
	}

	differ := [][2]string{
		{"int", "float"},
		{"[]int", "[]str"},
		{"[]int", "int"},
		{"{str: int}", "{int: int}"},
		{"?int", "int"},
		{"int!", "int"},
		{"?int", "int!"},
		{"fn(int)", "fn(str)"},
		{"fn(int)", "fn(int) -> int"},
		{"fn(int, int)", "fn(int)"},
	}
	for _, pair := range differ {
		if ParseType(pair[0]).Equal(ParseType(pair[1])) {
			t.Errorf("%s should not equal %s", pair[0], pair[1])
		}
	}
}

func TestTypeAccepts(t *testing.T) {
	cases := []struct {
		want, got string
		ok        bool
		why       string
	}{
		{"int", "int", true, "identical"},
		{"int", "float", false, "no implicit conversion"},
		{"float", "int", false, "not even widening"},

		{"?int", "int", true, "a value widens into a nullable"},
		{"int", "?int", false, "a nullable does not narrow on its own"},
		{"?int", "?int", true, "identical"},

		{"int!", "int", true, "a value is a successful result"},
		{"int", "int!", false, "a result must be unwrapped"},

		{"[]int", "[]int", true, "identical"},
		{"[]int", "[]float", false, "element types differ"},
	}

	for _, c := range cases {
		want, got := ParseType(c.want), ParseType(c.got)
		if want.Accepts(got) != c.ok {
			t.Errorf("%s.Accepts(%s) = %t, want %t - %s",
				c.want, c.got, !c.ok, c.ok, c.why)
		}
	}

	// The signature-only kinds, which have no source spelling.
	if !Numeric.Accepts(Int) || !Numeric.Accepts(Float) {
		t.Error("Numeric should accept both int and float")
	}
	if Numeric.Accepts(Str) {
		t.Error("Numeric should not accept str")
	}
	if !Any.Accepts(ParseType("[]int")) {
		t.Error("Any should accept a list")
	}
	if Any.Accepts(ParseType("int!")) {
		t.Error("Any should not accept a result - it has to be unwrapped first")
	}
	if Any.Accepts(Void) {
		t.Error("Any should not accept a call that returns nothing")
	}
}

func TestTypeGo(t *testing.T) {
	cases := map[string]string{
		"int":            "int",
		"float":          "float64",
		"str":            "string",
		"bool":           "bool",
		"[]int":          "[]int",
		"[][]str":        "[][]string",
		"{str: int}":     "map[string]int",
		"?int":           "*int",
		"int!":           "__Res[int]",
		"fn(int) -> str": "func(int) string",
		"fn(str)":        "func(string)",
	}
	for src, want := range cases {
		if got := ParseType(src).Go(); got != want {
			t.Errorf("ParseType(%q).Go() = %q, want %q", src, got, want)
		}
	}
}
