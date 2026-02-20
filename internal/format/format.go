package format

import (
	"sort"
	"strings"

	"github.com/rdeusser/cleave/internal/ast"
	"github.com/rdeusser/cleave/internal/token"
)

func Format(file *ast.File) string {
	f := &formatter{
		comments: file.Comments,
	}
	return f.format(file)
}

type formatter struct {
	buf      strings.Builder
	comments []*ast.Comment
	nextCmt  int // index into comments slice
}

func (f *formatter) format(file *ast.File) string {
	// Sort comments by position.
	sort.Slice(f.comments, func(i, j int) bool {
		return f.comments[i].Span.Start < f.comments[j].Span.Start
	})

	f.emitCommentsBefore(file.Package.Keyword)
	f.buf.WriteString("package ")
	f.buf.WriteString(file.Package.Name.Name)
	f.buf.WriteString(";\n")

	if len(file.Imports) > 0 {
		f.buf.WriteString("\n")
		for _, imp := range file.Imports {
			f.emitCommentsBefore(imp.Keyword)
			f.buf.WriteString("import \"")
			f.buf.WriteString(imp.Path.Value)
			f.buf.WriteString("\";\n")
		}
	}

	for _, decl := range file.Decls {
		f.buf.WriteString("\n")
		switch d := decl.(type) {
		case *ast.FormatDecl:
			f.formatFormat(d)
		case *ast.UnionDecl:
			f.formatUnion(d)
		case *ast.EnumDecl:
			f.formatEnum(d)
		case *ast.StructDecl:
			f.formatStruct(d)
		}
	}

	// Emit any trailing comments.
	for f.nextCmt < len(f.comments) {
		f.buf.WriteString("\n")
		f.buf.WriteString(f.comments[f.nextCmt].Text)
		f.buf.WriteString("\n")
		f.nextCmt++
	}

	return f.buf.String()
}

func (f *formatter) formatFormat(fd *ast.FormatDecl) {
	f.emitCommentsBefore(fd.Keyword)
	f.buf.WriteString("format ")
	f.buf.WriteString(fd.Name.Name)
	f.buf.WriteString(" {\n")

	// Column-align keys and values.
	maxKey := 0
	for _, kv := range fd.Entries {
		if len(kv.Key.Name) > maxKey {
			maxKey = len(kv.Key.Name)
		}
	}

	for _, kv := range fd.Entries {
		f.emitCommentsBefore(kv.Key.Span.Start)
		f.buf.WriteString("    ")
		f.buf.WriteString(kv.Key.Name)
		f.buf.WriteString(strings.Repeat(" ", maxKey-len(kv.Key.Name)))
		f.buf.WriteString(" = ")
		f.writeExpr(kv.Value)
		f.buf.WriteString(";\n")
	}

	f.emitCommentsBefore(fd.RBrace)
	f.buf.WriteString("}\n")
}

func (f *formatter) formatUnion(u *ast.UnionDecl) {
	f.emitCommentsBefore(u.Keyword)
	f.buf.WriteString("union ")
	f.buf.WriteString(u.Name.Name)
	f.buf.WriteString(" : ")
	f.buf.WriteString(u.BackingType.Name.Name)
	f.buf.WriteString(" {\n")
	f.formatCases(u.Cases, "    ")
	f.emitCommentsBefore(u.RBrace)
	f.buf.WriteString("}\n")
}

func (f *formatter) formatCases(cases []*ast.UnionCase, indent string) {
	maxVal := 0
	for _, c := range cases {
		vs := caseValueStr(c.Value)
		if len(vs) > maxVal {
			maxVal = len(vs)
		}
	}

	for _, c := range cases {
		f.emitCommentsBefore(c.Arrow)
		vs := caseValueStr(c.Value)
		f.buf.WriteString(indent)
		f.buf.WriteString(vs)
		f.buf.WriteString(strings.Repeat(" ", maxVal-len(vs)))
		f.buf.WriteString(" => ")
		f.buf.WriteString(typeStr(c.Type))
		f.buf.WriteString(";\n")
	}
}

