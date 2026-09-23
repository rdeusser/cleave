package format

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rdeusser/cleave/internal/ast"
	"github.com/rdeusser/cleave/internal/token"
)

// printer lays out one parsed file and records which comments it has printed.
type printer struct {
	lineStarts []int          // offset of the first byte of each source line
	comments   []*ast.Comment // ordered by position
	isPlaced   []bool         // isPlaced[i] reports whether comments[i] is in the output
}

func newPrinter(src []byte, comments []*ast.Comment) *printer {
	p := &printer{
		lineStarts: []int{0},
		comments:   comments,
		isPlaced:   make([]bool, len(comments)),
	}
	for i, b := range src {
		if b == '\n' {
			p.lineStarts = append(p.lineStarts, i+1)
		}
	}
	// within binary-searches comments by position.
	slices.SortFunc(p.comments, func(a, b *ast.Comment) int { return cmp.Compare(a.Span.Start, b.Span.Start) })
	return p
}

// file lays out a whole file. end is the length of the source, so the
// comments after the last declaration print too.
func (p *printer) file(f *ast.File, end token.Pos) []line {
	items := []item{{
		span:  ast.SpanOf(f.Package),
		cells: []string{"package " + f.Package.Name.Name + ";"},
	}}
	for i, imp := range f.Imports {
		items = append(items, item{
			span:        ast.SpanOf(imp),
			cells:       []string{"import " + valueString(imp.Path) + ";"},
			isSeparated: i == 0,
		})
	}
	for _, d := range f.Decls {
		it := p.decl(d)
		it.isSeparated = true
		items = append(items, it)
	}
	return p.list(0, end, items)
}

func (p *printer) decl(d ast.Decl) item {
	switch d := d.(type) {
	case *ast.StructDecl:
		return p.structDecl(d)
	case *ast.EnumDecl:
		return p.enumDecl(d)
	case *ast.UnionDecl:
		return p.unionDecl(d)
	case *ast.FormatDecl:
		return p.formatDecl(d)
	default:
		panic(fmt.Sprintf("format: unexpected declaration %T", d))
	}
}

func (p *printer) structDecl(s *ast.StructDecl) item {
	// Fields and option blocks keep their source order. A blank line sets each
	// option block apart from its neighbors.
	members := make([]item, 0, len(s.Fields)+len(s.Options))
	fields, options := s.Fields, s.Options
	wasOption := false
	for len(fields) > 0 || len(options) > 0 {
		isOption := len(fields) == 0 || (len(options) > 0 && options[0].Keyword < fields[0].Name.Span.Start)
		var member item
		if isOption {
			member, options = p.optionBlock(options[0]), options[1:]
		} else {
			member, fields = p.field(fields[0]), fields[1:]
		}
		member.isSeparated = isOption || wasOption
		wasOption = isOption
		members = append(members, member)
	}

	header := item{span: ast.SpanOf(s), cells: []string{"struct " + s.Name.Name + " {"}}
	return p.nest(header, token.Span{Start: s.LBrace, End: s.RBrace + 1}, members, "}")
}

func (p *printer) enumDecl(e *ast.EnumDecl) item {
	variants := make([]item, len(e.Variants))
	for i, v := range e.Variants {
		variants[i] = item{span: ast.SpanOf(v), cells: []string{v.Name.Name, "= " + valueString(v.Value) + ";"}}
	}

	header := item{span: ast.SpanOf(e), cells: []string{"enum " + e.Name.Name + " : " + typeString(e.BackingType) + " {"}}
	return p.nest(header, token.Span{Start: e.LBrace, End: e.RBrace + 1}, variants, "}")
}

func (p *printer) unionDecl(u *ast.UnionDecl) item {
	header := item{span: ast.SpanOf(u), cells: []string{"union " + u.Name.Name + " : " + typeString(u.BackingType) + " {"}}
	return p.nest(header, token.Span{Start: u.LBrace, End: u.RBrace + 1}, cases(u.Cases), "}")
}

func (p *printer) formatDecl(f *ast.FormatDecl) item {
	entries := make([]item, len(f.Entries))
	for i, kv := range f.Entries {
		entries[i] = p.assignment(ast.SpanOf(kv), kv.Key.Name, kv.Value, ";")
	}

	header := item{span: ast.SpanOf(f), cells: []string{"format " + f.Name.Name + " {"}}
	return p.nest(header, token.Span{Start: f.LBrace, End: f.RBrace + 1}, entries, "}")
}

