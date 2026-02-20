package cpp

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

func TestGenerateHeaderAndSource(t *testing.T) {
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
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Name != "test.h" {
		t.Errorf("header name: got %q, want %q", files[0].Name, "test.h")
	}
	if files[1].Name != "test.cpp" {
		t.Errorf("source name: got %q, want %q", files[1].Name, "test.cpp")
	}
}

func TestGenerateEnumClass(t *testing.T) {
	src := `package test;

enum Color : u8 {
    Red = 0;
    Green = 1;
    Blue = 2;
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	if !strings.Contains(header, "enum class Color : uint8_t") {
		t.Error("missing enum class declaration")
	}
	if !strings.Contains(header, "Red = 0") {
		t.Error("missing Red variant")
	}
}

func TestGenerateStructDecl(t *testing.T) {
	src := `package test;

struct Header {
    magic   u32;
    version u16;
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	if !strings.Contains(header, "struct Header") {
		t.Error("missing struct Header")
	}
	if !strings.Contains(header, "uint32_t magic") {
		t.Error("missing magic field")
	}
	if !strings.Contains(header, "static Header parse") {
		t.Error("missing parse declaration")
	}
	if !strings.Contains(header, "std::string to_json() const") {
		t.Error("missing to_json declaration")
	}
}

func TestGenerateParseImpl(t *testing.T) {
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
	source := string(files[1].Content)
	if !strings.Contains(source, "Header Header::parse") {
		t.Error("missing parse implementation")
	}
	if !strings.Contains(source, "swap32") {
		t.Error("missing endian swap for u32")
	}
	if !strings.Contains(source, "swap16") {
		t.Error("missing endian swap for u16")
	}
}

func TestGenerateNamespace(t *testing.T) {
	src := `package myns;

struct Foo {
    x u32;
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	if !strings.Contains(header, "namespace myns {") {
		t.Error("missing namespace in header")
	}
	source := string(files[1].Content)
	if !strings.Contains(source, "namespace myns {") {
		t.Error("missing namespace in source")
	}
}

func TestGeneratePragmaOnce(t *testing.T) {
	src := `package test;

struct Foo {
    x u32;
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	if !strings.Contains(header, "#pragma once") {
		t.Error("missing #pragma once")
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
	header := string(files[0].Content)
	if !strings.Contains(header, "std::vector<uint8_t> payload") {
		t.Error("missing vector for length-ref bytes")
	}
	if !strings.Contains(header, "uint8_t fixed[8]") {
		t.Errorf("missing fixed array: got %s", header)
	}
}

func TestGenerateFixedTerminatorString(t *testing.T) {
	src := `package test;

struct Record {
    name string[64, terminator = 0x00];
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	source := string(files[1].Content)

	if !strings.Contains(header, "std::string name") {
		t.Error("missing std::string field type for FixedTerminator")
	}
	if !strings.Contains(source, "strnlen") {
		t.Error("missing strnlen for FixedTerminator parse")
	}
	if !strings.Contains(source, "offset += 64") {
		t.Error("missing fixed offset advance")
	}
	if !strings.Contains(source, "\"\\\"\" + name + \"\\\"\"") {
		t.Error("missing JSON string serialization for FixedTerminator")
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
	source := string(files[1].Content)
	if !strings.Contains(source, "if (!((result.version >= 1)))") {
		t.Errorf("missing validate check, got:\n%s", source)
	}
	if !strings.Contains(source, `throw std::runtime_error("version must be at least 1")`) {
		t.Errorf("missing runtime_error with custom message, got:\n%s", source)
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
	source := string(files[1].Content)
	if !strings.Contains(source, "if (!((result.version >= 1)))") {
		t.Error("missing first validate check")
	}
	if !strings.Contains(source, "if (!((result.version <= 10)))") {
		t.Error("missing second validate check")
	}
}

func TestGenerateConditionalField(t *testing.T) {
	src := `package test;

struct Header {
    has_crc u8;
    crc     u32 [if = "has_crc != 0"];
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	source := string(files[1].Content)
	if !strings.Contains(source, "if ((result.has_crc != 0))") {
		t.Errorf("missing conditional if check, got:\n%s", source)
	}
	if !strings.Contains(header, "std::optional<uint32_t> crc") {
		t.Errorf("missing std::optional field type, got:\n%s", header)
	}
}

func TestGenerateConditionalToJSON(t *testing.T) {
	src := `package test;

struct Header {
    has_crc u8;
    crc     u32 [if = "has_crc != 0"];
}
`
	files := generateFromSource(t, src)
	source := string(files[1].Content)
	if !strings.Contains(source, ".has_value()") {
		t.Errorf("missing .has_value() check in to_json, got:\n%s", source)
	}
	if !strings.Contains(source, "\"null\"") {
		t.Errorf("missing null fallback in to_json, got:\n%s", source)
	}
}

func TestGenerateOptionalInclude(t *testing.T) {
	src := `package test;

struct Header {
    has_crc u8;
    crc     u32 [if = "has_crc != 0"];
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	if !strings.Contains(header, "#include <optional>") {
		t.Errorf("missing #include <optional>, got:\n%s", header)
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
	header := string(files[0].Content)
	source := string(files[1].Content)

	if !strings.Contains(header, "std::variant<") {
		t.Error("missing std::variant in header field type")
	}
	if !strings.Contains(header, "#include <variant>") {
		t.Error("missing #include <variant>")
	}
	if !strings.Contains(source, "if (result.type == 0)") {
		t.Error("missing case 0 check in parse")
	}
	if !strings.Contains(source, "} else if (result.type == 1)") {
		t.Error("missing case 1 check in parse")
	}
	if !strings.Contains(source, "} else {") {
		t.Error("missing else branch in parse")
	}
	if !strings.Contains(source, "emplace<0>()") {
		t.Error("missing emplace<0> for case 0")
	}
	if !strings.Contains(source, "emplace<1>()") {
		t.Error("missing emplace<1> for case 1")
	}
	if !strings.Contains(source, "emplace<2>()") {
		t.Error("missing emplace<2> for default case")
	}
	if !strings.Contains(source, "value.index()") {
		t.Error("missing variant index check in to_json")
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
	source := string(files[1].Content)
	if !strings.Contains(source, `throw std::runtime_error("unknown tag value for Event.value")`) {
		t.Error("missing runtime_error for no-default match")
	}
}

func TestGenerateVariantInclude(t *testing.T) {
	src := `package test;

struct Simple {
    x u32;
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	if strings.Contains(header, "#include <variant>") {
		t.Error("variant should not be included when no match fields exist")
	}
}

func TestGenerateToBytesDecl(t *testing.T) {
	src := `package test;

struct Header {
    magic   u32;
    version u16;
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	if !strings.Contains(header, "std::vector<uint8_t> to_bytes() const") {
		t.Error("missing to_bytes declaration in header")
	}
}

func TestGenerateToBytesImpl(t *testing.T) {
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
	source := string(files[1].Content)
	if !strings.Contains(source, "std::vector<uint8_t> Header::to_bytes() const") {
		t.Error("missing to_bytes implementation")
	}
	if !strings.Contains(source, "swap32(magic)") {
		t.Error("missing endian swap for u32 in to_bytes")
	}
	if !strings.Contains(source, "swap16(version)") {
		t.Error("missing endian swap for u16 in to_bytes")
	}
	if !strings.Contains(source, "return _buf") {
		t.Error("missing return _buf")
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
	source := string(files[1].Content)
	if !strings.Contains(source, "data.size()") {
		t.Error("missing auto-computed length: expected data.size()")
	}
	if !strings.Contains(source, "_buf.insert(_buf.end(), data.begin(), data.end())") {
		t.Error("missing bytes insert for data")
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
	source := string(files[1].Content)
	if !strings.Contains(source, "value.index() == 0") {
		t.Error("missing variant index 0 check in to_bytes")
	}
	if !strings.Contains(source, "value.index() == 1") {
		t.Error("missing variant index 1 check in to_bytes")
	}
	if !strings.Contains(source, "value.index() == 2") {
		t.Error("missing variant index 2 check in to_bytes")
	}
}

func TestGenerateFixedSizeStringWithEncoding(t *testing.T) {
	src := `package test;

struct Record {
    name string[32, encoding = "CP949"];
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	source := string(files[1].Content)
	if !strings.Contains(header, "std::string name") {
		t.Error("missing std::string field type for encoded fixed-size string")
	}
	if !strings.Contains(source, "reinterpret_cast<const char*>") {
		t.Error("missing reinterpret_cast for encoded fixed-size string parse")
	}
	if !strings.Contains(source, "CP949") {
		t.Error("missing encoding comment for CP949")
	}
}

func TestGenerateTerminatorStringWithEncoding(t *testing.T) {
	src := `package test;

struct Record {
    name string [terminator = 0x00, encoding = "CP949"];
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	source := string(files[1].Content)
	if !strings.Contains(header, "std::string name") {
		t.Error("missing std::string field type for encoded terminator string")
	}
	if !strings.Contains(source, "push_back(static_cast<char>") {
		t.Error("missing push_back(static_cast<char>) for encoded terminator string parse")
	}
	if !strings.Contains(source, "CP949") {
		t.Error("missing encoding comment for CP949")
	}
}

func TestGenerateValidationGolden(t *testing.T) {
	src, err := os.ReadFile("../../../testdata/validation.clv")
	if err != nil {
		t.Fatal(err)
	}

	files := generateFromSource(t, string(src))

	// Write golden files.
	for _, f := range files {
		goldenPath := "../../../testdata/golden/" + f.Name
		if err := os.WriteFile(goldenPath, f.Content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	header := string(files[0].Content)
	source := string(files[1].Content)

	// Verify magic number validation.
	if !strings.Contains(source, "if (!((result.magic == 1129070934)))") {
		t.Error("missing magic number validation check")
	}
	if !strings.Contains(source, `throw std::runtime_error("invalid magic number")`) {
		t.Error("missing magic number error message")
	}

	// Verify version range validation (two rules on one field).
	if !strings.Contains(source, "if (!((result.version >= 1)))") {
		t.Error("missing version min check")
	}
	if !strings.Contains(source, "if (!((result.version <= 100)))") {
		t.Error("missing version max check")
	}

	// Verify compound boolean expression for flags.
	if !strings.Contains(source, "if (!(((result.flags >= 0) && (result.flags <= 7))))") {
		t.Error("missing flags compound boolean check")
	}

	// Verify priority validation.
	if !strings.Contains(source, "if (!(((result.priority >= 1) && (result.priority <= 3))))") {
		t.Error("missing priority validation")
	}

	// Verify conditional field.
	if !strings.Contains(source, "if ((result.has_data != 0))") {
		t.Error("missing conditional field check")
	}
	if !strings.Contains(header, "std::optional<uint32_t> data") {
		t.Error("missing std::optional for conditional field")
	}
}

func TestGeneratePngGolden(t *testing.T) {
	src, err := os.ReadFile("../../../testdata/png.clv")
	if err != nil {
		t.Fatal(err)
	}

	files := generateFromSource(t, string(src))

	// Write golden files.
	for _, f := range files {
		goldenPath := "../../../testdata/golden/" + f.Name
		if err := os.WriteFile(goldenPath, f.Content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	header := string(files[0].Content)
	source := string(files[1].Content)

	// Verify key elements in header.
	if !strings.Contains(header, "namespace png") {
		t.Error("missing namespace in header")
	}
	if !strings.Contains(header, "enum class ColorType") {
		t.Error("missing ColorType enum class")
	}
	if !strings.Contains(header, "struct Chunk") {
		t.Error("missing Chunk struct")
	}
	if !strings.Contains(header, "struct PngFile") {
		t.Error("missing PngFile struct")
	}

	// Verify parse impls in source.
	if !strings.Contains(source, "Chunk Chunk::parse") {
		t.Error("missing Chunk parse impl")
	}
	if !strings.Contains(source, "PngFile PngFile::parse") {
		t.Error("missing PngFile parse impl")
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
	header := string(files[0].Content)
	source := string(files[1].Content)

	if !strings.Contains(header, "std::vector<Inner> items") {
		t.Errorf("expected std::vector<Inner> items, got:\n%s", header)
	}
	if !strings.Contains(source, "for (size_t i = 0; i < result.count; ++i)") {
		t.Errorf("expected count-ref loop, got:\n%s", source)
	}
	if !strings.Contains(source, "Inner::parse(buf, len, offset)") {
		t.Error("expected Inner::parse call in loop")
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
	source := string(files[1].Content)

	if !strings.Contains(source, "result.rest.assign(buf + offset, buf + len)") {
		t.Errorf("expected bulk bytes assign for rest array, got:\n%s", source)
	}
	if !strings.Contains(source, "offset = len") {
		t.Error("expected offset = len after rest bytes read")
	}
	if strings.Contains(source, "while (offset + 0") {
		t.Error("should not have infinite loop with stride 0")
	}
}

func TestGenerateMatchDuplicateTypes(t *testing.T) {
	src := `package test;

struct Event {
    type  u8;
    value match type {
        0 => u32;
        1 => f32;
        2 => u32;
    };
}
`
	files := generateFromSource(t, src)
	header := string(files[0].Content)
	source := string(files[1].Content)

	// The variant should deduplicate u32, producing std::variant<uint32_t, float>.
	if strings.Contains(header, "std::variant<uint32_t, float, uint32_t>") {
		t.Error("variant should deduplicate types, found duplicate uint32_t")
	}
	if !strings.Contains(header, "std::variant<uint32_t, float>") {
		t.Errorf("expected deduped variant, got:\n%s", header)
	}

	// Case 2 (u32) should use emplace<0> since u32 is at index 0.
	if !strings.Contains(source, "result.type == 2") {
		t.Error("missing case 2 check")
	}
}

func TestGenerateFixedSizePrimitiveArrayJSON(t *testing.T) {
	src := `package test;

struct Data {
    values u32[20];
}
`
	files := generateFromSource(t, src)
	source := string(files[1].Content)

	if !strings.Contains(source, `s += "["`) {
		t.Error("expected array JSON output with '['")
	}
	if !strings.Contains(source, "values.size()") {
		t.Error("expected .size() loop for fixed-size primitive array JSON")
	}
	if strings.Contains(source, "s += std::to_string(values)") {
		t.Error("should not call std::to_string on vector directly")
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
	header := string(files[0].Content)
	source := string(files[1].Content)

	if !strings.Contains(header, "#include <unordered_map>") {
		t.Error("expected unordered_map include")
	}
	if !strings.Contains(header, "static std::unordered_map<std::string, std::string> refs()") {
		t.Error("expected refs() declaration in Source struct")
	}
	if !strings.Contains(source, "Source::refs()") {
		t.Error("expected refs() implementation")
	}
	if !strings.Contains(source, `"target_id"`) {
		t.Error("expected target_id key in refs()")
	}
	if !strings.Contains(source, `"Target.id"`) {
		t.Error("expected Target.id value in refs()")
	}
}

func TestGenerateFixedSizeFloatArrayJSON(t *testing.T) {
	src := `package test;

struct Data {
    stats f32[20];
}
`
	files := generateFromSource(t, src)
	source := string(files[1].Content)

	if !strings.Contains(source, `s += "["`) {
		t.Error("expected array JSON output with '['")
	}
	if !strings.Contains(source, "stats.size()") {
		t.Error("expected .size() loop for fixed-size float array JSON")
	}
	if strings.Contains(source, "s += std::to_string(stats)") {
		t.Error("should not call std::to_string on vector directly")
	}
}
