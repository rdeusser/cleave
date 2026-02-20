package parser

import (
	"fmt"
	"strconv"

	"github.com/rdeusser/cleave/internal/ast"
	"github.com/rdeusser/cleave/internal/lexer"
	"github.com/rdeusser/cleave/internal/token"
)

type Parser struct {
	lex      *lexer.Lexer
	cur      token.Token
	peek     []token.Token
	errors   []Error
	comments []*ast.Comment
}

type Error struct {
	Pos     token.Position
	Message string
}

func New(file string, src []byte) *Parser {
	l := lexer.New(file, src)
	p := &Parser{lex: l}
	p.next() // prime first token
	return p
}

func (p *Parser) Parse() (*ast.File, []Error) {
	file := &ast.File{}

	if p.cur.Type == token.Package {
		file.Package = p.parsePackageDecl()
	} else {
		p.errorf("expected package declaration")
	}

	for p.cur.Type == token.Import {
		file.Imports = append(file.Imports, p.parseImportDecl())
	}

	for p.cur.Type != token.EOF {
		switch p.cur.Type {
		case token.Struct:
			file.Decls = append(file.Decls, p.parseStructDecl())
		case token.Enum:
			file.Decls = append(file.Decls, p.parseEnumDecl())
		case token.Format:
			file.Decls = append(file.Decls, p.parseFormatDecl())
		case token.Union:
			file.Decls = append(file.Decls, p.parseUnionDecl())
		default:
			p.errorf("expected declaration, got %s", p.cur.Type)
			p.synchronize()
		}
	}

	file.Comments = p.comments
	return file, p.errors
}

func (e Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Message)
}

func (p *Parser) parsePackageDecl() *ast.PackageDecl {
	decl := &ast.PackageDecl{Keyword: p.cur.Span.Start}
	p.expect(token.Package)
	decl.Name = p.parseIdent()
	decl.Semicolon = p.cur.Span.Start
	p.expect(token.Semicolon)
	return decl
}

func (p *Parser) parseImportDecl() *ast.ImportDecl {
	decl := &ast.ImportDecl{Keyword: p.cur.Span.Start}
	p.expect(token.Import)
	if p.cur.Type != token.String {
		p.errorf("expected string literal for import path, got %s", p.cur.Type)
		decl.Path = &ast.StringLit{Span: p.cur.Span, Value: ""}
	} else {
		decl.Path = p.parseStringLit()
	}
	decl.Semicolon = p.cur.Span.Start
	p.expect(token.Semicolon)
	return decl
}

func (p *Parser) parseStructDecl() *ast.StructDecl {
	s := &ast.StructDecl{Keyword: p.cur.Span.Start}
	p.expect(token.Struct)
	s.Name = p.parseIdent()
	s.LBrace = p.cur.Span.Start
	p.expect(token.LBrace)

	for p.cur.Type != token.RBrace && p.cur.Type != token.EOF {
		if p.cur.Type == token.Option {
			s.Options = append(s.Options, p.parseOptionBlock())
		} else if p.cur.Type == token.Ident {
			s.Fields = append(s.Fields, p.parseFieldDecl())
		} else {
			p.errorf("expected field or option declaration, got %s", p.cur.Type)
			p.synchronize()
		}
	}

	s.RBrace = p.cur.Span.Start
	p.expect(token.RBrace)
	return s
}

func (p *Parser) parseEnumDecl() *ast.EnumDecl {
	e := &ast.EnumDecl{Keyword: p.cur.Span.Start}
	p.expect(token.Enum)
	e.Name = p.parseIdent()
	e.Colon = p.cur.Span.Start
	p.expect(token.Colon)
	e.BackingType = p.parseTypeExpr()
	e.LBrace = p.cur.Span.Start
	p.expect(token.LBrace)

	for p.cur.Type != token.RBrace && p.cur.Type != token.EOF {
		if p.cur.Type == token.Ident {
			e.Variants = append(e.Variants, p.parseEnumVariant())
		} else {
			p.errorf("expected enum variant, got %s", p.cur.Type)
			p.synchronize()
		}
	}

	e.RBrace = p.cur.Span.Start
	p.expect(token.RBrace)
	return e
}

