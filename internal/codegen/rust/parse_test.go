package rust

import (
	"strings"
	"testing"

	"github.com/rdeusser/cleave/internal/ir"
)

func TestGenerator_GenerateScalarParse(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{NoCargo: true}, `package sample;

enum Color : u8 {
    Red = 1;
    Blue = 2;
}

struct Child {
    value u16;
}

struct Packet {
    magic  u16;
    signed i32;
    ratio  f32;
    color  Color;
    child  Child;

    option (builtin) = {
        endian = big;
    };
}
`)
	source := string(files[0].Content)
	for _, want := range []string{
		"pub enum Error {",
		"UnexpectedEof {",
		"InvalidEnum {",
		"impl ::std::fmt::Display for Error",
		"impl ::std::error::Error for Error",
		"impl ::std::convert::TryFrom<u8> for Color",
		"pub fn parse(buf: &[u8], offset: &mut usize) -> ::std::result::Result<Self, Error>",
		"u16::from_be_bytes(self::read_array::<2>(buf, offset)?)",
		"i32::from_be_bytes(self::read_array::<4>(buf, offset)?)",
		"f32::from_be_bytes(self::read_array::<4>(buf, offset)?)",
		"Color::try_from(",
		"Child::parse(buf, offset)?",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated source is missing %q:\n%s", want, source)
		}
	}
}

func TestGenerator_GenerateArrayAndValidationParse(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{NoCargo: true}, `package sample;

struct Item {
    value u8;
}

struct Header {
    n       u8;
    enabled u8;
    extra   u8 [if = "enabled != 0"];
}

struct Packet {
    header  Header;
    fixed   u16[2];
    length  u8;
    payload bytes[length];
    name    string[6, terminator = 0x00];
    values  u8 [terminator = 0xff];
    items   Item [count = header.n];
    guarded u8 [if = "has(header.extra)"];
    checked u8 [
        (builtin).cel = {
            id: "checked.range"
            message: "checked must be between 1 and 3"
            expression: "this >= 1 && this <= 3"
        }
    ];
    tail    bytes [rest = true];
}
`)
	source := string(files[0].Content)
	for _, want := range []string{
		"pub fixed: [u16; 2],",
		"pub payload: ::std::vec::Vec<u8>,",
		"pub name: ::std::string::String,",
		"pub items: ::std::vec::Vec<Item>,",
		"let count = usize::try_from(raw_count)",
		"while *offset < buf.len()",
		"decode_text(",
		"if enabled != 0",
		"if (header.extra).is_some()",
		"Error::Validation {",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated source is missing %q:\n%s", want, source)
		}
	}

	restFiles := generateFromSource(t, &Generator{NoCargo: true}, `package sample;

struct Empty {
}

struct Container {
    items Empty [rest = true];
}
`)
	restSource := string(restFiles[0].Content)
	if !strings.Contains(restSource, "Error::NoProgress") {
		t.Errorf("generated nested rest parser lacks progress guard:\n%s", restSource)
	}
}

func TestEmitRustExpr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		this   string
		want   string
	}{
		{name: "binary", source: "this >= 1 && this <= 3", this: "value", want: "((value >= 1) && (value <= 3))"},
		{name: "global size", source: "size(this) > 0", this: "value", want: "((value).len() > 0)"},
		{name: "member size", source: "this.size() == 2", this: "value", want: "((value).len() == 2)"},
		{name: "has selection", source: "has(header.extra)", want: "(header.extra).is_some()"},
		{name: "starts with", source: `this.startsWith("a")`, this: "value", want: `self::text_starts_with((value).as_ref(), "a")`},
		{name: "ends with", source: `this.endsWith("z")`, this: "value", want: `self::text_ends_with((value).as_ref(), "z")`},
		{name: "contains", source: `this.contains("x")`, this: "value", want: `self::text_contains((value).as_ref(), "x")`},
		{name: "matches", source: `this.matches("^a")`, this: "value", want: `self::regex_matches((value).as_ref(), "^a")?`},
		{name: "dynamic prefix", source: `this.startsWith(prefix)`, this: "value", want: `self::text_starts_with((value).as_ref(), (prefix).as_ref())`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expression, err := ir.ParseCELExpr(tt.source)
			if err != nil {
				t.Fatalf("ir.ParseCELExpr(%q) error = %v", tt.source, err)
			}
			got, err := emitRustExpr(expression, tt.this)
			if err != nil {
				t.Fatalf("emitRustExpr(%q) error = %v", tt.source, err)
			}
			if got != tt.want {
				t.Errorf("emitRustExpr(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestTrimOuterParentheses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expression string
		want       string
		wantOK     bool
	}{
		{name: "binary", expression: "(enabled != 0)", want: "enabled != 0", wantOK: true},
		{
			name:       "nested binary",
			expression: "((value >= 1) && (value <= 3))",
			want:       "(value >= 1) && (value <= 3)",
			wantOK:     true,
		},
		{
			name:       "member call",
			expression: "(header.extra).is_some()",
			want:       "(header.extra).is_some()",
		},
		{
			name:       "parenthesis in string",
			expression: `(regex_matches(value, ")")?)`,
			want:       `regex_matches(value, ")")?`,
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := trimOuterParentheses(tt.expression)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf(
					"trimOuterParentheses(%q) = (%q, %v), want (%q, %v)",
					tt.expression,
					got,
					ok,
					tt.want,
					tt.wantOK,
				)
			}
		})
	}
}

func TestGenerator_GenerateMatchParse(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{NoCargo: true}, `package sample;

struct Bytes {
    value u8;
}

struct Event {
    tag   u8;
    value match tag {
        0 => u32;
        1 => f32;
        2 => u32;
        3 => Bytes;
        _ => bytes[4];
    };
}
`)
	source := string(files[0].Content)
	for _, want := range []string{
		"pub enum EventValue {",
		"U32(u32),",
		"F32(f32),",
		"Bytes(Bytes),",
		"Bytes2([u8; 4]),",
		"0 => EventValue::U32(",
		"1 => EventValue::F32(",
		"2 => EventValue::U32(",
		"3 => EventValue::Bytes(",
		"_ => EventValue::Bytes2(",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated source is missing %q:\n%s", want, source)
		}
	}
	if got := strings.Count(source, "U32(u32),"); got != 1 {
		t.Errorf("generated U32 payload variants = %d, want 1:\n%s", got, source)
	}
}

func TestGenerator_MatchDeduplicatesConcreteTypes(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{NoCargo: true}, `package sample;

struct Event {
    first_length  u8;
    second_length u8;
    tag           u8;
    value match tag {
        1 => bytes[first_length];
        2 => bytes[second_length];
    };
}
`)
	source := string(files[0].Content)
	if strings.Contains(source, "Bytes2(::std::vec::Vec<u8>)") {
		t.Errorf("generated distinct variants for one concrete Rust type:\n%s", source)
	}
	if got := strings.Count(source, "=> EventValue::Bytes("); got != 2 {
		t.Errorf("generated EventValue::Bytes parse arms = %d, want 2:\n%s", got, source)
	}
}

func TestGenerator_GenerateMatchWithoutDefault(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{NoCargo: true}, `package sample;

struct Event {
    tag   u8;
    value match tag {
        1 => u16;
    };
}
`)
	source := string(files[0].Content)
	for _, want := range []string{
		"return Err(Error::UnknownTag {",
		`struct_name: "Event"`,
		`field: "value"`,
		"value: i128::from(tag)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated source is missing %q:\n%s", want, source)
		}
	}
}
