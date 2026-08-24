//go:build integration

package rust

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rdeusser/cleave/internal/codegen"
)

func TestGeneratedRust_ScalarParse(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

enum Color : u8 {
    Red = 1;
    Blue = 2;
}

struct Child {
    value u16;

    option (builtin) = {
        endian = little;
    };
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

	runGeneratedRustTests(t, files, `use sample::{Color, Error, Packet};

#[test]
fn parses_scalars_and_nested_structs() {
    let buf = [
        0x12, 0x34,
        0xff, 0xff, 0xff, 0xfe,
        0x3f, 0xc0, 0x00, 0x00,
        0x02,
        0x34, 0x12,
    ];
    let mut offset = 0;
    let got = Packet::parse(&buf, &mut offset).expect("valid packet");
    assert_eq!(got.magic, 0x1234);
    assert_eq!(got.signed, -2);
    assert_eq!(got.ratio, 1.5);
    assert_eq!(got.color, Color::Blue);
    assert_eq!(got.child.value, 0x1234);
    assert_eq!(offset, buf.len());
}

#[test]
fn reports_unexpected_eof() {
    let mut offset = 0;
    let error = Packet::parse(&[0x12], &mut offset).expect_err("truncated packet");
    assert!(matches!(error, Error::UnexpectedEof { .. }));
}

#[test]
fn reports_invalid_enum() {
    let buf = [
        0x12, 0x34,
        0xff, 0xff, 0xff, 0xfe,
        0x3f, 0xc0, 0x00, 0x00,
        0x09,
        0x34, 0x12,
    ];
    let mut offset = 0;
    let error = Packet::parse(&buf, &mut offset).expect_err("invalid enum");
    assert!(matches!(
        error,
        Error::InvalidEnum {
            enum_name: "Color",
            value: 9,
        }
    ));
}
`)
}

func TestGeneratedRust_ArraysConditionsAndValidation(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

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
    tail bytes [rest = true];
}
`)

	runGeneratedRustTests(t, files, `use sample::{Error, Packet};

#[test]
fn parses_arrays_strings_and_conditions() {
    let buf = [
        2, 1, 9,
        1, 0, 2, 0,
        3, 7, 8, 9,
        b'h', b'i', 0, 0, 0, 0,
        4, 5, 0xff,
        6, 7,
        10,
        2,
        11, 12,
    ];
    let mut offset = 0;
    let got = Packet::parse(&buf, &mut offset).expect("valid packet");
    assert_eq!(got.header.extra, Some(9));
    assert_eq!(got.fixed, [1, 2]);
    assert_eq!(got.payload, vec![7, 8, 9]);
    assert_eq!(got.name, "hi");
    assert_eq!(got.values, vec![4, 5]);
    assert_eq!(got.items.len(), 2);
    assert_eq!(got.items[0].value, 6);
    assert_eq!(got.items[1].value, 7);
    assert_eq!(got.guarded, Some(10));
    assert_eq!(got.checked, 2);
    assert_eq!(got.tail, vec![11, 12]);
    assert_eq!(offset, buf.len());
}

#[test]
fn rejects_failed_validation() {
    let buf = [
        0, 0,
        1, 0, 2, 0,
        0,
        b'h', b'i', 0, 0, 0, 0,
        0xff,
        9,
        0,
    ];
    let mut offset = 0;
    let error = Packet::parse(&buf, &mut offset).expect_err("invalid checked field");
    assert!(matches!(
        error,
        Error::Validation {
            id: "checked.range",
            message: "checked must be between 1 and 3",
            field: "checked",
        }
    ));
}
`)
}

func TestGeneratedRust_RestArrayDetectsNoProgress(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

struct Empty {
}

struct Container {
    items Empty [rest = true];
}
`)

	runGeneratedRustTests(t, files, `use sample::{Container, Error};

#[test]
fn detects_no_progress() {
    let mut offset = 0;
    let error = Container::parse(&[1], &mut offset).expect_err("empty child cannot consume input");
    assert!(matches!(
        error,
        Error::NoProgress {
            field: "items",
            offset: 0,
        }
    ));
}
`)
}

