package main

import "strings"

// The formatter.
//
// It works on the token stream, not the AST, and that is a deliberate
// choice. An AST-based printer would have to reconstruct every comment
// from scratch, because the parser throws them away - and a formatter
// that silently deletes comments is worse than no formatter at all.
// Working from tokens means anything the formatter does not understand
// passes through untouched.
//
// What it does: fixes indentation, normalises spacing around operators
// and punctuation, collapses runs of blank lines to one, and strips
// trailing whitespace. What it deliberately does not do: reflow long
// lines, or reorder anything. A formatter that rewrites your line
// breaks has to be much cleverer than this one before it earns the
// right.

// Format reformats Veyl source. It returns the source unchanged if
// the input does not lex cleanly - reformatting a file with a broken
// string literal in it is a good way to lose work.
func Format(file, src string) (string, bool) {
	lx := NewLexer(file, src)
	lx.KeepComments = true
	toks := lx.Scan()
	if len(lx.Errors) > 0 {
		return src, false
	}

	f := &formatter{toks: toks}
	out := f.run()

	// Exactly one newline at the end: no trailing blank lines, but never
	// a file without a final newline either.
	out = strings.TrimRight(out, "\n")
	if out != "" {
		out += "\n"
	}
	return out, true
}

type formatter struct {
	toks   []Token
	out    strings.Builder
	indent int

	lineStarted bool // something has been written on the current line
	blankRun    int  // consecutive blank lines just emitted
	lineIndent  int  // indent the current line began at

	// literalBrace marks each `{` that opens a map or struct literal
	// rather than a block. Literals hug their contents and do not
	// indent; blocks do the opposite.
	literalBrace map[int]bool
	matching     map[int]int // `{` index -> its `}` index
}

// findLiteralBraces decides, for every `{`, whether it opens a value or
// a block.
//
// The parser settles this with context the formatter does not have, so
// this uses a shape test instead: a brace group entirely on one line
// that contains a top-level `:` is a map or struct literal. That covers
// {"a": 1} and Point{x: 1}, and correctly leaves `if x { run() }` alone
// because it has no colon at that depth.
func (f *formatter) findLiteralBraces() {
	f.literalBrace = map[int]bool{}
	f.matching = map[int]int{}

	var stack []int
	for i, t := range f.toks {
		switch t.Kind {
		case LBRACE:
			stack = append(stack, i)
		case RBRACE:
			if len(stack) == 0 {
				continue
			}
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			f.matching[open] = i

			oneLine, hasColon, depth := true, false, 0
			for j := open + 1; j < i; j++ {
				switch f.toks[j].Kind {
				case NEWLINE:
					oneLine = false
				case LBRACE, LBRACKET, LPAREN:
					depth++
				case RBRACE, RBRACKET, RPAREN:
					depth--
				case COLON:
					if depth == 0 {
						hasColon = true
					}
				}
			}
			// An empty pair is a literal only where a value belongs -
			// Point{} and `= {}`, but not the body of `fn f() {}`.
			empty := i == open+1
			if empty {
				before := f.prevMeaningful(open)
				f.literalBrace[open] = before >= 0 && startsValuePosition(f.toks[before].Kind)
				continue
			}
			f.literalBrace[open] = oneLine && hasColon
		}
	}
}

// isLiteralClose reports whether the `}` at i closes a literal brace.
func (f *formatter) isLiteralClose(i int) bool {
	for open, close := range f.matching {
		if close == i {
			return f.literalBrace[open]
		}
	}
	return false
}

