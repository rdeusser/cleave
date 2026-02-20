package parser

import (
	"os"
	"testing"

	"github.com/rdeusser/cleave/internal/ast"
)

func TestParsePackageDecl(t *testing.T) {
	src := `package foo;`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if file.Package == nil {
		t.Fatal("expected package declaration")
	}
	if file.Package.Name.Name != "foo" {
		t.Errorf("package name: got %q, want %q", file.Package.Name.Name, "foo")
	}
}

func TestParseEnum(t *testing.T) {
	src := `package test;

enum Color : u8 {
    Red = 0;
    Green = 1;
    Blue = 2;
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(file.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(file.Decls))
	}
	enum, ok := file.Decls[0].(*ast.EnumDecl)
	if !ok {
		t.Fatalf("expected EnumDecl, got %T", file.Decls[0])
	}
	if enum.Name.Name != "Color" {
		t.Errorf("enum name: got %q, want %q", enum.Name.Name, "Color")
	}
	if enum.BackingType.Name.Name != "u8" {
		t.Errorf("backing type: got %q, want %q", enum.BackingType.Name.Name, "u8")
	}
	if len(enum.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(enum.Variants))
	}
	if enum.Variants[0].Name.Name != "Red" {
		t.Errorf("variant 0: got %q, want %q", enum.Variants[0].Name.Name, "Red")
	}
}

func TestParseStruct(t *testing.T) {
	src := `package test;

struct Header {
    magic   u32;
    version u16;
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(file.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(file.Decls))
	}
	s, ok := file.Decls[0].(*ast.StructDecl)
	if !ok {
		t.Fatalf("expected StructDecl, got %T", file.Decls[0])
	}
	if s.Name.Name != "Header" {
		t.Errorf("struct name: got %q, want %q", s.Name.Name, "Header")
	}
	if len(s.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(s.Fields))
	}
	if s.Fields[0].Name.Name != "magic" {
		t.Errorf("field 0 name: got %q, want %q", s.Fields[0].Name.Name, "magic")
	}
	if s.Fields[0].Type.Name.Name != "u32" {
		t.Errorf("field 0 type: got %q, want %q", s.Fields[0].Type.Name.Name, "u32")
	}
}

func TestParseArrayField(t *testing.T) {
	src := `package test;

struct Data {
    length  u32;
    payload bytes[length];
    fixed   bytes[8];
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	s := file.Decls[0].(*ast.StructDecl)

	// payload bytes[length]
	f1 := s.Fields[1]
	if f1.Type.Dim == nil {
		t.Fatal("expected array dimension on payload")
	}
	dimIdent, ok := f1.Type.Dim.(*ast.Ident)
	if !ok {
		t.Fatalf("expected ident dim, got %T", f1.Type.Dim)
	}
	if dimIdent.Name != "length" {
		t.Errorf("dim name: got %q, want %q", dimIdent.Name, "length")
	}

	// fixed bytes[8]
	f2 := s.Fields[2]
	if f2.Type.Dim == nil {
		t.Fatal("expected array dimension on fixed")
	}
	dimInt, ok := f2.Type.Dim.(*ast.IntegerLit)
	if !ok {
		t.Fatalf("expected int dim, got %T", f2.Type.Dim)
	}
	if dimInt.Value != 8 {
		t.Errorf("dim value: got %d, want %d", dimInt.Value, 8)
	}
}

func TestParseFieldOptions(t *testing.T) {
	src := `package test;

struct Data {
    items Chunk [rest = true];
    name  string [terminator = 0x00];
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	s := file.Decls[0].(*ast.StructDecl)

	// items Chunk [rest = true]
	f0 := s.Fields[0]
	if len(f0.Options) != 1 {
		t.Fatalf("expected 1 field option, got %d", len(f0.Options))
	}
	if f0.Options[0].Key.Name != "rest" {
		t.Errorf("option key: got %q, want %q", f0.Options[0].Key.Name, "rest")
	}
	boolVal, ok := f0.Options[0].Value.(*ast.BoolLit)
	if !ok {
		t.Fatalf("expected BoolLit, got %T", f0.Options[0].Value)
	}
	if !boolVal.Value {
		t.Error("expected rest = true")
	}

	// name string [terminator = 0x00]
	f1 := s.Fields[1]
	if len(f1.Options) != 1 {
		t.Fatalf("expected 1 field option, got %d", len(f1.Options))
	}
	if f1.Options[0].Key.Name != "terminator" {
		t.Errorf("option key: got %q, want %q", f1.Options[0].Key.Name, "terminator")
	}
}

func TestParseOptionBlock(t *testing.T) {
	src := `package test;

struct Chunk {
    length u32;

    option (builtin) = {
        endian = big;
    };
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	s := file.Decls[0].(*ast.StructDecl)
	if len(s.Options) != 1 {
		t.Fatalf("expected 1 option block, got %d", len(s.Options))
	}
	ob := s.Options[0]
	if ob.Namespace.Name != "builtin" {
		t.Errorf("namespace: got %q, want %q", ob.Namespace.Name, "builtin")
	}
	if len(ob.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(ob.Entries))
	}
	if ob.Entries[0].Key.Name != "endian" {
		t.Errorf("key: got %q, want %q", ob.Entries[0].Key.Name, "endian")
	}
}

func TestParseComments(t *testing.T) {
	src := `// file comment
package test;

// struct comment
struct Foo {
    x u32;
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(file.Comments) < 2 {
		t.Errorf("expected at least 2 comments, got %d", len(file.Comments))
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"missing package", "struct Foo { x u32; }"},
		{"missing semicolon", "package test; struct Foo { x u32 }"},
		{"missing brace", "package test; struct Foo { x u32;"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New("test.clv", []byte(tt.src))
			_, errs := p.Parse()
			if len(errs) == 0 {
				t.Error("expected parse errors")
			}
		})
	}
}

