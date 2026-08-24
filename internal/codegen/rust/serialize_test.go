package rust

import (
	"strings"
	"testing"
)

func TestGenerator_GenerateSerialization(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{NoCargo: true}, `package sample;

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
	source := string(files[0].Content)
	for _, want := range []string{
		"pub fn to_bytes(&self) -> ::std::result::Result<::std::vec::Vec<u8>, Error>",
		"let mut buf = ::std::vec::Vec::new();",
		"let actual = self.items.len()",
		"let actual = self.payload.len()",
		"u8::try_from(actual)",
		"Error::LengthOverflow {",
		"Error::FixedSize {",
		"encode_text(",
		"buf.extend_from_slice(",
		"Ok(buf)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated source is missing %q:\n%s", want, source)
		}
	}
}

func TestGenerator_GenerateMatchSerialization(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{NoCargo: true}, `package sample;

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
	source := string(files[0].Content)
	for _, want := range []string{
		"match (tag, value) {",
		"(1, EventValue::U32(payload)) =>",
		"(2, EventValue::Payload(payload)) =>",
		"return Err(Error::MatchType {",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated source is missing %q:\n%s", want, source)
		}
	}
}
