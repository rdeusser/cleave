package rust

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rdeusser/cleave/internal/ir"
)

type parseContext struct {
	buf    string
	offset string
	parsed string
}

func newParseContext(structure *ir.Struct) parseContext {
	used := make(map[string]struct{}, len(structure.Fields)+3)
	for _, field := range structure.Fields {
		used[rustIdent(field.Name)] = struct{}{}
	}
	fresh := func(base string) string {
		name := base
		for suffix := 2; ; suffix++ {
			if _, exists := used[name]; !exists {
				used[name] = struct{}{}
				return name
			}
			name = fmt.Sprintf("%s_%d", base, suffix)
		}
	}
	return parseContext{
		buf:    fresh("buf"),
		offset: fresh("offset"),
		parsed: fresh("parsed"),
	}
}

func (w *writer) writeParse(structure *ir.Struct) error {
	context := newParseContext(structure)
	fmt.Fprintf(&w.sb, "impl %s {\n", rustIdent(structure.Name))
	w.sb.WriteString("    #[allow(non_snake_case, reason = \"schema field names are preserved\")]\n")
	w.sb.WriteString("    #[allow(clippy::too_many_lines, reason = \"schema methods follow field order\")]\n")
	if len(structure.Fields) == 0 {
		w.sb.WriteString("    pub fn parse(_buf: &[u8], _offset: &mut usize) -> ::std::result::Result<Self, Error> {\n")
	} else {
		fmt.Fprintf(
			&w.sb,
			"    pub fn parse(%s: &[u8], %s: &mut usize) -> ::std::result::Result<Self, Error> {\n",
			context.buf,
			context.offset,
		)
	}
	for _, field := range structure.Fields {
		if err := w.writeFieldParse(structure, field, context); err != nil {
			return err
		}
	}
	switch len(structure.Fields) {
	case 0:
		w.sb.WriteString("        Ok(Self {})\n")
	default:
		fields := make([]string, 0, len(structure.Fields))
		for _, field := range structure.Fields {
			fields = append(fields, rustIdent(field.Name))
		}
		body := strings.Join(fields, ", ")
		line := "        Ok(Self { " + body + " })"
		if len(body) <= 18 {
			w.sb.WriteString(line)
			w.sb.WriteString("\n")
			break
		}
		w.sb.WriteString("        Ok(Self {\n")
		for _, field := range fields {
			fmt.Fprintf(&w.sb, "            %s,\n", field)
		}
		w.sb.WriteString("        })\n")
	}
	w.sb.WriteString("    }\n")
	w.sb.WriteString("}\n")
	return nil
}

func (w *writer) writeFieldParse(
	structure *ir.Struct,
	field *ir.Field,
	context parseContext,
) error {
	if field.Condition == nil {
		expression, err := w.fieldParseExpression(structure, field, "        ", context)
		if err != nil {
			return fmt.Errorf("rust codegen: parsing %s.%s: %w", structure.Name, field.Name, err)
		}
		writeRustLet(&w.sb, "        ", rustIdent(field.Name), expression)
		return w.writeValidations(field, rustIdent(field.Name), "        ")
	}

	condition, err := emitRustExpr(field.Condition, "")
	if err != nil {
		return fmt.Errorf("rust codegen: condition on %s.%s: %w", structure.Name, field.Name, err)
	}
	if inner, ok := trimOuterParentheses(condition); ok {
		condition = inner
	}
	expression, err := w.fieldParseExpression(structure, field, "            ", context)
	if err != nil {
		return fmt.Errorf("rust codegen: parsing %s.%s: %w", structure.Name, field.Name, err)
	}
	fmt.Fprintf(&w.sb, "        let %s = if %s {\n", rustIdent(field.Name), condition)
	writeRustLet(&w.sb, "            ", context.parsed, expression)
	if err := w.writeValidations(field, context.parsed, "            "); err != nil {
		return err
	}
	fmt.Fprintf(
		&w.sb,
		"            ::std::option::Option::Some(%s)\n",
		context.parsed,
	)
	w.sb.WriteString("        } else {\n")
	w.sb.WriteString("            ::std::option::Option::None\n")
	w.sb.WriteString("        };\n")
	return nil
}

