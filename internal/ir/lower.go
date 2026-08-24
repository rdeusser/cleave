package ir

import (
	"fmt"

	"github.com/rdeusser/cleave/internal/ast"
	"github.com/rdeusser/cleave/internal/token"
)

func Lower(file *ast.File, posFunc func(token.Pos) token.Position) (*Package, []LowerError) {
	l := &lowerer{
		pos:           posFunc,
		enums:         make(map[string]*Enum),
		unions:        make(map[string]*Union),
		structs:       make(map[string]*Struct),
		imported:      make(map[string]bool),
		structEndians: make(map[string]bool),
	}
	return l.lower(file)
}

type lowerer struct {
	pos           func(token.Pos) token.Position
	errors        []LowerError
	enums         map[string]*Enum
	unions        map[string]*Union
	structs       map[string]*Struct
	imported      map[string]bool // type names from imports (can be shadowed by entry file)
	formatEndian  Endian
	hasFormat     bool
	structEndians map[string]bool // tracks which structs explicitly set endian
}

func (l *lowerer) lower(file *ast.File) (*Package, []LowerError) {
	pkg := &Package{Name: file.Package.Name.Name}

	// Pre-pass: register imported types in lookup maps for [ref] validation.
	// These are NOT added to pkg (no code generated for them).
	for _, decl := range file.ImportedDecls {
		switch d := decl.(type) {
		case *ast.EnumDecl:
			if !l.typeExists(d.Name.Name) {
				l.enums[d.Name.Name] = &Enum{Name: d.Name.Name}
				l.imported[d.Name.Name] = true
			}
		case *ast.UnionDecl:
			if !l.typeExists(d.Name.Name) {
				l.unions[d.Name.Name] = &Union{Name: d.Name.Name}
				l.imported[d.Name.Name] = true
			}
		case *ast.StructDecl:
			if !l.typeExists(d.Name.Name) {
				l.structs[d.Name.Name] = &Struct{Name: d.Name.Name}
				l.imported[d.Name.Name] = true
			}
		}
	}

	// Resolve imported types so they're available for [ref] validation
	// and usable as field types in the entry file.
	for _, decl := range file.ImportedDecls {
		switch d := decl.(type) {
		case *ast.StructDecl:
			if s, exists := l.structs[d.Name.Name]; exists && len(s.Fields) == 0 {
				l.resolveStruct(d)
			}
		case *ast.EnumDecl:
			if e, exists := l.enums[d.Name.Name]; exists && len(e.Variants) == 0 {
				l.resolveEnum(d)
			}
		}
	}

	// First pass: collect all type names for forward reference resolution.
	// Entry file types shadow imported types with the same name.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.EnumDecl:
			name := d.Name.Name
			if l.typeExists(name) && !l.imported[name] {
				l.errorf(d.Name.Span.Start, "duplicate type %q", name)
				continue
			}
			e := &Enum{Name: name}
			l.enums[name] = e
			delete(l.imported, name)
			pkg.Enums = append(pkg.Enums, e)
		case *ast.UnionDecl:
			name := d.Name.Name
			if l.typeExists(name) && !l.imported[name] {
				l.errorf(d.Name.Span.Start, "duplicate type %q", name)
				continue
			}
			u := &Union{Name: name}
			l.unions[name] = u
			delete(l.imported, name)
			pkg.Unions = append(pkg.Unions, u)
		case *ast.StructDecl:
			name := d.Name.Name
			if l.typeExists(name) && !l.imported[name] {
				l.errorf(d.Name.Span.Start, "duplicate type %q", name)
				continue
			}
			s := &Struct{Name: name}
			l.structs[name] = s
			delete(l.imported, name)
			pkg.Structs = append(pkg.Structs, s)
		}
	}

	// Second pass: resolve format block first (to get default endian).
	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FormatDecl); ok {
			pkg.Format = l.resolveFormat(d)
		}
	}

	// Third pass: resolve declarations.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.EnumDecl:
			l.resolveEnum(d)
		case *ast.UnionDecl:
			l.resolveUnion(d)
		case *ast.StructDecl:
			l.resolveStruct(d)
		}
	}

	// Fourth pass: pull imported types into pkg if referenced by entry file fields.
	// The codegen needs all types that appear in the output's struct fields.
	l.addReferencedImports(pkg)

	return pkg, l.errors
}

