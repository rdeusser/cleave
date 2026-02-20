package lexer

import (
	"os"
	"testing"

	"github.com/rdeusser/cleave/internal/token"
)

func TestNextPunctuation(t *testing.T) {
	src := `; : , = { } [ ] ( )`
	lex := New("test.clv", []byte(src))

	want := []token.Type{
		token.Semicolon, token.Colon, token.Comma, token.Assign,
		token.LBrace, token.RBrace, token.LBracket, token.RBracket,
		token.LParen, token.RParen, token.EOF,
	}

	for i, w := range want {
		tok := lex.Next()
		if tok.Type != w {
			t.Errorf("token %d: got %v, want %v", i, tok.Type, w)
		}
	}
}

func TestNextUnionTokens(t *testing.T) {
	src := `=> - _ match union`
	lex := New("test.clv", []byte(src))

	want := []struct {
		typ token.Type
		lit string
	}{
		{token.FatArrow, "=>"},
		{token.Minus, "-"},
		{token.Ident, "_"},
		{token.Match, "match"},
		{token.Union, "union"},
		{token.EOF, ""},
	}

	for i, w := range want {
		tok := lex.Next()
		if tok.Type != w.typ {
			t.Errorf("token %d: type got %v, want %v", i, tok.Type, w.typ)
		}
		if tok.Literal != w.lit {
			t.Errorf("token %d: literal got %q, want %q", i, tok.Literal, w.lit)
		}
	}
}

func TestNextKeywords(t *testing.T) {
	src := `package struct enum option true false`
	lex := New("test.clv", []byte(src))

	want := []struct {
		typ token.Type
		lit string
	}{
		{token.Package, "package"},
		{token.Struct, "struct"},
		{token.Enum, "enum"},
		{token.Option, "option"},
		{token.True, "true"},
		{token.False, "false"},
		{token.EOF, ""},
	}

	for i, w := range want {
		tok := lex.Next()
		if tok.Type != w.typ {
			t.Errorf("token %d: type got %v, want %v", i, tok.Type, w.typ)
		}
		if tok.Literal != w.lit {
			t.Errorf("token %d: literal got %q, want %q", i, tok.Literal, w.lit)
		}
	}
}

func TestNextIdentifiers(t *testing.T) {
	src := `foo bar_baz u32 MyStruct _private`
	lex := New("test.clv", []byte(src))

	want := []string{"foo", "bar_baz", "u32", "MyStruct", "_private"}

	for i, w := range want {
		tok := lex.Next()
		if tok.Type != token.Ident {
			t.Errorf("token %d: type got %v, want Ident", i, tok.Type)
		}
		if tok.Literal != w {
			t.Errorf("token %d: literal got %q, want %q", i, tok.Literal, w)
		}
	}
}

func TestNextNumbers(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"42", "42"},
		{"0", "0"},
		{"0xFF", "0xFF"},
		{"0x0A", "0x0A"},
		{"0b1010", "0b1010"},
		{"0B110", "0B110"},
		{"12345", "12345"},
	}

	for _, tt := range tests {
		lex := New("test.clv", []byte(tt.src))
		tok := lex.Next()
		if tok.Type != token.Integer {
			t.Errorf("src %q: type got %v, want Integer", tt.src, tok.Type)
		}
		if tok.Literal != tt.want {
			t.Errorf("src %q: literal got %q, want %q", tt.src, tok.Literal, tt.want)
		}
	}
}

func TestNextStrings(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`"hello"`, `"hello"`},
		{`"with \"escape\""`, `"with \"escape\""`},
		{`""`, `""`},
	}

	for _, tt := range tests {
		lex := New("test.clv", []byte(tt.src))
		tok := lex.Next()
		if tok.Type != token.String {
			t.Errorf("src %q: type got %v, want String", tt.src, tok.Type)
		}
		if tok.Literal != tt.want {
			t.Errorf("src %q: literal got %q, want %q", tt.src, tok.Literal, tt.want)
		}
	}
}

func TestNextComments(t *testing.T) {
	src := "// line comment\nx /* block */ y"
	lex := New("test.clv", []byte(src))

	want := []struct {
		typ token.Type
		lit string
	}{
		{token.Comment, "// line comment"},
		{token.Ident, "x"},
		{token.Comment, "/* block */"},
		{token.Ident, "y"},
		{token.EOF, ""},
	}

	for i, w := range want {
		tok := lex.Next()
		if tok.Type != w.typ {
			t.Errorf("token %d: type got %v, want %v", i, tok.Type, w.typ)
		}
		if tok.Literal != w.lit {
			t.Errorf("token %d: literal got %q, want %q", i, tok.Literal, w.lit)
		}
	}
}

func TestNextIllegal(t *testing.T) {
	tests := []struct {
		src  string
		desc string
	}{
		{"@", "unexpected character"},
		{`"unterminator`, "unterminator string"},
		{"0x", "invalid hex literal"},
		{"0b", "invalid binary literal"},
	}

	for _, tt := range tests {
		lex := New("test.clv", []byte(tt.src))
		tok := lex.Next()
		if tok.Type != token.Illegal {
			t.Errorf("src %q (%s): type got %v, want Illegal", tt.src, tt.desc, tok.Type)
		}
	}
}

func TestPositionTracking(t *testing.T) {
	src := "x\ny\nz"
	lex := New("test.clv", []byte(src))

	lex.Next()        // x
	tok := lex.Next() // y

	pos := lex.Position(tok.Span.Start)
	if pos.Line != 2 || pos.Column != 1 {
		t.Errorf("y position: got %d:%d, want 2:1", pos.Line, pos.Column)
	}
}

func TestLexTestdataSimple(t *testing.T) {
	src, err := os.ReadFile("../../testdata/simple.clv")
	if err != nil {
		t.Fatal(err)
	}

	lex := New("simple.clv", src)
	var tokens []token.Token
	for {
		tok := lex.Next()
		tokens = append(tokens, tok)
		if tok.Type == token.EOF || tok.Type == token.Illegal {
			break
		}
	}

	last := tokens[len(tokens)-1]
	if last.Type != token.EOF {
		t.Errorf("expected EOF at end, got %v: %q", last.Type, last.Literal)
	}
}

func TestLexTestdataPng(t *testing.T) {
	src, err := os.ReadFile("../../testdata/png.clv")
	if err != nil {
		t.Fatal(err)
	}

	lex := New("png.clv", src)
	var tokens []token.Token
	for {
		tok := lex.Next()
		tokens = append(tokens, tok)
		if tok.Type == token.EOF || tok.Type == token.Illegal {
			break
		}
	}

	last := tokens[len(tokens)-1]
	if last.Type != token.EOF {
		t.Errorf("expected EOF at end, got %v: %q", last.Type, last.Literal)
	}
}