func TestParseFormatDecl(t *testing.T) {
	src := `package test;

format MyFormat {
    title     = "Test Format";
    extension = "bin";
    endian    = little;
    root      = Data;
}

struct Data {
    x u32;
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(file.Decls) != 2 {
		t.Fatalf("expected 2 decls, got %d", len(file.Decls))
	}
	fd, ok := file.Decls[0].(*ast.FormatDecl)
	if !ok {
		t.Fatalf("expected FormatDecl, got %T", file.Decls[0])
	}
	if fd.Name.Name != "MyFormat" {
		t.Errorf("format name: got %q, want %q", fd.Name.Name, "MyFormat")
	}
	if len(fd.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(fd.Entries))
	}
	if fd.Entries[0].Key.Name != "title" {
		t.Errorf("entry 0 key: got %q, want %q", fd.Entries[0].Key.Name, "title")
	}
	titleVal, ok := fd.Entries[0].Value.(*ast.StringLit)
	if !ok {
		t.Fatalf("expected StringLit for title, got %T", fd.Entries[0].Value)
	}
	if titleVal.Value != "Test Format" {
		t.Errorf("title: got %q, want %q", titleVal.Value, "Test Format")
	}
}

func TestParseDottedIdentInFieldOption(t *testing.T) {
	src := `package test;

struct File {
    header  Header;
    records Record [count = header.record_count];
}

struct Header {
    record_count u32;
}

struct Record {
    x u32;
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	s := file.Decls[0].(*ast.StructDecl)
	f := s.Fields[1] // records
	if len(f.Options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(f.Options))
	}
	if f.Options[0].Key.Name != "count" {
		t.Errorf("option key: got %q, want %q", f.Options[0].Key.Name, "count")
	}
	dotted, ok := f.Options[0].Value.(*ast.DottedIdent)
	if !ok {
		t.Fatalf("expected DottedIdent, got %T", f.Options[0].Value)
	}
	if len(dotted.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(dotted.Parts))
	}
	if dotted.Parts[0].Name != "header" || dotted.Parts[1].Name != "record_count" {
		t.Errorf("dotted ident: got %s.%s, want header.record_count", dotted.Parts[0].Name, dotted.Parts[1].Name)
	}
}