func writeRustLet(result *strings.Builder, indent, name, expression string) {
	line := fmt.Sprintf("%slet %s = %s;", indent, name, expression)
	if !strings.Contains(expression, "\n") && len(line) > 100 {
		fmt.Fprintf(result, "%slet %s =\n", indent, name)
		fmt.Fprintf(result, "%s    %s;\n", indent, expression)
		return
	}
	result.WriteString(line)
	result.WriteString("\n")
}

func (w *writer) writeValidations(field *ir.Field, thisName, indent string) error {
	for _, validation := range field.Validations {
		expression, err := emitRustExpr(validation.Expression, thisName)
		if err != nil {
			return fmt.Errorf(
				"rust codegen: validation %q on field %s: %w",
				validation.ID,
				field.Name,
				err,
			)
		}
		fmt.Fprintf(&w.sb, "%s#[allow(\n", indent)
		fmt.Fprintf(&w.sb, "%s    clippy::absurd_extreme_comparisons,\n", indent)
		fmt.Fprintf(&w.sb, "%s    clippy::float_cmp,\n", indent)
		fmt.Fprintf(&w.sb, "%s    clippy::manual_range_contains,\n", indent)
		fmt.Fprintf(&w.sb, "%s    clippy::nonminimal_bool,\n", indent)
		fmt.Fprintf(&w.sb, "%s    unused_comparisons,\n", indent)
		fmt.Fprintf(
			&w.sb,
			"%s    reason = \"validation expressions preserve schema comparisons\"\n",
			indent,
		)
		fmt.Fprintf(&w.sb, "%s)]\n", indent)
		if _, ok := trimOuterParentheses(expression); ok {
			fmt.Fprintf(&w.sb, "%sif !%s {\n", indent, expression)
		} else {
			fmt.Fprintf(&w.sb, "%sif !(%s) {\n", indent, expression)
		}
		fmt.Fprintf(&w.sb, "%s    return Err(Error::Validation {\n", indent)
		fmt.Fprintf(&w.sb, "%s        id: %s,\n", indent, rustString(validation.ID))
		fmt.Fprintf(&w.sb, "%s        message: %s,\n", indent, rustString(validation.Message))
		fmt.Fprintf(&w.sb, "%s        field: %s,\n", indent, rustString(field.Name))
		fmt.Fprintf(&w.sb, "%s    });\n", indent)
		fmt.Fprintf(&w.sb, "%s}\n", indent)
	}
	return nil
}

func trimOuterParentheses(expression string) (string, bool) {
	if len(expression) < 2 || expression[0] != '(' || expression[len(expression)-1] != ')' {
		return expression, false
	}

	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(expression); i++ {
		character := expression[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inString = false
			}
			continue
		}
		if character == '"' {
			inString = true
			continue
		}
		switch character {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(expression)-1 {
				return expression, false
			}
			if depth < 0 {
				return expression, false
			}
		}
	}
	if depth != 0 || inString {
		return expression, false
	}
	return expression[1 : len(expression)-1], true
}

func (w *writer) fieldParseExpression(
	structure *ir.Struct,
	field *ir.Field,
	indent string,
	context parseContext,
) (string, error) {
	endian := field.EffectiveEndian(structure.Options.Endian)
	if field.Type.Kind == ir.KindMatch {
		return w.matchParseExpression(structure, field, endian, indent, context)
	}
	return w.typeParseExpression(field.Type, field.Encoding, field.Name, endian, indent, context)
}

func (w *writer) typeParseExpression(
	fieldType ir.FieldType,
	encoding,
	fieldName string,
	endian ir.Endian,
	indent string,
	context parseContext,
) (string, error) {
	field := &ir.Field{
		Name:     fieldName,
		Type:     fieldType,
		Encoding: encoding,
	}
	if isTextField(field) {
		return w.textParseExpression(field, indent, context)
	}
	if isRawBytesField(field) {
		return w.rawBytesParseExpression(field, indent, context)
	}
	return w.arrayParseExpression(fieldType, endian, fieldName, indent, context)
}