func (l *lowerer) addReferencedImports(pkg *Package) {
	pkgEnums := make(map[string]bool)
	for _, e := range pkg.Enums {
		pkgEnums[e.Name] = true
	}
	pkgStructs := make(map[string]bool)
	for _, s := range pkg.Structs {
		pkgStructs[s.Name] = true
	}
	var importedEnums []*Enum
	var importedStructs []*Struct

	for _, s := range pkg.Structs {
		for _, f := range s.Fields {
			if f.Type.Kind == KindEnum && !pkgEnums[f.Type.Ref] {
				if e, ok := l.enums[f.Type.Ref]; ok {
					importedEnums = append(importedEnums, e)
					pkgEnums[f.Type.Ref] = true
				}
			}
			if f.Type.Kind == KindStruct && !pkgStructs[f.Type.Ref] {
				if st, ok := l.structs[f.Type.Ref]; ok {
					importedStructs = append(importedStructs, st)
					pkgStructs[f.Type.Ref] = true
				}
			}
		}
	}

	pkg.Enums = append(importedEnums, pkg.Enums...)
	pkg.Structs = append(importedStructs, pkg.Structs...)
}

func (l *lowerer) resolveFormat(d *ast.FormatDecl) *Format {
	if l.hasFormat {
		l.errorf(d.Keyword, "only one format block is allowed per file")
		return nil
	}
	l.hasFormat = true

	f := &Format{Name: d.Name.Name}

	for _, kv := range d.Entries {
		switch kv.Key.Name {
		case "root":
			ident, ok := kv.Value.(*ast.Ident)
			if !ok {
				l.errorf(kv.Key.Span.Start, "root must be a struct name")
				continue
			}
			if _, exists := l.structs[ident.Name]; !exists {
				l.errorf(ident.Span.Start, "root references unknown struct %q", ident.Name)
				continue
			}
			f.Root = ident.Name
		case "title":
			strVal, ok := kv.Value.(*ast.StringLit)
			if !ok {
				l.errorf(kv.Key.Span.Start, "title must be a string")
				continue
			}
			f.Title = strVal.Value
		case "extension":
			strVal, ok := kv.Value.(*ast.StringLit)
			if !ok {
				l.errorf(kv.Key.Span.Start, "extension must be a string")
				continue
			}
			f.Extension = strVal.Value
		case "endian":
			ident, ok := kv.Value.(*ast.Ident)
			if !ok {
				l.errorf(kv.Key.Span.Start, "endian must be 'big' or 'little'")
				continue
			}
			endian, ok := parseEndian(ident.Name)
			if !ok {
				l.errorf(kv.Key.Span.Start, "endian must be 'big' or 'little', got %q", ident.Name)
				continue
			}
			f.Endian = endian
			l.formatEndian = endian
		default:
			l.errorf(kv.Key.Span.Start, "unknown format option %q", kv.Key.Name)
		}
	}

	if f.Root == "" {
		l.errorf(d.Name.Span.Start, "format block requires a root property")
	}

	return f
}

func (l *lowerer) typeExists(name string) bool {
	if _, ok := l.enums[name]; ok {
		return true
	}
	if _, ok := l.unions[name]; ok {
		return true
	}
	if _, ok := l.structs[name]; ok {
		return true
	}
	return false
}

func (l *lowerer) resolveUnion(d *ast.UnionDecl) {
	u := l.unions[d.Name.Name]

	prim, ok := LookupPrimitive(d.BackingType.Name.Name)
	if !ok {
		l.errorf(d.BackingType.Name.Span.Start, "invalid union backing type %q", d.BackingType.Name.Name)
		return
	}
	if !prim.IsInteger() {
		l.errorf(d.BackingType.Name.Span.Start, "union backing type must be an integer type, got %q", d.BackingType.Name.Name)
		return
	}
	u.BackingType = prim

	l.resolveCases(d.Cases, prim, &u.Cases, &u.Default)
}

func (l *lowerer) resolveCases(cases []*ast.UnionCase, tagType PrimitiveType, outCases *[]MatchCase, outDefault **FieldType) {
	seen := make(map[int64]bool)
	for _, c := range cases {
		if ident, ok := c.Value.(*ast.Ident); ok && ident.Name == "_" {
			if *outDefault != nil {
				l.errorf(ident.Span.Start, "duplicate default case")
				continue
			}
			ft := l.resolveTypeExpr(c.Type)
			if ft != nil {
				*outDefault = ft
			}
			continue
		}

		intLit, ok := c.Value.(*ast.IntegerLit)
		if !ok {
			l.errorf(c.Arrow, "case value must be an integer or '_'")
			continue
		}

		if !fitsInType(intLit.Value, tagType) {
			l.errorf(intLit.Span.Start, "case value %d out of range for %s", intLit.Value, tagType)
			continue
		}

		if seen[intLit.Value] {
			l.errorf(intLit.Span.Start, "duplicate case value %d", intLit.Value)
			continue
		}
		seen[intLit.Value] = true

		ft := l.resolveTypeExpr(c.Type)
		if ft == nil {
			continue
		}
		*outCases = append(*outCases, MatchCase{Value: intLit.Value, Type: *ft})
	}
}

