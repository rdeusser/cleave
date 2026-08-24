package ast

import "github.com/rdeusser/cleave/internal/token"

type Node interface {
	nodeSpan() token.Span
}

type Decl interface {
	Node
	declNode()
}

type Expr interface {
	Node
	exprNode()
}

type File struct {
	Package       *PackageDecl
	Imports       []*ImportDecl
	Decls         []Decl
	ImportedDecls []Decl // declarations from imported files, for type lookup only
	Comments      []*Comment
}

type PackageDecl struct {
	Keyword   token.Pos
	Name      *Ident
	Semicolon token.Pos
}

type ImportDecl struct {
	Keyword   token.Pos
	Path      *StringLit
	Semicolon token.Pos
}

type StructDecl struct {
	Keyword token.Pos
	Name    *Ident
	LBrace  token.Pos
	Fields  []*FieldDecl
	Options []*OptionBlock
	RBrace  token.Pos
}

type EnumDecl struct {
	Keyword     token.Pos
	Name        *Ident
	Colon       token.Pos
	BackingType *TypeExpr
	LBrace      token.Pos
	Variants    []*EnumVariant
	RBrace      token.Pos
}

type FormatDecl struct {
	Keyword token.Pos
	Name    *Ident
	LBrace  token.Pos
	Entries []*FormatKV
	RBrace  token.Pos
}

type FormatKV struct {
	Key       *Ident
	Assign    token.Pos
	Value     Expr
	Semicolon token.Pos
}

type UnionDecl struct {
	Keyword     token.Pos
	Name        *Ident
	Colon       token.Pos
	BackingType *TypeExpr
	LBrace      token.Pos
	Cases       []*UnionCase
	RBrace      token.Pos
}

type UnionCase struct {
	Value     Expr      // IntegerLit for valued cases, Ident with Name "_" for default
	Arrow     token.Pos // position of =>
	Type      *TypeExpr
	Semicolon token.Pos
}

type MatchExpr struct {
	Keyword token.Pos
	Tag     *Ident
	LBrace  token.Pos
	Cases   []*UnionCase
	RBrace  token.Pos
}

type EnumVariant struct {
	Name      *Ident
	Assign    token.Pos
	Value     Expr
	Semicolon token.Pos
}

type FieldDecl struct {
	Name      *Ident
	Type      *TypeExpr  // set for regular fields
	Match     *MatchExpr // set for inline match fields
	Options   []*FieldOption
	Semicolon token.Pos
}

type TypeExpr struct {
	Name          *Ident
	LBracket      token.Pos
	Dim           Expr
	InlineOptions []*FieldOption
	RBracket      token.Pos
}

// For namespaced options like (builtin).cel, Namespace is set and LParen/RParen/Dot are populated.
type FieldOption struct {
	LParen    token.Pos // zero if no namespace
	Namespace *Ident    // nil if simple key
	RParen    token.Pos
	Dot       token.Pos
	Key       *Ident
	Assign    token.Pos
	Value     Expr
}

type BlockEntry struct {
	Key   *Ident
	Colon token.Pos
	Value Expr
}

type BlockExpr struct {
	LBrace  token.Pos
	Entries []*BlockEntry
	RBrace  token.Pos
}

type OptionBlock struct {
	Keyword   token.Pos
	LParen    token.Pos
	Namespace *Ident
	RParen    token.Pos
	Assign    token.Pos
	LBrace    token.Pos
	Entries   []*OptionKV
	RBrace    token.Pos
	Semicolon token.Pos
}

type OptionKV struct {
	Key       *Ident
	Assign    token.Pos
	Value     Expr
	Semicolon token.Pos
}

type Comment struct {
	Span token.Span
	Text string
}

type Ident struct {
	Span token.Span
	Name string
}

type DottedIdent struct {
	Parts []*Ident
}

type IntegerLit struct {
	Span  token.Span
	Value int64
	Raw   string
}

