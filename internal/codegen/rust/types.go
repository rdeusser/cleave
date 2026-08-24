package rust

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rdeusser/cleave/internal/ir"
)

type matchType struct {
	name     string
	dslName  string
	variants []matchVariant
}

type matchVariant struct {
	name      string
	typ       string
	fieldType ir.FieldType
	key       string
}

func (w *writer) prepareMatches() error {
	for _, structure := range w.pkg.Structs {
		for _, field := range structure.Fields {
			if field.Type.Kind != ir.KindMatch {
				continue
			}
			if field.Match == nil {
				return fmt.Errorf("rust codegen: match field %s.%s has no match specification", structure.Name, field.Name)
			}

			match := &matchType{
				name:    matchName(structure.Name, field.Name),
				dslName: structure.Name + "." + field.Name,
			}
			keys := make(map[string]int)
			bases := make(map[string]int)
			for _, matchCase := range field.Match.Cases {
				if err := w.addMatchVariant(match, matchCase.Type, keys, bases); err != nil {
					return fmt.Errorf("rust codegen: match field %s.%s: %w", structure.Name, field.Name, err)
				}
			}
			if field.Match.Default != nil {
				if err := w.addMatchVariant(match, *field.Match.Default, keys, bases); err != nil {
					return fmt.Errorf("rust codegen: match field %s.%s default: %w", structure.Name, field.Name, err)
				}
			}
			w.matches = append(w.matches, match)
			w.matchFor[field] = match
		}
	}
	return nil
}

func (w *writer) addMatchVariant(
	match *matchType,
	fieldType ir.FieldType,
	keys map[string]int,
	bases map[string]int,
) error {
	typ, err := w.rustType(fieldType, "")
	if err != nil {
		return err
	}
	key := typ
	if _, ok := keys[key]; ok {
		return nil
	}

	base := matchVariantBase(fieldType, typ)
	bases[base]++
	name := base
	if bases[base] > 1 {
		name = fmt.Sprintf("%s%d", base, bases[base])
	}
	keys[key] = len(match.variants)
	match.variants = append(match.variants, matchVariant{
		name:      rustIdent(name),
		typ:       typ,
		fieldType: fieldType,
		key:       key,
	})
	return nil
}

func (w *writer) matchVariant(match *matchType, fieldType ir.FieldType) (*matchVariant, error) {
	typ, err := w.rustType(fieldType, "")
	if err != nil {
		return nil, err
	}
	key := typ
	for i := range match.variants {
		if match.variants[i].key == key {
			return &match.variants[i], nil
		}
	}
	return nil, fmt.Errorf("no generated variant for Rust type %s", typ)
}

func matchVariantBase(fieldType ir.FieldType, typ string) string {
	switch fieldType.Kind {
	case ir.KindEnum, ir.KindStruct:
		return pascal(fieldType.Ref)
	case ir.KindPrimitive:
		switch fieldType.Primitive {
		case ir.Bytes:
			return "Bytes"
		case ir.String:
			return "String"
		default:
			base := strings.ToUpper(fieldType.Primitive.String())
			if fieldType.Array.Kind != ir.NotArray {
				return base + "Array"
			}
			return base
		}
	default:
		return pascal(strings.NewReplacer("<", "_", ">", "_", "[", "_", "]", "_", ";", "_").Replace(typ))
	}
}

func (w *writer) writeDeclarations() error {
	for i, enum := range w.pkg.Enums {
		if i > 0 {
			w.sb.WriteString("\n")
		}
		if err := w.writeEnum(enum); err != nil {
			return err
		}
		w.sb.WriteString("\n")
	}
	for _, match := range w.matches {
		w.writeMatchEnum(match)
		w.sb.WriteString("\n")
	}
	for i, structure := range w.pkg.Structs {
		if i > 0 {
			w.sb.WriteString("\n")
		}
		if err := w.writeStruct(structure); err != nil {
			return err
		}
	}
	return nil
}

func (w *writer) writeEnum(enum *ir.Enum) error {
	backing, err := rustPrimitive(enum.BackingType)
	if err != nil {
		return fmt.Errorf("rust codegen: enum %s: %w", enum.Name, err)
	}
	if !enum.BackingType.IsInteger() {
		return fmt.Errorf("rust codegen: enum %s has non-integer backing type %s", enum.Name, enum.BackingType)
	}

	w.sb.WriteString("#[allow(non_camel_case_types, reason = \"schema names are preserved\")]\n")
	fmt.Fprintf(&w.sb, "#[repr(%s)]\n", backing)
	w.sb.WriteString("#[derive(Debug, Clone, Copy, PartialEq, Eq)]\n")
	fmt.Fprintf(&w.sb, "pub enum %s {\n", rustIdent(enum.Name))
	for _, variant := range enum.Variants {
		fmt.Fprintf(&w.sb, "    %s = %s,\n", rustIdent(variant.Name), rustInt(variant.Value))
	}
	w.sb.WriteString("}\n")
	fmt.Fprintf(&w.sb, "\nimpl ::std::convert::TryFrom<%s> for %s {\n", backing, rustIdent(enum.Name))
	w.sb.WriteString("    type Error = Error;\n\n")
	fmt.Fprintf(
		&w.sb,
		"    fn try_from(value: %s) -> ::std::result::Result<Self, Self::Error> {\n",
		backing,
	)
	w.sb.WriteString("        match value {\n")
	for _, variant := range enum.Variants {
		fmt.Fprintf(
			&w.sb,
			"            %s => Ok(Self::%s),\n",
			rustInt(variant.Value),
			rustIdent(variant.Name),
		)
	}
	w.sb.WriteString("            _ => Err(Error::InvalidEnum {\n")
	fmt.Fprintf(&w.sb, "                enum_name: %s,\n", rustString(enum.Name))
	w.sb.WriteString("                value: i128::from(value),\n")
	w.sb.WriteString("            }),\n")
	w.sb.WriteString("        }\n")
	w.sb.WriteString("    }\n")
	w.sb.WriteString("}\n")
	fmt.Fprintf(&w.sb, "\nimpl ::std::convert::From<%s> for %s {\n", rustIdent(enum.Name), backing)
	fmt.Fprintf(&w.sb, "    fn from(value: %s) -> Self {\n", rustIdent(enum.Name))
	fmt.Fprintf(&w.sb, "        value as %s\n", backing)
	w.sb.WriteString("    }\n")
	w.sb.WriteString("}\n")
	return nil
}

