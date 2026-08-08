package main

import (
	"fmt"
	"strings"
)

type Parser struct {
	toks   []Token
	i      int
	file   string
	Errors []string
}

func NewParser(file string, toks []Token) *Parser {
	return &Parser{toks: toks, file: file}
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

		if p.check(FN) {
			if f := p.parseFn(); f != nil {
				prog.Funcs = append(prog.Funcs, f)
			}
		} else if s := p.parseStmt(); s != nil {
			prog.Main = append(prog.Main, s)
		}

		if p.i == before { // guarantee forward progress
			p.advance()
		}
		p.skipNewlines()
	}
	return prog
}

func (p *Parser) parseFn() *FnDecl {
	kw := p.advance() // 'fn'
	name := p.expect(IDENT, "a function name")
	f := &FnDecl{pos: at(kw), Name: name.Lex}

	p.expect(LPAREN, "'('")
	p.skipNewlines()

	if !p.check(RPAREN) {
		for {
			pn := p.expect(IDENT, "a parameter name")
			prm := Param{pos: at(pn), Name: pn.Lex}
			if p.match(COLON) {
				prm.Type = p.expect(IDENT, "a type name").Lex
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
		f.Ret = p.expect(IDENT, "a return type").Lex
	}

	f.Body = p.parseBlock()
	p.endStmt()
	return f
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
	default:
		return p.parseSimpleStmt()
	}
}

func (p *Parser) parseLet() Stmt {
	kw := p.advance()
	name := p.expect(IDENT, "a variable name")

	st := &LetStmt{pos: at(kw), Name: name.Lex, Const: kw.Kind == CONST}

	if p.match(COLON) {
		st.Type = p.expect(IDENT, "a type name").Lex
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
	st.Cond = p.parseExpr(0)
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
	st.Cond = p.parseExpr(0)
	st.Body = p.parseBlock()
	p.endStmt()
	return st
}

func (p *Parser) parseFor() Stmt {
	kw := p.advance()
	st := &ForStmt{pos: at(kw)}

	name := p.expect(IDENT, "a loop variable name")
	st.Var = name.Lex

	if !p.match(IN) {
		p.errorAt(p.cur(), "expected 'in', as in: for %s in 0..10 { ... }", st.Var)
	}

	st.Start = p.parseExpr(0)

	switch {
	case p.match(DOTDOTEQ):
		st.Inclusive = true
	case p.match(DOTDOT):
	default:
		p.errorAt(p.cur(), "expected '..' or '..=' to give the loop a range")
	}
	st.End = p.parseExpr(0)

	if p.match(STEP) {
		st.Step = p.parseExpr(0)
	}

	st.Body = p.parseBlock()
	p.endStmt()
	return st
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
	case ASSIGN, PLUSEQ, MINUSEQ, STAREQ, SLASHEQ:
		op := p.advance()
		id, ok := x.(*Ident)
		if !ok {
			p.errorAt(start, "left side of assignment must be a variable")
			p.synchronize()
			return nil
		}
		st := &AssignStmt{pos: at(start), Name: id.Name, Op: op.Kind}
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
func precOf(k Kind) int {
	switch k {
	case OR:
		return 1
	case AND:
		return 2
	case EQ, NEQ:
		return 3
	case LT, LTE, GT, GTE:
		return 4
	case PLUS, MINUS:
		return 5
	case STAR, SLASH, PERCENT:
		return 6
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
	if p.check(BANG) || p.check(MINUS) {
		op := p.advance()
		return &Unary{pos: at(op), Op: op.Kind, X: p.parseUnary()}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() Expr {
	x := p.parsePrimary()
	for p.check(LPAREN) {
		lp := p.advance()
		call := &Call{pos: at(lp), Callee: x}
		p.skipNewlines()

		if !p.check(RPAREN) {
			for {
				call.Args = append(call.Args, p.parseExpr(0))
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
	}
	return x
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

	case IDENT:
		p.advance()
		return &Ident{pos: at(t), Name: t.Lex}

	case LPAREN:
		p.advance()
		p.skipNewlines()
		x := p.parseExpr(0)
		p.skipNewlines()
		p.expect(RPAREN, "')'")
		return x
	}

	p.errorAt(t, "expected an expression, found %s", describe(t))
	p.advance()
	return &StrLit{pos: at(t), Val: ""} // placeholder so later passes don't nil-panic
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
			p.errorAt(t, "empty {} in string")
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
		p.errorAt(host, "unexpected %s inside {} in string", describe(sub.cur()))
	}
	p.Errors = append(p.Errors, sub.Errors...)

	// Re-anchor to the host string so error positions stay in range.
	if id, ok := x.(*Ident); ok {
		id.pos = at(host)
	}
	return x
}
