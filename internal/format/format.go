// Package format prints cleave source in its canonical style.
//
// The style indents with four spaces and puts each declaration, field, variant,
// case, and entry on its own line. Consecutive lines align their columns (field
// names, types, and option lists, and the keys of variants, cases, and entries).
// Trailing comments in a run of such lines start in one column. A blank line
// or a line that opens a block ends the run. A comment line does not.
//
// Comments stay where the source puts them: above a line, at the end of a line,
// after an opening brace or bracket, or before a closing one. A comment inside
// a construct that prints on one line, such as a type, moves to its own line
// above that construct. Continuation lines of a block comment print unchanged.
//
// A field's option list stays on the field's line when it holds no block value
// and no comment, and the line fits in 100 columns before alignment padding.
// Otherwise each option gets its own line. Block values print one entry per
// line, except inside the brackets of a type.
//
// One blank line separates top-level declarations and sets struct option blocks
// apart from fields. Elsewhere one blank line appears wherever the source has
// one or more.
package format

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/rdeusser/cleave/internal/ast"
	"github.com/rdeusser/cleave/internal/parser"
	"github.com/rdeusser/cleave/internal/token"
)

// indentUnit is one level of indentation.
const indentUnit = "    "

// maxWidth limits the width of a field printed with its options on one line.
// The width leaves out alignment padding. The padding depends on which rows
// share the field's alignment section, and that depends on whether the field's
// option list breaks.
const maxWidth = 100

// Source parses src and returns it in the canonical style. filename labels
// the positions in parse errors. If src does not parse, Source returns the
// parse errors joined and no output.
func Source(filename string, src []byte) ([]byte, error) {
	file, parseErrs := parser.New(filename, src).Parse()
	if len(parseErrs) > 0 {
		errs := make([]error, len(parseErrs))
		for i, e := range parseErrs {
			errs[i] = e
		}
		return nil, errors.Join(errs...)
	}

	p := newPrinter(src, file.Comments)
	lines := p.file(file, token.Pos(len(src)))
	if i := slices.Index(p.isPlaced, false); i >= 0 {
		panic(fmt.Sprintf("format: comment at offset %d was not printed", p.comments[i].Span.Start))
	}

	var buf bytes.Buffer
	for _, l := range lines {
		buf.WriteString(l.text)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// cases lays out union or match cases, one "value => type;" per line.
func cases(cs []*ast.UnionCase) []item {
	items := make([]item, len(cs))
	for i, c := range cs {
		items[i] = item{span: ast.SpanOf(c), cells: []string{valueString(c.Value), "=> " + typeString(c.Type) + ";"}}
	}
	return items
}

// typeString prints a type with its dimension and inline options, as in
// string[64, terminator = 0x00].
func typeString(t *ast.TypeExpr) string {
	if t.Dim == nil {
		return t.Name.Name
	}
	s := t.Name.Name + "[" + valueString(t.Dim)
	for _, o := range t.InlineOptions {
		s += ", " + optionKey(o) + " = " + valueString(o.Value)
	}
	return s + "]"
}

// valueString prints a value on one line. A block value prints as
// { key: value key: value }.
func valueString(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.IntegerLit:
		return e.Raw
	case *ast.StringLit:
		return `"` + e.Value + `"`
	case *ast.BoolLit:
		return strconv.FormatBool(e.Value)
	case *ast.Ident:
		return e.Name
	case *ast.DottedIdent:
		parts := make([]string, len(e.Parts))
		for i, part := range e.Parts {
			parts[i] = part.Name
		}
		return strings.Join(parts, ".")
	case *ast.BlockExpr:
		if len(e.Entries) == 0 {
			return "{}"
		}
		s := "{"
		for _, entry := range e.Entries {
			s += " " + entry.Key.Name + ": " + valueString(entry.Value)
		}
		return s + " }"
	default:
		panic(fmt.Sprintf("format: unexpected expression %T", e))
	}
}

// optionKey prints an option's key, with its namespace when it has one, as in
// (builtin).cel.
func optionKey(o *ast.FieldOption) string {
	if o.Namespace == nil {
		return o.Key.Name
	}
	return "(" + o.Namespace.Name + ")." + o.Key.Name
}

// inline joins comments that share a source line with single spaces and splits
// the result into lines. The first line can follow other text. The rest
// continue a block comment and keep their indentation. Every line loses its
// trailing whitespace.
func inline(cs []*ast.Comment) []string {
	texts := make([]string, len(cs))
	for i, c := range cs {
		texts[i] = c.Text
	}
	lines := strings.Split(strings.Join(texts, " "), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return lines
}