func (l *lowerer) resolveTypeExpr(te *ast.TypeExpr) *FieldType {
	typeName := te.Name.Name
	ft := &FieldType{}

	if prim, ok := LookupPrimitive(typeName); ok {
		ft.Kind = KindPrimitive
		ft.Primitive = prim
	} else if _, ok := l.enums[typeName]; ok {
		ft.Kind = KindEnum
		ft.Ref = typeName
	} else if _, ok := l.structs[typeName]; ok {
		ft.Kind = KindStruct
		ft.Ref = typeName
	} else {
		l.errorf(te.Name.Span.Start, "unknown type %q", typeName)
		return nil
	}

	if te.Dim != nil {
		switch dim := te.Dim.(type) {
		case *ast.IntegerLit:
			ft.Array = ArraySpec{Kind: FixedSize, FixedSize: dim.Value}
		case *ast.Ident:
			ft.Array = ArraySpec{Kind: LengthRef, LengthRef: dim.Name}
		}
	}

	return ft
}

func (l *lowerer) resolveEnum(d *ast.EnumDecl) {
	e := l.enums[d.Name.Name]

	prim, ok := LookupPrimitive(d.BackingType.Name.Name)
	if !ok {
		l.errorf(d.BackingType.Name.Span.Start, "invalid enum backing type %q", d.BackingType.Name.Name)
		return
	}
	if !prim.IsInteger() {
		l.errorf(d.BackingType.Name.Span.Start, "enum backing type must be an integer type, got %q", d.BackingType.Name.Name)
		return
	}
	e.BackingType = prim

	seen := make(map[string]bool)
	for _, v := range d.Variants {
		name := v.Name.Name
		if seen[name] {
			l.errorf(v.Name.Span.Start, "duplicate enum variant %q", name)
			continue
		}
		seen[name] = true

		intLit, ok := v.Value.(*ast.IntegerLit)
		if !ok {
			l.errorf(v.Name.Span.Start, "enum variant value must be an integer literal")
			continue
		}
		e.Variants = append(e.Variants, EnumVariant{Name: name, Value: intLit.Value})
	}
}

func (l *lowerer) resolveStruct(d *ast.StructDecl) {
	s := l.structs[d.Name.Name]
	s.Options = l.resolveStructOptions(d.Name.Name, d.Options)

	seen := make(map[string]bool)
	for _, f := range d.Fields {
		name := f.Name.Name
		if seen[name] {
			l.errorf(f.Name.Span.Start, "duplicate field %q in struct %q", name, d.Name.Name)
			continue
		}
		seen[name] = true

		field := l.resolveField(f, s)
		if field != nil {
			s.Fields = append(s.Fields, field)
		}
	}
}