func (w *writer) writeMatchEnum(match *matchType) {
	w.sb.WriteString("#[derive(Debug, Clone, PartialEq)]\n")
	fmt.Fprintf(&w.sb, "pub enum %s {\n", match.name)
	for _, variant := range match.variants {
		fmt.Fprintf(&w.sb, "    %s(%s),\n", variant.name, variant.typ)
	}
	w.sb.WriteString("}\n")
}

func (w *writer) writeStruct(structure *ir.Struct) error {
	w.sb.WriteString("#[allow(non_camel_case_types, reason = \"schema names are preserved\")]\n")
	w.sb.WriteString("#[allow(non_snake_case, reason = \"schema field names are preserved\")]\n")
	w.sb.WriteString("#[derive(Debug, Clone, PartialEq)]\n")
	if len(structure.Fields) == 0 {
		fmt.Fprintf(&w.sb, "pub struct %s {}\n", rustIdent(structure.Name))
	} else {
		fmt.Fprintf(&w.sb, "pub struct %s {\n", rustIdent(structure.Name))
		for _, field := range structure.Fields {
			typ, err := w.fieldType(field)
			if err != nil {
				return fmt.Errorf("rust codegen: field %s.%s: %w", structure.Name, field.Name, err)
			}
			fmt.Fprintf(&w.sb, "    pub %s: %s,\n", rustIdent(field.Name), typ)
		}
		w.sb.WriteString("}\n")
	}
	w.sb.WriteString("\n")
	if err := w.writeParse(structure); err != nil {
		return err
	}
	w.sb.WriteString("\n")
	if err := w.writeSerialize(structure); err != nil {
		return err
	}
	w.sb.WriteString("\n")
	return w.writeJSON(structure)
}

func (w *writer) fieldType(field *ir.Field) (string, error) {
	var typ string
	if field.Type.Kind == ir.KindMatch {
		match, ok := w.matchFor[field]
		if !ok {
			return "", errors.New("missing generated match type")
		}
		typ = match.name
	} else {
		var err error
		typ, err = w.rustType(field.Type, field.Encoding)
		if err != nil {
			return "", err
		}
	}
	if field.Condition != nil {
		return "::std::option::Option<" + typ + ">", nil
	}
	return typ, nil
}

func (w *writer) rustType(fieldType ir.FieldType, encoding string) (string, error) {
	if fieldType.Array.FixedSize < 0 {
		return "", fmt.Errorf("negative fixed size %d", fieldType.Array.FixedSize)
	}

	switch fieldType.Kind {
	case ir.KindPrimitive:
		return rustPrimitiveFieldType(fieldType, encoding)
	case ir.KindEnum, ir.KindStruct:
		base := rustIdent(fieldType.Ref)
		return rustArrayType(base, fieldType.Array), nil
	default:
		return "", fmt.Errorf("unsupported field kind %d", fieldType.Kind)
	}
}

func rustPrimitiveFieldType(fieldType ir.FieldType, encoding string) (string, error) {
	primitive := fieldType.Primitive
	if primitive == ir.String && (encoding != "" || fieldType.Array.Kind == ir.FixedTerminator) {
		return "::std::string::String", nil
	}
	if primitive == ir.Bytes || primitive == ir.String {
		if fieldType.Array.Kind == ir.FixedSize {
			return fmt.Sprintf("[u8; %s]", rustInt(fieldType.Array.FixedSize)), nil
		}
		return "::std::vec::Vec<u8>", nil
	}

	base, err := rustPrimitive(primitive)
	if err != nil {
		return "", err
	}
	return rustArrayType(base, fieldType.Array), nil
}

func rustArrayType(base string, array ir.ArraySpec) string {
	switch array.Kind {
	case ir.NotArray:
		return base
	case ir.FixedSize:
		return fmt.Sprintf("[%s; %s]", base, rustInt(array.FixedSize))
	default:
		return "::std::vec::Vec<" + base + ">"
	}
}

func rustPrimitive(primitive ir.PrimitiveType) (string, error) {
	switch primitive {
	case ir.U8:
		return "u8", nil
	case ir.U16:
		return "u16", nil
	case ir.U32:
		return "u32", nil
	case ir.U64:
		return "u64", nil
	case ir.I8:
		return "i8", nil
	case ir.I16:
		return "i16", nil
	case ir.I32:
		return "i32", nil
	case ir.I64:
		return "i64", nil
	case ir.F32:
		return "f32", nil
	case ir.F64:
		return "f64", nil
	default:
		return "", fmt.Errorf("primitive %s has no scalar Rust type", primitive)
	}
}