func TestGeneratedRust_DiscriminatedUnions(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

struct Payload {
    n u16;
}

struct Event {
    tag   u8;
    value match tag {
        1 => u32;
        2 => Payload;
        3 => u32;
        _ => bytes[2];
    };
}

struct Closed {
    tag   u8;
    value match tag {
        1 => u16;
    };
}
`)

	runGeneratedRustTests(t, files, `use sample::{Closed, Error, Event, EventValue};

#[test]
fn parses_each_payload_shape() {
    let mut offset = 0;
    let integer = Event::parse(&[1, 0x78, 0x56, 0x34, 0x12], &mut offset).expect("integer");
    assert_eq!(integer.value, EventValue::U32(0x1234_5678));

    offset = 0;
    let nested = Event::parse(&[2, 0x34, 0x12], &mut offset).expect("nested");
    match nested.value {
        EventValue::Payload(value) => assert_eq!(value.n, 0x1234),
        value => panic!("unexpected payload: {value:?}"),
    }

    offset = 0;
    let repeated = Event::parse(&[3, 1, 0, 0, 0], &mut offset).expect("repeated type");
    assert_eq!(repeated.value, EventValue::U32(1));

    offset = 0;
    let fallback = Event::parse(&[9, 7, 8], &mut offset).expect("default");
    assert_eq!(fallback.value, EventValue::Bytes([7, 8]));
}

#[test]
fn rejects_unknown_tag_without_default() {
    let mut offset = 0;
    let error = Closed::parse(&[9], &mut offset).expect_err("unknown tag");
    assert!(matches!(
        error,
        Error::UnknownTag {
            struct_name: "Closed",
            field: "value",
            value: 9,
        }
    ));
}
`)
}

func TestGeneratedRust_Serialization(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

struct Item {
    value u16;
}

struct Packet {
    count   u8;
    items   Item [count = count];
    length  u8;
    payload bytes[length];
    fixed   string[4, encoding = "utf-8"];
    name    string[5, terminator = 0x00];
    values  u16[2];
    tail    u8 [terminator = 0xff];
}
`)

	runGeneratedRustTests(t, files, `use sample::{Error, Item, Packet};

#[test]
fn serializes_and_round_trips() {
    let packet = Packet {
        count: 99,
        items: vec![Item { value: 0x1234 }, Item { value: 0x5678 }],
        length: 99,
        payload: vec![1, 2, 3],
        fixed: "ABCD".to_owned(),
        name: "hi".to_owned(),
        values: [4, 5],
        tail: vec![6, 7],
    };
    let bytes = packet.to_bytes().expect("serializable packet");
    assert_eq!(
        bytes,
        vec![
            2,
            0x34, 0x12, 0x78, 0x56,
            3, 1, 2, 3,
            b'A', b'B', b'C', b'D',
            b'h', b'i', 0, 0, 0,
            4, 0, 5, 0,
            6, 7, 0xff,
        ],
    );

    let mut offset = 0;
    let parsed = Packet::parse(&bytes, &mut offset).expect("round trip parse");
    assert_eq!(parsed.count, 2);
    assert_eq!(parsed.items.len(), 2);
    assert_eq!(parsed.length, 3);
    assert_eq!(parsed.payload, vec![1, 2, 3]);
    assert_eq!(parsed.fixed, "ABCD");
    assert_eq!(parsed.name, "hi");
    assert_eq!(parsed.values, [4, 5]);
    assert_eq!(parsed.tail, vec![6, 7]);
}

#[test]
fn rejects_auto_length_overflow() {
    let packet = Packet {
        count: 0,
        items: Vec::new(),
        length: 0,
        payload: vec![0; 256],
        fixed: "ABCD".to_owned(),
        name: String::new(),
        values: [0, 0],
        tail: Vec::new(),
    };
    let error = packet.to_bytes().expect_err("u8 length overflow");
    assert!(matches!(
        error,
        Error::LengthOverflow {
            field: "length",
            target: "u8",
            ..
        }
    ));
}

#[test]
fn rejects_inexact_fixed_text() {
    let packet = Packet {
        count: 0,
        items: Vec::new(),
        length: 0,
        payload: Vec::new(),
        fixed: "no".to_owned(),
        name: String::new(),
        values: [0, 0],
        tail: Vec::new(),
    };
    let error = packet.to_bytes().expect_err("short fixed string");
    assert!(matches!(
        error,
        Error::FixedSize {
            field: "fixed",
            expected: 4,
            actual: 2,
        }
    ));
}
`)
}