func (l *lowerer) resolveField(f *ast.FieldDecl, s *Struct) *Field {
	field := &Field{Name: f.Name.Name}

	// Match field — inline match or union reference via [tag = ...].
	if f.Match != nil {
		return l.resolveMatchField(f, s)
	}

	typeName := f.Type.Name.Name

	// Check if this is a union type reference with [tag = ...].
	if u, ok := l.unions[typeName]; ok {
		return l.resolveUnionRefField(f, s, u)
	}

	if prim, ok := LookupPrimitive(typeName); ok {
		field.Type.Kind = KindPrimitive
		field.Type.Primitive = prim
	} else if _, ok := l.enums[typeName]; ok {
		field.Type.Kind = KindEnum
		field.Type.Ref = typeName
	} else if _, ok := l.structs[typeName]; ok {
		field.Type.Kind = KindStruct
		field.Type.Ref = typeName
	} else {
		l.errorf(f.Type.Name.Span.Start, "unknown type %q", typeName)
		return nil
	}

	if f.Type.Dim != nil {
		switch dim := f.Type.Dim.(type) {
		case *ast.IntegerLit:
			field.Type.Array = ArraySpec{Kind: FixedSize, FixedSize: dim.Value}
		case *ast.Ident:
			// Validate length reference points to a preceding integer field.
			found := false
			for _, prev := range s.Fields {
				if prev.Name == dim.Name {
					if prev.Type.Kind != KindPrimitive || !prev.Type.Primitive.IsInteger() {
						l.errorf(f.Type.Name.Span.Start, "length reference %q must be an integer field", dim.Name)
						return nil
					}
					found = true
					break
				}
			}
			if !found {
				l.errorf(f.Type.Name.Span.Start, "length reference %q must refer to a preceding field", dim.Name)
				return nil
			}
			if field.Type.Kind == KindStruct {
				field.Type.Array = ArraySpec{Kind: CountRef, CountRef: dim.Name}
			} else {
				field.Type.Array = ArraySpec{Kind: LengthRef, LengthRef: dim.Name}
			}
		}

		// Check for inline options that combine with the dimension (e.g., FixedTerminator).
		if len(f.Type.InlineOptions) > 0 {
			for _, opt := range f.Type.InlineOptions {
				switch opt.Key.Name {
				case "terminator":
					intVal, ok := opt.Value.(*ast.IntegerLit)
					if !ok {
						l.errorf(opt.Key.Span.Start, "terminator option must be an integer")
						continue
					}
					if field.Type.Array.Kind == FixedSize {
						field.Type.Array = ArraySpec{
							Kind:      FixedTerminator,
							FixedSize: field.Type.Array.FixedSize,
							Sentinel:  intVal.Value,
						}
					} else {
						l.errorf(opt.Key.Span.Start, "terminator inline option requires a fixed-size dimension")
					}
				case "encoding":
					strVal, ok := opt.Value.(*ast.StringLit)
					if !ok {
						l.errorf(opt.Key.Span.Start, "encoding option must be a string")
						continue
					}
					if field.Type.Primitive != String {
						l.errorf(opt.Key.Span.Start, "encoding option is only valid on string fields")
						continue
					}
					field.Encoding = strVal.Value
				default:
					l.errorf(opt.Key.Span.Start, "unknown inline option %q", opt.Key.Name)
				}
			}
		}
	}

	for _, opt := range f.Options {
		// Namespaced options: (builtin).cel = { ... }
		if opt.Namespace != nil {
			if opt.Namespace.Name != "builtin" {
				l.errorf(opt.Namespace.Span.Start, "unknown option namespace %q", opt.Namespace.Name)
				continue
			}
			switch opt.Key.Name {
			case "cel":
				rule, ok := l.resolveValidationBlock(opt)
				if ok {
					field.Validations = append(field.Validations, rule)
				}
			default:
				l.errorf(opt.Key.Span.Start, "unknown builtin option %q", opt.Key.Name)
			}
			continue
		}

		switch opt.Key.Name {
		case "rest":
			boolVal, ok := opt.Value.(*ast.BoolLit)
			if !ok || !boolVal.Value {
				l.errorf(opt.Key.Span.Start, "rest option must be true")
				continue
			}
			field.Type.Array = ArraySpec{Kind: RestArray}
		case "terminator":
			intVal, ok := opt.Value.(*ast.IntegerLit)
			if !ok {
				l.errorf(opt.Key.Span.Start, "terminator option must be an integer")
				continue
			}
			field.Type.Array = ArraySpec{Kind: Terminator, Sentinel: intVal.Value}
		case "count":
			ref := l.resolveCountRef(opt)
			if ref != "" {
				field.Type.Array = ArraySpec{Kind: CountRef, CountRef: ref}
			}
		case "if":
			strVal, ok := opt.Value.(*ast.StringLit)
			if !ok {
				l.errorf(opt.Key.Span.Start, "if option must be a string expression")
				continue
			}
			expr, err := ParseCELExpr(strVal.Value)
			if err != nil {
				l.errorf(opt.Key.Span.Start, "invalid if expression: %v", err)
				continue
			}
			if containsThis(expr) {
				l.errorf(opt.Key.Span.Start, "if expression cannot reference 'this' (field not yet parsed)")
				continue
			}
			field.Condition = expr
		case "endian":
			identVal, ok := opt.Value.(*ast.Ident)
			if !ok {
				l.errorf(opt.Key.Span.Start, "endian option must be 'big' or 'little'")
				continue
			}
			endian, ok := parseEndian(identVal.Name)
			if !ok {
				l.errorf(opt.Key.Span.Start, "endian option must be 'big' or 'little', got %q", identVal.Name)
				continue
			}
			field.Options.Endian = &endian
		case "encoding":
			strVal, ok := opt.Value.(*ast.StringLit)
			if !ok {
				l.errorf(opt.Key.Span.Start, "encoding option must be a string")
				continue
			}
			if field.Type.Kind != KindPrimitive || field.Type.Primitive != String {
				l.errorf(opt.Key.Span.Start, "encoding option is only valid on string fields")
				continue
			}
			if field.Encoding != "" {
				l.errorf(opt.Key.Span.Start, "encoding already specified as inline option")
				continue
			}
			field.Encoding = strVal.Value
		case "ref":
			switch v := opt.Value.(type) {
			case *ast.DottedIdent:
				if len(v.Parts) != 2 {
					l.errorf(opt.Key.Span.Start, "ref must be Struct.field (got %d parts)", len(v.Parts))
					continue
				}
				structName := v.Parts[0].Name
				fieldName := v.Parts[1].Name
				refStruct, exists := l.structs[structName]
				if !exists {
					l.errorf(v.Parts[0].Span.Start, "ref references unknown struct %q", structName)
					continue
				}
				found := false
				for _, rf := range refStruct.Fields {
					if rf.Name == fieldName {
						found = true
						break
					}
				}
				if !found {
					l.errorf(v.Parts[1].Span.Start, "struct %q has no field %q", structName, fieldName)
					continue
				}
				field.CrossRef = structName + "." + fieldName
			default:
				l.errorf(opt.Key.Span.Start, "ref must be Struct.field")
			}
		default:
			l.errorf(opt.Key.Span.Start, "unknown field option %q", opt.Key.Name)
		}
	}

	return field
}