func (f *formatter) run() string {
	f.findLiteralBraces()
	for i := 0; i < len(f.toks); i++ {
		t := f.toks[i]

		switch t.Kind {
		case EOF:
			f.endLine()

		case NEWLINE:
			f.endLine()

		case RBRACE, RBRACKET, RPAREN:
			// A closing bracket outdents itself, so it lines up with the
			// line that opened the group rather than its contents.
			if !f.lineStarted && !(t.Kind == RBRACE && f.isLiteralClose(i)) {
				f.indent--
				if f.indent < 0 {
					f.indent = 0
				}
			}
			f.write(t.Text(), i)

		default:
			f.write(t.Text(), i)
		}

		switch t.Kind {
		case LBRACKET, LPAREN:
			f.indent++
		case LBRACE:
			if !f.literalBrace[i] {
				f.indent++
			}
		case RBRACKET, RPAREN:
			if f.wasMidLine(i) {
				f.indent--
			}
		case RBRACE:
			// Only outdent here if the brace was mid-line, since the
			// branch above already handled the start-of-line case.
			if !f.isLiteralClose(i) && f.wasMidLine(i) {
				f.indent--
			}
		}
		if f.indent < 0 {
			f.indent = 0
		}
	}
	return f.out.String()
}

// wasMidLine reports whether the token at i had something before it on
// its own line, which decides who owns the outdent.
func (f *formatter) wasMidLine(i int) bool {
	for j := i - 1; j >= 0; j-- {
		switch f.toks[j].Kind {
		case NEWLINE:
			return false
		case COMMENT:
			continue
		default:
			return true
		}
	}
	return false
}

func (f *formatter) write(text string, i int) {
	if text == "" {
		return
	}
	if !f.lineStarted {
		f.out.WriteString(strings.Repeat("    ", f.indent))
		f.lineStarted = true
		f.lineIndent = f.indent
		f.blankRun = 0
	} else if f.needsSpace(i) || f.wouldMerge(i, text) {
		f.out.WriteByte(' ')
	}
	f.out.WriteString(text)
}

// wouldMerge reports whether writing this token straight after the
// previous one would lex as something else.
//
// The case that motivated it: `Person! = x` has no space before `!` and
// none after, which produces `Person!=` - and `!=` is a single token.
// The formatter would have silently changed the program. Rather than
// enumerating the dangerous pairs, this re-lexes the join and checks
// that the first token still ends where it should.
func (f *formatter) wouldMerge(i int, text string) bool {
	prev := f.prevMeaningful(i)
	if prev < 0 {
		return false
	}
	left := f.toks[prev].Text()
	if left == "" || !mergeable(last(left)) || !mergeable(text[0]) {
		return false
	}

	probe := NewLexer("", left+text)
	toks := probe.Scan()
	if len(toks) == 0 {
		return false
	}
	return toks[0].Text() != left && toks[0].Lex != left
}

// mergeable reports whether a character can be part of a
// multi-character token, which is the only way two tokens can run
// together into a third.
func mergeable(c byte) bool {
	switch c {
	case '=', '!', '<', '>', '&', '|', '+', '-', '*', '/', '%', '^', '.', '?', ':':
		return true
	}
	return IsAlpha(c) || IsDigit(c)
}

func last(s string) byte { return s[len(s)-1] }

func (f *formatter) endLine() {
	if f.lineStarted {
		f.out.WriteByte('\n')
		f.lineStarted = false
		f.blankRun = 0
		// One line never adds more than one level of indent. Without
		// this, `f(a, fn(n: int) {` opens a paren and a brace on the
		// same line and the body lands two levels in, which is not how
		// anyone writes it.
		if f.indent > f.lineIndent+1 {
			f.indent = f.lineIndent + 1
		}
		return
	}
	// A blank line. One is a paragraph break and worth keeping; three
	// in a row is just drift.
	if f.blankRun < 1 && f.out.Len() > 0 {
		f.out.WriteByte('\n')
		f.blankRun++
	}
}

