package format

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/rdeusser/cleave/internal/ast"
	"github.com/rdeusser/cleave/internal/parser"
	"github.com/rdeusser/cleave/internal/token"
)

// long makes the field line `    f u8 [if = "<long>"];` exactly 100 columns wide.
var long = strings.Repeat("x", 100-len(`    f u8 [if = ""];`))

var sourceTests = []struct {
	name    string
	src     string
	want    string
	wantErr string
}{
	{
		name: "aligns field names and types",
		src: `package test;
struct Data {
x u32;
longname u16;
      mid   bytes[4];
}
`,
		want: `package test;

struct Data {
    x        u32;
    longname u16;
    mid      bytes[4];
}
`,
	},
	{
		name: "aligns option lists after the widest type",
		src: `package test;

struct Chunk {
    length u32;
    data bytes[length];
    items Chunk [rest = true];
    name string [terminator = 0x00, encoding = "SJIS"];
}
`,
		want: `package test;

struct Chunk {
    length u32;
    data   bytes[length];
    items  Chunk  [rest = true];
    name   string [terminator = 0x00, encoding = "SJIS"];
}
`,
	},
	{
		name: "keeps literals, dotted names, and inline options as written",
		src: `package test;

struct Record {
    tail string[16,count=header.n];
    flags u8 [if = "name == \"a\" && flags & 0x01 != 0"];
    mask bytes[ 0b1010 ];
    label string[32, terminator = 0x00, encoding = "CP949"];
}
`,
		want: `package test;

struct Record {
    tail  string[16, count = header.n];
    flags u8 [if = "name == \"a\" && flags & 0x01 != 0"];
    mask  bytes[0b1010];
    label string[32, terminator = 0x00, encoding = "CP949"];
}
`,
	},
	{
		name: "keeps enum values as written",
		src: `package test;

enum Signed : i8 {
    Neg = -5;
    Zero = 0;
    Hex = 0x7F;
}
`,
		want: `package test;

enum Signed : i8 {
    Neg  = -5;
    Zero = 0;
    Hex  = 0x7F;
}
`,
	},
	{
		name: "keeps an option list on the field line at 100 columns",
		src:  "package test;\n\nstruct S {\n    f u8 [if = \"" + long + "\"];\n}\n",
		want: "package test;\n\nstruct S {\n    f u8 [if = \"" + long + "\"];\n}\n",
	},
	{
		name: "breaks an option list at 101 columns",
		src:  "package test;\n\nstruct S {\n    f u8 [if = \"" + long + "x\"];\n}\n",
		want: "package test;\n\nstruct S {\n    f u8 [\n        if = \"" + long + "x\"\n    ];\n}\n",
	},
	{
		name: "breaks an option list that holds a block value",
		src: `package test;

struct Record {
    name string[64, terminator = 0x00];
    version u16 [(builtin).cel = { id: "version.min" message: "version must be at least 1" expression: "this >= 1" }, (builtin).cel = { id: "version.max" message: "at most 100" expression: "this <= 100" }];
}
`,
		want: `package test;

struct Record {
    name    string[64, terminator = 0x00];
    version u16 [
        (builtin).cel = {
            id:         "version.min"
            message:    "version must be at least 1"
            expression: "this >= 1"
        },
        (builtin).cel = {
            id:         "version.max"
            message:    "at most 100"
            expression: "this <= 100"
        }
    ];
}
`,
	},
	{
		name: "aligns the keys of a broken option list up to a block value",
		src: `package test;

struct S {
    v u8 [if = "a", (builtin).cel = { id: "x" message: "y" expression: "this > 0" }, rest = true];
}
`,
		want: `package test;

struct S {
    v u8 [
        if            = "a",
        (builtin).cel = {
            id:         "x"
            message:    "y"
            expression: "this > 0"
        },
        rest = true
    ];
}
`,
	},
	{
		name: "breaks an option list that holds a comment",
		src: `package test;

struct S { // about S
    v u8 [ // why
        rest = true];
}
`,
		want: `package test;

struct S { // about S
    v u8 [ // why
        rest = true
    ];
}
`,
	},
	{
		name: "drops an empty option list",
		src:  "package test;\n\nstruct S {\n    data bytes[4] [];\n}\n",
		want: "package test;\n\nstruct S {\n    data bytes[4];\n}\n",
	},
	{
		name: "ends an alignment section at a blank line but not at a comment",
		src: `package test;

struct S {
    a u8;
    // about bb
    bb u16;


    ccc u32;
    d u8;
}
`,
		want: `package test;

struct S {
    a  u8;
    // about bb
    bb u16;

    ccc u32;
    d   u8;
}
`,
	},
	{
		name: "ends an alignment section after a field that spans several lines",
		src: `package test;

struct S {
    tag u8;
    value match tag {
        0 => u32;
        -1 => bytes[4];
        _ => Other;
    };
    longer_name u16;
    x u8;
}
`,
		want: `package test;

struct S {
    tag   u8;
    value match tag {
        0  => u32;
        -1 => bytes[4];
        _  => Other;
    };
    longer_name u16;
    x           u8;
}
`,
	},
	{
		name: "aligns trailing comments of one-line fields",
		src: `package test;

struct S {
    magic u32; // file signature
    version u16;   // format version
    reserved bytes[16] [if = "version >= 2"];
    count u32; // records
}
`,
		want: `package test;

struct S {
    magic    u32; // file signature
    version  u16; // format version
    reserved bytes[16] [if = "version >= 2"];
    count    u32; // records
}
`,
	},
	{
		name: "moves a comment inside a type above its field",
		src: `package test;

struct S {
    // leading
    data bytes[/* size */ 4]; // trailing
}
`,
		want: `package test;

struct S {
    // leading
    /* size */
    data bytes[4]; // trailing
}
`,
	},
	{
		name: "keeps block comment continuation lines unchanged",
		src: `package test;

/* Header
     keeps its indentation */
struct S {
  /* inner
   comment */
  a u8;
}
`,
		want: `package test;

/* Header
     keeps its indentation */
struct S {
    /* inner
   comment */
    a u8;
}
`,
	},
	{
		name: "joins comments that share a line",
		src: `package test;

struct S {
    /* a */    /* b */
    x u8;   /* c */   // d
}
`,
		want: `package test;

struct S {
    /* a */ /* b */
    x u8; /* c */ // d
}
`,
	},
	{
		name: "separates declarations and option blocks with blank lines",
		src: `package test;
import "a.clv";
import "b.clv";
enum E : u8 { A = 1; BB = 2; }
struct S {
    option (builtin) = { endian = big; };
    a u8;
    option (other) = { x = 1; };
    b u8;
}
union U : u8 { 0 => u8; _ => bytes[2]; }
format F { root = S; endian = little; }
`,
		want: `package test;

import "a.clv";
import "b.clv";

enum E : u8 {
    A  = 1;
    BB = 2;
}

struct S {
    option (builtin) = {
        endian = big;
    };

    a u8;

    option (other) = {
        x = 1;
    };

    b u8;
}

union U : u8 {
    0 => u8;
    _ => bytes[2];
}

format F {
    root   = S;
    endian = little;
}
`,
	},
	{
		name: "prints empty bodies on one line",
		src: `package test;

struct Empty {
}

enum None : u8 {

}

struct WithOption {
    option (builtin) = {
    };
}
`,
		want: `package test;

struct Empty {}

enum None : u8 {}

struct WithOption {
    option (builtin) = {};
}
`,
	},
	{
		name: "keeps comments in an empty body",
		src: `package test;

struct A { // note
}

struct B {
    // nothing yet
}
`,
		want: `package test;

struct A { // note
}

struct B {
    // nothing yet
}
`,
	},
	{
		name: "collapses blank lines and drops them at the edges of a body",
		src: `


// header


package test;



struct S {

    a u8;



    b u8;

}


// footer


`,
		want: `// header

package test;

struct S {
    a u8;

    b u8;
}

// footer
`,
	},
	{
		name: "aligns block entries and prints nested blocks one entry per line",
		src: `package test;

format F {
    title = "T";
    meta = { author: "me" nested: { depth: 2 } };
}
`,
		want: `package test;

format F {
    title = "T";
    meta  = {
        author: "me"
        nested: {
            depth: 2
        }
    };
}
`,
	},
	{
		name: "converts CRLF line endings and trims trailing space from comments",
		src:  "package test; // pkg   \r\n\r\nstruct S {\r\n    a u8;   /* c */  \r\n}\r\n",
		want: "package test; // pkg\n\nstruct S {\n    a u8; /* c */\n}\n",
	},
	{
		name: "keeps every comment in place",
		src: `package torture; // trailing on package

import "a.clv"; // why a
// about b
import "b.clv";

/* block comment
   spanning lines */
format Torture {
    root = Main; // the root
    endian = big;
}

// Kinds of things.
enum Kind : u8 {
    // the first
    A = 1; // trailing A


    LongerName = 0x02;
    // dangling before close
}

union Payload : u8 {
    0 => u32; // zero
    -1 => bytes[4];
    _ => Kind;
}

struct Main {
    option (builtin) = { endian = little; }; // opt trailing

    // group one
    magic u32; // trailing magic
    count u16;

    // group two
    items Item [count = count, if = "magic != 0"];
    tail string[16, count = header.n];
    value match count {
        0 => u32; // zero case
        _ => bytes[4];
    };
    checked u8 [
        // explain the rule
        (builtin).cel = {
            id: "c" // the id
            message: "m"
            expression: "this > 0"
        }
    ];
    // dangling at end of struct
}

struct Empty {}
// trailing file comment
`,
		want: `package torture; // trailing on package

import "a.clv"; // why a
// about b
import "b.clv";

/* block comment
   spanning lines */
format Torture {
    root   = Main; // the root
    endian = big;
}

// Kinds of things.
enum Kind : u8 {
    // the first
    A = 1; // trailing A

    LongerName = 0x02;
    // dangling before close
}

union Payload : u8 {
    0  => u32; // zero
    -1 => bytes[4];
    _  => Kind;
}

struct Main {
    option (builtin) = {
        endian = little;
    }; // opt trailing

    // group one
    magic u32; // trailing magic
    count u16;

    // group two
    items Item [count = count, if = "magic != 0"];
    tail  string[16, count = header.n];
    value match count {
        0 => u32; // zero case
        _ => bytes[4];
    };
    checked u8 [
        // explain the rule
        (builtin).cel = {
            id:         "c" // the id
            message:    "m"
            expression: "this > 0"
        }
    ];
    // dangling at end of struct
}

struct Empty {}
// trailing file comment
`,
	},
	{
		name:    "reports a parse error with its position",
		src:     "package test;\n\nstruct S {\n    a u8\n}\n",
		wantErr: "test.clv:5:1: expected ;",
	},
}