func (w *writer) matchParseExpression(
	structure *ir.Struct,
	field *ir.Field,
	endian ir.Endian,
	indent string,
	context parseContext,
) (string, error) {
	match, ok := w.matchFor[field]
	if !ok {
		return "", errors.New("missing generated match type")
	}

	var result strings.Builder
	result.WriteString("{\n")
	fmt.Fprintf(&result, "%s    #[allow(\n", indent)
	fmt.Fprintf(&result, "%s        clippy::match_same_arms,\n", indent)
	fmt.Fprintf(
		&result,
		"%s        reason = \"schema cases can share payload parsing\"\n",
		indent,
	)
	fmt.Fprintf(&result, "%s    )]\n", indent)
	fmt.Fprintf(
		&result,
		"%s    match %s {\n",
		indent,
		referenceExpression(field.Match.TagField),
	)
	for _, matchCase := range field.Match.Cases {
		variant, err := w.matchVariant(match, matchCase.Type)
		if err != nil {
			return "", err
		}
		payload, err := w.typeParseExpression(
			matchCase.Type,
			"",
			field.Name,
			endian,
			indent+"        ",
			context,
		)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(
			&result,
			"%s        %s => %s::%s(%s),\n",
			indent,
			rustInt(matchCase.Value),
			match.name,
			variant.name,
			payload,
		)
	}
	if field.Match.Default != nil {
		variant, err := w.matchVariant(match, *field.Match.Default)
		if err != nil {
			return "", err
		}
		payload, err := w.typeParseExpression(
			*field.Match.Default,
			"",
			field.Name,
			endian,
			indent+"        ",
			context,
		)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(
			&result,
			"%s        _ => %s::%s(%s),\n",
			indent,
			match.name,
			variant.name,
			payload,
		)
	} else {
		fmt.Fprintf(&result, "%s        _ => {\n", indent)
		fmt.Fprintf(&result, "%s            return Err(Error::UnknownTag {\n", indent)
		fmt.Fprintf(
			&result,
			"%s                struct_name: %s,\n",
			indent,
			rustString(structure.Name),
		)
		fmt.Fprintf(
			&result,
			"%s                field: %s,\n",
			indent,
			rustString(field.Name),
		)
		fmt.Fprintf(
			&result,
			"%s                value: i128::from(%s),\n",
			indent,
			referenceExpression(field.Match.TagField),
		)
		fmt.Fprintf(&result, "%s            });\n", indent)
		fmt.Fprintf(&result, "%s        }\n", indent)
	}
	fmt.Fprintf(&result, "%s    }\n", indent)
	fmt.Fprintf(&result, "%s}", indent)
	return result.String(), nil
}

func (w *writer) arrayParseExpression(
	fieldType ir.FieldType,
	endian ir.Endian,
	fieldName string,
	indent string,
	context parseContext,
) (string, error) {
	array := fieldType.Array
	elementType := fieldType
	elementType.Array = ir.ArraySpec{}
	element, err := w.elementParseExpression(elementType, endian, context)
	if err != nil {
		return "", err
	}

	switch array.Kind {
	case ir.NotArray:
		return element, nil
	case ir.FixedSize:
		return fixedArrayParseExpression(element, fieldName, array.FixedSize, indent), nil
	case ir.LengthRef:
		return countedArrayParseExpression(
			element,
			fieldName,
			array.LengthRef,
			indent,
		), nil
	case ir.CountRef:
		return countedArrayParseExpression(
			element,
			fieldName,
			array.CountRef,
			indent,
		), nil
	case ir.RestArray:
		return restArrayParseExpression(
			element,
			fieldType.Kind == ir.KindStruct,
			fieldName,
			indent,
			context,
		), nil
	case ir.Terminator:
		if fieldType.Kind != ir.KindPrimitive || !fieldType.Primitive.IsInteger() {
			return "", errors.New("terminator arrays require an integer primitive")
		}
		return terminatedArrayParseExpression(element, array.Sentinel, indent, context), nil
	default:
		return "", fmt.Errorf("unsupported array kind %d", array.Kind)
	}
}

