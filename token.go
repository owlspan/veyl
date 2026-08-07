package main

import "fmt"

// Kind is the category of a token.
type Kind int

const (
	EOF Kind = iota
	NEWLINE
	IDENT
	NUMBER
	STRING

	// keywords
	LET
	FN
	IF
	ELSE
	WHILE
	FOR
	RETURN
	BREAK
	CONTINUE
	TRUE
	FALSE
	NIL
	IMPORT

	// reserved for later — recognised but not yet implemented
	CONST
	STRUCT
	IMPL
	SELF
	MATCH
	IN
	PUB
	DEFER
	OWN
	UNSAFE

	// operators
	PLUS
	MINUS
	STAR
	SLASH
	PERCENT
	ASSIGN
	EQ
	NEQ
	LT
	LTE
	GT
	GTE
	AND
	OR
	BANG
	PLUSEQ
	MINUSEQ
	STAREQ
	SLASHEQ
	ARROW

	// punctuation
	LPAREN
	RPAREN
	LBRACE
	RBRACE
	LBRACKET
	RBRACKET
	COMMA
	DOT
	COLON

	ILLEGAL
)

// kindNames must stay in the exact same order as the constants above.
var kindNames = [...]string{
	"EOF",
	"NEWLINE",
	"IDENT",
	"NUMBER",
	"STRING",

	"LET",
	"FN",
	"IF",
	"ELSE",
	"WHILE",
	"FOR",
	"RETURN",
	"BREAK",
	"CONTINUE",
	"TRUE",
	"FALSE",
	"NIL",
	"IMPORT",

	"CONST",
	"STRUCT",
	"IMPL",
	"SELF",
	"MATCH",
	"IN",
	"PUB",
	"DEFER",
	"OWN",
	"UNSAFE",

	"PLUS",
	"MINUS",
	"STAR",
	"SLASH",
	"PERCENT",
	"ASSIGN",
	"EQ",
	"NEQ",
	"LT",
	"LTE",
	"GT",
	"GTE",
	"AND",
	"OR",
	"BANG",
	"PLUSEQ",
	"MINUSEQ",
	"STAREQ",
	"SLASHEQ",
	"ARROW",

	"LPAREN",
	"RPAREN",
	"LBRACE",
	"RBRACE",
	"LBRACKET",
	"RBRACKET",
	"COMMA",
	"DOT",
	"COLON",

	"ILLEGAL",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// keywords maps source text to its keyword Kind.
var keywords = map[string]Kind{
	"let":      LET,
	"fn":       FN,
	"if":       IF,
	"else":     ELSE,
	"while":    WHILE,
	"for":      FOR,
	"return":   RETURN,
	"break":    BREAK,
	"continue": CONTINUE,
	"true":     TRUE,
	"false":    FALSE,
	"nil":      NIL,
	"import":   IMPORT,

	"const":  CONST,
	"struct": STRUCT,
	"impl":   IMPL,
	"self":   SELF,
	"match":  MATCH,
	"in":     IN,
	"pub":    PUB,
	"defer":  DEFER,
	"own":    OWN,
	"unsafe": UNSAFE,
}

// Token is a single lexical unit, tagged with where it came from.
// Line and Col are 1-based. Never drop these — every good error message
// and every //line directive in codegen depends on them.
type Token struct {
	Kind Kind
	Lex  string // for STRING this is the *decoded* value, not the raw source
	Line int
	Col  int
}

func (t Token) String() string {
	switch t.Kind {
	case EOF:
		return fmt.Sprintf("%d:%d  EOF", t.Line, t.Col)
	case NEWLINE:
		return fmt.Sprintf("%d:%d  NEWLINE", t.Line, t.Col)
	default:
		return fmt.Sprintf("%d:%d  %-8s %q", t.Line, t.Col, t.Kind, t.Lex)
	}
}