func TestParseValidateBlock(t *testing.T) {
	src := `package test;

struct Header {
    version u16 [
        (builtin).cel = {
            id: "version.min"
            message: "version must be at least 1"
            expression: "this >= 1"
        }
    ];
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	s := file.Decls[0].(*ast.StructDecl)
	f := s.Fields[0]
	if len(f.Options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(f.Options))
	}
	opt := f.Options[0]
	if opt.Namespace == nil {
		t.Fatal("expected namespace on option")
	}
	if opt.Namespace.Name != "builtin" {
		t.Errorf("namespace: got %q, want %q", opt.Namespace.Name, "builtin")
	}
	if opt.Key.Name != "cel" {
		t.Errorf("key: got %q, want %q", opt.Key.Name, "cel")
	}
	block, ok := opt.Value.(*ast.BlockExpr)
	if !ok {
		t.Fatalf("expected BlockExpr, got %T", opt.Value)
	}
	if len(block.Entries) != 3 {
		t.Fatalf("expected 3 block entries, got %d", len(block.Entries))
	}
	if block.Entries[0].Key.Name != "id" {
		t.Errorf("entry 0 key: got %q, want %q", block.Entries[0].Key.Name, "id")
	}
	if block.Entries[1].Key.Name != "message" {
		t.Errorf("entry 1 key: got %q, want %q", block.Entries[1].Key.Name, "message")
	}
	if block.Entries[2].Key.Name != "expression" {
		t.Errorf("entry 2 key: got %q, want %q", block.Entries[2].Key.Name, "expression")
	}
}

func TestParseMultipleValidateBlocks(t *testing.T) {
	src := `package test;

struct Header {
    version u16 [
        (builtin).cel = {
            id: "version.min"
            message: "too low"
            expression: "this >= 1"
        },
        (builtin).cel = {
            id: "version.max"
            message: "too high"
            expression: "this <= 10"
        }
    ];
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	s := file.Decls[0].(*ast.StructDecl)
	f := s.Fields[0]
	if len(f.Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(f.Options))
	}
	for i, opt := range f.Options {
		if opt.Namespace == nil || opt.Namespace.Name != "builtin" {
			t.Errorf("option %d: expected builtin namespace", i)
		}
		if opt.Key.Name != "cel" {
			t.Errorf("option %d: expected key %q, got %q", i, "cel", opt.Key.Name)
		}
		if _, ok := opt.Value.(*ast.BlockExpr); !ok {
			t.Errorf("option %d: expected BlockExpr, got %T", i, opt.Value)
		}
	}
}

