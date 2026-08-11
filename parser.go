package main

import (
	"fmt"
	"strings"
)

// noBrace suppresses struct literals while parsing the header of an if,
// while or for. Without it `if ready {` reads as the start of a struct
// literal named ready, and the block brace is never found. Go has the
// same rule for the same reason; inside brackets the suppression lifts,
// so `if (Point{x: 1}).x > 0 {` still works.
type Parser struct {
	toks    []Token
	i       int
	file    string
	noBrace int
	Errors  []string
}

func NewParser(file string, toks []Token) *Parser {
	return &Parser{toks: toks, file: file}
}

// header parses an expression in a position where a following '{' opens
// a block rather than a struct literal.
func (p *Parser) header(parse func() Expr) Expr {
	p.noBrace++
	x := parse()
	p.noBrace--
	return x
}

// grouped parses an expression inside brackets, where a '{' is
// unambiguous again.
func (p *Parser) grouped(parse func() Expr) Expr {
	saved := p.noBrace
	p.noBrace = 0
	x := parse()
	p.noBrace = saved
	return x
}

// ---- cursor helpers ----

func (p *Parser) cur() Token { return p.toks[p.i] }

func (p *Parser) advance() Token {
	t := p.toks[p.i]
	if t.Kind != EOF {
		p.i++
	}
	return t
}

func (p *Parser) check(k Kind) bool { return p.cur().Kind == k }

func (p *Parser) match(k Kind) bool {
	if p.check(k) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) expect(k Kind, what string) Token {
	if p.check(k) {
		return p.advance()
	}
	p.errorAt(p.cur(), "expected %s, found %s", what, describe(p.cur()))
	return p.cur()
}

func describe(t Token) string {
	switch t.Kind {
	case EOF:
		return "end of file"
	case NEWLINE:
		return "end of line"
	default:
		return fmt.Sprintf("%q", t.Lex)
	}
}

func (p *Parser) errorAt(t Token, format string, args ...any) {
	p.Errors = append(p.Errors,
		fmt.Sprintf("%s:%d:%d: %s", p.file, t.Line, t.Col, fmt.Sprintf(format, args...)))
}

func (p *Parser) skipNewlines() {
	for p.check(NEWLINE) {
		p.advance()
	}
}

// synchronize skips ahead to the next plausible statement start so that
// one syntax error doesn't cascade into twenty bogus ones.
func (p *Parser) synchronize() {
	for !p.check(EOF) {
		if p.check(NEWLINE) {
			p.advance()
			return
		}
		switch p.cur().Kind {
		case LET, CONST, IF, WHILE, FOR, FN, RETURN, BREAK, CONTINUE, RBRACE:
			return
		}
		p.advance()
	}
}

// ---- top level ----

func (p *Parser) ParseProgram() *Program {
	prog := &Program{}
	p.skipNewlines()

	for !p.check(EOF) {
		before := p.i

		switch {
		case p.check(FN):
			if f := p.parseFn(); f != nil {
				prog.Funcs = append(prog.Funcs, f)
			}
		case p.check(STRUCT):
			if d := p.parseStruct(); d != nil {
				prog.Structs = append(prog.Structs, d)
			}
		case p.check(IMPL):
			// Methods are hoisted straight into the function list with a
			// receiver attached, so nothing downstream needs a notion of an
			// impl block.
			if b := p.parseImpl(); b != nil {
				prog.Funcs = append(prog.Funcs, b.Methods...)
			}
		default:
			if s := p.parseStmt(); s != nil {
				prog.Main = append(prog.Main, s)
			}
		}

		if p.i == before { // guarantee forward progress
			p.advance()
		}
		p.skipNewlines()
	}
	return prog
}

