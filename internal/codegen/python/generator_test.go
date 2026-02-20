package python

import (
	"os"
	"strings"
	"testing"

	"github.com/rdeusser/cleave/internal/codegen"
	"github.com/rdeusser/cleave/internal/ir"
	"github.com/rdeusser/cleave/internal/lexer"
	"github.com/rdeusser/cleave/internal/parser"
)

func generateFromSource(t *testing.T, src string) []codegen.OutputFile {
	t.Helper()
	p := parser.New("test.clv", []byte(src))
	file, parseErrs := p.Parse()
	if len(parseErrs) > 0 {
		t.Fatalf("parse errors: %v", parseErrs)
	}
	lex := lexer.New("test.clv", []byte(src))
	pkg, lowerErrs := ir.Lower(file, lex.Position)
	if len(lowerErrs) > 0 {
		t.Fatalf("lower errors: %v", lowerErrs)
	}

	gen := &Generator{}
	files, err := gen.Generate(pkg)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestGenerateEnum(t *testing.T) {
	src := `package test;

enum Color : u8 {
    Red = 0;
    Green = 1;
    Blue = 2;
}
`
	files := generateFromSource(t, src)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Name != "test.py" {
		t.Errorf("filename: got %q, want %q", files[0].Name, "test.py")
	}
	content := string(files[0].Content)
	if !strings.Contains(content, "class Color(enum.IntEnum):") {
		t.Error("missing Color enum class")
	}
	if !strings.Contains(content, "Red = 0") {
		t.Error("missing Red variant")
	}
}

func TestGenerateStruct(t *testing.T) {
	src := `package test;

struct Header {
    magic   u32;
    version u16;

    option (builtin) = {
        endian = big;
    };
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "@dataclass") {
		t.Error("missing @dataclass")
	}
	if !strings.Contains(content, "class Header:") {
		t.Error("missing Header class")
	}
	if !strings.Contains(content, "def parse(cls, buf: bytes, offset: int = 0)") {
		t.Error("missing parse method")
	}
	if !strings.Contains(content, "def to_json(self)") {
		t.Error("missing to_json method")
	}
	// Big endian prefix.
	if !strings.Contains(content, "\">I\"") {
		t.Error("missing big-endian u32 unpack")
	}
}

func TestGenerateArrayField(t *testing.T) {
	src := `package test;

struct Data {
    length  u32;
    payload bytes[length];
    fixed   bytes[8];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "buf[offset:offset + length]") {
		t.Error("missing length-ref bytes read")
	}
	if !strings.Contains(content, "buf[offset:offset + 8]") {
		t.Error("missing fixed bytes read")
	}
}

func TestGenerateRestArray(t *testing.T) {
	src := `package test;

struct Item {
    value u32;
}

struct Container {
    items Item [rest = true];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "while offset < len(buf):") {
		t.Error("missing rest array loop")
	}
}

func TestGenerateCountRef(t *testing.T) {
	src := `package test;

format TestFormat {
    root = File;
}

struct File {
    header  Header;
    records Record [count = header.n];
}

struct Header {
    n u32;
}

struct Record {
    x u32;
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "for _ in range(header.n):") {
		t.Error("missing count-ref loop")
	}
	if !strings.Contains(content, `list["Record"]`) {
		t.Error("missing list type annotation for count-ref field")
	}
}

func TestGenerateFixedTerminatorString(t *testing.T) {
	src := `package test;

struct Record {
    name string[64, terminator = 0x00];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "buf[offset:offset + 64]") {
		t.Error("missing fixed-size read")
	}
	if !strings.Contains(content, "_raw.find(0)") {
		t.Error("missing sentinel find")
	}
	if !strings.Contains(content, ".decode(\"utf-8\")") {
		t.Error("missing utf-8 decode")
	}
	if !strings.Contains(content, ".encode(\"utf-8\")") {
		t.Error("missing utf-8 encode in to_bytes")
	}
	if !strings.Contains(content, "name: str") {
		t.Error("missing str type annotation for FixedTerminator")
	}
}

func TestGenerateValidateSimple(t *testing.T) {
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
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "if not (version >= 1):") {
		t.Error("missing validate check")
	}
	if !strings.Contains(content, `raise ValueError("version must be at least 1")`) {
		t.Error("missing ValueError raise with custom message")
	}
}

func TestGenerateValidateFieldRef(t *testing.T) {
	src := `package test;

struct Header {
    version u16;
    check   u16 [
        (builtin).cel = {
            id: "check.match"
            message: "check must match version"
            expression: "this == version"
        }
    ];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "if not (check == version):") {
		t.Error("missing validate field-ref check")
	}
}

func TestGenerateMultipleValidations(t *testing.T) {
	src := `package test;

struct Header {
    version u16 [
        (builtin).cel = {
            id: "version.min"
            message: "version must be at least 1"
            expression: "this >= 1"
        },
        (builtin).cel = {
            id: "version.max"
            message: "version must be at most 10"
            expression: "this <= 10"
        }
    ];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "if not (version >= 1):") {
		t.Error("missing first validate check")
	}
	if !strings.Contains(content, "if not (version <= 10):") {
		t.Error("missing second validate check")
	}
	if !strings.Contains(content, `raise ValueError("version must be at least 1")`) {
		t.Error("missing first custom message")
	}
	if !strings.Contains(content, `raise ValueError("version must be at most 10")`) {
		t.Error("missing second custom message")
	}
}

func TestGenerateConditionalField(t *testing.T) {
	src := `package test;

struct Packet {
    has_crc u8;
    crc     u32 [if = "has_crc != 0"];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "if (has_crc != 0):") {
		t.Error("missing conditional if check")
	}
	if !strings.Contains(content, "else:") {
		t.Error("missing else branch for conditional")
	}
	if !strings.Contains(content, "crc = None") {
		t.Error("missing None assignment in else branch")
	}
	if !strings.Contains(content, "Optional[int]") {
		t.Error("missing Optional type annotation for conditional field")
	}
}

func TestGenerateConditionalToJSON(t *testing.T) {
	src := `package test;

struct Packet {
    has_crc u8;
    crc     u32 [if = "has_crc != 0"];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "if self.crc is not None:") {
		t.Error("missing None check in _to_dict for conditional field")
	}
}

func TestGenerateConditionalToBytes(t *testing.T) {
	src := `package test;

struct Packet {
    has_crc u8;
    crc     u32 [if = "has_crc != 0"];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "if self.crc is not None:") {
		t.Error("missing None check in to_bytes for conditional field")
	}
}

func TestGenerateOptionalImport(t *testing.T) {
	src := `package test;

struct Packet {
    has_crc u8;
    crc     u32 [if = "has_crc != 0"];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "from typing import Optional") {
		t.Error("missing Optional import when conditional field exists")
	}
}

func TestGenerateInlineMatch(t *testing.T) {
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
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "if type == 0:") {
		t.Error("missing case 0 check")
	}
	if !strings.Contains(content, "elif type == 1:") {
		t.Error("missing case 1 check")
	}
	if !strings.Contains(content, "else:") {
		t.Error("missing else branch")
	}
	if !strings.Contains(content, "buf[offset:offset + 4]") {
		t.Error("missing default bytes read")
	}
}

func TestGenerateMatchNoDefault(t *testing.T) {
	src := `package test;

struct Event {
    type  u8;
    value match type {
        0 => u32;
    };
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "raise ValueError") {
		t.Error("missing ValueError for no-default match")
	}
}

func TestGenerateUnionRef(t *testing.T) {
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
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "if type == 0:") {
		t.Error("missing case 0 from union ref")
	}
	if !strings.Contains(content, "int | float") {
		t.Error("missing union type annotation")
	}
}