func (p *printer) optionBlock(o *ast.OptionBlock) item {
	entries := make([]item, len(o.Entries))
	for i, kv := range o.Entries {
		entries[i] = p.assignment(ast.SpanOf(kv), kv.Key.Name, kv.Value, ";")
	}

	header := item{span: ast.SpanOf(o), cells: []string{"option (" + o.Namespace.Name + ") = {"}}
	return p.nest(header, token.Span{Start: o.LBrace, End: o.RBrace + 1}, entries, "};")
}

// field lays out a field as its name, its type, and its option list, each in
// its own column. An inline match prints its cases one per line.
func (p *printer) field(f *ast.FieldDecl) item {
	it := item{span: ast.SpanOf(f), cells: []string{f.Name.Name}}
	if m := f.Match; m != nil {
		it.cells = append(it.cells, "match "+m.Tag.Name+" {")
		return p.nest(it, token.Span{Start: m.LBrace, End: m.RBrace + 1}, cases(m.Cases), "};")
	}

	typ := typeString(f.Type)
	if len(f.Options) == 0 {
		it.cells = append(it.cells, typ+";")
		return it
	}

	opts := make([]string, len(f.Options))
	for i, o := range f.Options {
		opts[i] = optionKey(o) + " = " + valueString(o.Value)
	}
	flat := "[" + strings.Join(opts, ", ") + "];"

	hasBlock := slices.ContainsFunc(f.Options, func(o *ast.FieldOption) bool {
		_, isBlock := o.Value.(*ast.BlockExpr)
		return isBlock
	})
	hasComment := p.hasComments(f.LBracket, f.RBracket)
	fits := utf8.RuneCountInString(indentUnit+f.Name.Name+" "+typ+" "+flat) <= maxWidth
	if !hasBlock && !hasComment && fits {
		it.cells = append(it.cells, typ, flat)
		return it
	}

	options := make([]item, len(f.Options))
	for i, o := range f.Options {
		separator := ","
		if i == len(f.Options)-1 {
			separator = ""
		}
		options[i] = p.assignment(ast.SpanOf(o), optionKey(o), o.Value, separator)
	}
	it.cells = append(it.cells, typ, "[")
	return p.nest(it, token.Span{Start: f.LBracket, End: f.RBracket + 1}, options, "];")
}

// assignment lays out "key = value" followed by terminator. A block value
// opens on the key's line and prints its entries one per line.
func (p *printer) assignment(span token.Span, key string, value ast.Expr, terminator string) item {
	block, isBlock := value.(*ast.BlockExpr)
	if !isBlock {
		return item{span: span, cells: []string{key, "= " + valueString(value) + terminator}}
	}
	header := item{span: span, cells: []string{key, "= {"}}
	return p.nest(header, token.Span{Start: block.LBrace, End: block.RBrace + 1}, p.entries(block), "}"+terminator)
}

// entries lays out the entries of a block value, one "key: value" per line.
func (p *printer) entries(b *ast.BlockExpr) []item {
	items := make([]item, len(b.Entries))
	for i, e := range b.Entries {
		it := item{span: ast.SpanOf(e), cells: []string{e.Key.Name + ":"}}
		block, isBlock := e.Value.(*ast.BlockExpr)
		if !isBlock {
			it.cells = append(it.cells, valueString(e.Value))
			items[i] = it
			continue
		}
		it.cells = append(it.cells, "{")
		items[i] = p.nest(it, token.Span{Start: block.LBrace, End: block.RBrace + 1}, p.entries(block), "}")
	}
	return items
}

// nest gives it a body between brackets, which spans from the opening bracket
// through the closing one. items are the entries of the body, and closer is
// the text of the closing line. A comment after the opening bracket stays on
// the opening line. A body with no entries and no comments closes on the
// opening line.
func (p *printer) nest(it item, brackets token.Span, items []item, closer string) item {
	start, end := brackets.Start+1, brackets.End-1
	first := end
	if len(items) > 0 {
		first = items[0].span.Start
	}
	note := p.takeLine(start, first, p.line(brackets.Start))
	body := p.list(start, end, items)
	if len(note) == 0 && len(body) == 0 {
		it.cells[len(it.cells)-1] += closer
		return it
	}

	if len(note) > 0 {
		it.note = inline(note)
	}
	it.tail = append(indent(body), line{text: closer})
	return it
}

