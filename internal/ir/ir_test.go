package ir

import (
	"testing"

	"github.com/rdeusser/cleave/internal/lexer"
	"github.com/rdeusser/cleave/internal/parser"
	"github.com/rdeusser/cleave/internal/token"
)

func lowerSrc(t *testing.T, src string) (*Package, []LowerError) {
	t.Helper()
	p := parser.New("test.clv", []byte(src))
	file, parseErrs := p.Parse()
	if len(parseErrs) > 0 {
		t.Fatalf("parse errors: %v", parseErrs)
	}
	lex := lexer.New("test.clv", []byte(src))
	return Lower(file, lex.Position)
}

func TestLowerEnum(t *testing.T) {
	src := `package test;

enum Color : u8 {
    Red = 0;
    Green = 1;
    Blue = 2;
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(pkg.Enums) != 1 {
		t.Fatalf("expected 1 enum, got %d", len(pkg.Enums))
	}
	e := pkg.Enums[0]
	if e.Name != "Color" {
		t.Errorf("enum name: got %q, want %q", e.Name, "Color")
	}
	if e.BackingType != U8 {
		t.Errorf("backing type: got %v, want U8", e.BackingType)
	}
	if len(e.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(e.Variants))
	}
	if e.Variants[1].Name != "Green" || e.Variants[1].Value != 1 {
		t.Errorf("variant 1: got %+v", e.Variants[1])
	}
}

func TestLowerStruct(t *testing.T) {
	src := `package test;

struct Header {
    magic   u32;
    version u16;
    flags   u8;

    option (builtin) = {
        endian = big;
    };
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(pkg.Structs) != 1 {
		t.Fatalf("expected 1 struct, got %d", len(pkg.Structs))
	}
	s := pkg.Structs[0]
	if s.Name != "Header" {
		t.Errorf("struct name: got %q, want %q", s.Name, "Header")
	}
	if s.Options.Endian != BigEndian {
		t.Errorf("endian: got %v, want BigEndian", s.Options.Endian)
	}
	if len(s.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(s.Fields))
	}
	if s.Fields[0].Type.Primitive != U32 {
		t.Errorf("field 0 type: got %v, want U32", s.Fields[0].Type.Primitive)
	}
}