func (p *Parser) parseFormatDecl() *ast.FormatDecl {
	f := &ast.FormatDecl{Keyword: p.cur.Span.Start}
	p.expect(token.Format)
	f.Name = p.parseIdent()
	f.LBrace = p.cur.Span.Start
	p.expect(token.LBrace)

	for p.cur.Type != token.RBrace && p.cur.Type != token.EOF {
		kv := &ast.FormatKV{}
		kv.Key = p.parseIdent()
		kv.Assign = p.cur.Span.Start
		p.expect(token.Assign)
		kv.Value = p.parseValue()
		kv.Semicolon = p.cur.Span.Start
		p.expect(token.Semicolon)
		f.Entries = append(f.Entries, kv)
	}

	f.RBrace = p.cur.Span.Start
	p.expect(token.RBrace)
	return f
}

func (p *Parser) parseUnionDecl() *ast.UnionDecl {
	u := &ast.UnionDecl{Keyword: p.cur.Span.Start}
	p.expect(token.Union)
	u.Name = p.parseIdent()
	u.Colon = p.cur.Span.Start
	p.expect(token.Colon)
	u.BackingType = p.parseTypeExpr()
	u.LBrace = p.cur.Span.Start
	p.expect(token.LBrace)

	for p.cur.Type != token.RBrace && p.cur.Type != token.EOF {
		u.Cases = append(u.Cases, p.parseUnionCase())
	}

	if len(u.Cases) == 0 {
		p.errorf("union must have at least one case")
	}

	u.RBrace = p.cur.Span.Start
	p.expect(token.RBrace)
	return u
}

func (p *Parser) parseUnionCase() *ast.UnionCase {
	c := &ast.UnionCase{}

	switch {
	case p.cur.Type == token.Ident && p.cur.Literal == "_":
		c.Value = &ast.Ident{Span: p.cur.Span, Name: "_"}
		p.next()
	case p.cur.Type == token.Minus:
		minusPos := p.cur.Span.Start
		p.next()
		if p.cur.Type != token.Integer {
			p.errorf("expected integer after '-', got %s", p.cur.Type)
			c.Value = &ast.IntegerLit{Span: p.cur.Span, Value: 0, Raw: "0"}
		} else {
			lit := p.parseIntegerLit()
			lit.Value = -lit.Value
			lit.Raw = "-" + lit.Raw
			lit.Span.Start = minusPos
			c.Value = lit
		}
	case p.cur.Type == token.Integer:
		c.Value = p.parseIntegerLit()
	default:
		p.errorf("expected integer, '-', or '_' in union case, got %s", p.cur.Type)
		c.Value = &ast.IntegerLit{Span: p.cur.Span, Value: 0, Raw: "0"}
		p.next()
	}

	c.Arrow = p.cur.Span.Start
	p.expect(token.FatArrow)
	c.Type = p.parseTypeExpr()
	c.Semicolon = p.cur.Span.Start
	p.expect(token.Semicolon)
	return c
}

func (p *Parser) parseMatchExpr() *ast.MatchExpr {
	m := &ast.MatchExpr{Keyword: p.cur.Span.Start}
	p.expect(token.Match)
	m.Tag = p.parseIdent()
	m.LBrace = p.cur.Span.Start
	p.expect(token.LBrace)

	for p.cur.Type != token.RBrace && p.cur.Type != token.EOF {
		m.Cases = append(m.Cases, p.parseUnionCase())
	}

	if len(m.Cases) == 0 {
		p.errorf("match must have at least one case")
	}

	m.RBrace = p.cur.Span.Start
	p.expect(token.RBrace)
	return m
}

func (p *Parser) parseEnumVariant() *ast.EnumVariant {
	v := &ast.EnumVariant{}
	v.Name = p.parseIdent()
	v.Assign = p.cur.Span.Start
	p.expect(token.Assign)
	v.Value = p.parseExpr()
	v.Semicolon = p.cur.Span.Start
	p.expect(token.Semicolon)
	return v
}

func (p *Parser) parseFieldDecl() *ast.FieldDecl {
	f := &ast.FieldDecl{}
	f.Name = p.parseIdent()

	if p.cur.Type == token.Match {
		f.Match = p.parseMatchExpr()
	} else {
		f.Type = p.parseTypeExpr()

		// Disambiguate: after type, [ could start field options or array dimension.
		// If the type already has a dimension (from parseTypeExpr), then [ here must be field options.
		// If we see [ after a bare type name, peek: IDENT = means field options, else array dim.
		if p.cur.Type == token.LBracket {
			f.Options = p.parseFieldOptions()
		}
	}

	f.Semicolon = p.cur.Span.Start
	p.expect(token.Semicolon)
	return f
}