type StringLit struct {
	Span  token.Span
	Value string
}

type BoolLit struct {
	Span  token.Span
	Value bool
}

func (f *File) nodeSpan() token.Span {
	if f.Package != nil {
		return f.Package.nodeSpan()
	}
	return token.Span{}
}

func (p *PackageDecl) nodeSpan() token.Span {
	return token.Span{Start: p.Keyword, End: p.Semicolon + 1}
}

func (d *ImportDecl) nodeSpan() token.Span { return token.Span{Start: d.Keyword, End: d.Semicolon + 1} }

func (s *StructDecl) nodeSpan() token.Span { return token.Span{Start: s.Keyword, End: s.RBrace + 1} }
func (s *StructDecl) declNode()            {}

func (e *EnumDecl) nodeSpan() token.Span { return token.Span{Start: e.Keyword, End: e.RBrace + 1} }
func (e *EnumDecl) declNode()            {}

func (f *FormatDecl) nodeSpan() token.Span { return token.Span{Start: f.Keyword, End: f.RBrace + 1} }
func (f *FormatDecl) declNode()            {}

func (kv *FormatKV) nodeSpan() token.Span {
	return token.Span{Start: kv.Key.Span.Start, End: kv.Semicolon + 1}
}

func (u *UnionDecl) nodeSpan() token.Span { return token.Span{Start: u.Keyword, End: u.RBrace + 1} }
func (u *UnionDecl) declNode()            {}

func (c *UnionCase) nodeSpan() token.Span {
	return token.Span{Start: c.Value.nodeSpan().Start, End: c.Semicolon + 1}
}

func (m *MatchExpr) nodeSpan() token.Span { return token.Span{Start: m.Keyword, End: m.RBrace + 1} }

func (v *EnumVariant) nodeSpan() token.Span {
	return token.Span{Start: v.Name.Span.Start, End: v.Semicolon + 1}
}

func (f *FieldDecl) nodeSpan() token.Span {
	return token.Span{Start: f.Name.Span.Start, End: f.Semicolon + 1}
}

func (t *TypeExpr) nodeSpan() token.Span {
	if t.RBracket != token.NoPos {
		return token.Span{Start: t.Name.Span.Start, End: t.RBracket + 1}
	}
	return t.Name.nodeSpan()
}

func (o *FieldOption) nodeSpan() token.Span {
	start := o.Key.Span.Start
	if o.LParen != token.NoPos {
		start = o.LParen
	}
	return token.Span{Start: start, End: o.Value.nodeSpan().End}
}

func (e *BlockEntry) nodeSpan() token.Span {
	return token.Span{Start: e.Key.Span.Start, End: e.Value.nodeSpan().End}
}

func (b *BlockExpr) nodeSpan() token.Span { return token.Span{Start: b.LBrace, End: b.RBrace + 1} }
func (b *BlockExpr) exprNode()            {}

func (o *OptionBlock) nodeSpan() token.Span {
	return token.Span{Start: o.Keyword, End: o.Semicolon + 1}
}

func (kv *OptionKV) nodeSpan() token.Span {
	return token.Span{Start: kv.Key.Span.Start, End: kv.Semicolon + 1}
}

func (c *Comment) nodeSpan() token.Span { return c.Span }

func (i *Ident) nodeSpan() token.Span { return i.Span }
func (i *Ident) exprNode()            {}

func (d *DottedIdent) nodeSpan() token.Span {
	return token.Span{Start: d.Parts[0].Span.Start, End: d.Parts[len(d.Parts)-1].Span.End}
}

func (d *DottedIdent) exprNode() {}

func (l *IntegerLit) nodeSpan() token.Span { return l.Span }
func (l *IntegerLit) exprNode()            {}

func (l *StringLit) nodeSpan() token.Span { return l.Span }
func (l *StringLit) exprNode()            {}

func (l *BoolLit) nodeSpan() token.Span { return l.Span }
func (l *BoolLit) exprNode()            {}