func (w *writer) elementParseExpression(
	fieldType ir.FieldType,
	endian ir.Endian,
	context parseContext,
) (string, error) {
	switch fieldType.Kind {
	case ir.KindPrimitive:
		return primitiveParseExpression(fieldType.Primitive, endian, context)
	case ir.KindEnum:
		enum := w.lookupEnum(fieldType.Ref)
		if enum == nil {
			return "", fmt.Errorf("unknown enum %q", fieldType.Ref)
		}
		raw, err := primitiveParseExpression(enum.BackingType, endian, context)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s::try_from(%s)?", rustIdent(fieldType.Ref), raw), nil
	case ir.KindStruct:
		return fmt.Sprintf(
			"%s::parse(%s, %s)?",
			rustIdent(fieldType.Ref),
			context.buf,
			context.offset,
		), nil
	default:
		return "", fmt.Errorf("unsupported element kind %d", fieldType.Kind)
	}
}

func primitiveParseExpression(
	primitive ir.PrimitiveType,
	endian ir.Endian,
	context parseContext,
) (string, error) {
	typ, err := rustPrimitive(primitive)
	if err != nil {
		return "", err
	}
	order := "le"
	if endian == ir.BigEndian {
		order = "be"
	}
	if primitive.Size() == 1 {
		order = "ne"
	}
	return fmt.Sprintf(
		"%s::from_%s_bytes(self::read_array::<%d>(%s, %s)?)",
		typ,
		order,
		primitive.Size(),
		context.buf,
		context.offset,
	), nil
}

func fixedArrayParseExpression(element, fieldName string, size int64, indent string) string {
	var result strings.Builder
	result.WriteString("{\n")
	fmt.Fprintf(&result, "%s    let mut values = ::std::vec::Vec::with_capacity(%s);\n", indent, rustInt(size))
	fmt.Fprintf(&result, "%s    for _ in 0..%s {\n", indent, rustInt(size))
	fmt.Fprintf(&result, "%s        values.push(%s);\n", indent, element)
	fmt.Fprintf(&result, "%s    }\n", indent)
	fmt.Fprintf(
		&result,
		"%s    <[_; %s]>::try_from(values).map_err(|values| Error::FixedSize {\n",
		indent,
		rustInt(size),
	)
	fmt.Fprintf(&result, "%s        field: %s,\n", indent, rustString(fieldName))
	fmt.Fprintf(&result, "%s        expected: %s,\n", indent, rustInt(size))
	fmt.Fprintf(&result, "%s        actual: values.len(),\n", indent)
	fmt.Fprintf(&result, "%s    })?\n", indent)
	fmt.Fprintf(&result, "%s}", indent)
	return result.String()
}

func countedArrayParseExpression(element, fieldName, reference, indent string) string {
	var result strings.Builder
	result.WriteString("{\n")
	writeCountConversion(&result, indent+"    ", fieldName, reference)
	fmt.Fprintf(&result, "%s    let mut values = ::std::vec::Vec::with_capacity(count);\n", indent)
	fmt.Fprintf(&result, "%s    for _ in 0..count {\n", indent)
	fmt.Fprintf(&result, "%s        values.push(%s);\n", indent, element)
	fmt.Fprintf(&result, "%s    }\n", indent)
	fmt.Fprintf(&result, "%s    values\n", indent)
	fmt.Fprintf(&result, "%s}", indent)
	return result.String()
}

func restArrayParseExpression(
	element string,
	checkProgress bool,
	fieldName,
	indent string,
	context parseContext,
) string {
	var result strings.Builder
	result.WriteString("{\n")
	fmt.Fprintf(&result, "%s    let mut values = ::std::vec::Vec::new();\n", indent)
	fmt.Fprintf(
		&result,
		"%s    while *%s < %s.len() {\n",
		indent,
		context.offset,
		context.buf,
	)
	if checkProgress {
		fmt.Fprintf(&result, "%s        let previous = *%s;\n", indent, context.offset)
	}
	fmt.Fprintf(&result, "%s        let value = %s;\n", indent, element)
	if checkProgress {
		fmt.Fprintf(&result, "%s        if *%s == previous {\n", indent, context.offset)
		fmt.Fprintf(&result, "%s            return Err(Error::NoProgress {\n", indent)
		fmt.Fprintf(&result, "%s                field: %s,\n", indent, rustString(fieldName))
		fmt.Fprintf(&result, "%s                offset: previous,\n", indent)
		fmt.Fprintf(&result, "%s            });\n", indent)
		fmt.Fprintf(&result, "%s        }\n", indent)
	}
	fmt.Fprintf(&result, "%s        values.push(value);\n", indent)
	fmt.Fprintf(&result, "%s    }\n", indent)
	fmt.Fprintf(&result, "%s    values\n", indent)
	fmt.Fprintf(&result, "%s}", indent)
	return result.String()
}