func (p *Parser) parseTypeExpr() *ast.TypeExpr {
	te := &ast.TypeExpr{}
	te.Name = p.parseIdent()

	if p.cur.Type == token.LBracket && !p.isFieldOptionStart() {
		te.LBracket = p.cur.Span.Start
		p.expect(token.LBracket)
		te.Dim = p.parseExpr()

		if p.cur.Type == token.Comma {
			te.InlineOptions = p.parseInlineOptions()
		}

		te.RBracket = p.cur.Span.Start
		p.expect(token.RBracket)
	}

	return te
}

func (p *Parser) parseInlineOptions() []*ast.FieldOption {
	var opts []*ast.FieldOption
	for p.cur.Type == token.Comma {
		p.next() // consume comma
		opt := &ast.FieldOption{}
		opt.Key = p.parseIdent()
		opt.Assign = p.cur.Span.Start
		p.expect(token.Assign)
		opt.Value = p.parseValue()
		opts = append(opts, opt)
	}
	return opts
}

func (p *Parser) isFieldOptionStart() bool {
	if p.cur.Type != token.LBracket {
		return false
	}
	t1 := p.peekAt(0) // token after [
	if t1.Type == token.LParen {
		return true
	}
	t2 := p.peekAt(1) // token after that
	return t1.Type == token.Ident && t2.Type == token.Assign
}

func (p *Parser) parseFieldOptions() []*ast.FieldOption {
	p.expect(token.LBracket)
	var opts []*ast.FieldOption

	for p.cur.Type != token.RBracket && p.cur.Type != token.EOF {
		opt := &ast.FieldOption{}

		if p.cur.Type == token.LParen {
			opt.LParen = p.cur.Span.Start
			p.expect(token.LParen)
			opt.Namespace = p.parseIdent()
			opt.RParen = p.cur.Span.Start
			p.expect(token.RParen)
			opt.Dot = p.cur.Span.Start
			p.expect(token.Dot)
		}

		opt.Key = p.parseIdent()
		opt.Assign = p.cur.Span.Start
		p.expect(token.Assign)
		opt.Value = p.parseValue()
		opts = append(opts, opt)

		if p.cur.Type == token.Comma {
			p.next()
		}
	}

	p.expect(token.RBracket)
	return opts
}

func (p *Parser) parseOptionBlock() *ast.OptionBlock {
	ob := &ast.OptionBlock{Keyword: p.cur.Span.Start}
	p.expect(token.Option)
	ob.LParen = p.cur.Span.Start
	p.expect(token.LParen)
	ob.Namespace = p.parseIdent()
	ob.RParen = p.cur.Span.Start
	p.expect(token.RParen)
	ob.Assign = p.cur.Span.Start
	p.expect(token.Assign)
	ob.LBrace = p.cur.Span.Start
	p.expect(token.LBrace)

	for p.cur.Type != token.RBrace && p.cur.Type != token.EOF {
		kv := &ast.OptionKV{}
		kv.Key = p.parseIdent()
		kv.Assign = p.cur.Span.Start
		p.expect(token.Assign)
		kv.Value = p.parseValue()
		kv.Semicolon = p.cur.Span.Start
		p.expect(token.Semicolon)
		ob.Entries = append(ob.Entries, kv)
	}

	ob.RBrace = p.cur.Span.Start
	p.expect(token.RBrace)
	ob.Semicolon = p.cur.Span.Start
	p.expect(token.Semicolon)
	return ob
}

func (p *Parser) parseExpr() ast.Expr {
	switch p.cur.Type {
	case token.Integer:
		return p.parseIntegerLit()
	case token.Minus:
		minusPos := p.cur.Span.Start
		p.next()
		if p.cur.Type != token.Integer {
			p.errorf("expected integer after '-', got %s", p.cur.Type)
			return &ast.IntegerLit{Span: p.cur.Span, Value: 0, Raw: "0"}
		}
		lit := p.parseIntegerLit()
		lit.Value = -lit.Value
		lit.Raw = "-" + lit.Raw
		lit.Span.Start = minusPos
		return lit
	case token.Ident:
		return p.parseIdentExpr()
	default:
		p.errorf("expected expression, got %s", p.cur.Type)
		return &ast.IntegerLit{Span: p.cur.Span, Value: 0, Raw: "0"}
	}
}