func TestGeneratedRust_SerializesConditionalLengthSources(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

struct Item {
    value u8;
}

struct Packet {
    enabled u8;
    length  u8;
    payload bytes[length] [if = "enabled != 0"];
    count   u8;
    items   Item [count = count, if = "enabled != 0"];
    text_length u8;
    text        string[text_length, encoding = "utf-8"] [if = "enabled != 0"];
}
`)
	runGeneratedRustTests(t, files, `use sample::{Item, Packet};

#[test]
fn computes_zero_for_absent_conditional_collections() {
    let value = Packet {
        enabled: 0,
        length: 99,
        payload: None,
        count: 99,
        items: None,
        text_length: 99,
        text: None,
    };
    let bytes = value.to_bytes().expect("serialize absent collections");
    assert_eq!(bytes, [0, 0, 0, 0]);

    let mut offset = 0;
    let parsed = Packet::parse(&bytes, &mut offset).expect("parse absent collections");
    assert_eq!(parsed.payload, None);
    assert_eq!(parsed.items, None);
    assert_eq!(parsed.text, None);
    assert_eq!(offset, bytes.len());
}

#[test]
fn computes_lengths_for_present_conditional_collections() {
    let value = Packet {
        enabled: 1,
        length: 0,
        payload: Some(vec![2, 3]),
        count: 0,
        items: Some(vec![Item { value: 4 }, Item { value: 5 }]),
        text_length: 0,
        text: Some("hi".to_owned()),
    };
    let bytes = value.to_bytes().expect("serialize present collections");
    assert_eq!(bytes, [1, 2, 2, 3, 2, 4, 5, 2, b'h', b'i']);

    let mut offset = 0;
    let parsed = Packet::parse(&bytes, &mut offset).expect("parse present collections");
    assert_eq!(parsed.payload, Some(vec![2, 3]));
    assert_eq!(parsed.items, Some(vec![Item { value: 4 }, Item { value: 5 }]));
    assert_eq!(parsed.text.as_deref(), Some("hi"));
    assert_eq!(offset, bytes.len());
}
`)
}

func TestGeneratedRust_MatchSerialization(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

struct Payload {
    value u16;
}

struct Event {
    tag   u8;
    value match tag {
        1 => u32;
        2 => Payload;
    };
}
`)

	runGeneratedRustTests(t, files, `use sample::{Error, Event, EventValue, Payload};

#[test]
fn serializes_matching_payload() {
    let event = Event {
        tag: 2,
        value: EventValue::Payload(Payload { value: 0x1234 }),
    };
    assert_eq!(event.to_bytes().expect("matching payload"), vec![2, 0x34, 0x12]);
}

#[test]
fn rejects_mismatched_payload() {
    let event = Event {
        tag: 1,
        value: EventValue::Payload(Payload { value: 0x1234 }),
    };
    let error = event.to_bytes().expect_err("mismatched payload");
    assert!(matches!(
        error,
        Error::MatchType {
            field: "value",
            tag: 1,
        }
    ));
}
`)
}

