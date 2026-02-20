package format

import (
	"os"
	"strings"
	"testing"

	"github.com/rdeusser/cleave/internal/parser"
)

func TestFormatIdempotent(t *testing.T) {
	src := `package test;

enum Color : u8 {
    Red   = 0;
    Green = 1;
    Blue  = 2;
}

struct Header {
    magic    u32;
    version  u16;
    flags    u8;
}
`
	// Parse, format, parse again, format again — must be identical.
	p1 := parser.New("test.clv", []byte(src))
	file1, errs := p1.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out1 := Format(file1)

	p2 := parser.New("test.clv", []byte(out1))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)

	if out1 != out2 {
		t.Errorf("format is not idempotent\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}
}

func TestFormatColumnAlignment(t *testing.T) {
	src := `package test;
struct Data {
    x u32;
    longname u16;
}
`
	p := parser.New("test.clv", []byte(src))
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out := Format(file)

	want := `package test;

struct Data {
    x         u32;
    longname  u16;
}
`
	if out != want {
		t.Errorf("format mismatch\ngot:\n%s\nwant:\n%s", out, want)
	}
}

func TestFormatWithOptions(t *testing.T) {
	src := `package test;
struct Chunk {
    length u32;
    data bytes[length];
    items Chunk [rest = true];
    option (builtin) = { endian = big; };
}
`
	p := parser.New("test.clv", []byte(src))
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out := Format(file)

	want := `package test;

struct Chunk {
    length  u32;
    data    bytes[length];
    items   Chunk         [rest = true];

    option (builtin) = {
        endian = big;
    };
}
`
	if out != want {
		t.Errorf("format mismatch\ngot:\n%s\nwant:\n%s", out, want)
	}
}

func TestFormatImports(t *testing.T) {
	src := `package test;

import "common.clv";
import "other.clv";

struct Foo {
    x u32;
}
`
	p := parser.New("test.clv", []byte(src))
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out := Format(file)

	want := `package test;

import "common.clv";
import "other.clv";

struct Foo {
    x  u32;
}
`
	if out != want {
		t.Errorf("format mismatch\ngot:\n%s\nwant:\n%s", out, want)
	}

	// Verify idempotency.
	p2 := parser.New("test.clv", []byte(out))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)
	if out != out2 {
		t.Errorf("format is not idempotent\nfirst:\n%s\nsecond:\n%s", out, out2)
	}
}

func TestFormatFixedTerminator(t *testing.T) {
	src := `package test;

struct Record {
    name string[64, terminator = 0x00];
}
`
	p := parser.New("test.clv", []byte(src))
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out := Format(file)

	want := `package test;

struct Record {
    name  string[64, terminator = 0x00];
}
`
	if out != want {
		t.Errorf("format mismatch\ngot:\n%s\nwant:\n%s", out, want)
	}

	// Verify idempotency.
	p2 := parser.New("test.clv", []byte(out))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)
	if out != out2 {
		t.Errorf("format is not idempotent\nfirst:\n%s\nsecond:\n%s", out, out2)
	}
}

func TestFormatEncodingInlineOption(t *testing.T) {
	src := `package test;

struct Record {
    name string[32, encoding = "CP949"];
}
`
	p := parser.New("test.clv", []byte(src))
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out := Format(file)

	want := `package test;

struct Record {
    name  string[32, encoding = "CP949"];
}
`
	if out != want {
		t.Errorf("format mismatch\ngot:\n%s\nwant:\n%s", out, want)
	}

	// Verify idempotency.
	p2 := parser.New("test.clv", []byte(out))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)
	if out != out2 {
		t.Errorf("format is not idempotent\nfirst:\n%s\nsecond:\n%s", out, out2)
	}
}

func TestFormatEncodingFieldOption(t *testing.T) {
	src := `package test;

struct Record {
    name string [terminator = 0x00, encoding = "CP949"];
}
`
	p := parser.New("test.clv", []byte(src))
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out := Format(file)

	want := `package test;

struct Record {
    name  string [terminator = 0x00, encoding = "CP949"];
}
`
	if out != want {
		t.Errorf("format mismatch\ngot:\n%s\nwant:\n%s", out, want)
	}

	// Verify idempotency.
	p2 := parser.New("test.clv", []byte(out))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)
	if out != out2 {
		t.Errorf("format is not idempotent\nfirst:\n%s\nsecond:\n%s", out, out2)
	}
}

func TestFormatValidateBlock(t *testing.T) {
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
	p := parser.New("test.clv", []byte(src))
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out1 := Format(file)

	p2 := parser.New("test.clv", []byte(out1))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)

	if out1 != out2 {
		t.Errorf("format is not idempotent\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}

	// Verify the output contains the expected multi-line structure.
	if !strings.Contains(out1, "(builtin).cel = {") {
		t.Error("missing (builtin).cel block in formatted output")
	}
	if !strings.Contains(out1, `id: "version.min"`) {
		t.Errorf("missing id entry in formatted output:\n%s", out1)
	}
}

func TestFormatMultipleValidateBlocks(t *testing.T) {
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
	p := parser.New("test.clv", []byte(src))
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out1 := Format(file)

	p2 := parser.New("test.clv", []byte(out1))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)

	if out1 != out2 {
		t.Errorf("format is not idempotent\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}
}

func TestFormatIfOption(t *testing.T) {
	src := `package test;

struct Footer {
    has_crc u8;
    crc     u32 [if = "has_crc != 0"];
}
`
	p := parser.New("test.clv", []byte(src))
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out1 := Format(file)

	p2 := parser.New("test.clv", []byte(out1))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)

	if out1 != out2 {
		t.Errorf("format is not idempotent\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}
}

func TestFormatTestdataSimple(t *testing.T) {
	src, err := os.ReadFile("../../testdata/simple.clv")
	if err != nil {
		t.Fatal(err)
	}

	p := parser.New("simple.clv", src)
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out1 := Format(file)

	// Re-parse the formatted output.
	p2 := parser.New("simple.clv", []byte(out1))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)

	if out1 != out2 {
		t.Errorf("format is not idempotent for simple.clv\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}
}

func TestFormatTestdataValidation(t *testing.T) {
	src, err := os.ReadFile("../../testdata/validation.clv")
	if err != nil {
		t.Fatal(err)
	}

	p := parser.New("validation.clv", src)
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out1 := Format(file)

	p2 := parser.New("validation.clv", []byte(out1))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)

	if out1 != out2 {
		t.Errorf("format is not idempotent for validation.clv\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}

	// Verify the output contains the expected multi-line validation blocks.
	if !strings.Contains(out1, "(builtin).cel = {") {
		t.Error("missing (builtin).cel block in formatted output")
	}
	if !strings.Contains(out1, `id: "magic.check"`) {
		t.Error("missing magic.check rule id")
	}
	if !strings.Contains(out1, `id: "version.min"`) {
		t.Error("missing version.min rule id")
	}
	if !strings.Contains(out1, `id: "version.max"`) {
		t.Error("missing version.max rule id")
	}
}

func TestFormatTestdataPng(t *testing.T) {
	src, err := os.ReadFile("../../testdata/png.clv")
	if err != nil {
		t.Fatal(err)
	}

	p := parser.New("png.clv", src)
	file, errs := p.Parse()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	out1 := Format(file)

	p2 := parser.New("png.clv", []byte(out1))
	file2, errs := p2.Parse()
	if len(errs) > 0 {
		t.Fatalf("reparse errors: %v", errs)
	}
	out2 := Format(file2)

	if out1 != out2 {
		t.Errorf("format is not idempotent for png.clv\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}
}
