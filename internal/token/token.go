package token

import "fmt"

type Pos int

const NoPos Pos = 0

type Span struct {
	Start Pos
	End   Pos
}

type Type int

const (
	Illegal Type = iota
	EOF
	Comment

	// Literals.
	Ident
	Integer
	String

	// Punctuation.
	Semicolon // ;
	Colon     // :
	Comma     // ,
	Assign    // =
	FatArrow  // =>
	Minus     // -
	LBrace    // {
	RBrace    // }
	LBracket  // [
	RBracket  // ]
	Dot       // .
	LParen    // (
	RParen    // )

	keywordStart
	// Keywords.
	Package
	Import
	Struct
	Enum
	Format
	Union
	Match
	Option
	True
	False
	keywordEnd
)

type Token struct {
	Type    Type
	Span    Span
	Literal string
}

type Position struct {
	File   string
	Line   int
	Column int
}

func Lookup(ident string) Type {
	if t, ok := keywords[ident]; ok {
		return t
	}
	return Ident
}

func (t Type) String() string {
	if int(t) < len(typeNames) && typeNames[t] != "" {
		return typeNames[t]
	}
	return fmt.Sprintf("Type(%d)", t)
}

func (t Type) IsKeyword() bool {
	return t > keywordStart && t < keywordEnd
}

func (p Position) String() string {
	if p.File != "" {
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

var typeNames = [...]string{
	Illegal:   "Illegal",
	EOF:       "EOF",
	Comment:   "Comment",
	Ident:     "Ident",
	Integer:   "Integer",
	String:    "String",
	Semicolon: ";",
	Colon:     ":",
	Comma:     ",",
	Assign:    "=",
	FatArrow:  "=>",
	Minus:     "-",
	LBrace:    "{",
	RBrace:    "}",
	LBracket:  "[",
	RBracket:  "]",
	Dot:       ".",
	LParen:    "(",
	RParen:    ")",
	Package:   "package",
	Import:    "import",
	Struct:    "struct",
	Enum:      "enum",
	Format:    "format",
	Union:     "union",
	Match:     "match",
	Option:    "option",
	True:      "true",
	False:     "false",
}

var keywords = map[string]Type{
	"package": Package,
	"import":  Import,
	"struct":  Struct,
	"enum":    Enum,
	"format":  Format,
	"union":   Union,
	"match":   Match,
	"option":  Option,
	"true":    True,
	"false":   False,
}