// parseStruct reads `struct User { name: str, age: int }`. Fields are
// separated by line breaks or commas, whichever the author prefers.
func (p *Parser) parseStruct() *StructDecl {
	kw := p.advance() // 'struct'
	name := p.expect(IDENT, "a struct name")
	d := &StructDecl{pos: at(kw), Name: name.Lex}

	p.expect(LBRACE, "'{'")
	p.skipNewlines()

	for !p.check(RBRACE) && !p.check(EOF) {
		before := p.i

		fn := p.expect(IDENT, "a field name")
		f := StructField{pos: at(fn), Name: fn.Lex}
		if p.match(COLON) {
			f.Type = p.parseTypeRef()
		} else {
			p.errorAt(fn, "field %q needs a type, like %s: int", fn.Lex, fn.Lex)
		}
		d.Fields = append(d.Fields, f)

		p.match(COMMA)
		p.skipNewlines()
		if p.i == before { // guarantee forward progress
			p.advance()
		}
	}
	p.expect(RBRACE, "'}' to close the struct")
	p.endStmt()
	return d
}

// parseImpl reads `impl User { fn greet(self) -> str { ... } }` and
// returns the methods with their receiver attached.
func (p *Parser) parseImpl() *ImplBlock {
	kw := p.advance() // 'impl'
	name := p.expect(IDENT, "a struct name after 'impl'")
	b := &ImplBlock{pos: at(kw), Type: name.Lex}

	p.expect(LBRACE, "'{'")
	p.skipNewlines()

	for !p.check(RBRACE) && !p.check(EOF) {
		before := p.i
		if p.check(FN) {
			m := p.parseFn()
			if m != nil {
				m.Recv = b.Type
				b.Methods = append(b.Methods, m)
			}
		} else {
			p.errorAt(p.cur(), "an impl block can only contain functions")
			p.advance()
		}
		p.skipNewlines()
		if p.i == before {
			p.advance()
		}
	}
	p.expect(RBRACE, "'}' to close the impl block")
	p.endStmt()
	return b
}

func (p *Parser) parseFn() *FnDecl {
	kw := p.advance() // 'fn'
	name := p.expect(IDENT, "a function name")
	f := &FnDecl{pos: at(kw), Name: name.Lex}

	p.expect(LPAREN, "'('")
	p.skipNewlines()

	if !p.check(RPAREN) {
		for {
			// `self` is a parameter with no type: the impl block supplies it.
			if p.check(SELF) {
				sf := p.advance()
				f.Params = append(f.Params, Param{pos: at(sf), Name: "self"})
				p.skipNewlines()
				if !p.match(COMMA) {
					break
				}
				p.skipNewlines()
				continue
			}
			pn := p.expect(IDENT, "a parameter name")
			prm := Param{pos: at(pn), Name: pn.Lex}
			if p.match(COLON) {
				prm.Type = p.parseTypeRef()
			} else {
				p.errorAt(pn, "parameter %q needs a type, like %s: int", pn.Lex, pn.Lex)
			}
			f.Params = append(f.Params, prm)

			p.skipNewlines()
			if !p.match(COMMA) {
				break
			}
			p.skipNewlines()
		}
	}
	p.skipNewlines()
	p.expect(RPAREN, "')'")

	if p.match(ARROW) {
		f.Ret = p.parseTypeRef()
	}

	f.Body = p.parseBlock()
	p.endStmt()
	return f
}

// parseTypeRef reads a type annotation and returns its canonical source
// text. The checker turns that text into a Type; keeping it as a string
// here means the parser needs no knowledge of the type system.
//
//	int    []str    [][]int    {str: int}    {str: []int}
func (p *Parser) parseTypeRef() string {
	var base string
	switch {
	case p.match(QUESTION):
		base = "?" + p.parseTypeRef()

	case p.match(LBRACKET):
		p.expect(RBRACKET, "']' — a list type is written []int")
		base = "[]" + p.parseTypeRef()

	case p.match(LBRACE):
		key := p.parseTypeRef()
		p.expect(COLON, "':' — a map type is written {str: int}")
		val := p.parseTypeRef()
		p.expect(RBRACE, "'}'")
		base = "{" + key + ": " + val + "}"

	default:
		base = p.expect(IDENT, "a type name").Lex
	}

	// A trailing `!` makes it a result type: str! is a str or a failure.
	if p.match(BANG) {
		base += "!"
	}
	return base
}