func (f *formatter) formatEnum(e *ast.EnumDecl) {
	f.emitCommentsBefore(e.Keyword)
	f.buf.WriteString("enum ")
	f.buf.WriteString(e.Name.Name)
	f.buf.WriteString(" : ")
	f.buf.WriteString(e.BackingType.Name.Name)
	f.buf.WriteString(" {\n")

	// Column-align variant names and values.
	maxName := 0
	for _, v := range e.Variants {
		if len(v.Name.Name) > maxName {
			maxName = len(v.Name.Name)
		}
	}

	for _, v := range e.Variants {
		f.emitCommentsBefore(v.Name.Span.Start)
		f.buf.WriteString("    ")
		f.buf.WriteString(v.Name.Name)
		f.buf.WriteString(strings.Repeat(" ", maxName-len(v.Name.Name)))
		f.buf.WriteString(" = ")
		f.writeExpr(v.Value)
		f.buf.WriteString(";\n")
	}

	f.emitCommentsBefore(e.RBrace)
	f.buf.WriteString("}\n")
}

func (f *formatter) formatStruct(s *ast.StructDecl) {
	f.emitCommentsBefore(s.Keyword)
	f.buf.WriteString("struct ")
	f.buf.WriteString(s.Name.Name)
	f.buf.WriteString(" {\n")

	// Compute column widths for name and type alignment (regular fields only).
	maxFieldName := 0
	maxTypeStr := 0
	for _, field := range s.Fields {
		if field.Match != nil {
			if len(field.Name.Name) > maxFieldName {
				maxFieldName = len(field.Name.Name)
			}
			continue
		}
		if len(field.Name.Name) > maxFieldName {
			maxFieldName = len(field.Name.Name)
		}
		ts := typeStr(field.Type)
		if len(ts) > maxTypeStr {
			maxTypeStr = len(ts)
		}
	}

	for _, field := range s.Fields {
		f.emitCommentsBefore(field.Name.Span.Start)
		f.buf.WriteString("    ")
		f.buf.WriteString(field.Name.Name)

		if field.Match != nil {
			f.buf.WriteString(strings.Repeat(" ", maxFieldName-len(field.Name.Name)))
			f.buf.WriteString("  match ")
			f.buf.WriteString(field.Match.Tag.Name)
			f.buf.WriteString(" {\n")
			f.formatCases(field.Match.Cases, "        ")
			f.buf.WriteString("    };\n")
			continue
		}

		f.buf.WriteString(strings.Repeat(" ", maxFieldName-len(field.Name.Name)))
		f.buf.WriteString("  ")
		ts := typeStr(field.Type)
		f.buf.WriteString(ts)

		if len(field.Options) > 0 {
			f.buf.WriteString(strings.Repeat(" ", maxTypeStr-len(ts)))
			f.buf.WriteString(" ")
			f.writeFieldOptions(field.Options)
		}

		f.buf.WriteString(";\n")
	}

	for _, opt := range s.Options {
		f.buf.WriteString("\n")
		f.formatOptionBlock(opt)
	}

	f.emitCommentsBefore(s.RBrace)
	f.buf.WriteString("}\n")
}

func (f *formatter) formatOptionBlock(ob *ast.OptionBlock) {
	f.emitCommentsBefore(ob.Keyword)
	f.buf.WriteString("    option (")
	f.buf.WriteString(ob.Namespace.Name)
	f.buf.WriteString(") = {\n")

	for _, kv := range ob.Entries {
		f.emitCommentsBefore(kv.Key.Span.Start)
		f.buf.WriteString("        ")
		f.buf.WriteString(kv.Key.Name)
		f.buf.WriteString(" = ")
		f.writeExpr(kv.Value)
		f.buf.WriteString(";\n")
	}

	f.buf.WriteString("    };\n")
}

func (f *formatter) hasBlockOptions(opts []*ast.FieldOption) bool {
	for _, opt := range opts {
		if opt.Namespace != nil {
			return true
		}
		if _, ok := opt.Value.(*ast.BlockExpr); ok {
			return true
		}
	}
	return false
}