// list lays out items, the entries of one body or of the whole file, and
// places the unprinted comments in [start, end) around them. A comment goes
// on the line of the entry before it when it starts there, and above the next
// entry otherwise. Comments after the last entry print at the end.
func (p *printer) list(start, end token.Pos, items []item) []line {
	rows := make([]row, 0, len(items))
	last := -1 // source line where the output so far ends, or -1 before the first entry
	prev := start
	for i, it := range items {
		r := row{item: it}
		r.above, last = p.commentLines(p.take(prev, it.span.Start), last)
		if last >= 0 && p.line(it.span.Start) > last+1 {
			r.above = append(r.above, line{})
		}
		// Comments inside an entry that printed on one line, such as in a
		// type, have no place of their own, so they go above the entry.
		interior, _ := p.commentLines(p.take(it.span.Start, it.span.End), -1)
		r.above = append(r.above, interior...)

		needsBlank := it.isSeparated && i > 0
		hasBlank := len(r.above) > 0 && r.above[0].isBlank()
		if needsBlank && !hasBlank {
			r.above = slices.Insert(r.above, 0, line{})
		}

		last = p.line(it.span.End - 1)
		next := end
		if i+1 < len(items) {
			next = items[i+1].span.Start
		}
		if trailing := p.takeLine(it.span.End, next, last); len(trailing) > 0 {
			r.trailing = inline(trailing)
			last = p.line(trailing[len(trailing)-1].Span.End - 1)
		}

		prev = it.span.End
		rows = append(rows, r)
	}

	dangling, _ := p.commentLines(p.take(prev, end), last)
	return append(layout(rows), dangling...)
}

// commentLines prints cs one source line at a time, with comments that share
// a line joined by spaces. One blank line goes wherever the source has at
// least one before a comment. last is the source line where the preceding
// output ends, or -1 when nothing precedes cs. commentLines returns the lines
// and the source line where the last comment ends.
func (p *printer) commentLines(cs []*ast.Comment, last int) ([]line, int) {
	var lines []line
	for len(cs) > 0 {
		n := 1
		for n < len(cs) && p.line(cs[n].Span.Start) == p.line(cs[n-1].Span.End-1) {
			n++
		}
		if last >= 0 && p.line(cs[0].Span.Start) > last+1 {
			lines = append(lines, line{})
		}
		lines = attach(append(lines, line{}), inline(cs[:n]), "")
		last = p.line(cs[n-1].Span.End - 1)
		cs = cs[n:]
	}
	return lines, last
}

// line returns the zero-based source line that holds pos.
func (p *printer) line(pos token.Pos) int { return sort.SearchInts(p.lineStarts, int(pos)+1) - 1 }

// take marks the unprinted comments that start in [start, end) as printed and
// returns them.
func (p *printer) take(start, end token.Pos) []*ast.Comment {
	var cs []*ast.Comment
	for _, i := range p.within(start, end) {
		p.isPlaced[i] = true
		cs = append(cs, p.comments[i])
	}
	return cs
}

// takeLine is take limited to the comments that start on source line n.
func (p *printer) takeLine(start, end token.Pos, n int) []*ast.Comment {
	var cs []*ast.Comment
	for _, i := range p.within(start, end) {
		if p.line(p.comments[i].Span.Start) == n {
			p.isPlaced[i] = true
			cs = append(cs, p.comments[i])
		}
	}
	return cs
}

// hasComments reports whether an unprinted comment starts in [start, end).
func (p *printer) hasComments(start, end token.Pos) bool { return len(p.within(start, end)) > 0 }

// within returns the indexes of the unprinted comments that start in [start, end).
func (p *printer) within(start, end token.Pos) []int {
	var indexes []int
	first := sort.Search(len(p.comments), func(i int) bool { return p.comments[i].Span.Start >= start })
	for i := first; i < len(p.comments) && p.comments[i].Span.Start < end; i++ {
		if !p.isPlaced[i] {
			indexes = append(indexes, i)
		}
	}
	return indexes
}