func TestGenerateToBytes(t *testing.T) {
	src := `package test;

struct Header {
    magic   u32;
    version u16;

    option (builtin) = {
        endian = big;
    };
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "def to_bytes(self) -> bytes:") {
		t.Error("missing to_bytes method signature")
	}
	if !strings.Contains(content, "_buf = bytearray()") {
		t.Error("missing bytearray initialization")
	}
	if !strings.Contains(content, "return bytes(_buf)") {
		t.Error("missing return bytes(_buf)")
	}
	if !strings.Contains(content, `struct.pack(">I", self.magic)`) {
		t.Error("missing big-endian u32 pack for magic")
	}
}

func TestGenerateToBytesAutoLength(t *testing.T) {
	src := `package test;

struct Chunk {
    length  u32;
    data    bytes[length];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "len(self.data)") {
		t.Error("missing auto-computed length: expected len(self.data)")
	}
	if !strings.Contains(content, "_buf += self.data") {
		t.Error("missing raw bytes write for data")
	}
}

func TestGenerateToBytesTerminator(t *testing.T) {
	src := `package test;

struct Msg {
    name string [terminator = 0];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "_buf += self.name") {
		t.Error("missing raw bytes write for terminator field")
	}
	if !strings.Contains(content, "bytes([0])") {
		t.Error("missing sentinel append for terminator field")
	}
}

func TestGenerateToBytesMatch(t *testing.T) {
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
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "if self.type == 0:") {
		t.Error("missing match case 0 in to_bytes")
	}
	if !strings.Contains(content, "elif self.type == 1:") {
		t.Error("missing match case 1 in to_bytes")
	}
	if !strings.Contains(content, "else:") {
		t.Error("missing default case in to_bytes")
	}
}

func TestGenerateToBytesEnum(t *testing.T) {
	src := `package test;

enum Color : u8 {
    Red = 0;
    Green = 1;
}

struct Pixel {
    color Color;
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, "int(self.color)") {
		t.Error("missing int() cast for enum in to_bytes")
	}
}

