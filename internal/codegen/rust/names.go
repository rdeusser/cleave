package rust

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/rdeusser/cleave/internal/ir"
)

var rustKeywords = map[string]struct{}{
	"abstract": {}, "as": {}, "async": {}, "await": {}, "become": {},
	"box": {}, "break": {}, "const": {}, "continue": {}, "do": {},
	"dyn": {}, "else": {}, "enum": {}, "extern": {}, "false": {},
	"final": {}, "fn": {}, "for": {}, "gen": {}, "if": {},
	"impl": {}, "in": {}, "let": {}, "loop": {}, "macro": {},
	"match": {}, "mod": {}, "move": {}, "mut": {}, "override": {},
	"priv": {}, "pub": {}, "ref": {}, "return": {}, "static": {},
	"struct": {}, "trait": {}, "true": {}, "try": {}, "type": {},
	"typeof": {}, "union": {}, "unsafe": {}, "unsized": {}, "use": {},
	"virtual": {}, "where": {}, "while": {}, "yield": {},
}

var rustSpecialKeywords = map[string]struct{}{
	"Self":  {},
	"crate": {},
	"self":  {},
	"super": {},
}

func rustIdent(name string) string {
	if _, ok := rustSpecialKeywords[name]; ok {
		return name + "_"
	}
	if _, ok := rustKeywords[name]; ok {
		return "r#" + name
	}
	return name
}

func matchName(structName, fieldName string) string {
	return rustIdent(pascal(structName) + pascal(fieldName))
}

func pascal(name string) string {
	parts := strings.Split(name, "_")
	var result strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		result.WriteString(string(runes))
	}
	if result.Len() == 0 {
		return "Value"
	}
	return result.String()
}

func rustString(value string) string {
	var result strings.Builder
	result.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			result.WriteString("\\\\")
		case '"':
			result.WriteString("\\\"")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		case '\x00':
			result.WriteString("\\0")
		default:
			if unicode.IsControl(r) {
				fmt.Fprintf(&result, "\\u{%x}", r)
			} else {
				result.WriteRune(r)
			}
		}
	}
	result.WriteByte('"')
	return result.String()
}

func rustInt(value int64) string {
	formatted := strconv.FormatInt(value, 10)
	sign := ""
	digits := formatted
	if strings.HasPrefix(digits, "-") {
		sign = "-"
		digits = strings.TrimPrefix(digits, "-")
	}
	if len(digits) <= 4 {
		return formatted
	}

	var result strings.Builder
	result.WriteString(sign)
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			result.WriteByte('_')
		}
		result.WriteRune(digit)
	}
	return result.String()
}

func (w *writer) validateNames() error {
	types := make(map[string]string)
	for _, enum := range w.pkg.Enums {
		if err := recordTypeName(types, rustIdent(enum.Name), enum.Name); err != nil {
			return err
		}
		if err := validateEnumVariants(enum.Name, enum.Variants); err != nil {
			return err
		}
	}
	for _, structure := range w.pkg.Structs {
		if err := recordTypeName(types, rustIdent(structure.Name), structure.Name); err != nil {
			return err
		}
		if err := validateFields(structure); err != nil {
			return err
		}
	}
	for _, match := range w.matches {
		if err := recordTypeName(types, match.name, match.dslName); err != nil {
			return err
		}
	}
	return nil
}

func recordTypeName(names map[string]string, mapped, original string) error {
	if mapped == "Error" {
		return fmt.Errorf("type %q conflicts with generated Rust name %q", original, mapped)
	}
	return recordName(names, mapped, original, "types")
}

func validateEnumVariants(enumName string, variants []ir.EnumVariant) error {
	names := make(map[string]string)
	for _, variant := range variants {
		if err := recordName(names, rustIdent(variant.Name), variant.Name, "variants"); err != nil {
			return fmt.Errorf("enum %q: %w", enumName, err)
		}
	}
	return nil
}

func validateFields(structure *ir.Struct) error {
	names := make(map[string]string)
	for _, field := range structure.Fields {
		mapped := rustIdent(field.Name)
		if previous, ok := names[mapped]; ok {
			return fmt.Errorf(
				"struct %q: fields %q and %q map to %q",
				structure.Name,
				previous,
				field.Name,
				mapped,
			)
		}
		names[mapped] = field.Name
	}
	return nil
}

func recordName(names map[string]string, mapped, original, kind string) error {
	if previous, ok := names[mapped]; ok {
		return fmt.Errorf("%s %q and %q map to %q", kind, previous, original, mapped)
	}
	names[mapped] = original
	return nil
}
