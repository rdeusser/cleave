package rust

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rdeusser/cleave/internal/ir"
)

func (w *writer) writeSerialize(structure *ir.Struct) error {
	lengthSources, countSources := autoComputeSources(structure)

	fmt.Fprintf(&w.sb, "impl %s {\n", rustIdent(structure.Name))
	w.sb.WriteString("    #[allow(non_snake_case, reason = \"schema field names are preserved\")]\n")
	w.sb.WriteString("    #[allow(clippy::too_many_lines, reason = \"schema methods follow field order\")]\n")
	w.sb.WriteString("    pub fn to_bytes(&self) -> ::std::result::Result<::std::vec::Vec<u8>, Error> {\n")
	if len(structure.Fields) == 0 {
		w.sb.WriteString("        Ok(::std::vec::Vec::new())\n")
		w.sb.WriteString("    }\n")
		w.sb.WriteString("}\n")
		return nil
	}
	w.sb.WriteString("        let mut buf = ::std::vec::Vec::new();\n")
	for _, field := range structure.Fields {
		source := lengthSources[field.Name]
		if source == nil {
			source = countSources[field.Name]
		}
		if field.Condition != nil {
			fmt.Fprintf(
				&w.sb,
				"        if let ::std::option::Option::Some(value) = &self.%s {\n",
				rustIdent(field.Name),
			)
			if err := w.writeFieldBytes(structure, field, source, "value", "            "); err != nil {
				return err
			}
			w.sb.WriteString("        }\n")
			continue
		}

		if source == nil {
			fmt.Fprintf(&w.sb, "        let value = &self.%s;\n", rustIdent(field.Name))
		}
		if err := w.writeFieldBytes(structure, field, source, "value", "        "); err != nil {
			return err
		}
	}
	w.sb.WriteString("        Ok(buf)\n")
	w.sb.WriteString("    }\n")
	w.sb.WriteString("}\n")
	return nil
}

func (w *writer) writeFieldBytes(
	structure *ir.Struct,
	field,
	source *ir.Field,
	access,
	indent string,
) error {
	endian := field.EffectiveEndian(structure.Options.Endian)
	if source != nil {
		return w.writeComputedLength(field, source, endian, indent)
	}
	if field.Type.Kind == ir.KindMatch {
		return w.writeMatchBytes(structure, field, access, endian, indent)
	}
	return w.writeTypeBytes(field.Type, field.Encoding, field.Name, access, endian, indent)
}

func (w *writer) writeTypeBytes(
	fieldType ir.FieldType,
	encoding,
	fieldName,
	access string,
	endian ir.Endian,
	indent string,
) error {
	field := &ir.Field{
		Name:     fieldName,
		Type:     fieldType,
		Encoding: encoding,
	}
	if isTextField(field) {
		return w.writeTextBytes(field, access, indent)
	}
	if isRawBytesField(field) {
		return writeRawBytes(field, access, indent, &w.sb)
	}

	array := fieldType.Array
	elementType := fieldType
	elementType.Array = ir.ArraySpec{}
	switch array.Kind {
	case ir.NotArray:
		return w.writeElementBytes(elementType, access, endian, indent)
	case ir.FixedSize, ir.LengthRef, ir.CountRef, ir.RestArray:
		fmt.Fprintf(&w.sb, "%sfor item in %s {\n", indent, access)
		if err := w.writeElementBytes(elementType, "item", endian, indent+"    "); err != nil {
			return err
		}
		fmt.Fprintf(&w.sb, "%s}\n", indent)
		return nil
	case ir.Terminator:
		if fieldType.Kind != ir.KindPrimitive || !fieldType.Primitive.IsInteger() {
			return errors.New("rust codegen: terminator arrays require an integer primitive")
		}
		fmt.Fprintf(&w.sb, "%sfor item in %s {\n", indent, access)
		if err := w.writeElementBytes(elementType, "item", endian, indent+"    "); err != nil {
			return err
		}
		fmt.Fprintf(&w.sb, "%s}\n", indent)
		typ, err := rustPrimitive(fieldType.Primitive)
		if err != nil {
			return err
		}
		return writePrimitiveBytes(
			fieldType.Primitive,
			rustInt(array.Sentinel)+"_"+typ,
			endian,
			indent,
			&w.sb,
		)
	default:
		return fmt.Errorf("rust codegen: unsupported array kind %d", array.Kind)
	}
}