func TestParseImportDecl(t *testing.T) {
	src := `package test;

import "common.clv";
import "other.clv";

struct Foo {
    x u32;
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(file.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(file.Imports))
	}
	if file.Imports[0].Path.Value != "common.clv" {
		t.Errorf("import 0 path: got %q, want %q", file.Imports[0].Path.Value, "common.clv")
	}
	if file.Imports[1].Path.Value != "other.clv" {
		t.Errorf("import 1 path: got %q, want %q", file.Imports[1].Path.Value, "other.clv")
	}
	if len(file.Decls) != 1 {
		t.Errorf("expected 1 decl, got %d", len(file.Decls))
	}
}

func TestParseInlineOptions(t *testing.T) {
	src := `package test;

struct Record {
    name string[64, terminator = 0x00];
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	s := file.Decls[0].(*ast.StructDecl)
	f := s.Fields[0]
	if f.Type.Dim == nil {
		t.Fatal("expected array dimension on name")
	}
	dimInt, ok := f.Type.Dim.(*ast.IntegerLit)
	if !ok {
		t.Fatalf("expected IntegerLit dim, got %T", f.Type.Dim)
	}
	if dimInt.Value != 64 {
		t.Errorf("dim value: got %d, want %d", dimInt.Value, 64)
	}
	if len(f.Type.InlineOptions) != 1 {
		t.Fatalf("expected 1 inline option, got %d", len(f.Type.InlineOptions))
	}
	if f.Type.InlineOptions[0].Key.Name != "terminator" {
		t.Errorf("inline option key: got %q, want %q", f.Type.InlineOptions[0].Key.Name, "terminator")
	}
	intVal, ok := f.Type.InlineOptions[0].Value.(*ast.IntegerLit)
	if !ok {
		t.Fatalf("expected IntegerLit value, got %T", f.Type.InlineOptions[0].Value)
	}
	if intVal.Value != 0 {
		t.Errorf("inline option value: got %d, want %d", intVal.Value, 0)
	}
}

func TestParseInlineMatch(t *testing.T) {
	src := `package test;

struct Event {
    type  u8;
    value match type {
        0 => u32;
        1 => f32;
        _ => bytes[4];
    };
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	s := file.Decls[0].(*ast.StructDecl)
	f := s.Fields[1] // value
	if f.Match == nil {
		t.Fatal("expected match expression on value field")
	}
	if f.Match.Tag.Name != "type" {
		t.Errorf("tag: got %q, want %q", f.Match.Tag.Name, "type")
	}
	if len(f.Match.Cases) != 3 {
		t.Fatalf("expected 3 cases, got %d", len(f.Match.Cases))
	}
	// Check default case (underscore ident).
	defaultCase := f.Match.Cases[2]
	ident, ok := defaultCase.Value.(*ast.Ident)
	if !ok || ident.Name != "_" {
		t.Errorf("expected default case '_', got %T", defaultCase.Value)
	}
}

func TestParseUnionDecl(t *testing.T) {
	src := `package test;

union ParamValue : u32 {
    0 => u32;
    1 => f32;
    2 => StringData;
    _ => bytes[4];
}

struct StringData {
    length u16;
    data   bytes[length];
}

struct Event {
    tag   u32;
    value ParamValue [tag = tag];
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(file.Decls) != 3 {
		t.Fatalf("expected 3 decls, got %d", len(file.Decls))
	}
	ud, ok := file.Decls[0].(*ast.UnionDecl)
	if !ok {
		t.Fatalf("expected UnionDecl, got %T", file.Decls[0])
	}
	if ud.Name.Name != "ParamValue" {
		t.Errorf("union name: got %q, want %q", ud.Name.Name, "ParamValue")
	}
	if ud.BackingType.Name.Name != "u32" {
		t.Errorf("backing type: got %q, want %q", ud.BackingType.Name.Name, "u32")
	}
	if len(ud.Cases) != 4 {
		t.Fatalf("expected 4 cases, got %d", len(ud.Cases))
	}
}

func TestParseNegativeCase(t *testing.T) {
	src := `package test;

struct Event {
    type  i8;
    value match type {
        -1 => u32;
        0  => f32;
    };
}
`
	p := New("test.clv", []byte(src))
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	s := file.Decls[0].(*ast.StructDecl)
	f := s.Fields[1]
	c := f.Match.Cases[0]
	intLit, ok := c.Value.(*ast.IntegerLit)
	if !ok {
		t.Fatalf("expected IntegerLit, got %T", c.Value)
	}
	if intLit.Value != -1 {
		t.Errorf("case value: got %d, want -1", intLit.Value)
	}
}

func TestParseTestdataSimple(t *testing.T) {
	src, err := os.ReadFile("../../testdata/simple.clv")
	if err != nil {
		t.Fatal(err)
	}

	p := New("simple.clv", src)
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if file.Package.Name.Name != "simple" {
		t.Errorf("package: got %q, want %q", file.Package.Name.Name, "simple")
	}
	if len(file.Decls) != 3 {
		t.Errorf("expected 3 decls, got %d", len(file.Decls))
	}
}

func TestParseEnumNegativeValues(t *testing.T) {
	src := []byte(`package test;
enum SkillType : i32 {
    NONE = -1;
    Melee = 0;
    Range = 1;
}
`)
	p := New("test.clv", src)
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(file.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(file.Decls))
	}

	enumDecl, ok := file.Decls[0].(*ast.EnumDecl)
	if !ok {
		t.Fatalf("expected EnumDecl, got %T", file.Decls[0])
	}
	if len(enumDecl.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(enumDecl.Variants))
	}

	v0 := enumDecl.Variants[0]
	intLit, ok := v0.Value.(*ast.IntegerLit)
	if !ok {
		t.Fatalf("variant 0 value: expected *ast.IntegerLit, got %T", v0.Value)
	}
	if intLit.Value != -1 {
		t.Errorf("variant 0 value: got %d, want -1", intLit.Value)
	}
	if intLit.Raw != "-1" {
		t.Errorf("variant 0 raw: got %q, want %q", intLit.Raw, "-1")
	}
}

func TestParseTestdataPng(t *testing.T) {
	src, err := os.ReadFile("../../testdata/png.clv")
	if err != nil {
		t.Fatal(err)
	}

	p := New("png.clv", src)
	file, errs := p.Parse()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if file.Package.Name.Name != "png" {
		t.Errorf("package: got %q, want %q", file.Package.Name.Name, "png")
	}
	if len(file.Decls) != 5 {
		t.Errorf("expected 5 decls (1 enum + 4 structs), got %d", len(file.Decls))
	}
}
