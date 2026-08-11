package main

import "strings"

// TypeKind is the category of a Quartz type.
type TypeKind int

const (
	// KUnknown is the error-suppression type. Once an expression is
	// unknown, every check downstream of it stays quiet, so a single
	// mistake produces one error instead of ten.
	KUnknown TypeKind = iota
	KVoid             // no value at all: a function with no return type
	KInt
	KFloat
	KStr
	KBool
	KList
	KMap

	// Signature-only kinds. These never appear as the type of a real
	// expression — they exist so a builtin can say "any number" or
	// "anything at all" in its parameter list.
	KNumeric
	KAny
)

// Type is a Quartz type. Composite types point at their element types,
// so nesting like [][]int or {str: []int} needs no special cases.
type Type struct {
	Kind TypeKind
	Elem *Type // list element, or map value
	Key  *Type // map key
}

// The scalar types are singletons; nothing ever mutates a Type.
var (
	Unknown = &Type{Kind: KUnknown}
	Void    = &Type{Kind: KVoid}
	Int     = &Type{Kind: KInt}
	Float   = &Type{Kind: KFloat}
	Str     = &Type{Kind: KStr}
	Bool    = &Type{Kind: KBool}
	Numeric = &Type{Kind: KNumeric}
	Any     = &Type{Kind: KAny}
)

func ListOf(elem *Type) *Type      { return &Type{Kind: KList, Elem: elem} }
func MapOf(key, val *Type) *Type   { return &Type{Kind: KMap, Key: key, Elem: val} }
func (t *Type) IsUnknown() bool    { return t == nil || t.Kind == KUnknown }
func (t *Type) IsNumeric() bool    { return t != nil && (t.Kind == KInt || t.Kind == KFloat) }
func (t *Type) IsCollection() bool { return t != nil && (t.Kind == KList || t.Kind == KMap) }

// String prints a type in Quartz's vocabulary. The whole point of the
// checker is that users never see "float64" or "string" again.
func (t *Type) String() string {
	if t == nil {
		return "unknown"
	}
	switch t.Kind {
	case KVoid:
		return "nothing"
	case KInt:
		return "int"
	case KFloat:
		return "float"
	case KStr:
		return "str"
	case KBool:
		return "bool"
	case KList:
		return "[]" + t.Elem.String()
	case KMap:
		return "{" + t.Key.String() + ": " + t.Elem.String() + "}"
	case KNumeric:
		return "int or float"
	case KAny:
		return "any"
	}
	return "unknown"
}

// Equal reports whether two types are identical.
func (t *Type) Equal(u *Type) bool {
	if t == nil || u == nil {
		return false
	}
	if t.Kind != u.Kind {
		return false
	}
	switch t.Kind {
	case KList:
		return t.Elem.Equal(u.Elem)
	case KMap:
		return t.Key.Equal(u.Key) && t.Elem.Equal(u.Elem)
	}
	return true
}

// Accepts reports whether a value of type `got` may be passed where this
// type is expected. It is deliberately almost as strict as Equal —
// Quartz has no implicit conversions. The exceptions are the two
// signature-only kinds and KUnknown, which is already an error.
func (t *Type) Accepts(got *Type) bool {
	if t == nil || got == nil {
		return true
	}
	if t.Kind == KUnknown || got.Kind == KUnknown {
		return true // an error was already reported; don't pile on
	}
	switch t.Kind {
	case KAny:
		return got.Kind != KVoid
	case KNumeric:
		return got.IsNumeric()
	}
	return t.Equal(got)
}

// Go returns the Go type this compiles to.
func (t *Type) Go() string {
	if t == nil {
		return "any"
	}
	switch t.Kind {
	case KInt:
		return "int"
	case KFloat:
		return "float64"
	case KStr:
		return "string"
	case KBool:
		return "bool"
	case KList:
		return "[]" + t.Elem.Go()
	case KMap:
		return "map[" + t.Key.Go() + "]" + t.Elem.Go()
	}
	return "any"
}

// Zero is the Go expression for this type's zero value. Needed wherever
// codegen must produce a value out of nothing — a missing map key, an
// out-of-range index, a declared-but-unassigned variable.
func (t *Type) Zero() string {
	if t == nil {
		return "nil"
	}
	switch t.Kind {
	case KInt:
		return "0"
	case KFloat:
		return "0.0"
	case KStr:
		return `""`
	case KBool:
		return "false"
	case KList, KMap:
		return t.Go() + "{}"
	}
	return "nil"
}

// ---- parsing type annotations ----

// ParseType turns the source text of a type annotation into a Type.
// It returns nil for anything it does not recognise, leaving the caller
// to report the error with a position attached.
//
// Grammar, such as it is:
//
//	type := "int" | "float" | "str" | "bool"
//	      | "[]" type
//	      | "{" type ":" type "}"
func ParseType(s string) *Type {
	s = strings.TrimSpace(s)
	switch s {
	case "int":
		return Int
	case "float":
		return Float
	case "str":
		return Str
	case "bool":
		return Bool
	}

	if strings.HasPrefix(s, "[]") {
		elem := ParseType(s[2:])
		if elem == nil {
			return nil
		}
		return ListOf(elem)
	}

	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		inner := s[1 : len(s)-1]
		// Split on the colon that separates key from value, skipping any
		// colon nested inside a braced map type.
		depth := 0
		for i := 0; i < len(inner); i++ {
			switch inner[i] {
			case '{':
				depth++
			case '}':
				depth--
			case ':':
				if depth != 0 {
					continue
				}
				key := ParseType(inner[:i])
				val := ParseType(inner[i+1:])
				if key == nil || val == nil {
					return nil
				}
				// Go map keys must be comparable, and Quartz restricts
				// them further to keep error messages simple.
				if key.Kind != KStr && key.Kind != KInt {
					return nil
				}
				return MapOf(key, val)
			}
		}
	}

	return nil
}
