package frontend

import "fmt"

// Kind is the category of a token.
type Kind int

const (
	EOF Kind = iota
	NEWLINE
	IDENT
	NUMBER
	STRING
	RAWSTRING

	// Keywords
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

	// reserved for later - recognised but not yet implemented
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
	STEP

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
	DOTDOT
	DOTDOTEQ
	FATARROW

	// bitwise
	AMP
	PIPE
	CARET
	TILDE
	SHL
	SHR
	PERCENTEQ
	AMPEQ
	PIPEEQ
	CARETEQ
	SHLEQ
	SHREQ

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
	QUESTION

	// COMMENT is only produced when the lexer is asked to keep them,
	// which only the formatter does.
	COMMENT

	ILLEGAL
)

// kindNames must stay in the exact same order as the constants above.
var kindNames = [...]string{
	"EOF",
	"NEWLINE",
	"IDENT",
	"NUMBER",
	"STRING",
	"RAWSTRING",

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
	"STEP",

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
	"DOTDOT",
	"DOTDOTEQ",
	"FATARROW",

	"AMP",
	"PIPE",
	"CARET",
	"TILDE",
	"SHL",
	"SHR",
	"PERCENTEQ",
	"AMPEQ",
	"PIPEEQ",
	"CARETEQ",
	"SHLEQ",
	"SHREQ",

	"LPAREN",
	"RPAREN",
	"LBRACE",
	"RBRACE",
	"LBRACKET",
	"RBRACKET",
	"COMMA",
	"DOT",
	"COLON",
	"QUESTION",
	"COMMENT",

	"ILLEGAL",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Keywords maps source text to its keyword Kind.
var Keywords = map[string]Kind{
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
	"step":   STEP,
}

// Token is a single lexical unit, tagged with where it came from.
// Line and Col are 1-based. Never drop these - every good error message
// and every //line directive in codegen depends on them.
type Token struct {
	Kind Kind
	Lex  string // for STRING this is the *decoded* value, not the raw source
	Line int
	Col  int

	// Raw is the exact source text, kept only when the lexer is asked to
	// preserve trivia. The formatter needs it: Lex has already had its
	// escapes decoded and its quotes stripped, so writing Lex back out
	// would turn "a\nb" into a real newline and lose the quotes with it.
	Raw string
}

// Text returns what should be written back to a source file.
func (t Token) Text() string {
	if t.Raw != "" {
		return t.Raw
	}
	return t.Lex
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