func (w *writer) writeElementBytes(
	fieldType ir.FieldType,
	access string,
	endian ir.Endian,
	indent string,
) error {
	switch fieldType.Kind {
	case ir.KindPrimitive:
		return writePrimitiveBytes(fieldType.Primitive, "*"+access, endian, indent, &w.sb)
	case ir.KindEnum:
		enum := w.lookupEnum(fieldType.Ref)
		if enum == nil {
			return fmt.Errorf("rust codegen: unknown enum %q", fieldType.Ref)
		}
		backing, err := rustPrimitive(enum.BackingType)
		if err != nil {
			return err
		}
		fmt.Fprintf(&w.sb, "%slet raw: %s = (*%s).into();\n", indent, backing, access)
		return writePrimitiveBytes(enum.BackingType, "raw", endian, indent, &w.sb)
	case ir.KindStruct:
		fmt.Fprintf(&w.sb, "%sbuf.extend_from_slice(&%s.to_bytes()?);\n", indent, access)
		return nil
	default:
		return fmt.Errorf("rust codegen: unsupported serialized element kind %d", fieldType.Kind)
	}
}

func writePrimitiveBytes(
	primitive ir.PrimitiveType,
	access string,
	endian ir.Endian,
	indent string,
	result *strings.Builder,
) error {
	if primitive == ir.Bytes || primitive == ir.String {
		return fmt.Errorf("rust codegen: %s is not a scalar primitive", primitive)
	}
	order := "le"
	if endian == ir.BigEndian {
		order = "be"
	}
	if primitive.Size() == 1 {
		order = "ne"
	}
	fmt.Fprintf(
		result,
		"%sbuf.extend_from_slice(&(%s).to_%s_bytes());\n",
		indent,
		access,
		order,
	)
	return nil
}

func writeRawBytes(
	field *ir.Field,
	access,
	indent string,
	result *strings.Builder,
) error {
	switch field.Type.Array.Kind {
	case ir.NotArray, ir.FixedSize, ir.LengthRef, ir.CountRef, ir.RestArray:
		fmt.Fprintf(result, "%sbuf.extend_from_slice(%s);\n", indent, access)
	case ir.Terminator:
		fmt.Fprintf(result, "%sbuf.extend_from_slice(%s);\n", indent, access)
		fmt.Fprintf(result, "%sbuf.push(%s_u8);\n", indent, rustInt(field.Type.Array.Sentinel))
	default:
		return fmt.Errorf("rust codegen: unsupported raw byte array kind %d", field.Type.Array.Kind)
	}
	return nil
}

func (w *writer) writeTextBytes(field *ir.Field, access, indent string) error {
	encoding := field.Encoding
	if encoding == "" {
		encoding = "utf-8"
	}
	fmt.Fprintf(
		&w.sb,
		"%slet bytes = encode_text(%s.as_str(), %s)?;\n",
		indent,
		access,
		rustString(encoding),
	)

	switch field.Type.Array.Kind {
	case ir.FixedSize:
		w.writeFixedSizeCheck(field.Name, field.Type.Array.FixedSize, "bytes.len()", indent)
		fmt.Fprintf(&w.sb, "%sbuf.extend_from_slice(&bytes);\n", indent)
	case ir.FixedTerminator:
		size := field.Type.Array.FixedSize
		fmt.Fprintf(&w.sb, "%sif bytes.len() > %s {\n", indent, rustInt(size))
		fmt.Fprintf(&w.sb, "%s    return Err(Error::FixedSize {\n", indent)
		fmt.Fprintf(&w.sb, "%s        field: %s,\n", indent, rustString(field.Name))
		fmt.Fprintf(&w.sb, "%s        expected: %s,\n", indent, rustInt(size))
		fmt.Fprintf(&w.sb, "%s        actual: bytes.len(),\n", indent)
		fmt.Fprintf(&w.sb, "%s    });\n", indent)
		fmt.Fprintf(&w.sb, "%s}\n", indent)
		fmt.Fprintf(&w.sb, "%sbuf.extend_from_slice(&bytes);\n", indent)
		fmt.Fprintf(&w.sb, "%slet padding = %s - bytes.len();\n", indent, rustInt(size))
		fmt.Fprintf(&w.sb, "%slet new_len = buf\n", indent)
		fmt.Fprintf(&w.sb, "%s    .len()\n", indent)
		fmt.Fprintf(&w.sb, "%s    .checked_add(padding)\n", indent)
		fmt.Fprintf(&w.sb, "%s    .ok_or_else(|| Error::LengthOverflow {\n", indent)
		fmt.Fprintf(&w.sb, "%s        field: %s,\n", indent, rustString(field.Name))
		fmt.Fprintf(&w.sb, "%s        value: padding.to_string(),\n", indent)
		fmt.Fprintf(&w.sb, "%s        target: \"usize\",\n", indent)
		fmt.Fprintf(&w.sb, "%s    })?;\n", indent)
		fmt.Fprintf(
			&w.sb,
			"%sbuf.resize(new_len, %s_u8);\n",
			indent,
			rustInt(field.Type.Array.Sentinel),
		)
	case ir.Terminator:
		fmt.Fprintf(&w.sb, "%sbuf.extend_from_slice(&bytes);\n", indent)
		fmt.Fprintf(&w.sb, "%sbuf.push(%s_u8);\n", indent, rustInt(field.Type.Array.Sentinel))
	default:
		fmt.Fprintf(&w.sb, "%sbuf.extend_from_slice(&bytes);\n", indent)
	}
	return nil
}