func terminatedArrayParseExpression(
	element string,
	sentinel int64,
	indent string,
	context parseContext,
) string {
	var result strings.Builder
	result.WriteString("{\n")
	fmt.Fprintf(&result, "%s    let mut values = ::std::vec::Vec::new();\n", indent)
	fmt.Fprintf(
		&result,
		"%s    while *%s < %s.len() {\n",
		indent,
		context.offset,
		context.buf,
	)
	fmt.Fprintf(&result, "%s        let value = %s;\n", indent, element)
	fmt.Fprintf(&result, "%s        if value == %s {\n", indent, rustInt(sentinel))
	fmt.Fprintf(&result, "%s            break;\n", indent)
	fmt.Fprintf(&result, "%s        }\n", indent)
	fmt.Fprintf(&result, "%s        values.push(value);\n", indent)
	fmt.Fprintf(&result, "%s    }\n", indent)
	fmt.Fprintf(&result, "%s    values\n", indent)
	fmt.Fprintf(&result, "%s}", indent)
	return result.String()
}

func (w *writer) rawBytesParseExpression(
	field *ir.Field,
	indent string,
	context parseContext,
) (string, error) {
	array := field.Type.Array
	switch array.Kind {
	case ir.NotArray:
		return "::std::vec::Vec::new()", nil
	case ir.FixedSize:
		var result strings.Builder
		result.WriteString("{\n")
		fmt.Fprintf(
			&result,
			"%s    let bytes = self::take(%s, %s, %s)?;\n",
			indent,
			context.buf,
			context.offset,
			rustInt(array.FixedSize),
		)
		fmt.Fprintf(
			&result,
			"%s    <[u8; %s]>::try_from(bytes).map_err(|_| Error::FixedSize {\n",
			indent,
			rustInt(array.FixedSize),
		)
		fmt.Fprintf(&result, "%s        field: %s,\n", indent, rustString(field.Name))
		fmt.Fprintf(&result, "%s        expected: %s,\n", indent, rustInt(array.FixedSize))
		fmt.Fprintf(&result, "%s        actual: bytes.len(),\n", indent)
		fmt.Fprintf(&result, "%s    })?\n", indent)
		fmt.Fprintf(&result, "%s}", indent)
		return result.String(), nil
	case ir.LengthRef:
		return countedBytesParseExpression(field.Name, array.LengthRef, indent, context), nil
	case ir.CountRef:
		return countedBytesParseExpression(field.Name, array.CountRef, indent, context), nil
	case ir.RestArray:
		return restBytesParseExpression(indent, context), nil
	case ir.Terminator:
		return terminatedBytesParseExpression(field.Name, array.Sentinel, indent, context), nil
	default:
		return "", fmt.Errorf("unsupported raw byte array kind %d", array.Kind)
	}
}

func countedBytesParseExpression(
	fieldName,
	reference,
	indent string,
	context parseContext,
) string {
	var result strings.Builder
	result.WriteString("{\n")
	writeCountConversion(&result, indent+"    ", fieldName, reference)
	fmt.Fprintf(
		&result,
		"%s    self::take(%s, %s, count)?.to_vec()\n",
		indent,
		context.buf,
		context.offset,
	)
	fmt.Fprintf(&result, "%s}", indent)
	return result.String()
}

func restBytesParseExpression(indent string, context parseContext) string {
	var result strings.Builder
	result.WriteString("{\n")
	fmt.Fprintf(
		&result,
		"%s    let remaining = %s.len().saturating_sub(*%s);\n",
		indent,
		context.buf,
		context.offset,
	)
	fmt.Fprintf(
		&result,
		"%s    self::take(%s, %s, remaining)?.to_vec()\n",
		indent,
		context.buf,
		context.offset,
	)
	fmt.Fprintf(&result, "%s}", indent)
	return result.String()
}