func (l *lowerer) resolveMatchField(f *ast.FieldDecl, s *Struct) *Field {
	field := &Field{Name: f.Name.Name}
	m := f.Match

	// Validate tag field exists and precedes this field.
	tagType, ok := l.validateTagField(m.Tag.Name, m.Tag.Span.Start, s)
	if !ok {
		return nil
	}

	spec := &MatchSpec{TagField: m.Tag.Name}
	l.resolveCases(m.Cases, tagType, &spec.Cases, &spec.Default)
	field.Match = spec
	field.Type.Kind = KindMatch
	return field
}

func (l *lowerer) resolveUnionRefField(f *ast.FieldDecl, s *Struct, u *Union) *Field {
	field := &Field{Name: f.Name.Name}

	// Find the [tag = ...] option.
	var tagName string
	var tagPos token.Pos
	for _, opt := range f.Options {
		if opt.Key.Name == "tag" {
			ident, ok := opt.Value.(*ast.Ident)
			if !ok {
				l.errorf(opt.Key.Span.Start, "tag option must be a field name")
				return nil
			}
			tagName = ident.Name
			tagPos = ident.Span.Start
			break
		}
	}
	if tagName == "" {
		l.errorf(f.Type.Name.Span.Start, "union type %q requires [tag = field] option", f.Type.Name.Name)
		return nil
	}

	// Validate tag field exists and precedes this field.
	tagType, ok := l.validateTagField(tagName, tagPos, s)
	if !ok {
		return nil
	}

	// Validate tag type matches the union's declared backing type.
	if tagType != u.BackingType {
		l.errorf(tagPos, "tag field type %s does not match union backing type %s", tagType, u.BackingType)
		return nil
	}

	// Copy the union's resolved cases into a MatchSpec.
	spec := &MatchSpec{
		TagField: tagName,
		Cases:    make([]MatchCase, len(u.Cases)),
		Default:  u.Default,
	}
	copy(spec.Cases, u.Cases)

	field.Match = spec
	field.Type.Kind = KindMatch
	return field
}

func (l *lowerer) validateTagField(tagName string, pos token.Pos, s *Struct) (PrimitiveType, bool) {
	for _, prev := range s.Fields {
		if prev.Name == tagName {
			if prev.Type.Kind != KindPrimitive || !prev.Type.Primitive.IsInteger() {
				l.errorf(pos, "tag field %q must be an integer type", tagName)
				return 0, false
			}
			return prev.Type.Primitive, true
		}
	}
	l.errorf(pos, "tag field %q must refer to a preceding field in the same struct", tagName)
	return 0, false
}

