package format

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rdeusser/cleave/internal/token"
)

// row is an item with its comments placed.
type row struct {
	item

	above    []line   // blank lines and comments printed before the item
	trailing []string // comment at the end of the item's last line, from inline
}

// item is one entry of a list before its comments are placed: a declaration,
// field, variant, case, or key-value entry.
type item struct {
	span        token.Span // source extent, which decides where comments go
	cells       []string   // the first line split into columns that align across rows
	note        []string   // comment after the opening bracket, from inline
	tail        []line     // lines after the first: the indented body and the closing line
	isSeparated bool       // follows a blank line even when the source has none
}

// line is one line of output without its newline.
type line struct {
	text       string
	isVerbatim bool // continues a block comment and keeps its source indentation
}

func (l line) isBlank() bool { return l.text == "" && !l.isVerbatim }

// layout prints rows in alignment sections. A blank line before a row starts
// a new section, and so does a row that spans several lines, after it.
func layout(rows []row) []line {
	var lines []line
	start := 0
	for i := 1; i < len(rows); i++ {
		hasGap := slices.ContainsFunc(rows[i].above, line.isBlank)
		if hasGap || len(rows[i-1].tail) > 0 {
			lines = append(lines, section(rows[start:i])...)
			start = i
		}
	}
	return append(lines, section(rows[start:])...)
}

// section prints rows that align as a group. Each cell except a row's last pads
// to the widest cell in its column among the rows that continue past it, plus
// one space. The trailing comments of one-line rows start one space after the
// longest of those rows.
func section(rows []row) []line {
	var widths []int
	for _, r := range rows {
		for c, cell := range r.cells[:len(r.cells)-1] {
			if c == len(widths) {
				widths = append(widths, 0)
			}
			widths[c] = max(widths[c], utf8.RuneCountInString(cell))
		}
	}

	firsts := make([]string, len(rows))
	column := 0 // widest one-line row with a trailing comment
	for i, r := range rows {
		var sb strings.Builder
		for c, cell := range r.cells[:len(r.cells)-1] {
			sb.WriteString(cell)
			sb.WriteString(strings.Repeat(" ", widths[c]-utf8.RuneCountInString(cell)+1))
		}
		sb.WriteString(r.cells[len(r.cells)-1])
		firsts[i] = sb.String()
		if len(r.tail) == 0 && len(r.trailing) > 0 {
			column = max(column, utf8.RuneCountInString(firsts[i]))
		}
	}

	var lines []line
	for i, r := range rows {
		lines = append(lines, r.above...)
		lines = attach(append(lines, line{text: firsts[i]}), r.note, " ")
		lines = append(lines, r.tail...)
		if len(r.trailing) == 0 {
			continue
		}
		separator := " "
		if len(r.tail) == 0 {
			separator = strings.Repeat(" ", column-utf8.RuneCountInString(firsts[i])+1)
		}
		lines = attach(lines, r.trailing, separator)
	}
	return lines
}

// attach appends comment, as returned by inline, to the last of lines after
// separator. The comment's continuation lines follow as lines of their own.
func attach(lines []line, comment []string, separator string) []line {
	if len(comment) == 0 {
		return lines
	}
	lines[len(lines)-1].text += separator + comment[0]
	for _, text := range comment[1:] {
		lines = append(lines, line{text: text, isVerbatim: true})
	}
	return lines
}

// indent shifts lines one level right in place. Blank lines and block comment
// continuation lines stay as they are.
func indent(lines []line) []line {
	for i, l := range lines {
		if l.text != "" && !l.isVerbatim {
			lines[i].text = indentUnit + l.text
		}
	}
	return lines
}