// ---- statements ----

func (p *Parser) parseStmt() Stmt {
	switch p.cur().Kind {
	case LET, CONST:
		return p.parseLet()
	case IF:
		return p.parseIf()
	case WHILE:
		return p.parseWhile()
	case FOR:
		return p.parseFor()
	case MATCH:
		return p.parseMatch()
	case BREAK:
		t := p.advance()
		p.endStmt()
		return &BreakStmt{pos: at(t)}
	case CONTINUE:
		t := p.advance()
		p.endStmt()
		return &ContinueStmt{pos: at(t)}
	case RETURN:
		return p.parseReturn()
	case LBRACE:
		return p.parseBlock()
	case FN:
		p.errorAt(p.cur(), "functions can only be declared at the top level")
		p.synchronize()
		return nil
	case STRUCT, IMPL:
		p.errorAt(p.cur(), "'%s' can only appear at the top level", p.cur().Lex)
		p.synchronize()
		return nil
	default:
		return p.parseSimpleStmt()
	}
}

func (p *Parser) parseLet() Stmt {
	kw := p.advance()
	name := p.expect(IDENT, "a variable name")

	st := &LetStmt{pos: at(kw), Name: name.Lex, Const: kw.Kind == CONST}

	if p.match(COLON) {
		st.Type = p.parseTypeRef()
	}
	p.expect(ASSIGN, "'='")
	st.Value = p.parseExpr(0)
	p.endStmt()
	return st
}

func (p *Parser) parseReturn() Stmt {
	kw := p.advance()
	st := &ReturnStmt{pos: at(kw)}

	// A bare `return` is one followed immediately by a line break or '}'.
	if !p.check(NEWLINE) && !p.check(RBRACE) && !p.check(EOF) {
		st.Value = p.parseExpr(0)
	}
	p.endStmt()
	return st
}

func (p *Parser) parseIf() Stmt {
	kw := p.advance()
	st := &IfStmt{pos: at(kw)}
	st.Cond = p.header(func() Expr { return p.parseExpr(0) })
	st.Then = p.parseBlock()

	if p.check(ELSE) {
		p.advance()
		if p.check(IF) {
			st.Else = p.parseIf()
			return st // the nested if already consumed its terminator
		}
		st.Else = p.parseBlock()
	}
	p.endStmt()
	return st
}

func (p *Parser) parseWhile() Stmt {
	kw := p.advance()
	st := &WhileStmt{pos: at(kw)}
	st.Cond = p.header(func() Expr { return p.parseExpr(0) })
	st.Body = p.parseBlock()
	p.endStmt()
	return st
}

func (p *Parser) parseFor() Stmt {
	kw := p.advance()
	st := &ForStmt{pos: at(kw)}

	name := p.expect(IDENT, "a loop variable name")
	st.Var = name.Lex

	// `for k, v in map` binds two names.
	if p.match(COMMA) {
		st.Var2 = p.expect(IDENT, "a second loop variable name").Lex
	}

	if !p.match(IN) {
		p.errorAt(p.cur(), "expected 'in', as in: for %s in 0..10 { ... }", st.Var)
	}

	first := p.header(func() Expr { return p.parseExpr(0) })

	// A range if `..` or `..=` follows; otherwise a collection to iterate.
	switch {
	case p.match(DOTDOTEQ):
		st.Inclusive = true
	case p.match(DOTDOT):
	default:
		st.Coll = first
		if p.check(STEP) {
			p.errorAt(p.cur(), "'step' only applies to a range, as in: for i in 0..10 step 2")
		}
		st.Body = p.parseBlock()
		p.endStmt()
		return st
	}

	if st.Var2 != "" {
		p.errorAt(kw, "a range loop binds one variable, not two")
	}

	st.Start = first
	st.End = p.header(func() Expr { return p.parseExpr(0) })

	if p.match(STEP) {
		st.Step = p.header(func() Expr { return p.parseExpr(0) })
	}

	st.Body = p.parseBlock()
	p.endStmt()
	return st
}

