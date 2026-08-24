package rust

import (
	"strings"
	"testing"
)

func TestGenerator_GenerateJSON(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{NoCargo: true}, `package sample;

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
	source := string(files[0].Content)
	for _, want := range []string{
		"pub fn to_json(&self) -> ::std::string::String",
		"write_json_string(&mut json,",
		"json.push('[');",
		"if value.is_finite()",
		"json.push_str(\"null\")",
		"Mode",
		"payload.to_json()",
		"pub const REFS: &'static [(&'static str, &'static str)]",
		"(\"target_id\", \"Child.value\")",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated source is missing %q:\n%s", want, source)
		}
	}
}