func TestLowerArrayFields(t *testing.T) {
	src := `package test;

struct Data {
    length  u32;
    payload bytes[length];
    fixed   bytes[8];
    items   u8 [rest = true];
    name    string [terminator = 0x00];
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	s := pkg.Structs[0]

	tests := []struct {
		name string
		kind ArrayKind
	}{
		{"length", NotArray},
		{"payload", LengthRef},
		{"fixed", FixedSize},
		{"items", RestArray},
		{"name", Terminator},
	}

	for i, tt := range tests {
		f := s.Fields[i]
		if f.Name != tt.name {
			t.Errorf("field %d: got name %q, want %q", i, f.Name, tt.name)
		}
		if f.Type.Array.Kind != tt.kind {
			t.Errorf("field %q: got array kind %v, want %v", tt.name, f.Type.Array.Kind, tt.kind)
		}
	}
}

func TestLowerEnumReference(t *testing.T) {
	src := `package test;

enum Status : u8 {
    Active = 1;
    Inactive = 2;
}

struct Record {
    id     u32;
    status Status;
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	s := pkg.Structs[0]
	f := s.Fields[1]
	if f.Type.Kind != KindEnum {
		t.Errorf("expected KindEnum, got %v", f.Type.Kind)
	}
	if f.Type.Ref != "Status" {
		t.Errorf("expected ref %q, got %q", "Status", f.Type.Ref)
	}
}

func TestLowerStructReference(t *testing.T) {
	src := `package test;

struct Inner {
    x u32;
}

struct Outer {
    inner Inner;
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	s := pkg.Structs[1]
	f := s.Fields[0]
	if f.Type.Kind != KindStruct {
		t.Errorf("expected KindStruct, got %v", f.Type.Kind)
	}
	if f.Type.Ref != "Inner" {
		t.Errorf("expected ref %q, got %q", "Inner", f.Type.Ref)
	}
}

func TestLowerErrorUnknownType(t *testing.T) {
	src := `package test;

struct Bad {
    data Nonexistent;
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected errors for unknown type")
	}
}

func TestLowerErrorDuplicateField(t *testing.T) {
	src := `package test;

struct Bad {
    x u32;
    x u16;
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected errors for duplicate field")
	}
}

func TestLowerErrorInvalidLengthRef(t *testing.T) {
	src := `package test;

struct Bad {
    data bytes[length];
    length u32;
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid length reference")
	}
}

func TestLowerErrorPositions(t *testing.T) {
	src := `package test;

struct Bad {
    data Nonexistent;
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected errors")
	}
	if errs[0].Pos == (token.Position{}) {
		t.Error("expected non-zero position on error")
	}
}

func TestLowerFormat(t *testing.T) {
	src := `package test;

format MyFormat {
    title     = "Test Format";
    extension = "bin";
    endian    = big;
    root      = Data;
}

struct Data {
    x u32;
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if pkg.Format == nil {
		t.Fatal("expected format block")
	}
	if pkg.Format.Name != "MyFormat" {
		t.Errorf("format name: got %q, want %q", pkg.Format.Name, "MyFormat")
	}
	if pkg.Format.Title != "Test Format" {
		t.Errorf("title: got %q, want %q", pkg.Format.Title, "Test Format")
	}
	if pkg.Format.Extension != "bin" {
		t.Errorf("extension: got %q, want %q", pkg.Format.Extension, "bin")
	}
	if pkg.Format.Root != "Data" {
		t.Errorf("root: got %q, want %q", pkg.Format.Root, "Data")
	}
	if pkg.Format.Endian != BigEndian {
		t.Errorf("endian: got %v, want big", pkg.Format.Endian)
	}
}

func TestLowerFormatEndianInheritance(t *testing.T) {
	src := `package test;

format MyFormat {
    endian = big;
    root   = Data;
}

struct Data {
    x u32;
}

struct Other {
    y u32;

    option (builtin) = {
        endian = little;
    };
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	// Data should inherit big from format.
	if pkg.Structs[0].Options.Endian != BigEndian {
		t.Errorf("Data endian: got %v, want big", pkg.Structs[0].Options.Endian)
	}
	// Other explicitly sets little.
	if pkg.Structs[1].Options.Endian != LittleEndian {
		t.Errorf("Other endian: got %v, want little", pkg.Structs[1].Options.Endian)
	}
}

func TestLowerCountRef(t *testing.T) {
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
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[1] // records
	if f.Type.Array.Kind != CountRef {
		t.Errorf("array kind: got %v, want CountRef", f.Type.Array.Kind)
	}
	if f.Type.Array.CountRef != "header.record_count" {
		t.Errorf("count ref: got %q, want %q", f.Type.Array.CountRef, "header.record_count")
	}
}

func TestLowerValidate(t *testing.T) {
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
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[0]
	if len(f.Validations) != 1 {
		t.Fatalf("expected 1 validation, got %d", len(f.Validations))
	}
	v := f.Validations[0]
	if v.ID != "version.min" {
		t.Errorf("id: got %q, want %q", v.ID, "version.min")
	}
	if v.Message != "version must be at least 1" {
		t.Errorf("message: got %q, want %q", v.Message, "version must be at least 1")
	}
	if v.Expression.Kind != ExprBinary || v.Expression.Op != ">=" {
		t.Errorf("expression: expected >= comparison, got %+v", v.Expression)
	}
}

func TestLowerCondition(t *testing.T) {
	src := `package test;

struct Footer {
    has_crc32 u8;
    crc32     u32 [if = "has_crc32 != 0"];
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[1]
	if f.Condition == nil {
		t.Fatal("expected condition on crc32 field")
	}
	if f.Condition.Kind != ExprBinary || f.Condition.Op != "!=" {
		t.Errorf("condition: expected != comparison, got %+v", f.Condition)
	}
}

func TestLowerConditionWithThis(t *testing.T) {
	src := `package test;

struct Bad {
    crc32 u32 [if = "this != 0"];
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for 'this' in if expression")
	}
}

func TestLowerValidateInvalidCEL(t *testing.T) {
	src := `package test;

struct Bad {
    version u16 [
        (builtin).cel = {
            id: "version.check"
            message: "bad"
            expression: "!!!"
        }
    ];
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for invalid CEL expression")
	}
}

func TestLowerConditionAndValidate(t *testing.T) {
	src := `package test;

struct Footer {
    flag  u8;
    value u32 [
        if = "flag != 0",
        (builtin).cel = {
            id: "value.positive"
            message: "value must be positive"
            expression: "this > 0"
        }
    ];
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[1]
	if f.Condition == nil {
		t.Fatal("expected condition on value field")
	}
	if len(f.Validations) != 1 {
		t.Fatalf("expected 1 validation, got %d", len(f.Validations))
	}
}

func TestLowerMultipleValidations(t *testing.T) {
	src := `package test;

struct Record {
    hostname string[256, terminator = 0x00] [
        (builtin).cel = {
            id: "hostname.valid"
            message: "hostname must be valid"
            expression: "size(this) > 0"
        },
        (builtin).cel = {
            id: "hostname.notlocalhost"
            message: "localhost is not permitted"
            expression: "this != 'localhost'"
        }
    ];
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[0]
	if len(f.Validations) != 2 {
		t.Fatalf("expected 2 validations, got %d", len(f.Validations))
	}
	if f.Validations[0].ID != "hostname.valid" {
		t.Errorf("validation 0 id: got %q, want %q", f.Validations[0].ID, "hostname.valid")
	}
	if f.Validations[1].ID != "hostname.notlocalhost" {
		t.Errorf("validation 1 id: got %q, want %q", f.Validations[1].ID, "hostname.notlocalhost")
	}
}

func TestLowerValidateMissingID(t *testing.T) {
	src := `package test;

struct Bad {
    version u16 [
        (builtin).cel = {
            message: "bad"
            expression: "this >= 1"
        }
    ];
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for missing id")
	}
}

func TestLowerValidateMissingMessage(t *testing.T) {
	src := `package test;

struct Bad {
    version u16 [
        (builtin).cel = {
            id: "version.check"
            expression: "this >= 1"
        }
    ];
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for missing message")
	}
}

func TestLowerValidateMissingExpression(t *testing.T) {
	src := `package test;

struct Bad {
    version u16 [
        (builtin).cel = {
            id: "version.check"
            message: "bad"
        }
    ];
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for missing expression")
	}
}

func TestLowerValidateNonBlock(t *testing.T) {
	src := `package test;

struct Bad {
    version u16 [(builtin).cel = "this >= 1"];
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for non-block (builtin).cel value")
	}
}

func TestLowerFixedTerminator(t *testing.T) {
	src := `package test;

struct Record {
    name string[64, terminator = 0x00];
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[0]
	if f.Type.Array.Kind != FixedTerminator {
		t.Errorf("array kind: got %v, want FixedTerminator", f.Type.Array.Kind)
	}
	if f.Type.Array.FixedSize != 64 {
		t.Errorf("fixed size: got %d, want %d", f.Type.Array.FixedSize, 64)
	}
	if f.Type.Array.Sentinel != 0 {
		t.Errorf("sentinel: got %d, want %d", f.Type.Array.Sentinel, 0)
	}
}

func TestLowerPng(t *testing.T) {
	src := `package png;

enum ColorType : u8 {
    Grayscale = 0;
    RGB = 2;
    Palette = 3;
}

struct Header {
    signature bytes[8];

    option (builtin) = {
        endian = big;
    };
}

struct Chunk {
    length  u32;
    type    bytes[4];
    data    bytes[length];
    crc     u32;

    option (builtin) = {
        endian = big;
    };
}

struct PngFile {
    header  Header;
    chunks  Chunk [rest = true];

    option (builtin) = {
        endian = big;
    };
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if pkg.Name != "png" {
		t.Errorf("package name: got %q, want %q", pkg.Name, "png")
	}
	if len(pkg.Enums) != 1 {
		t.Errorf("expected 1 enum, got %d", len(pkg.Enums))
	}
	if len(pkg.Structs) != 3 {
		t.Errorf("expected 3 structs, got %d", len(pkg.Structs))
	}
}

func TestLowerInlineMatch(t *testing.T) {
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
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[1]
	if f.Type.Kind != KindMatch {
		t.Fatalf("expected KindMatch, got %v", f.Type.Kind)
	}
	if f.Match == nil {
		t.Fatal("expected MatchSpec")
	}
	if f.Match.TagField != "type" {
		t.Errorf("tag field: got %q, want %q", f.Match.TagField, "type")
	}
	if len(f.Match.Cases) != 2 {
		t.Fatalf("expected 2 valued cases, got %d", len(f.Match.Cases))
	}
	if f.Match.Default == nil {
		t.Fatal("expected default case")
	}
	if f.Match.Cases[0].Type.Primitive != U32 {
		t.Errorf("case 0: got %v, want U32", f.Match.Cases[0].Type.Primitive)
	}
	if f.Match.Cases[1].Type.Primitive != F32 {
		t.Errorf("case 1: got %v, want F32", f.Match.Cases[1].Type.Primitive)
	}
}

func TestLowerUnionRef(t *testing.T) {
	src := `package test;

union ParamValue : u8 {
    0 => u32;
    1 => f32;
}

struct Event {
    type  u8;
    value ParamValue [tag = type];
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(pkg.Unions) != 1 {
		t.Fatalf("expected 1 union, got %d", len(pkg.Unions))
	}
	f := pkg.Structs[0].Fields[1]
	if f.Type.Kind != KindMatch {
		t.Fatalf("expected KindMatch, got %v", f.Type.Kind)
	}
	if f.Match.TagField != "type" {
		t.Errorf("tag field: got %q, want %q", f.Match.TagField, "type")
	}
	if len(f.Match.Cases) != 2 {
		t.Errorf("expected 2 cases, got %d", len(f.Match.Cases))
	}
}

func TestLowerMatchErrorTagMissing(t *testing.T) {
	src := `package test;

struct Bad {
    value match nonexistent {
        0 => u32;
    };
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for missing tag field")
	}
}

func TestLowerMatchErrorTagNotInteger(t *testing.T) {
	src := `package test;

struct Bad {
    name  bytes[8];
    value match name {
        0 => u32;
    };
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for non-integer tag field")
	}
}

func TestLowerMatchErrorDuplicateCase(t *testing.T) {
	src := `package test;

struct Bad {
    type  u8;
    value match type {
        0 => u32;
        0 => f32;
    };
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for duplicate case value")
	}
}

func TestLowerMatchErrorOutOfRange(t *testing.T) {
	src := `package test;

struct Bad {
    type  u8;
    value match type {
        256 => u32;
    };
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for out-of-range case value")
	}
}

func TestLowerUnionTagTypeMismatch(t *testing.T) {
	src := `package test;

union ParamValue : u8 {
    0 => u32;
}

struct Bad {
    type  u32;
    value ParamValue [tag = type];
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for tag type mismatch")
	}
}

func TestLowerUnionMissingTagOption(t *testing.T) {
	src := `package test;

union ParamValue : u8 {
    0 => u32;
}

struct Bad {
    type  u8;
    value ParamValue;
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for missing [tag = ...] option")
	}
}

func TestLowerEncodingInlineOption(t *testing.T) {
	src := `package test;

struct Record {
    name string[32, encoding = "CP949"];
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[0]
	if f.Encoding != "CP949" {
		t.Errorf("encoding: got %q, want %q", f.Encoding, "CP949")
	}
}

func TestLowerEncodingFieldOption(t *testing.T) {
	src := `package test;

struct Record {
    name string [terminator = 0x00, encoding = "SJIS"];
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[0]
	if f.Encoding != "SJIS" {
		t.Errorf("encoding: got %q, want %q", f.Encoding, "SJIS")
	}
	if f.Type.Array.Kind != Terminator {
		t.Errorf("array kind: got %v, want Terminator", f.Type.Array.Kind)
	}
}

func TestLowerEncodingFixedTerminator(t *testing.T) {
	src := `package test;

struct Record {
    name string[64, terminator = 0x00, encoding = "CP949"];
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[0]
	if f.Type.Array.Kind != FixedTerminator {
		t.Errorf("array kind: got %v, want FixedTerminator", f.Type.Array.Kind)
	}
	if f.Encoding != "CP949" {
		t.Errorf("encoding: got %q, want %q", f.Encoding, "CP949")
	}
}

func TestLowerEncodingOnBytesError(t *testing.T) {
	src := `package test;

struct Bad {
    data bytes[32, encoding = "CP949"];
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for encoding on bytes field")
	}
}

func TestLowerEncodingNonStringLiteral(t *testing.T) {
	src := `package test;

struct Bad {
    name string[32, encoding = 42];
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for non-string encoding value")
	}
}

func TestLowerEncodingConflict(t *testing.T) {
	src := `package test;

struct Bad {
    name string[32, encoding = "CP949"] [encoding = "SJIS"];
}
`
	_, errs := lowerSrc(t, src)
	if len(errs) == 0 {
		t.Fatal("expected error for duplicate encoding specification")
	}
}

func TestLowerMatchNoDefault(t *testing.T) {
	src := `package test;

struct Event {
    type  u8;
    value match type {
        0 => u32;
        1 => f32;
    };
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[0].Fields[1]
	if f.Match.Default != nil {
		t.Error("expected nil default")
	}
}

func TestLowerStructArrayDim(t *testing.T) {
	src := `package test;

struct Inner {
    x u32;
}

struct Outer {
    count  u32;
    items  Inner[count];
}
`
	pkg, errs := lowerSrc(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	f := pkg.Structs[1].Fields[1]
	if f.Type.Kind != KindStruct {
		t.Fatalf("expected KindStruct, got %v", f.Type.Kind)
	}
	if f.Type.Array.Kind != CountRef {
		t.Errorf("array kind: got %v, want CountRef", f.Type.Array.Kind)
	}
	if f.Type.Array.CountRef != "count" {
		t.Errorf("count ref: got %q, want %q", f.Type.Array.CountRef, "count")
	}
}