func terminatedBytesParseExpression(
	fieldName string,
	sentinel int64,
	indent string,
	context parseContext,
) string {
	var result strings.Builder
	result.WriteString("{\n")
	fmt.Fprintf(&result, "%s    let start = *%s;\n", indent, context.offset)
	fmt.Fprintf(&result, "%s    let mut end = %s.len();\n", indent, context.buf)
	fmt.Fprintf(
		&result,
		"%s    while let Some(byte) = %s.get(*%s) {\n",
		indent,
		context.buf,
		context.offset,
	)
	fmt.Fprintf(&result, "%s        if *byte == %s {\n", indent, rustInt(sentinel))
	fmt.Fprintf(&result, "%s            end = *%s;\n", indent, context.offset)
	fmt.Fprintf(&result, "%s            *%s += 1;\n", indent, context.offset)
	fmt.Fprintf(&result, "%s            break;\n", indent)
	fmt.Fprintf(&result, "%s        }\n", indent)
	fmt.Fprintf(&result, "%s        *%s += 1;\n", indent, context.offset)
	fmt.Fprintf(&result, "%s    }\n", indent)
	fmt.Fprintf(&result, "%s    %s.get(start..end)\n", indent, context.buf)
	fmt.Fprintf(&result, "%s        .ok_or(Error::UnexpectedEof {\n", indent)
	fmt.Fprintf(&result, "%s            offset: start,\n", indent)
	fmt.Fprintf(&result, "%s            needed: end.saturating_sub(start),\n", indent)
	fmt.Fprintf(
		&result,
		"%s            remaining: %s.len().saturating_sub(start),\n",
		indent,
		context.buf,
	)
	fmt.Fprintf(&result, "%s        })?\n", indent)
	fmt.Fprintf(&result, "%s        .to_vec()\n", indent)
	fmt.Fprintf(&result, "%s}", indent)
	_ = fieldName
	return result.String()
}

func (w *writer) textParseExpression(
	field *ir.Field,
	indent string,
	context parseContext,
) (string, error) {
	encoding := field.Encoding
	if encoding == "" {
		encoding = "utf-8"
	}
	array := field.Type.Array
	switch array.Kind {
	case ir.NotArray:
		return fmt.Sprintf("self::decode_text(&[], %s)?", rustString(encoding)), nil
	case ir.FixedSize:
		return fmt.Sprintf(
			"self::decode_text(self::take(%s, %s, %s)?, %s)?",
			context.buf,
			context.offset,
			rustInt(array.FixedSize),
			rustString(encoding),
		), nil
	case ir.LengthRef:
		return decodedCountedBytesParseExpression(
			field.Name,
			array.LengthRef,
			encoding,
			indent,
			context,
		), nil
	case ir.CountRef:
		return decodedCountedBytesParseExpression(
			field.Name,
			array.CountRef,
			encoding,
			indent,
			context,
		), nil
	case ir.RestArray:
		var result strings.Builder
		result.WriteString("{\n")
		fmt.Fprintf(
			&result,
			"%s    let remaining = %s.len().saturating_sub(*%s);\n",
			indent,
			context.buf,
			context.offset,
		)
		fmt.Fprintf(
			&result,
			"%s    let bytes = self::take(%s, %s, remaining)?;\n",
			indent,
			context.buf,
			context.offset,
		)
		fmt.Fprintf(
			&result,
			"%s    self::decode_text(bytes, %s)?\n",
			indent,
			rustString(encoding),
		)
		fmt.Fprintf(&result, "%s}", indent)
		return result.String(), nil
	case ir.Terminator:
		raw := terminatedBytesParseExpression(
			field.Name,
			array.Sentinel,
			indent+"    ",
			context,
		)
		var result strings.Builder
		result.WriteString("{\n")
		fmt.Fprintf(&result, "%s    let bytes = %s;\n", indent, raw)
		fmt.Fprintf(
			&result,
			"%s    self::decode_text(&bytes, %s)?\n",
			indent,
			rustString(encoding),
		)
		fmt.Fprintf(&result, "%s}", indent)
		return result.String(), nil
	case ir.FixedTerminator:
		var result strings.Builder
		result.WriteString("{\n")
		fmt.Fprintf(
			&result,
			"%s    let bytes = self::take(%s, %s, %s)?;\n",
			indent,
			context.buf,
			context.offset,
			rustInt(array.FixedSize),
		)
		fmt.Fprintf(&result, "%s    let end = bytes\n", indent)
		fmt.Fprintf(&result, "%s        .iter()\n", indent)
		fmt.Fprintf(
			&result,
			"%s        .position(|byte| *byte == %s)\n",
			indent,
			rustInt(array.Sentinel),
		)
		fmt.Fprintf(&result, "%s        .unwrap_or(bytes.len());\n", indent)
		fmt.Fprintf(&result, "%s    let text = bytes.get(..end).ok_or(Error::UnexpectedEof {\n", indent)
		fmt.Fprintf(&result, "%s        offset: *%s,\n", indent, context.offset)
		fmt.Fprintf(&result, "%s        needed: end,\n", indent)
		fmt.Fprintf(&result, "%s        remaining: bytes.len(),\n", indent)
		fmt.Fprintf(&result, "%s    })?;\n", indent)
		fmt.Fprintf(
			&result,
			"%s    self::decode_text(text, %s)?\n",
			indent,
			rustString(encoding),
		)
		fmt.Fprintf(&result, "%s}", indent)
		return result.String(), nil
	default:
		return "", fmt.Errorf("unsupported text array kind %d", array.Kind)
	}
}