func (l *lowerer) resolveStructOptions(structName string, blocks []*ast.OptionBlock) StructOptions {
	opts := StructOptions{Endian: l.formatEndian}
	for _, block := range blocks {
		if block.Namespace.Name != "builtin" {
			l.errorf(block.Namespace.Span.Start, "unknown option namespace %q", block.Namespace.Name)
			continue
		}
		for _, kv := range block.Entries {
			switch kv.Key.Name {
			case "endian":
				identVal, ok := kv.Value.(*ast.Ident)
				if !ok {
					l.errorf(kv.Key.Span.Start, "endian must be 'big' or 'little'")
					continue
				}
				endian, ok := parseEndian(identVal.Name)
				if !ok {
					l.errorf(kv.Key.Span.Start, "endian must be 'big' or 'little', got %q", identVal.Name)
					continue
				}
				opts.Endian = endian
				l.structEndians[structName] = true
			default:
				l.errorf(kv.Key.Span.Start, "unknown option %q", kv.Key.Name)
			}
		}
	}
	return opts
}

func (l *lowerer) resolveValidationBlock(opt *ast.FieldOption) (ValidationRule, bool) {
	block, ok := opt.Value.(*ast.BlockExpr)
	if !ok {
		l.errorf(opt.Key.Span.Start, "(builtin).cel value must be a block { ... }")
		return ValidationRule{}, false
	}

	var rule ValidationRule
	var hasID, hasMessage, hasExpression bool
	for _, entry := range block.Entries {
		switch entry.Key.Name {
		case "id":
			strVal, ok := entry.Value.(*ast.StringLit)
			if !ok {
				l.errorf(entry.Key.Span.Start, "id must be a string")
				return ValidationRule{}, false
			}
			rule.ID = strVal.Value
			hasID = true
		case "message":
			strVal, ok := entry.Value.(*ast.StringLit)
			if !ok {
				l.errorf(entry.Key.Span.Start, "message must be a string")
				return ValidationRule{}, false
			}
			rule.Message = strVal.Value
			hasMessage = true
		case "expression":
			strVal, ok := entry.Value.(*ast.StringLit)
			if !ok {
				l.errorf(entry.Key.Span.Start, "expression must be a string")
				return ValidationRule{}, false
			}
			expr, err := ParseCELExpr(strVal.Value)
			if err != nil {
				l.errorf(entry.Key.Span.Start, "invalid CEL expression: %v", err)
				return ValidationRule{}, false
			}
			rule.Expression = expr
			hasExpression = true
		default:
			l.errorf(entry.Key.Span.Start, "unknown CEL validation key %q", entry.Key.Name)
			return ValidationRule{}, false
		}
	}

	if !hasID {
		l.errorf(opt.Key.Span.Start, "(builtin).cel block requires 'id' field")
		return ValidationRule{}, false
	}
	if !hasMessage {
		l.errorf(opt.Key.Span.Start, "(builtin).cel block requires 'message' field")
		return ValidationRule{}, false
	}
	if !hasExpression {
		l.errorf(opt.Key.Span.Start, "(builtin).cel block requires 'expression' field")
		return ValidationRule{}, false
	}

	return rule, true
}

func (l *lowerer) resolveCountRef(opt *ast.FieldOption) string {
	switch v := opt.Value.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.DottedIdent:
		parts := make([]string, len(v.Parts))
		for i, p := range v.Parts {
			parts[i] = p.Name
		}
		return joinDotted(parts)
	default:
		l.errorf(opt.Key.Span.Start, "count option must be a field reference")
		return ""
	}
}

func (l *lowerer) errorf(pos token.Pos, format string, args ...any) {
	p := l.pos(pos)
	l.errors = append(l.errors, LowerError{Pos: p, Message: fmt.Sprintf(format, args...)})
}

type LowerError struct {
	Pos     token.Position
	Message string
}

func (e LowerError) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Message) }

func parseEndian(s string) (Endian, bool) {
	switch s {
	case "big":
		return BigEndian, true
	case "little":
		return LittleEndian, true
	default:
		return LittleEndian, false
	}
}

func fitsInType(val int64, prim PrimitiveType) bool {
	switch prim {
	case U8:
		return val >= 0 && val <= 255
	case U16:
		return val >= 0 && val <= 65535
	case U32:
		return val >= 0 && val <= 4294967295
	case U64:
		return val >= 0
	case I8:
		return val >= -128 && val <= 127
	case I16:
		return val >= -32768 && val <= 32767
	case I32:
		return val >= -2147483648 && val <= 2147483647
	case I64:
		return true
	default:
		return false
	}
}

func joinDotted(parts []string) string {
	result := parts[0]
	for _, p := range parts[1:] {
		result += "." + p
	}
	return result
}