func TestSource(t *testing.T) {
	t.Parallel()

	for _, tt := range sourceTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Source("test.clv", []byte(tt.src))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Source(%q) error = %v, want one containing %q", tt.src, err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("Source(%q) = %q, want no output with the error", tt.src, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Source(%q) error = %v", tt.src, err)
			}
			if string(got) != tt.want {
				t.Errorf("Source(%q) =\n%s\nwant:\n%s", tt.src, got, tt.want)
			}
			checkSource(t, []byte(tt.src))
		})
	}
}

func TestSource_Testdata(t *testing.T) {
	t.Parallel()

	for _, path := range testdataSpecs(t) {
		t.Run(filepath.ToSlash(path), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			checkSource(t, data)
		})
	}
}

func FuzzSource(f *testing.F) {
	for _, tt := range sourceTests {
		f.Add([]byte(tt.src))
	}
	for _, path := range testdataSpecs(f) {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		if _, errs := parser.New("fuzz.clv", src).Parse(); len(errs) > 0 {
			return
		}
		checkSource(t, src)
	})
}

// checkSource formats src, which must parse, and verifies the properties every
// output has. The output parses to the same declarations as src, holds the same
// comments apart from trailing whitespace, and formats to itself.
func checkSource(t *testing.T, src []byte) {
	t.Helper()

	got, err := Source("test.clv", src)
	if err != nil {
		t.Fatalf("Source(%q) error = %v", src, err)
	}

	want, _ := parser.New("test.clv", src).Parse()
	file, errs := parser.New("test.clv", got).Parse()
	if len(errs) > 0 {
		t.Fatalf("output of Source(%q) does not parse: %v\n%s", src, errs, got)
	}
	if g, w := commentTexts(file), commentTexts(want); !slices.Equal(g, w) {
		t.Errorf("comments of Source(%q) = %q, want %q", src, g, w)
	}
	if !reflect.DeepEqual(declarations(file), declarations(want)) {
		t.Errorf("Source(%q) changed the declarations:\n%s", src, got)
	}

	again, err := Source("test.clv", got)
	if err != nil {
		t.Fatalf("Source of the output of Source(%q) error = %v", src, err)
	}
	if !bytes.Equal(again, got) {
		t.Errorf("Source of its own output =\n%s\nwant it unchanged:\n%s", again, got)
	}
}