// parseMatch reads a multi-way branch:
//
//	match code {
//	    200      => print("ok")
//	    404, 410 => print("gone")
//	    else     => print("something else")
//	}
//
// An arm's body is a single statement or a block. Arms do not fall
// through — there is no `break` to forget.
func (p *Parser) parseMatch() Stmt {
	kw := p.advance()
	st := &MatchStmt{pos: at(kw)}
	st.Subject = p.header(func() Expr { return p.parseExpr(0) })

	p.expect(LBRACE, "'{' to open the match")
	p.skipNewlines()

	for !p.check(RBRACE) && !p.check(EOF) {
		before := p.i

		if p.check(ELSE) {
			el := p.advance()
			p.expect(FATARROW, "'=>' after 'else'")
			if st.Else != nil {
				p.errorAt(el, "a match can only have one 'else' arm")
			}
			st.Else = p.parseArmBody()
			p.skipNewlines()
			if p.i == before {
				p.advance()
			}
			continue
		}

		arm := MatchCase{pos: at(p.cur())}
		for {
			arm.Values = append(arm.Values, p.header(func() Expr { return p.parseExpr(0) }))
			if !p.match(COMMA) {
				break
			}
			p.skipNewlines()
		}
		p.expect(FATARROW, "'=>' after the values of a match arm")
		arm.Body = p.parseArmBody()
		st.Cases = append(st.Cases, arm)

		p.skipNewlines()
		if p.i == before { // guarantee forward progress
			p.advance()
		}
	}

	p.expect(RBRACE, "'}' to close the match")
	p.endStmt()
	return st
}

func (p *Parser) parseArmBody() Stmt {
	if p.check(LBRACE) {
		return p.parseBlock()
	}
	return p.parseStmt()
}

func (p *Parser) parseBlock() *Block {
	lb := p.expect(LBRACE, "'{'")
	b := &Block{pos: at(lb)}
	p.skipNewlines()

	for !p.check(RBRACE) && !p.check(EOF) {
		before := p.i
		s := p.parseStmt()
		if s != nil {
			b.Stmts = append(b.Stmts, s)
		}
		if p.i == before {
			p.advance()
		}
		p.skipNewlines()
	}
	p.expect(RBRACE, "'}'")
	return b
}

// parseSimpleStmt handles both `foo(1)` and `x += 2`.
func (p *Parser) parseSimpleStmt() Stmt {
	start := p.cur()
	x := p.parseExpr(0)

	switch p.cur().Kind {
	case ASSIGN, PLUSEQ, MINUSEQ, STAREQ, SLASHEQ,
		PERCENTEQ, AMPEQ, PIPEEQ, CARETEQ, SHLEQ, SHREQ:
		op := p.advance()
		switch x.(type) {
		case *Ident, *Index, *Field:
		default:
			p.errorAt(start, "left side of assignment must be a variable, a field, or an index like xs[0]")
			p.synchronize()
			return nil
		}
		st := &AssignStmt{pos: at(start), Target: x, Op: op.Kind}
		st.Value = p.parseExpr(0)
		p.endStmt()
		return st
	}

	p.endStmt()
	return &ExprStmt{pos: at(start), X: x}
}

// endStmt enforces that a statement is followed by a newline, '}' or EOF.
// This is what lets Quartz skip semicolons.
func (p *Parser) endStmt() {
	if p.check(NEWLINE) {
		p.advance()
		return
	}
	if p.check(RBRACE) || p.check(EOF) {
		return
	}
	p.errorAt(p.cur(), "unexpected %s after statement", describe(p.cur()))
	p.synchronize()
}

