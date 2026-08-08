package main

import (
	"fmt"
	"strings"
)

// Lexer turns Quartz source text into a flat slice of Tokens.
type Lexer struct {
	src  string
	file string
	pos  int // byte offset into src
	line int
	col  int

	tokens []Token
	Errors []string
}

func NewLexer(file, src string) *Lexer {
	return &Lexer{src: src, file: file, line: 1, col: 1}
}

// Scan runs the whole lexer and returns the token slice.
// It always ends with an EOF token, even on error.
func (l *Lexer) Scan() []Token {
	for {
		l.skipSpaceAndComments()
		if l.atEnd() {
			break
		}

		line, col := l.line, l.col
		c := l.peek()

		switch {
		case c == '\n':
			l.advance()
			l.emit(NEWLINE, "\n", line, col)
		case isAlpha(c):
			l.ident(line, col)
		case isDigit(c):
			l.number(line, col)
		case c == '"':
			l.str(line, col)
		default:
			l.operator(line, col)
		}
	}
	l.emit(EOF, "", l.line, l.col)
	return l.tokens
}

// ---- scanners for each token shape ----

func (l *Lexer) ident(line, col int) {
	start := l.pos
	for !l.atEnd() && (isAlpha(l.peek()) || isDigit(l.peek())) {
		l.advance()
	}
	text := l.src[start:l.pos]
	if kind, ok := keywords[text]; ok {
		l.emit(kind, text, line, col)
		return
	}
	l.emit(IDENT, text, line, col)
}

func (l *Lexer) number(line, col int) {
	start := l.pos
	for !l.atEnd() && isDigit(l.peek()) {
		l.advance()
	}
	// a fractional part, but only if a digit actually follows the dot,
	// so that `1.method()` still lexes as NUMBER DOT IDENT later on
	if !l.atEnd() && l.peek() == '.' && isDigit(l.peekNext()) {
		l.advance() // consume '.'
		for !l.atEnd() && isDigit(l.peek()) {
			l.advance()
		}
	}
	l.emit(NUMBER, l.src[start:l.pos], line, col)
}

func (l *Lexer) str(line, col int) {
	l.advance() // opening quote

	var sb strings.Builder
	depth := 0     // {} nesting inside an interpolation
	inner := false // inside a nested "..." within an interpolation

	for {
		if l.atEnd() || l.peek() == '\n' {
			l.errorf(line, col, "unterminated string literal")
			l.emit(ILLEGAL, sb.String(), line, col)
			return
		}
		c := l.advance()

		// Escapes are handled first so that \" and \\ can never affect
		// the nesting state below.
		if c == '\\' {
			if l.atEnd() {
				l.errorf(l.line, l.col, "unterminated escape sequence")
				l.emit(ILLEGAL, sb.String(), line, col)
				return
			}
			eLine, eCol := l.line, l.col
			switch e := l.advance(); e {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '0':
				sb.WriteByte(0)
			case '\\':
				sb.WriteByte('\\')
			case '"':
				sb.WriteByte('"')
			default:
				l.errorf(eLine, eCol, "unknown escape sequence \\%c", e)
				sb.WriteByte(e)
			}
			continue
		}

		switch {
		case c == '"' && depth == 0:
			// Only a quote at brace-depth zero ends the literal, which is
			// what lets "{f("x")}" nest a string inside an interpolation.
			l.emit(STRING, sb.String(), line, col)
			return

		case c == '"':
			inner = !inner

		case inner:
			// plain text inside a nested string; braces here mean nothing

		case c == '{':
			if depth == 0 && l.peek() == '{' {
				sb.WriteByte('{')
				sb.WriteByte(l.advance())
				continue
			}
			depth++

		case c == '}':
			if depth == 0 && l.peek() == '}' {
				sb.WriteByte('}')
				sb.WriteByte(l.advance())
				continue
			}
			if depth > 0 {
				depth--
			}
		}
		sb.WriteByte(c)
	}
}