func (w *writer) writeFixedSizeCheck(fieldName string, size int64, actual, indent string) {
	fmt.Fprintf(&w.sb, "%sif %s != %s {\n", indent, actual, rustInt(size))
	fmt.Fprintf(&w.sb, "%s    return Err(Error::FixedSize {\n", indent)
	fmt.Fprintf(&w.sb, "%s        field: %s,\n", indent, rustString(fieldName))
	fmt.Fprintf(&w.sb, "%s        expected: %s,\n", indent, rustInt(size))
	fmt.Fprintf(&w.sb, "%s        actual: %s,\n", indent, actual)
	fmt.Fprintf(&w.sb, "%s    });\n", indent)
	fmt.Fprintf(&w.sb, "%s}\n", indent)
}

func (w *writer) writeComputedLength(
	field,
	source *ir.Field,
	endian ir.Endian,
	indent string,
) error {
	if field.Type.Kind != ir.KindPrimitive || !field.Type.Primitive.IsInteger() {
		return fmt.Errorf("rust codegen: auto-computed field %s must be an integer primitive", field.Name)
	}
	typ, err := rustPrimitive(field.Type.Primitive)
	if err != nil {
		return err
	}
	length := serializedLengthExpression(source, indent)
	fmt.Fprintf(&w.sb, "%slet actual = %s;\n", indent, length)
	fmt.Fprintf(
		&w.sb,
		"%slet computed = %s::try_from(actual).map_err(|_| Error::LengthOverflow {\n",
		indent,
		typ,
	)
	fmt.Fprintf(&w.sb, "%s    field: %s,\n", indent, rustString(field.Name))
	fmt.Fprintf(&w.sb, "%s    value: actual.to_string(),\n", indent)
	fmt.Fprintf(&w.sb, "%s    target: %s,\n", indent, rustString(typ))
	fmt.Fprintf(&w.sb, "%s})?;\n", indent)
	return writePrimitiveBytes(field.Type.Primitive, "computed", endian, indent, &w.sb)
}

func serializedLengthExpression(field *ir.Field, indent string) string {
	access := "self." + rustIdent(field.Name)
	if isTextField(field) {
		encoding := field.Encoding
		if encoding == "" {
			encoding = "utf-8"
		}
		if field.Condition != nil {
			return fmt.Sprintf(
				"if let ::std::option::Option::Some(value) = &%s {\n%s    self::encode_text(value.as_str(), %s)?.len()\n%s} else {\n%s    0\n%s}",
				access,
				indent,
				rustString(encoding),
				indent,
				indent,
				indent,
			)
		}
		return fmt.Sprintf(
			"self::encode_text(%s.as_str(), %s)?.len()",
			access,
			rustString(encoding),
		)
	}
	if field.Condition != nil {
		return access + ".as_ref().map_or(0, ::std::vec::Vec::len)"
	}
	return access + ".len()"
}