// ---- expressions (Pratt) ----

// precOf returns binding power; 0 means "not a binary operator".
//
// The ladder follows C's, so anyone who has met `&` and `<<` before
// finds them where they expect. That does mean `a & b == c` parses as
// `a & (b == c)`, which is C's famous wart — the checker turns the
// resulting type error into a message suggesting parentheses.
func precOf(k Kind) int {
	switch k {
	case OR:
		return 1
	case AND:
		return 2
	case PIPE:
		return 3
	case CARET:
		return 4
	case AMP:
		return 5
	case EQ, NEQ:
		return 6
	case LT, LTE, GT, GTE:
		return 7
	case SHL, SHR:
		return 8
	case PLUS, MINUS:
		return 9
	case STAR, SLASH, PERCENT:
		return 10
	}
	return 0
}

func (p *Parser) parseExpr(minPrec int) Expr {
	left := p.parseUnary()

	for {
		prec := precOf(p.cur().Kind)
		if prec == 0 || prec < minPrec {
			break
		}
		op := p.advance()
		// prec+1 makes these left-associative: a-b-c parses as (a-b)-c
		right := p.parseExpr(prec + 1)
		left = &Binary{pos: at(op), Op: op.Kind, L: left, R: right}
	}
	return left
}

func (p *Parser) parseUnary() Expr {
	if p.check(BANG) || p.check(MINUS) || p.check(TILDE) {
		op := p.advance()
		return &Unary{pos: at(op), Op: op.Kind, X: p.parseUnary()}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() Expr {
	x := p.parsePrimary()
	for {
		switch {
		case p.check(LPAREN):
			lp := p.advance()
			call := &Call{pos: at(lp), Callee: x}
			p.skipNewlines()

			if !p.check(RPAREN) {
				for {
					call.Args = append(call.Args, p.grouped(func() Expr { return p.parseExpr(0) }))
					p.skipNewlines()
					if !p.match(COMMA) {
						break
					}
					p.skipNewlines()
				}
			}
			p.skipNewlines()
			p.expect(RPAREN, "')'")
			x = call

		case p.check(LBRACKET):
			lb := p.advance()
			p.skipNewlines()
			idx := p.grouped(func() Expr { return p.parseExpr(0) })
			p.skipNewlines()
			p.expect(RBRACKET, "']'")
			x = &Index{pos: at(lb), X: x, Idx: idx}

		case p.check(LBRACE) && p.noBrace == 0 && isStructLitTarget(x):
			x = p.parseStructLit(x.(*Ident))

		case p.check(QUESTION):
			q := p.advance()
			x = &Try{pos: at(q), X: x}

		case p.check(DOT):
			dot := p.advance()
			name := p.expect(IDENT, "a name after '.'")
			x = &Field{pos: at(dot), X: x, Name: name.Lex}

		default:
			return x
		}
	}
}

func (p *Parser) parsePrimary() Expr {
	t := p.cur()

	switch t.Kind {
	case NUMBER:
		p.advance()
		if strings.Contains(t.Lex, ".") {
			return &FloatLit{pos: at(t), Val: t.Lex}
		}
		return &IntLit{pos: at(t), Val: t.Lex}

	case STRING:
		p.advance()
		return p.parseStringLit(t)

	case TRUE:
		p.advance()
		return &BoolLit{pos: at(t), Val: true}

	case FALSE:
		p.advance()
		return &BoolLit{pos: at(t), Val: false}

	case NIL:
		p.advance()
		return &NilLit{pos: at(t)}

	case IDENT:
		p.advance()
		return &Ident{pos: at(t), Name: t.Lex}

	case SELF:
		// `self` is a keyword so it cannot be declared as a variable, but
		// inside a method it reads as an ordinary name.
		p.advance()
		return &Ident{pos: at(t), Name: "self"}

	case LPAREN:
		p.advance()
		p.skipNewlines()
		x := p.grouped(func() Expr { return p.parseExpr(0) })
		p.skipNewlines()
		p.expect(RPAREN, "')'")
		return x

	case LBRACKET:
		return p.parseListLit()

	case LBRACE:
		return p.parseMapLit()
	}

	p.errorAt(t, "expected an expression, found %s", describe(t))
	p.advance()
	return &StrLit{pos: at(t), Val: ""} // placeholder so later passes don't nil-panic
}

// isStructLitTarget reports whether `x {` should be read as a struct
// literal. Only a bare name can name a struct, and only a capitalised
// one by convention — but the convention is not enforced, so any plain
// identifier qualifies and the checker decides whether it exists.
func isStructLitTarget(x Expr) bool {
	_, ok := x.(*Ident)
	return ok
}

// parseStructLit reads the `{name: "ada", age: 36}` half of a struct
// literal, the name having already been consumed.
func (p *Parser) parseStructLit(name *Ident) Expr {
	line, col := name.Pos()
	lit := &StructLit{pos: pos{Line: line, Col: col}, Name: name.Name}

	p.advance() // '{'
	p.skipNewlines()

	for !p.check(RBRACE) && !p.check(EOF) {
		before := p.i

		fn := p.expect(IDENT, "a field name")
		p.expect(COLON, "':' after a field name")
		p.skipNewlines()
		lit.Fields = append(lit.Fields, fn.Lex)
		lit.Vals = append(lit.Vals, p.grouped(func() Expr { return p.parseExpr(0) }))

		p.skipNewlines()
		if !p.match(COMMA) {
			break
		}
		p.skipNewlines()
		if p.i == before {
			p.advance()
		}
	}
	p.skipNewlines()
	p.expect(RBRACE, "'}' to close the struct literal")
	return lit
}

// parseListLit reads `[]`, `[1, 2, 3]`, or a list spread over lines.
func (p *Parser) parseListLit() Expr {
	lb := p.advance() // '['
	lit := &ListLit{pos: at(lb)}
	p.skipNewlines()

	for !p.check(RBRACKET) && !p.check(EOF) {
		lit.Elems = append(lit.Elems, p.parseExpr(0))
		p.skipNewlines()
		if !p.match(COMMA) {
			break
		}
		p.skipNewlines()
	}
	p.skipNewlines()
	p.expect(RBRACKET, "']' to close the list")
	return lit
}

// parseMapLit reads `{}` or `{"a": 1, "b": 2}`.
//
// A map literal cannot start a statement, because `{` there opens a
// block. That costs nothing in practice — a bare map literal as a
// statement would do nothing anyway.
func (p *Parser) parseMapLit() Expr {
	lb := p.advance() // '{'
	lit := &MapLit{pos: at(lb)}
	p.skipNewlines()

	for !p.check(RBRACE) && !p.check(EOF) {
		lit.Keys = append(lit.Keys, p.parseExpr(0))
		p.expect(COLON, "':' between a map key and its value")
		p.skipNewlines()
		lit.Vals = append(lit.Vals, p.parseExpr(0))
		p.skipNewlines()
		if !p.match(COMMA) {
			break
		}
		p.skipNewlines()
	}
	p.skipNewlines()
	p.expect(RBRACE, "'}' to close the map")
	return lit
}

// parseStringLit splits "a {x} b" into literal and expression parts.
// Literal braces are written {{ and }}.
func (p *Parser) parseStringLit(t Token) Expr {
	src := t.Lex
	if !strings.ContainsAny(src, "{}") {
		return &StrLit{pos: at(t), Val: src}
	}

	var parts []InterpPart
	var lit strings.Builder

	flush := func() {
		if lit.Len() > 0 {
			parts = append(parts, InterpPart{Lit: lit.String()})
			lit.Reset()
		}
	}

	for i := 0; i < len(src); i++ {
		c := src[i]

		if c == '}' {
			if i+1 < len(src) && src[i+1] == '}' {
				lit.WriteByte('}')
				i++
				continue
			}
			p.errorAt(t, "unmatched '}' in string (write '}}' for a literal brace)")
			lit.WriteByte('}')
			continue
		}

		if c != '{' {
			lit.WriteByte(c)
			continue
		}
		if i+1 < len(src) && src[i+1] == '{' {
			lit.WriteByte('{')
			i++
			continue
		}

		// Find the matching close brace, stepping over nested braces and
		// over any string literal that appears inside the expression.
		j, d, quoted := i+1, 1, false
		for ; j < len(src); j++ {
			ch := src[j]
			if ch == '"' {
				quoted = !quoted
				continue
			}
			if quoted {
				continue
			}
			if ch == '{' {
				d++
			} else if ch == '}' {
				d--
				if d == 0 {
					break
				}
			}
		}
		if j >= len(src) {
			p.errorAt(t, "unclosed '{' in string")
			lit.WriteByte('{')
			continue
		}
		inner := strings.TrimSpace(src[i+1 : j])
		i = j

		if inner == "" {
			// Almost always someone writing JSON or a literal brace, now
			// that there is a json library to write it for.
			p.errorAt(t, "empty {} in string — for a literal brace write {{}}")
			continue
		}

		if x := p.parseSubExpr(inner, t); x != nil {
			flush()
			parts = append(parts, InterpPart{X: x})
		}
	}
	flush()

	if len(parts) == 1 && parts[0].X == nil {
		return &StrLit{pos: at(t), Val: parts[0].Lit}
	}
	return &Interp{pos: at(t), Parts: parts}
}

// parseSubExpr runs a nested lexer+parser over the text inside {}.
// Positions inside interpolations point at the enclosing string for now.
func (p *Parser) parseSubExpr(src string, host Token) Expr {
	lx := NewLexer(p.file, src)
	toks := lx.Scan()
	p.Errors = append(p.Errors, lx.Errors...)

	sub := NewParser(p.file, toks)
	sub.skipNewlines()
	x := sub.parseExpr(0)
	sub.skipNewlines()

	if !sub.check(EOF) {
		p.errorAt(host, "unexpected %s inside {} in string — for literal braces write {{ and }}",
			describe(sub.cur()))
	}
	p.Errors = append(p.Errors, sub.Errors...)

	// The nested lexer starts its own line and column count, so every
	// node in here would otherwise claim to be at 1:something. Point the
	// whole subtree at the string that contains it. That is coarse — one
	// position for the entire interpolation — but it is inside the right
	// line, which is what makes an error findable.
	reanchor(x, at(host))
	return x
}

// reanchor rewrites the position of an expression and everything under
// it. Nodes embed pos by value, so the pointer receiver reaches it.
func reanchor(e Expr, at pos) {
	if e == nil {
		return
	}
	if s, ok := e.(interface{ setPos(pos) }); ok {
		s.setPos(at)
	}
	switch x := e.(type) {
	case *Unary:
		reanchor(x.X, at)
	case *Binary:
		reanchor(x.L, at)
		reanchor(x.R, at)
	case *Call:
		reanchor(x.Callee, at)
		for _, a := range x.Args {
			reanchor(a, at)
		}
	case *Index:
		reanchor(x.X, at)
		reanchor(x.Idx, at)
	case *Field:
		reanchor(x.X, at)
	case *Try:
		reanchor(x.X, at)
	case *ListLit:
		for _, el := range x.Elems {
			reanchor(el, at)
		}
	case *MapLit:
		for i := range x.Keys {
			reanchor(x.Keys[i], at)
			reanchor(x.Vals[i], at)
		}
	case *StructLit:
		for _, v := range x.Vals {
			reanchor(v, at)
		}
	case *Interp:
		for _, part := range x.Parts {
			reanchor(part.X, at)
		}
	}
}