func (p *Parser) parseValue() ast.Expr {
	switch p.cur.Type {
	case token.Integer:
		return p.parseIntegerLit()
	case token.String:
		return p.parseStringLit()
	case token.Ident:
		ident := p.parseIdent()
		if p.cur.Type == token.Dot {
			return p.parseDottedIdent(ident)
		}
		return ident
	case token.True:
		lit := &ast.BoolLit{Span: p.cur.Span, Value: true}
		p.next()
		return lit
	case token.False:
		lit := &ast.BoolLit{Span: p.cur.Span, Value: false}
		p.next()
		return lit
	case token.LBrace:
		return p.parseBlockExpr()
	default:
		p.errorf("expected value, got %s", p.cur.Type)
		return &ast.Ident{Span: p.cur.Span, Name: ""}
	}
}

func (p *Parser) parseBlockExpr() *ast.BlockExpr {
	block := &ast.BlockExpr{LBrace: p.cur.Span.Start}
	p.expect(token.LBrace)

	for p.cur.Type != token.RBrace && p.cur.Type != token.EOF {
		entry := &ast.BlockEntry{}
		entry.Key = p.parseIdent()
		entry.Colon = p.cur.Span.Start
		p.expect(token.Colon)
		entry.Value = p.parseValue()
		block.Entries = append(block.Entries, entry)
	}

	block.RBrace = p.cur.Span.Start
	p.expect(token.RBrace)
	return block
}

func (p *Parser) parseDottedIdent(first *ast.Ident) *ast.DottedIdent {
	parts := []*ast.Ident{first}
	for p.cur.Type == token.Dot {
		p.next() // consume .
		parts = append(parts, p.parseIdent())
	}
	return &ast.DottedIdent{Parts: parts}
}

func (p *Parser) parseIntegerLit() *ast.IntegerLit {
	lit := &ast.IntegerLit{Span: p.cur.Span, Raw: p.cur.Literal}
	val, err := strconv.ParseInt(p.cur.Literal, 0, 64)
	if err != nil {
		p.errorf("invalid integer literal %q", p.cur.Literal)
	}
	lit.Value = val
	p.next()
	return lit
}

func (p *Parser) parseStringLit() *ast.StringLit {
	raw := p.cur.Literal
	// Strip quotes.
	value := raw
	if len(raw) >= 2 {
		value = raw[1 : len(raw)-1]
	}
	lit := &ast.StringLit{Span: p.cur.Span, Value: value}
	p.next()
	return lit
}

func (p *Parser) parseIdentExpr() *ast.Ident {
	return p.parseIdent()
}

func (p *Parser) parseIdent() *ast.Ident {
	if p.cur.Type != token.Ident {
		p.errorf("expected identifier, got %s", p.cur.Type)
		ident := &ast.Ident{Span: p.cur.Span, Name: ""}
		return ident
	}
	ident := &ast.Ident{Span: p.cur.Span, Name: p.cur.Literal}
	p.next()
	return ident
}

func (p *Parser) expect(typ token.Type) {
	if p.cur.Type != typ {
		p.errorf("expected %s, got %s", typ, p.cur.Type)
		return
	}
	p.next()
}

func (p *Parser) next() {
	if len(p.peek) > 0 {
		p.cur = p.peek[0]
		p.peek = p.peek[1:]
	} else {
		p.cur = p.lex.Next()
	}
	// Skip and collect comments.
	for p.cur.Type == token.Comment {
		p.comments = append(p.comments, &ast.Comment{
			Span: p.cur.Span,
			Text: p.cur.Literal,
		})
		if len(p.peek) > 0 {
			p.cur = p.peek[0]
			p.peek = p.peek[1:]
		} else {
			p.cur = p.lex.Next()
		}
	}
}

func (p *Parser) peekAt(n int) token.Token {
	for len(p.peek) <= n {
		tok := p.lex.Next()
		for tok.Type == token.Comment {
			p.comments = append(p.comments, &ast.Comment{
				Span: tok.Span,
				Text: tok.Literal,
			})
			tok = p.lex.Next()
		}
		p.peek = append(p.peek, tok)
	}
	return p.peek[n]
}

func (p *Parser) errorf(format string, args ...any) {
	pos := p.lex.Position(p.cur.Span.Start)
	p.errors = append(p.errors, Error{Pos: pos, Message: fmt.Sprintf(format, args...)})
}

func (p *Parser) synchronize() {
	for p.cur.Type != token.EOF {
		if p.cur.Type == token.Semicolon {
			p.next()
			return
		}
		if p.cur.Type == token.RBrace {
			return
		}
		if p.cur.Type == token.Struct || p.cur.Type == token.Enum || p.cur.Type == token.Format || p.cur.Type == token.Union || p.cur.Type == token.Import {
			return
		}
		p.next()
	}
}