func TestGenerateFixedSizeStringWithEncoding(t *testing.T) {
	src := `package test;

struct Record {
    name string[32, encoding = "CP949"];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, `.decode("CP949")`) {
		t.Error("missing .decode(\"CP949\") for encoded fixed-size string")
	}
	if !strings.Contains(content, `.encode("CP949")`) {
		t.Error("missing .encode(\"CP949\") in to_bytes for encoded fixed-size string")
	}
	if !strings.Contains(content, "name: str") {
		t.Error("missing str type annotation for encoded string")
	}
}

func TestGenerateLengthRefStringWithEncoding(t *testing.T) {
	src := `package test;

struct Record {
    length u32;
    name   string[length, encoding = "SJIS"];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, `.decode("SJIS")`) {
		t.Error("missing .decode(\"SJIS\") for encoded length-ref string")
	}
	if !strings.Contains(content, "name: str") {
		t.Error("missing str type annotation for encoded length-ref string")
	}
}

func TestGenerateTerminatorStringWithEncoding(t *testing.T) {
	src := `package test;

struct Record {
    name string [terminator = 0x00, encoding = "CP949"];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, `.decode("CP949")`) {
		t.Error("missing .decode(\"CP949\") for encoded terminator string")
	}
	if !strings.Contains(content, `.encode("CP949")`) {
		t.Error("missing .encode(\"CP949\") in to_bytes for encoded terminator string")
	}
}

func TestGenerateFixedTerminatorStringWithEncoding(t *testing.T) {
	src := `package test;

struct Record {
    name string[64, terminator = 0x00, encoding = "CP949"];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)
	if !strings.Contains(content, `.decode("CP949")`) {
		t.Error("missing .decode(\"CP949\") for encoded fixed-terminator string")
	}
	if !strings.Contains(content, `.encode("CP949")`) {
		t.Error("missing .encode(\"CP949\") in to_bytes for encoded fixed-terminator string")
	}
	if strings.Contains(content, `"utf-8"`) {
		t.Error("should not contain utf-8 when encoding is specified")
	}
}