func (l *Lexer) operator(line, col int) {
	c := l.advance()

	two := func(next byte, ifMatch Kind, matchLex string, otherwise Kind, otherLex string) {
		if l.match(next) {
			l.emit(ifMatch, matchLex, line, col)
		} else {
			l.emit(otherwise, otherLex, line, col)
		}
	}

	switch c {
	case '+':
		two('=', PLUSEQ, "+=", PLUS, "+")
	case '*':
		two('=', STAREQ, "*=", STAR, "*")
	case '/':
		two('=', SLASHEQ, "/=", SLASH, "/")
	case '=':
		two('=', EQ, "==", ASSIGN, "=")
	case '!':
		two('=', NEQ, "!=", BANG, "!")
	case '<':
		two('=', LTE, "<=", LT, "<")
	case '>':
		two('=', GTE, ">=", GT, ">")
	case '%':
		l.emit(PERCENT, "%", line, col)

	case '-':
		switch {
		case l.match('='):
			l.emit(MINUSEQ, "-=", line, col)
		case l.match('>'):
			l.emit(ARROW, "->", line, col)
		default:
			l.emit(MINUS, "-", line, col)
		}

	case '&':
		if l.match('&') {
			l.emit(AND, "&&", line, col)
		} else {
			l.illegal(line, col, "&")
		}
	case '|':
		if l.match('|') {
			l.emit(OR, "||", line, col)
		} else {
			l.illegal(line, col, "|")
		}

	case '(':
		l.emit(LPAREN, "(", line, col)
	case ')':
		l.emit(RPAREN, ")", line, col)
	case '{':
		l.emit(LBRACE, "{", line, col)
	case '}':
		l.emit(RBRACE, "}", line, col)
	case '[':
		l.emit(LBRACKET, "[", line, col)
	case ']':
		l.emit(RBRACKET, "]", line, col)
	case ',':
		l.emit(COMMA, ",", line, col)
	case '.':
		switch {
		case l.match('.'):
			if l.match('=') {
				l.emit(DOTDOTEQ, "..=", line, col)
			} else {
				l.emit(DOTDOT, "..", line, col)
			}
		default:
			l.emit(DOT, ".", line, col)
		}
	case ':':
		l.emit(COLON, ":", line, col)

	default:
		l.illegal(line, col, string(c))
	}
}

func (l *Lexer) skipSpaceAndComments() {
	for !l.atEnd() {
		switch c := l.peek(); {
		case c == ' ' || c == '\t' || c == '\r':
			l.advance()

		case c == '/' && l.peekNext() == '/':
			for !l.atEnd() && l.peek() != '\n' {
				l.advance()
			}

		case c == '/' && l.peekNext() == '*':
			line, col := l.line, l.col
			l.advance() // '/'
			l.advance() // '*'
			depth := 1
			for depth > 0 {
				if l.atEnd() {
					l.errorf(line, col, "unterminated block comment")
					return
				}
				switch {
				case l.peek() == '/' && l.peekNext() == '*':
					l.advance()
					l.advance()
					depth++
				case l.peek() == '*' && l.peekNext() == '/':
					l.advance()
					l.advance()
					depth--
				default:
					l.advance()
				}
			}

		default:
			return
		}
	}
}

// ---- low-level cursor helpers ----

func (l *Lexer) atEnd() bool { return l.pos >= len(l.src) }

func (l *Lexer) peek() byte {
	if l.atEnd() {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekNext() byte {
	if l.pos+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+1]
}

func (l *Lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

func (l *Lexer) match(expected byte) bool {
	if l.atEnd() || l.src[l.pos] != expected {
		return false
	}
	l.advance()
	return true
}

func (l *Lexer) emit(k Kind, lex string, line, col int) {
	l.tokens = append(l.tokens, Token{Kind: k, Lex: lex, Line: line, Col: col})
}

func (l *Lexer) illegal(line, col int, lex string) {
	l.errorf(line, col, "unexpected character %q", lex)
	l.emit(ILLEGAL, lex, line, col)
}

func (l *Lexer) errorf(line, col int, format string, args ...any) {
	l.Errors = append(l.Errors,
		fmt.Sprintf("%s:%d:%d: %s", l.file, line, col, fmt.Sprintf(format, args...)))
}

func isAlpha(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
