package format

import (
	"sort"
	"strings"

	"github.com/rdeusser/cleave/internal/ast"
	"github.com/rdeusser/cleave/internal/token"
)

type formatter struct {
	sb       strings.Builder
	comments []*ast.Comment
	nextCmt  int // index into comments slice
}

func (f *formatter) format(file *ast.File) string {
	// Sort comments by position.
	sort.Slice(f.comments, func(i, j int) bool {
		return f.comments[i].Span.Start < f.comments[j].Span.Start
	})

	f.emitCommentsBefore(file.Package.Keyword)
	f.sb.WriteString("package ")
	f.sb.WriteString(file.Package.Name.Name)
	f.sb.WriteString(";\n")

	if len(file.Imports) > 0 {
		f.sb.WriteString("\n")
		for _, imp := range file.Imports {
			f.emitCommentsBefore(imp.Keyword)
			f.sb.WriteString("import \"")
			f.sb.WriteString(imp.Path.Value)
			f.sb.WriteString("\";\n")
		}
	}

	for _, decl := range file.Decls {
		f.sb.WriteString("\n")
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
		f.sb.WriteString("\n")
		f.sb.WriteString(f.comments[f.nextCmt].Text)
		f.sb.WriteString("\n")
		f.nextCmt++
	}

	return f.sb.String()
}

func (f *formatter) formatFormat(fd *ast.FormatDecl) {
	f.emitCommentsBefore(fd.Keyword)
	f.sb.WriteString("format ")
	f.sb.WriteString(fd.Name.Name)
	f.sb.WriteString(" {\n")

	// Column-align keys and values.
	maxKey := 0
	for _, kv := range fd.Entries {
		if len(kv.Key.Name) > maxKey {
			maxKey = len(kv.Key.Name)
		}
	}

	for _, kv := range fd.Entries {
		f.emitCommentsBefore(kv.Key.Span.Start)
		f.sb.WriteString("    ")
		f.sb.WriteString(kv.Key.Name)
		f.sb.WriteString(strings.Repeat(" ", maxKey-len(kv.Key.Name)))
		f.sb.WriteString(" = ")
		f.writeExpr(kv.Value)
		f.sb.WriteString(";\n")
	}

	f.emitCommentsBefore(fd.RBrace)
	f.sb.WriteString("}\n")
}

func (f *formatter) formatUnion(u *ast.UnionDecl) {
	f.emitCommentsBefore(u.Keyword)
	f.sb.WriteString("union ")
	f.sb.WriteString(u.Name.Name)
	f.sb.WriteString(" : ")
	f.sb.WriteString(u.BackingType.Name.Name)
	f.sb.WriteString(" {\n")
	f.formatCases(u.Cases, "    ")
	f.emitCommentsBefore(u.RBrace)
	f.sb.WriteString("}\n")
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
		f.sb.WriteString(indent)
		f.sb.WriteString(vs)
		f.sb.WriteString(strings.Repeat(" ", maxVal-len(vs)))
		f.sb.WriteString(" => ")
		f.sb.WriteString(typeStr(c.Type))
		f.sb.WriteString(";\n")
	}
}

func (f *formatter) formatEnum(e *ast.EnumDecl) {
	f.emitCommentsBefore(e.Keyword)
	f.sb.WriteString("enum ")
	f.sb.WriteString(e.Name.Name)
	f.sb.WriteString(" : ")
	f.sb.WriteString(e.BackingType.Name.Name)
	f.sb.WriteString(" {\n")

	// Column-align variant names and values.
	maxName := 0
	for _, v := range e.Variants {
		if len(v.Name.Name) > maxName {
			maxName = len(v.Name.Name)
		}
	}

	for _, v := range e.Variants {
		f.emitCommentsBefore(v.Name.Span.Start)
		f.sb.WriteString("    ")
		f.sb.WriteString(v.Name.Name)
		f.sb.WriteString(strings.Repeat(" ", maxName-len(v.Name.Name)))
		f.sb.WriteString(" = ")
		f.writeExpr(v.Value)
		f.sb.WriteString(";\n")
	}

	f.emitCommentsBefore(e.RBrace)
	f.sb.WriteString("}\n")
}

func (f *formatter) formatStruct(s *ast.StructDecl) {
	f.emitCommentsBefore(s.Keyword)
	f.sb.WriteString("struct ")
	f.sb.WriteString(s.Name.Name)
	f.sb.WriteString(" {\n")

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
		f.sb.WriteString("    ")
		f.sb.WriteString(field.Name.Name)

		if field.Match != nil {
			f.sb.WriteString(strings.Repeat(" ", maxFieldName-len(field.Name.Name)))
			f.sb.WriteString("  match ")
			f.sb.WriteString(field.Match.Tag.Name)
			f.sb.WriteString(" {\n")
			f.formatCases(field.Match.Cases, "        ")
			f.sb.WriteString("    };\n")
			continue
		}

		f.sb.WriteString(strings.Repeat(" ", maxFieldName-len(field.Name.Name)))
		f.sb.WriteString("  ")
		ts := typeStr(field.Type)
		f.sb.WriteString(ts)

		if len(field.Options) > 0 {
			f.sb.WriteString(strings.Repeat(" ", maxTypeStr-len(ts)))
			f.sb.WriteString(" ")
			f.writeFieldOptions(field.Options)
		}

		f.sb.WriteString(";\n")
	}

	for _, opt := range s.Options {
		f.sb.WriteString("\n")
		f.formatOptionBlock(opt)
	}

	f.emitCommentsBefore(s.RBrace)
	f.sb.WriteString("}\n")
}