func TestGeneratedRust_JSONAndReferences(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

enum Mode : u8 {
    One = 1;
    Two = 2;
}

struct Child {
    value u8;
}

struct Record {
    raw       bytes[2];
    text      string[16, terminator = 0x00];
    mode      Mode;
    child     Child;
    floats    f32[2];
    flag      u8;
    optional  u8 [if = "flag != 0"];
    tag       u8;
    payload   match tag {
        1 => u32;
        2 => Child;
    };
    target_id i32 [ref = Child.value];
}
`)

	runGeneratedRustTests(t, files, `use sample::{Child, Mode, Record, RecordPayload};

#[test]
fn emits_valid_json_in_declaration_order() {
    let record = Record {
        raw: [0, 255],
        text: String::from_utf8(vec![
            b'q', b'u', b'o', b't', b'e', 34, 10, 92,
        ]).expect("valid UTF-8"),
        mode: Mode::Two,
        child: Child { value: 7 },
        floats: [1.5, f32::NAN],
        flag: 0,
        optional: None,
        tag: 2,
        payload: RecordPayload::Child(Child { value: 8 }),
        target_id: 42,
    };
    assert_eq!(
        record.to_json(),
        r#"{"raw":[0,255],"text":"quote\"\n\\","mode":2,"child":{"value":7},"floats":[1.5,null],"flag":0,"optional":null,"tag":2,"payload":{"value":8},"target_id":42}"#,
    );
}

#[test]
fn exposes_cross_references() {
    assert_eq!(Record::REFS, &[("target_id", "Child.value")]);
}
`)
}

func TestGeneratedRust_LegacyEncodings(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

struct Text {
    cp949   string[2, encoding = "CP949"];
    windows string[2, encoding = "windows-949"];
    sjis    string[2, encoding = "SJIS"];
    western string[1, encoding = "windows-1252"];
}

struct Unknown {
    value string[1, encoding = "X-CLEAVE-UNKNOWN"];
}
`)

	runGeneratedRustTests(t, files, `use sample::{Error, Text, Unknown};

#[test]
fn decodes_and_encodes_legacy_text() {
    let bytes = [0xb0, 0xa1, 0xb0, 0xa1, 0x82, 0xa0, 0x80];
    let mut offset = 0;
    let got = Text::parse(&bytes, &mut offset).expect("valid legacy text");
    assert_eq!(got.cp949, "가");
    assert_eq!(got.windows, "가");
    assert_eq!(got.sjis, "あ");
    assert_eq!(got.western, "€");
    assert_eq!(got.to_bytes().expect("encodable text"), bytes);
}

#[test]
fn rejects_invalid_legacy_bytes() {
    let bytes = [0xff, 0xff, 0xb0, 0xa1, 0x82, 0xa0, 0x80];
    let mut offset = 0;
    let error = Text::parse(&bytes, &mut offset).expect_err("invalid CP949 text");
    assert!(matches!(
        error,
        Error::InvalidEncoding { encoding: "CP949" }
    ));
}

#[test]
fn rejects_unencodable_legacy_text() {
    let value = Text {
        cp949: "🙂".to_owned(),
        windows: "가".to_owned(),
        sjis: "あ".to_owned(),
        western: "€".to_owned(),
    };
    let error = value.to_bytes().expect_err("emoji is not CP949");
    assert!(matches!(
        error,
        Error::InvalidEncoding { encoding: "CP949" }
    ));
}

#[test]
fn rejects_unknown_encoding() {
    let mut offset = 0;
    let error = Unknown::parse(&[0], &mut offset).expect_err("unsupported encoding");
    assert!(matches!(
        error,
        Error::UnsupportedEncoding { encoding: "X-CLEAVE-UNKNOWN" }
    ));
}
`)
}