// testdataSpecs returns the paths of the spec files in testdata and
// testdata/import. It leaves out testdata/errors, whose specs are broken on
// purpose and need not parse.
func testdataSpecs(tb testing.TB) []string {
	tb.Helper()

	var paths []string
	for _, pattern := range []string{"../../testdata/*.clv", "../../testdata/import/*.clv"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			tb.Fatal(err)
		}
		paths = append(paths, matches...)
	}
	if len(paths) == 0 {
		tb.Fatal("no spec files under testdata")
	}
	return paths
}

// commentTexts returns the text of every comment in file, sorted, with
// trailing whitespace removed from each line. A comment inside a one-line
// construct moves above it, so only the set of comments is stable.
func commentTexts(file *ast.File) []string {
	texts := make([]string, len(file.Comments))
	for i, c := range file.Comments {
		lines := strings.Split(c.Text, "\n")
		for j, l := range lines {
			lines[j] = strings.TrimRight(l, " \t\r")
		}
		texts[i] = strings.Join(lines, "\n")
	}
	slices.Sort(texts)
	return texts
}

// declarations returns file without comments or source positions, so two
// parses compare equal when they declare the same things.
func declarations(file *ast.File) *ast.File {
	file.Comments = nil
	clearPositions(reflect.ValueOf(file))
	return file
}

func clearPositions(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			clearPositions(v.Elem())
		}
	case reflect.Slice:
		for i := range v.Len() {
			clearPositions(v.Index(i))
		}
	case reflect.Struct:
		for i := range v.NumField() {
			switch field := v.Field(i); field.Type() {
			case reflect.TypeFor[token.Pos](), reflect.TypeFor[token.Span]():
				field.SetZero()
			default:
				clearPositions(field)
			}
		}
	}
}
