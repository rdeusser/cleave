package lexer

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/rdeusser/cleave/internal/token"
)

type Lexer struct {
	src   []byte
	file  string
	pos   int // current byte offset
	start int // start of current token
	line  int
	col   int
	lines []int // byte offset of each line start (for position decoding)
}

func New(file string, src []byte) *Lexer {
	return &Lexer{
		src:   src,
		file:  file,
		line:  1,
		col:   1,
		lines: []int{0},
	}
}

func (l *Lexer) Position(p token.Pos) token.Position {
	offset := int(p)
	line := 1
	col := offset + 1
	for i, start := range l.lines {
		if offset >= start {
			line = i + 1
			col = offset - start + 1
		}
	}
	return token.Position{File: l.file, Line: line, Column: col}
}

func (l *Lexer) Next() token.Token {
	l.skipWhitespace()

	if l.pos >= len(l.src) {
		return l.makeToken(token.EOF, l.pos, "")
	}

	l.start = l.pos
	ch := l.src[l.pos]

	// Line comments.
	if ch == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
		return l.scanLineComment()
	}

	// Block comments.
	if ch == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*' {
		return l.scanBlockComment()
	}

	// String literals.
	if ch == '"' {
		return l.scanString()
	}

	// Number literals.
	if ch >= '0' && ch <= '9' {
		return l.scanNumber()
	}

	// Identifiers and keywords.
	if isIdentStart(ch) {
		return l.scanIdent()
	}

	// Punctuation.
	l.advance()
	switch ch {
	case ';':
		return l.makeToken(token.Semicolon, l.start, ";")
	case ':':
		return l.makeToken(token.Colon, l.start, ":")
	case ',':
		return l.makeToken(token.Comma, l.start, ",")
	case '=':
		if l.pos < len(l.src) && l.src[l.pos] == '>' {
			l.advance()
			return l.makeToken(token.FatArrow, l.start, "=>")
		}
		return l.makeToken(token.Assign, l.start, "=")
	case '-':
		return l.makeToken(token.Minus, l.start, "-")
	case '{':
		return l.makeToken(token.LBrace, l.start, "{")
	case '}':
		return l.makeToken(token.RBrace, l.start, "}")
	case '[':
		return l.makeToken(token.LBracket, l.start, "[")
	case ']':
		return l.makeToken(token.RBracket, l.start, "]")
	case '.':
		return l.makeToken(token.Dot, l.start, ".")
	case '(':
		return l.makeToken(token.LParen, l.start, "(")
	case ')':
		return l.makeToken(token.RParen, l.start, ")")
	default:
		return l.makeToken(token.Illegal, l.start, fmt.Sprintf("unexpected character %q", ch))
	}
}

func (l *Lexer) scanLineComment() token.Token {
	start := l.pos
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.advance()
	}
	return l.makeToken(token.Comment, start, string(l.src[start:l.pos]))
}

func (l *Lexer) scanBlockComment() token.Token {
	start := l.pos
	l.advance() // /
	l.advance() // *
	for l.pos < len(l.src) {
		if l.src[l.pos] == '*' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			l.advance() // *
			l.advance() // /
			return l.makeToken(token.Comment, start, string(l.src[start:l.pos]))
		}
		l.advance()
	}
	return l.makeToken(token.Illegal, start, "unterminator block comment")
}

func (l *Lexer) scanString() token.Token {
	start := l.pos
	l.advance() // opening "
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '\\' && l.pos+1 < len(l.src) {
			l.advance() // backslash
			l.advance() // escaped char
			continue
		}
		if ch == '"' {
			l.advance()
			raw := string(l.src[start:l.pos])
			return l.makeToken(token.String, start, raw)
		}
		if ch == '\n' {
			return l.makeToken(token.Illegal, start, "unterminator string literal")
		}
		l.advance()
	}
	return l.makeToken(token.Illegal, start, "unterminator string literal")
}

func (l *Lexer) scanNumber() token.Token {
	start := l.pos
	if l.src[l.pos] == '0' && l.pos+1 < len(l.src) {
		next := l.src[l.pos+1]
		if next == 'x' || next == 'X' {
			l.advance() // 0
			l.advance() // x
			if l.pos >= len(l.src) || !isHexDigit(l.src[l.pos]) {
				return l.makeToken(token.Illegal, start, "invalid hex literal")
			}
			for l.pos < len(l.src) && isHexDigit(l.src[l.pos]) {
				l.advance()
			}
			return l.makeToken(token.Integer, start, string(l.src[start:l.pos]))
		}
		if next == 'b' || next == 'B' {
			l.advance() // 0
			l.advance() // b
			if l.pos >= len(l.src) || (l.src[l.pos] != '0' && l.src[l.pos] != '1') {
				return l.makeToken(token.Illegal, start, "invalid binary literal")
			}
			for l.pos < len(l.src) && (l.src[l.pos] == '0' || l.src[l.pos] == '1') {
				l.advance()
			}
			return l.makeToken(token.Integer, start, string(l.src[start:l.pos]))
		}
	}
	for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
		l.advance()
	}
	return l.makeToken(token.Integer, start, string(l.src[start:l.pos]))
}

func (l *Lexer) scanIdent() token.Token {
	start := l.pos
	for l.pos < len(l.src) && isIdentContinue(l.src[l.pos]) {
		l.advance()
	}
	lit := string(l.src[start:l.pos])
	typ := token.Lookup(lit)
	return l.makeToken(typ, start, lit)
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '\n' {
			l.advance()
		} else if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
		} else {
			break
		}
	}
}

func (l *Lexer) advance() {
	if l.pos < len(l.src) {
		if l.src[l.pos] == '\n' {
			l.line++
			l.col = 1
			l.pos++
			l.lines = append(l.lines, l.pos)
		} else {
			_, size := utf8.DecodeRune(l.src[l.pos:])
			l.pos += size
			l.col++
		}
	}
}

func (l *Lexer) makeToken(typ token.Type, start int, lit string) token.Token {
	return token.Token{
		Type:    typ,
		Span:    token.Span{Start: token.Pos(start), End: token.Pos(l.pos)},
		Literal: lit,
	}
}

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isIdentContinue(ch byte) bool {
	r := rune(ch)
	return ch == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}