// needsSpace decides whether to put a space before token i, given what
// came before it on the same line.
//
// What comes *before* is checked first. An opening bracket never has a
// space after it, whatever follows - getting that order wrong produces
// `push( [1, 2], 3)`.
func (f *formatter) needsSpace(i int) bool {
	prev := f.prevMeaningful(i)
	if prev < 0 {
		return false
	}
	left, right := f.toks[prev].Kind, f.toks[i].Kind

	// Never a space after these.
	switch left {
	case LPAREN, LBRACKET, DOT, DOTDOT, DOTDOTEQ, TILDE:
		return false
	case LBRACE:
		return !f.literalBrace[prev]
	case BANG:
		// `str! = x` needs the space; `!ready` does not. Which one this
		// is depends on whether a value came before the `!`.
		return f.isPostfix(prev)
	case QUESTION:
		// Mirror image: `?int` is a prefix, `load(p)?` is a postfix.
		return f.isPostfix(prev)
	case RBRACKET:
		// `[]int`, `[]fn()` and `[]?int` are each one type and hug.
		// `xs[0] + 1` does not - but an empty pair of brackets can only
		// be a list type.
		if prev > 0 && f.toks[prev-1].Kind == LBRACKET {
			switch right {
			case IDENT, FN, QUESTION, LBRACKET, LBRACE:
				return false
			}
		}
	case MINUS:
		// A unary minus hugs its operand; a binary one does not.
		if f.isUnary(prev) {
			return false
		}
	}

	// Never a space before these.
	switch right {
	case COMMA, COLON, RPAREN, RBRACKET, DOT, DOTDOT, DOTDOTEQ:
		return false
	case BANG, QUESTION:
		// A suffix hugs what it applies to; a prefix does not.
		return !endsValue(left)
	case RBRACE:
		return !f.isLiteralClose(i)
	case LBRACE:
		// A struct literal hugs the name in front of it - Point{x: 1},
		// not Point {x: 1}. A bare map literal has no name to hug, so
		// `let m = {"a": 1}` keeps its space after the `=`.
		return !(f.literalBrace[i] && hugsLeft(left))
	case LPAREN, LBRACKET:
		// `f(x)` and `xs[0]` hug what they apply to. `if (a)`,
		// `x & (y)` and `= [1, 2]` do not. `fn(` is one word.
		return !hugsLeft(left) && left != FN
	}
	return true
}

// isPostfix reports whether the `!` or `?` at i is a suffix on a value
// or type, rather than a prefix on what follows.
func (f *formatter) isPostfix(i int) bool {
	prev := f.prevMeaningful(i)
	return prev >= 0 && endsValue(f.toks[prev].Kind)
}

// isUnary reports whether the operator at i is prefix rather than
// infix, which is decided by whether a value could have preceded it.
func (f *formatter) isUnary(i int) bool {
	prev := f.prevMeaningful(i)
	if prev < 0 {
		return true
	}
	return !endsValue(f.toks[prev].Kind)
}

func (f *formatter) prevMeaningful(i int) int {
	for j := i - 1; j >= 0; j-- {
		switch f.toks[j].Kind {
		case NEWLINE:
			return -1
		case COMMENT:
			continue
		default:
			return j
		}
	}
	return -1
}

// endsValue reports whether a token can end an expression, which is how
// a binary minus is told from a unary one.
func endsValue(k Kind) bool {
	switch k {
	case IDENT, NUMBER, STRING, RAWSTRING, TRUE, FALSE, NIL, SELF,
		RPAREN, RBRACKET, RBRACE, QUESTION, BANG:
		return true
	}
	return false
}

// hugsLeft reports whether `[` directly follows something it indexes.
func hugsLeft(k Kind) bool {
	switch k {
	case IDENT, RPAREN, RBRACKET, STRING, RAWSTRING, SELF:
		return true
	}
	return false
}

// startsValuePosition reports whether a value could begin right after
// this token, which is how an empty struct literal is told from an
// empty block.
func startsValuePosition(k Kind) bool {
	switch k {
	case IDENT, ASSIGN, COMMA, LPAREN, COLON, RETURN, LBRACKET:
		return true
	}
	return false
}
