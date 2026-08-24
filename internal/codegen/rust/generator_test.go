package rust

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdeusser/cleave/internal/codegen"
	"github.com/rdeusser/cleave/internal/ir"
	"github.com/rdeusser/cleave/internal/lexer"
	"github.com/rdeusser/cleave/internal/parser"
)

func TestRustIdent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "value", want: "value"},
		{name: "type", want: "r#type"},
		{name: "gen", want: "r#gen"},
		{name: "crate", want: "crate_"},
		{name: "self", want: "self_"},
		{name: "Self", want: "Self_"},
		{name: "super", want: "super_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := rustIdent(tt.name); got != tt.want {
				t.Errorf("rustIdent(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestRustInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value int64
		want  string
	}{
		{value: 0, want: "0"},
		{value: 9999, want: "9999"},
		{value: 10000, want: "10_000"},
		{value: -1129070934, want: "-1_129_070_934"},
		{value: -9223372036854775808, want: "-9_223_372_036_854_775_808"},
	}

	for _, tt := range tests {
		if got := rustInt(tt.value); got != tt.want {
			t.Errorf("rustInt(%d) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestGenerator_GenerateRejectsMappedNameCollision(t *testing.T) {
	t.Parallel()

	pkg := &ir.Package{
		Name: "sample",
		Structs: []*ir.Struct{{
			Name: "Header",
			Fields: []*ir.Field{
				{Name: "crate", Type: ir.FieldType{Kind: ir.KindPrimitive, Primitive: ir.U8}},
				{Name: "crate_", Type: ir.FieldType{Kind: ir.KindPrimitive, Primitive: ir.U8}},
			},
		}},
	}

	_, err := (&Generator{NoCargo: true}).Generate(pkg)
	if err == nil {
		t.Fatal("Generator.Generate() error = nil, want mapped-name collision")
	}
	if !strings.Contains(err.Error(), `fields "crate" and "crate_"`) {
		t.Errorf("Generator.Generate() error = %q, want both colliding fields", err)
	}
}

func TestGenerator_GenerateRejectsRuntimeNameCollision(t *testing.T) {
	t.Parallel()

	pkg := &ir.Package{
		Name:    "sample",
		Structs: []*ir.Struct{{Name: "Error"}},
	}
	_, err := (&Generator{NoCargo: true}).Generate(pkg)
	if err == nil {
		t.Fatal("Generator.Generate() error = nil, want runtime-name collision")
	}
	if !strings.Contains(err.Error(), `type "Error" conflicts with generated Rust name "Error"`) {
		t.Errorf("Generator.Generate() error = %q, want generated Error collision", err)
	}
}

func TestGenerator_GenerateDeclarations(t *testing.T) {
	t.Parallel()

	pkg := &ir.Package{
		Name: "sample",
		Enums: []*ir.Enum{{
			Name:        "Color",
			BackingType: ir.U8,
			Variants: []ir.EnumVariant{
				{Name: "Red", Value: 1},
				{Name: "type", Value: 2},
			},
		}},
		Structs: []*ir.Struct{{
			Name: "Record",
			Fields: []*ir.Field{
				{Name: "id", Type: ir.FieldType{Kind: ir.KindPrimitive, Primitive: ir.U32}},
				{
					Name: "fixed",
					Type: ir.FieldType{
						Kind:      ir.KindPrimitive,
						Primitive: ir.U16,
						Array:     ir.ArraySpec{Kind: ir.FixedSize, FixedSize: 3},
					},
				},
				{
					Name: "payload",
					Type: ir.FieldType{
						Kind:      ir.KindPrimitive,
						Primitive: ir.Bytes,
						Array:     ir.ArraySpec{Kind: ir.LengthRef, LengthRef: "id"},
					},
				},
				{
					Name: "label",
					Type: ir.FieldType{
						Kind:      ir.KindPrimitive,
						Primitive: ir.String,
						Array:     ir.ArraySpec{Kind: ir.FixedTerminator, FixedSize: 16},
					},
				},
				{
					Name:      "color",
					Type:      ir.FieldType{Kind: ir.KindEnum, Ref: "Color"},
					Condition: &ir.ExprNode{Kind: ir.ExprIdent, Ident: "enabled"},
				},
			},
		}},
	}

	files, err := (&Generator{NoCargo: true}).Generate(pkg)
	if err != nil {
		t.Fatalf("Generator.Generate() error = %v", err)
	}
	source := string(files[0].Content)
	for _, want := range []string{
		"#[repr(u8)]",
		"#[derive(Debug, Clone, Copy, PartialEq, Eq)]",
		"pub enum Color {",
		"Red = 1,",
		"r#type = 2,",
		"#[derive(Debug, Clone, PartialEq)]",
		"pub struct Record {",
		"pub id: u32,",
		"pub fixed: [u16; 3],",
		"pub payload: ::std::vec::Vec<u8>,",
		"pub label: ::std::string::String,",
		"pub color: ::std::option::Option<Color>,",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated source is missing %q:\n%s", want, source)
		}
	}
}

func TestGenerator_GenerateLayout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		generator Generator
		wantNames []string
	}{
		{
			name:      "cargo crate",
			generator: Generator{},
			wantNames: []string{"sample/Cargo.toml", "sample/src/lib.rs"},
		},
		{
			name:      "source only",
			generator: Generator{NoCargo: true},
			wantNames: []string{"sample.rs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := generateFromSource(t, &tt.generator, `package sample;

struct Header {
    version u16;
}
`)
			if len(files) != len(tt.wantNames) {
				t.Fatalf("Generator.Generate() returned %d files, want %d", len(files), len(tt.wantNames))
			}
			for i, want := range tt.wantNames {
				if got := files[i].Name; got != want {
					t.Errorf("Generator.Generate()[%d].Name = %q, want %q", i, got, want)
				}
			}

			source := string(files[len(files)-1].Content)
			if strings.Contains(source, "#![") {
				t.Errorf("generated source contains crate-only inner attribute:\n%s", source)
			}
			if !strings.Contains(source, "// Generated by cleave. Do not edit.") {
				t.Errorf("generated source is missing marker:\n%s", source)
			}
		})
	}
}

func TestGenerator_GenerateCargoManifest(t *testing.T) {
	t.Parallel()

	files := generateFromSource(t, &Generator{}, `package sample;

struct Header {
    version u16;
}
`)
	manifest := string(files[0].Content)
	for _, want := range []string{
		`name = "sample"`,
		`version = "0.0.0"`,
		`edition = "2024"`,
		`publish = false`,
		"[lints.clippy]",
	} {
		if !strings.Contains(manifest, want) {
			t.Errorf("Cargo.toml is missing %q:\n%s", want, manifest)
		}
	}
	if strings.Contains(manifest, "[dependencies]") {
		t.Errorf("dependency-free package has dependencies:\n%s", manifest)
	}
}

func TestGenerator_GenerateLowercasePackagePath(t *testing.T) {
	t.Parallel()

	pkg := &ir.Package{Name: "Sample_Pkg"}
	files, err := (&Generator{}).Generate(pkg)
	if err != nil {
		t.Fatalf("Generator.Generate() error = %v", err)
	}
	if files[0].Name != "sample_pkg/Cargo.toml" || files[1].Name != "sample_pkg/src/lib.rs" {
		t.Fatalf("Generator.Generate() names = %q, %q", files[0].Name, files[1].Name)
	}
	if !strings.Contains(string(files[0].Content), `name = "sample_pkg"`) {
		t.Errorf("Cargo.toml does not use lowercase package name:\n%s", files[0].Content)
	}

	module, err := (&Generator{NoCargo: true}).Generate(pkg)
	if err != nil {
		t.Fatalf("source-only Generator.Generate() error = %v", err)
	}
	if module[0].Name != "sample_pkg.rs" {
		t.Errorf("source-only name = %q, want sample_pkg.rs", module[0].Name)
	}
}

func TestGenerator_GenerateConditionalDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		source       string
		wantEncoding bool
		wantRegex    bool
	}{
		{
			name: "utf8 only",
			source: `package sample;

struct Text {
    value string[4, encoding = "utf-8"];
}
`,
		},
		{
			name: "legacy encoding",
			source: `package sample;

struct Text {
    value string[4, encoding = "CP949"];
}
`,
			wantEncoding: true,
		},
		{
			name: "regex validation",
			source: `package sample;

struct Text {
    value string [
        terminator = 0x00,
        encoding = "utf-8",
        (builtin).cel = {
            id: "text.pattern"
            message: "text must match"
            expression: "this.matches('^[a-z]+$')"
        }
    ];
}
`,
			wantRegex: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := generateFromSource(t, &Generator{}, tt.source)
			manifest := string(files[0].Content)
			if got := strings.Contains(manifest, `encoding_rs = "0.8.35"`); got != tt.wantEncoding {
				t.Errorf("Cargo.toml encoding_rs dependency = %v, want %v:\n%s", got, tt.wantEncoding, manifest)
			}
			if got := strings.Contains(manifest, `regex = "1.13.1"`); got != tt.wantRegex {
				t.Errorf("Cargo.toml regex dependency = %v, want %v:\n%s", got, tt.wantRegex, manifest)
			}

			source := string(files[1].Content)
			if got := strings.Contains(source, "fn regex_matches("); got != tt.wantRegex {
				t.Errorf("generated regex helper = %v, want %v:\n%s", got, tt.wantRegex, source)
			}
			if tt.wantEncoding {
				if !strings.Contains(source, `// - encoding_rs = "0.8.35"`) {
					t.Errorf("generated source lacks encoding_rs dependency note:\n%s", source)
				}
			} else if strings.Contains(source, `// - encoding_rs`) {
				t.Errorf("generated source has unexpected encoding_rs dependency note:\n%s", source)
			}
			if tt.wantRegex {
				if !strings.Contains(source, `// - regex = "1.13.1"`) {
					t.Errorf("generated source lacks regex dependency note:\n%s", source)
				}
			} else if strings.Contains(source, `// - regex`) {
				t.Errorf("generated source has unexpected regex dependency note:\n%s", source)
			}
			if !tt.wantEncoding && !tt.wantRegex && !strings.Contains(source, "// Cargo dependencies: none.") {
				t.Errorf("dependency-free source lacks dependency note:\n%s", source)
			}
		})
	}
}