func (f *formatter) writeFieldOptionKey(opt *ast.FieldOption) {
	if opt.Namespace != nil {
		f.buf.WriteString("(")
		f.buf.WriteString(opt.Namespace.Name)
		f.buf.WriteString(").")
	}
	f.buf.WriteString(opt.Key.Name)
}

func (f *formatter) writeFieldOptions(opts []*ast.FieldOption) {
	if f.hasBlockOptions(opts) {
		f.buf.WriteString("[\n")
		for i, opt := range opts {
			f.buf.WriteString("        ")
			f.writeFieldOptionKey(opt)
			f.buf.WriteString(" = ")
			f.writeExprIndented(opt.Value, "        ")
			if i < len(opts)-1 {
				f.buf.WriteString(",")
			}
			f.buf.WriteString("\n")
		}
		f.buf.WriteString("    ]")
	} else {
		f.buf.WriteString("[")
		for i, opt := range opts {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.writeFieldOptionKey(opt)
			f.buf.WriteString(" = ")
			f.writeExpr(opt.Value)
		}
		f.buf.WriteString("]")
	}
}

func (f *formatter) writeExpr(expr ast.Expr) {
	f.writeExprIndented(expr, "")
}

func (f *formatter) writeExprIndented(expr ast.Expr, indent string) {
	switch e := expr.(type) {
	case *ast.IntegerLit:
		f.buf.WriteString(e.Raw)
	case *ast.StringLit:
		f.buf.WriteString("\"")
		f.buf.WriteString(e.Value)
		f.buf.WriteString("\"")
	case *ast.Ident:
		f.buf.WriteString(e.Name)
	case *ast.DottedIdent:
		for i, part := range e.Parts {
			if i > 0 {
				f.buf.WriteString(".")
			}
			f.buf.WriteString(part.Name)
		}
	case *ast.BoolLit:
		if e.Value {
			f.buf.WriteString("true")
		} else {
			f.buf.WriteString("false")
		}
	case *ast.BlockExpr:
		f.buf.WriteString("{\n")
		for _, entry := range e.Entries {
			f.buf.WriteString(indent)
			f.buf.WriteString("    ")
			f.buf.WriteString(entry.Key.Name)
			f.buf.WriteString(": ")
			f.writeExprIndented(entry.Value, indent+"    ")
			f.buf.WriteString("\n")
		}
		f.buf.WriteString(indent)
		f.buf.WriteString("}")
	}
}

func (f *formatter) emitCommentsBefore(pos token.Pos) {
	for f.nextCmt < len(f.comments) && f.comments[f.nextCmt].Span.Start < pos {
		f.buf.WriteString(f.comments[f.nextCmt].Text)
		f.buf.WriteString("\n")
		f.nextCmt++
	}
}

func caseValueStr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IntegerLit:
		return e.Raw
	case *ast.Ident:
		return e.Name
	default:
		return ""
	}
}

func typeStr(te *ast.TypeExpr) string {
	name := te.Name.Name
	if te.Dim == nil {
		return name
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteString("[")
	switch d := te.Dim.(type) {
	case *ast.IntegerLit:
		b.WriteString(d.Raw)
	case *ast.Ident:
		b.WriteString(d.Name)
	}
	for _, opt := range te.InlineOptions {
		b.WriteString(", ")
		b.WriteString(opt.Key.Name)
		b.WriteString(" = ")
		writeExprToBuilder(&b, opt.Value)
	}
	b.WriteString("]")
	return b.String()
}

func writeExprToBuilder(b *strings.Builder, expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.IntegerLit:
		b.WriteString(e.Raw)
	case *ast.StringLit:
		b.WriteString("\"")
		b.WriteString(e.Value)
		b.WriteString("\"")
	case *ast.Ident:
		b.WriteString(e.Name)
	case *ast.BoolLit:
		if e.Value {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case *ast.BlockExpr:
		b.WriteString("{ ")
		for i, entry := range e.Entries {
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(entry.Key.Name)
			b.WriteString(": ")
			writeExprToBuilder(b, entry.Value)
		}
		b.WriteString(" }")
	}
}