func TestGeneratedRust_RegexValidation(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

struct ValidName {
    name string [
        terminator = 0x00,
        encoding = "utf-8",
        (builtin).cel = {
            id: "name.format"
            message: "name must contain lowercase letters"
            expression: "this.matches('^[a-z]+$')"
        }
    ];
}

struct InvalidPattern {
    name string [
        terminator = 0x00,
        encoding = "utf-8",
        (builtin).cel = {
            id: "name.pattern"
            message: "pattern must compile"
            expression: "this.matches('[')"
        }
    ];
}
`)

	runGeneratedRustTests(t, files, `use sample::{Error, InvalidPattern, ValidName};

#[test]
fn accepts_matching_text() {
    let mut offset = 0;
    let got = ValidName::parse(b"abc\0", &mut offset).expect("matching text");
    assert_eq!(got.name, "abc");
}

#[test]
fn rejects_nonmatching_text() {
    let mut offset = 0;
    let error = ValidName::parse(b"123\0", &mut offset).expect_err("nonmatching text");
    assert!(matches!(
        error,
        Error::Validation {
            id: "name.format",
            field: "name",
            ..
        }
    ));
}

#[test]
fn reports_invalid_pattern() {
    let mut offset = 0;
    let error = InvalidPattern::parse(b"abc\0", &mut offset).expect_err("invalid pattern");
    assert!(matches!(
        error,
        Error::InvalidRegex { pattern, .. } if pattern == "["
    ));
}
`)
}

func TestGeneratedRust_ExistingSpecs(t *testing.T) {
	tests := []string{"png.clv", "validation.clv"}
	for _, file := range tests {
		t.Run(file, func(t *testing.T) {
			t.Parallel()

			source, err := os.ReadFile(filepath.Join("../../../testdata", file))
			if err != nil {
				t.Fatalf("os.ReadFile(%q) error = %v", file, err)
			}
			files := generateFromSource(t, &Generator{}, string(source))
			runGeneratedRustTests(t, files, "")
		})
	}
}

func TestGeneratedRust_SourceOnlyInclude(t *testing.T) {
	t.Parallel()

	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Skip("cargo is not installed")
	}
	files := generateFromSource(t, &Generator{NoCargo: true}, `package sample;

struct Packet {
    value u8;
}
`)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.rs"), files[0].Content, 0o644); err != nil {
		t.Fatalf("writing generated module: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "Cargo.toml"),
		[]byte(cargoManifest("host", requirements{})),
		0o644,
	); err != nil {
		t.Fatalf("writing host Cargo.toml: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("creating host source directory: %v", err)
	}
	host := `pub mod generated {
    include!(concat!(env!("CARGO_MANIFEST_DIR"), "/sample.rs"));
}
`
	if err := os.WriteFile(filepath.Join(root, "src", "lib.rs"), []byte(host), 0o644); err != nil {
		t.Fatalf("writing host lib.rs: %v", err)
	}

	target := filepath.Join(root, "target")
	runCargo(t, cargo, root, target, "cargo fmt", "+stable", "fmt", "--check")
	runCargo(
		t,
		cargo,
		root,
		target,
		"cargo clippy",
		"+stable",
		"clippy",
		"--all-targets",
		"--all-features",
		"--",
		"-D",
		"warnings",
	)
	runCargo(t, cargo, root, target, "cargo test", "+stable", "test", "--quiet")
}

func TestGeneratedRust_PreservesSchemaSpelling(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

enum lower_enum : u8 {
    snake_case = 1;
}

struct lower_struct {
    MixedCase lower_enum;
    type      u8;
}
`)
	runGeneratedRustTests(t, files, `use sample::{lower_enum, lower_struct};

#[test]
fn preserves_schema_names() {
    let mut offset = 0;
    let got = lower_struct::parse(&[1, 2], &mut offset).expect("valid value");
    assert_eq!(got.MixedCase, lower_enum::snake_case);
    assert_eq!(got.r#type, 2);
}
`)
}