func TestGenerateValidationGolden(t *testing.T) {
	src, err := os.ReadFile("../../../testdata/validation.clv")
	if err != nil {
		t.Fatal(err)
	}

	files := generateFromSource(t, string(src))
	content := string(files[0].Content)

	// Write golden file for reference.
	goldenPath := "../../../testdata/golden/validation.py"
	if err := os.WriteFile(goldenPath, files[0].Content, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify magic number validation.
	if !strings.Contains(content, "if not (magic == 1129070934):") {
		t.Error("missing magic number validation check")
	}
	if !strings.Contains(content, `raise ValueError("invalid magic number")`) {
		t.Error("missing magic number error message")
	}

	// Verify version range validation (two rules on one field).
	if !strings.Contains(content, "if not (version >= 1):") {
		t.Error("missing version min check")
	}
	if !strings.Contains(content, "if not (version <= 100):") {
		t.Error("missing version max check")
	}

	// Verify compound boolean expression.
	if !strings.Contains(content, "if not ((flags >= 0) and (flags <= 7)):") {
		t.Error("missing flags compound boolean check")
	}

	// Verify priority validation.
	if !strings.Contains(content, "if not ((priority >= 1) and (priority <= 3)):") {
		t.Error("missing priority validation")
	}

	// Verify conditional field still works alongside validation.
	if !strings.Contains(content, "if (has_data != 0):") {
		t.Error("missing conditional field check")
	}
	if !strings.Contains(content, "Optional[int]") {
		t.Error("missing Optional type for conditional field")
	}
}

func TestGeneratePngGolden(t *testing.T) {
	src, err := os.ReadFile("../../../testdata/png.clv")
	if err != nil {
		t.Fatal(err)
	}

	files := generateFromSource(t, string(src))
	content := string(files[0].Content)

	// Write golden file for reference.
	goldenPath := "../../../testdata/golden/png.py"
	if err := os.WriteFile(goldenPath, files[0].Content, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify key elements.
	if !strings.Contains(content, "class ColorType(enum.IntEnum):") {
		t.Error("missing ColorType enum")
	}
	if !strings.Contains(content, "class Chunk:") {
		t.Error("missing Chunk class")
	}
	if !strings.Contains(content, "class PngFile:") {
		t.Error("missing PngFile class")
	}
	if files[0].Name != "png.py" {
		t.Errorf("filename: got %q, want %q", files[0].Name, "png.py")
	}
}

func TestGenerateStructArrayWithCountRef(t *testing.T) {
	src := `package test;

struct Inner {
    x u32;
}

struct Outer {
    count u32;
    items Inner[count];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)

	if !strings.Contains(content, `list["Inner"]`) {
		t.Error("expected list[\"Inner\"] type annotation")
	}
	if !strings.Contains(content, "for _ in range(count):") {
		t.Error("expected count-ref loop")
	}
	if !strings.Contains(content, "Inner.parse(buf, offset)") {
		t.Error("expected Inner.parse call in loop")
	}
}

func TestGenerateStructWithRef(t *testing.T) {
	src := `package test;

struct Target {
    id   u32;
    name u32;
}

struct Source {
    target_id i32 [ref = Target.id];
    attr_idx  i32 [ref = Target.name];
    normal    u32;
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)

	if !strings.Contains(content, "from typing import ClassVar") {
		t.Error("expected ClassVar import")
	}
	if !strings.Contains(content, "_REFS: ClassVar[dict[str, str]]") {
		t.Error("expected _REFS class variable")
	}
	if !strings.Contains(content, `"target_id": "Target.id"`) {
		t.Error("expected target_id ref entry")
	}
	if !strings.Contains(content, `"attr_idx": "Target.name"`) {
		t.Error("expected attr_idx ref entry")
	}
}

func TestGenerateBytesRestArray(t *testing.T) {
	src := `package test;

struct Packet {
    header u32;
    rest   bytes [rest = true];
}
`
	files := generateFromSource(t, src)
	content := string(files[0].Content)

	if !strings.Contains(content, "rest = buf[offset:]") {
		t.Errorf("expected bulk bytes slice for rest array, got:\n%s", content)
	}
	if !strings.Contains(content, "offset = len(buf)") {
		t.Error("expected offset = len(buf) after rest bytes read")
	}
	if strings.Contains(content, "offset += 0") {
		t.Error("should not have infinite loop with stride 0")
	}
}