func TestGenerator_GoldenFiles(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "png", file: "png.clv"},
		{name: "validation", file: "validation.clv"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("../../../testdata", tt.file))
			if err != nil {
				t.Fatalf("os.ReadFile(%q) error = %v", tt.file, err)
			}

			files := generateFromSource(t, &Generator{}, string(source))
			repeated := generateFromSource(t, &Generator{}, string(source))
			if len(files) != len(repeated) {
				t.Fatalf("repeated generation returned %d files, want %d", len(repeated), len(files))
			}
			for i, file := range files {
				if repeated[i].Name != file.Name || !bytes.Equal(repeated[i].Content, file.Content) {
					t.Fatalf("repeated generation differs for output %d (%s)", i, file.Name)
				}

				golden := filepath.Join("../../../testdata/golden/rust", file.Name)
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatalf("os.MkdirAll(%q) error = %v", filepath.Dir(golden), err)
				}
				if err := os.WriteFile(golden, file.Content, 0o644); err != nil {
					t.Fatalf("os.WriteFile(%q) error = %v", golden, err)
				}
			}

			module := generateFromSource(t, &Generator{NoCargo: true}, string(source))
			if len(module) != 1 || !bytes.Equal(module[0].Content, files[1].Content) {
				t.Fatal("source-only output differs from generated crate source")
			}
		})
	}
}

func generateFromSource(t *testing.T, generator *Generator, source string) []codegen.OutputFile {
	t.Helper()

	p := parser.New("test.clv", []byte(source))
	file, parseErrors := p.Parse()
	if len(parseErrors) > 0 {
		t.Fatalf("parser.Parse() errors = %v", parseErrors)
	}

	lex := lexer.New("test.clv", []byte(source))
	pkg, lowerErrors := ir.Lower(file, lex.Position)
	if len(lowerErrors) > 0 {
		t.Fatalf("ir.Lower() errors = %v", lowerErrors)
	}

	files, err := generator.Generate(pkg)
	if err != nil {
		t.Fatalf("Generator.Generate() error = %v", err)
	}
	return files
}