func (f *formatter) formatOptionBlock(ob *ast.OptionBlock) {
	f.emitCommentsBefore(ob.Keyword)
	f.sb.WriteString("    option (")
	f.sb.WriteString(ob.Namespace.Name)
	f.sb.WriteString(") = {\n")

	for _, kv := range ob.Entries {
		f.emitCommentsBefore(kv.Key.Span.Start)
		f.sb.WriteString("        ")
		f.sb.WriteString(kv.Key.Name)
		f.sb.WriteString(" = ")
		f.writeExpr(kv.Value)
		f.sb.WriteString(";\n")
	}

	f.sb.WriteString("    };\n")
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
		f.sb.WriteString("(")
		f.sb.WriteString(opt.Namespace.Name)
		f.sb.WriteString(").")
	}
	f.sb.WriteString(opt.Key.Name)
}

func (f *formatter) writeFieldOptions(opts []*ast.FieldOption) {
	if f.hasBlockOptions(opts) {
		f.sb.WriteString("[\n")
		for i, opt := range opts {
			f.sb.WriteString("        ")
			f.writeFieldOptionKey(opt)
			f.sb.WriteString(" = ")
			f.writeExprIndented(opt.Value, "        ")
			if i < len(opts)-1 {
				f.sb.WriteString(",")
			}
			f.sb.WriteString("\n")
		}
		f.sb.WriteString("    ]")
	} else {
		f.sb.WriteString("[")
		for i, opt := range opts {
			if i > 0 {
				f.sb.WriteString(", ")
			}
			f.writeFieldOptionKey(opt)
			f.sb.WriteString(" = ")
			f.writeExpr(opt.Value)
		}
		f.sb.WriteString("]")
	}
}

func (f *formatter) writeExpr(expr ast.Expr) { f.writeExprIndented(expr, "") }

func (f *formatter) writeExprIndented(expr ast.Expr, indent string) {
	switch e := expr.(type) {
	case *ast.IntegerLit:
		f.sb.WriteString(e.Raw)
	case *ast.StringLit:
		f.sb.WriteString("\"")
		f.sb.WriteString(e.Value)
		f.sb.WriteString("\"")
	case *ast.Ident:
		f.sb.WriteString(e.Name)
	case *ast.DottedIdent:
		for i, part := range e.Parts {
			if i > 0 {
				f.sb.WriteString(".")
			}
			f.sb.WriteString(part.Name)
		}
	case *ast.BoolLit:
		if e.Value {
			f.sb.WriteString("true")
		} else {
			f.sb.WriteString("false")
		}
	case *ast.BlockExpr:
		f.sb.WriteString("{\n")
		for _, entry := range e.Entries {
			f.sb.WriteString(indent)
			f.sb.WriteString("    ")
			f.sb.WriteString(entry.Key.Name)
			f.sb.WriteString(": ")
			f.writeExprIndented(entry.Value, indent+"    ")
			f.sb.WriteString("\n")
		}
		f.sb.WriteString(indent)
		f.sb.WriteString("}")
	}
}

func (f *formatter) emitCommentsBefore(pos token.Pos) {
	for f.nextCmt < len(f.comments) && f.comments[f.nextCmt].Span.Start < pos {
		f.sb.WriteString(f.comments[f.nextCmt].Text)
		f.sb.WriteString("\n")
		f.nextCmt++
	}
}

func Format(file *ast.File) string {
	f := &formatter{
		comments: file.Comments,
	}
	return f.format(file)
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
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteString("[")
	switch d := te.Dim.(type) {
	case *ast.IntegerLit:
		sb.WriteString(d.Raw)
	case *ast.Ident:
		sb.WriteString(d.Name)
	}
	for _, opt := range te.InlineOptions {
		sb.WriteString(", ")
		sb.WriteString(opt.Key.Name)
		sb.WriteString(" = ")
		writeExprToBuilder(&sb, opt.Value)
	}
	sb.WriteString("]")
	return sb.String()
}

func writeExprToBuilder(sb *strings.Builder, expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.IntegerLit:
		sb.WriteString(e.Raw)
	case *ast.StringLit:
		sb.WriteString("\"")
		sb.WriteString(e.Value)
		sb.WriteString("\"")
	case *ast.Ident:
		sb.WriteString(e.Name)
	case *ast.BoolLit:
		if e.Value {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case *ast.BlockExpr:
		sb.WriteString("{ ")
		for i, entry := range e.Entries {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(entry.Key.Name)
			sb.WriteString(": ")
			writeExprToBuilder(sb, entry.Value)
		}
		sb.WriteString(" }")
	}
}