func TestGeneratedRust_AllowsPreludeTypeNames(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

enum Kind : u8 {
    One = 1;
}

struct String { value u8; }
struct Vec { value u8; }
struct Option { value u8; }
struct Result { value u8; }
struct TryFrom { value u8; }
struct From { value u8; }
struct Into { value u8; }
struct ToString { value u8; }
struct Iterator { value u8; }
struct std { value u8; }

struct Container {
    flag        u8;
    conditional u8 [if = "flag != 0"];
    kind        Kind;
    text        string[2, encoding = "utf-8"];
    values      u8[2];
    string_type String;
    vec_type    Vec;
    option_type Option;
    result_type Result;
    try_type    TryFrom;
    from_type   From;
    into_type   Into;
    stringer    ToString;
    iterator    Iterator;
    std_type    std;
}
`)
	runGeneratedRustTests(t, files, "")
}

func TestGeneratedRust_AllowsParserLocalNames(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

struct Packet {
    buf              u8;
    offset           u8;
    parsed           u8;
    read_array       u8;
    take             u8;
    decode_text      u8;
    text_starts_with u8;
    text             string[1, encoding = "utf-8"] [
        (builtin).cel = {
            id: "text.prefix"
            message: "text must start with a"
            expression: "this.startsWith('a')"
        }
    ];
    guarded u8 [
        if = "buf != 0",
        (builtin).cel = {
            id: "guarded.order"
            message: "guarded must follow parsed"
            expression: "this > parsed"
        }
    ];
    tail u8;
}
`)
	runGeneratedRustTests(t, files, `use sample::Packet;

#[test]
fn parser_locals_do_not_shadow_schema_fields() {
    let mut consumed = 0;
    let got = Packet::parse(&[1, 2, 3, 4, 5, 6, 7, b'a', 8, 9], &mut consumed)
        .expect("valid packet");
    assert_eq!(got.buf, 1);
    assert_eq!(got.offset, 2);
    assert_eq!(got.parsed, 3);
    assert_eq!(got.read_array, 4);
    assert_eq!(got.take, 5);
    assert_eq!(got.decode_text, 6);
    assert_eq!(got.text_starts_with, 7);
    assert_eq!(got.text, "a");
    assert_eq!(got.guarded, Some(8));
    assert_eq!(got.tail, 9);
    assert_eq!(consumed, 10);
}
`)
}

func runGeneratedRustTests(t *testing.T, files []codegen.OutputFile, source string) {
	t.Helper()

	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Skip("cargo is not installed")
	}

	root := t.TempDir()
	manifestDirectory := ""
	for _, file := range files {
		path := filepath.Join(root, file.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("os.MkdirAll(%q) error = %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, file.Content, 0o644); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", path, err)
		}
		if filepath.Base(file.Name) == "Cargo.toml" {
			manifestDirectory = filepath.Dir(file.Name)
		}
	}
	if manifestDirectory == "" {
		t.Fatal("generated files do not contain Cargo.toml")
	}

	crate := filepath.Join(root, manifestDirectory)
	target := filepath.Join(root, "target")
	runCargo(t, cargo, crate, target, "cargo fmt", "+stable", "fmt", "--check")
	runCargo(
		t,
		cargo,
		crate,
		target,
		"cargo clippy",
		"+stable",
		"clippy",
		"--all-targets",
		"--all-features",
		"--",
		"-D",
		"warnings",
	)

	tests := filepath.Join(crate, "tests")
	if err := os.MkdirAll(tests, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q) error = %v", tests, err)
	}
	if err := os.WriteFile(filepath.Join(tests, "generated.rs"), []byte(source), 0o644); err != nil {
		t.Fatalf("writing Rust test: %v", err)
	}

	runCargo(t, cargo, crate, target, "cargo test", "+stable", "test", "--quiet")
}

func runCargo(t *testing.T, cargo, crate, target, name string, arguments ...string) {
	t.Helper()

	command := exec.Command(cargo, arguments...)
	command.Dir = crate
	command.Env = append(os.Environ(), "CARGO_TARGET_DIR="+target)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s error = %v\n%s", name, err, output)
	}
}