func (w *writer) writeMatchBytes(
	structure *ir.Struct,
	field *ir.Field,
	access string,
	endian ir.Endian,
	indent string,
) error {
	match, ok := w.matchFor[field]
	if !ok {
		return errors.New("rust codegen: missing generated match type")
	}
	tag := selfExpression(field.Match.TagField)
	fmt.Fprintf(&w.sb, "%slet tag = %s;\n", indent, tag)
	fmt.Fprintf(&w.sb, "%s#[allow(\n", indent)
	fmt.Fprintf(&w.sb, "%s    clippy::match_same_arms,\n", indent)
	fmt.Fprintf(
		&w.sb,
		"%s    reason = \"schema cases can share serialization\"\n",
		indent,
	)
	fmt.Fprintf(&w.sb, "%s)]\n", indent)
	fmt.Fprintf(&w.sb, "%smatch (tag, %s) {\n", indent, access)
	for _, matchCase := range field.Match.Cases {
		variant, err := w.matchVariant(match, matchCase.Type)
		if err != nil {
			return err
		}
		fmt.Fprintf(
			&w.sb,
			"%s    (%s, %s::%s(payload)) => {\n",
			indent,
			rustInt(matchCase.Value),
			match.name,
			variant.name,
		)
		if err := w.writeTypeBytes(
			matchCase.Type,
			"",
			field.Name,
			"payload",
			endian,
			indent+"        ",
		); err != nil {
			return err
		}
		fmt.Fprintf(&w.sb, "%s    }\n", indent)
	}
	if len(match.variants) > 1 {
		for _, matchCase := range field.Match.Cases {
			fmt.Fprintf(&w.sb, "%s    (%s, _) => {\n", indent, rustInt(matchCase.Value))
			fmt.Fprintf(&w.sb, "%s        return Err(Error::MatchType {\n", indent)
			fmt.Fprintf(&w.sb, "%s            field: %s,\n", indent, rustString(field.Name))
			fmt.Fprintf(&w.sb, "%s            tag: i128::from(tag),\n", indent)
			fmt.Fprintf(&w.sb, "%s        });\n", indent)
			fmt.Fprintf(&w.sb, "%s    }\n", indent)
		}
	}
	if field.Match.Default != nil {
		variant, err := w.matchVariant(match, *field.Match.Default)
		if err != nil {
			return err
		}
		fmt.Fprintf(
			&w.sb,
			"%s    (_, %s::%s(payload)) => {\n",
			indent,
			match.name,
			variant.name,
		)
		if err := w.writeTypeBytes(
			*field.Match.Default,
			"",
			field.Name,
			"payload",
			endian,
			indent+"        ",
		); err != nil {
			return err
		}
		fmt.Fprintf(&w.sb, "%s    }\n", indent)
		if len(match.variants) > 1 {
			fmt.Fprintf(&w.sb, "%s    _ => {\n", indent)
			fmt.Fprintf(&w.sb, "%s        return Err(Error::MatchType {\n", indent)
			fmt.Fprintf(&w.sb, "%s            field: %s,\n", indent, rustString(field.Name))
			fmt.Fprintf(&w.sb, "%s            tag: i128::from(tag),\n", indent)
			fmt.Fprintf(&w.sb, "%s        });\n", indent)
			fmt.Fprintf(&w.sb, "%s    }\n", indent)
		}
	} else {
		fmt.Fprintf(&w.sb, "%s    _ => {\n", indent)
		fmt.Fprintf(&w.sb, "%s        return Err(Error::UnknownTag {\n", indent)
		fmt.Fprintf(&w.sb, "%s            struct_name: %s,\n", indent, rustString(structure.Name))
		fmt.Fprintf(&w.sb, "%s            field: %s,\n", indent, rustString(field.Name))
		fmt.Fprintf(&w.sb, "%s            value: i128::from(tag),\n", indent)
		fmt.Fprintf(&w.sb, "%s        });\n", indent)
		fmt.Fprintf(&w.sb, "%s    }\n", indent)
	}
	fmt.Fprintf(&w.sb, "%s}\n", indent)
	return nil
}

func autoComputeSources(structure *ir.Struct) (map[string]*ir.Field, map[string]*ir.Field) {
	lengths := make(map[string]*ir.Field)
	counts := make(map[string]*ir.Field)
	for _, field := range structure.Fields {
		switch field.Type.Array.Kind {
		case ir.LengthRef:
			lengths[field.Type.Array.LengthRef] = field
		case ir.CountRef:
			if !strings.Contains(field.Type.Array.CountRef, ".") {
				counts[field.Type.Array.CountRef] = field
			}
		}
	}
	return lengths, counts
}

func selfExpression(reference string) string {
	return "self." + referenceExpression(reference)
}