func decodedCountedBytesParseExpression(
	fieldName,
	reference,
	encoding,
	indent string,
	context parseContext,
) string {
	var result strings.Builder
	result.WriteString("{\n")
	writeCountConversion(&result, indent+"    ", fieldName, reference)
	fmt.Fprintf(
		&result,
		"%s    let bytes = self::take(%s, %s, count)?;\n",
		indent,
		context.buf,
		context.offset,
	)
	fmt.Fprintf(
		&result,
		"%s    self::decode_text(bytes, %s)?\n",
		indent,
		rustString(encoding),
	)
	fmt.Fprintf(&result, "%s}", indent)
	return result.String()
}

func writeCountConversion(result *strings.Builder, indent, fieldName, reference string) {
	fmt.Fprintf(result, "%slet raw_count = %s;\n", indent, referenceExpression(reference))
	fmt.Fprintf(result, "%s#[allow(\n", indent)
	fmt.Fprintf(result, "%s    clippy::unnecessary_fallible_conversions,\n", indent)
	fmt.Fprintf(
		result,
		"%s    reason = \"length fields can use signed or wide integer types\"\n",
		indent,
	)
	fmt.Fprintf(result, "%s)]\n", indent)
	fmt.Fprintf(
		result,
		"%slet count = usize::try_from(raw_count).map_err(|_| Error::InvalidLength {\n",
		indent,
	)
	fmt.Fprintf(result, "%s    field: %s,\n", indent, rustString(fieldName))
	fmt.Fprintf(result, "%s    value: i128::from(raw_count),\n", indent)
	fmt.Fprintf(result, "%s})?;\n", indent)
}

func referenceExpression(reference string) string {
	parts := strings.Split(reference, ".")
	for i := range parts {
		parts[i] = rustIdent(parts[i])
	}
	return strings.Join(parts, ".")
}

func isTextField(field *ir.Field) bool {
	return field.Type.Kind == ir.KindPrimitive &&
		field.Type.Primitive == ir.String &&
		(field.Encoding != "" || field.Type.Array.Kind == ir.FixedTerminator)
}

func isRawBytesField(field *ir.Field) bool {
	if field.Type.Kind != ir.KindPrimitive {
		return false
	}
	if field.Type.Primitive == ir.Bytes {
		return true
	}
	return field.Type.Primitive == ir.String && !isTextField(field)
}

func (w *writer) lookupEnum(name string) *ir.Enum {
	for _, enum := range w.pkg.Enums {
		if enum.Name == name {
			return enum
		}
	}
	return nil
}
