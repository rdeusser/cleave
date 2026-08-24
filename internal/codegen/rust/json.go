package rust

import (
	"fmt"
	"strings"

	"github.com/rdeusser/cleave/internal/ir"
)

func (w *writer) writeJSON(structure *ir.Struct) error {
	fmt.Fprintf(&w.sb, "impl %s {\n", rustIdent(structure.Name))
	refs := crossRefs(structure)
	if len(refs) > 0 {
		entries := make([]string, 0, len(refs))
		for _, field := range refs {
			entries = append(entries, fmt.Sprintf("(%s, %s)", rustString(field.Name), rustString(field.CrossRef)))
		}
		line := "    pub const REFS: &'static [(&'static str, &'static str)] = &[" +
			strings.Join(entries, ", ") + "];"
		if len(line) <= 100 {
			w.sb.WriteString(line)
			w.sb.WriteString("\n\n")
		} else {
			w.sb.WriteString("    pub const REFS: &'static [(&'static str, &'static str)] = &[\n")
			for _, entry := range entries {
				fmt.Fprintf(&w.sb, "        %s,\n", entry)
			}
			w.sb.WriteString("    ];\n\n")
		}
	}

	w.sb.WriteString("    #[allow(non_snake_case, reason = \"schema field names are preserved\")]\n")
	w.sb.WriteString("    #[allow(clippy::too_many_lines, reason = \"schema methods follow field order\")]\n")
	w.sb.WriteString("    pub fn to_json(&self) -> ::std::string::String {\n")
	w.sb.WriteString("        let mut json = ::std::string::String::from(\"{\");\n")
	for i, field := range structure.Fields {
		if i > 0 {
			w.sb.WriteString("        json.push(',');\n")
		}
		fmt.Fprintf(
			&w.sb,
			"        write_json_string(&mut json, %s);\n",
			rustString(field.Name),
		)
		w.sb.WriteString("        json.push(':');\n")
		if field.Condition != nil {
			fmt.Fprintf(
				&w.sb,
				"        if let ::std::option::Option::Some(value) = &self.%s {\n",
				rustIdent(field.Name),
			)
			if err := w.writeJSONField(field, "value", "            "); err != nil {
				return err
			}
			w.sb.WriteString("        } else {\n")
			w.sb.WriteString("            json.push_str(\"null\");\n")
			w.sb.WriteString("        }\n")
			continue
		}
		fmt.Fprintf(&w.sb, "        let value = &self.%s;\n", rustIdent(field.Name))
		if err := w.writeJSONField(field, "value", "        "); err != nil {
			return err
		}
	}
	w.sb.WriteString("        json.push('}');\n")
	w.sb.WriteString("        json\n")
	w.sb.WriteString("    }\n")
	w.sb.WriteString("}\n")
	return nil
}

func (w *writer) writeJSONField(field *ir.Field, access, indent string) error {
	if field.Type.Kind == ir.KindMatch {
		return w.writeMatchJSON(field, access, indent)
	}
	return w.writeJSONValue(field.Type, field.Encoding, access, indent)
}

func (w *writer) writeJSONValue(fieldType ir.FieldType, encoding, access, indent string) error {
	field := &ir.Field{Type: fieldType, Encoding: encoding}
	if isTextField(field) {
		fmt.Fprintf(&w.sb, "%swrite_json_string(&mut json, %s);\n", indent, access)
		return nil
	}
	if isRawBytesField(field) {
		return w.writeJSONSequence(
			ir.FieldType{Kind: ir.KindPrimitive, Primitive: ir.U8},
			access,
			indent,
		)
	}

	if fieldType.Array.Kind != ir.NotArray {
		elementType := fieldType
		elementType.Array = ir.ArraySpec{}
		return w.writeJSONSequence(elementType, access, indent)
	}
	return w.writeJSONElement(fieldType, access, indent)
}

func (w *writer) writeJSONSequence(fieldType ir.FieldType, access, indent string) error {
	fmt.Fprintf(&w.sb, "%sjson.push('[');\n", indent)
	fmt.Fprintf(
		&w.sb,
		"%sfor (index, item) in %s.iter().enumerate() {\n",
		indent,
		access,
	)
	fmt.Fprintf(&w.sb, "%s    if index > 0 {\n", indent)
	fmt.Fprintf(&w.sb, "%s        json.push(',');\n", indent)
	fmt.Fprintf(&w.sb, "%s    }\n", indent)
	fmt.Fprintf(&w.sb, "%s    let value = item;\n", indent)
	if err := w.writeJSONElement(fieldType, "value", indent+"    "); err != nil {
		return err
	}
	fmt.Fprintf(&w.sb, "%s}\n", indent)
	fmt.Fprintf(&w.sb, "%sjson.push(']');\n", indent)
	return nil
}

func (w *writer) writeJSONElement(fieldType ir.FieldType, access, indent string) error {
	switch fieldType.Kind {
	case ir.KindPrimitive:
		switch fieldType.Primitive {
		case ir.F32, ir.F64:
			fmt.Fprintf(&w.sb, "%sif %s.is_finite() {\n", indent, access)
			fmt.Fprintf(
				&w.sb,
				"%s    json.push_str(&%s.to_string());\n",
				indent,
				access,
			)
			fmt.Fprintf(&w.sb, "%s} else {\n", indent)
			fmt.Fprintf(&w.sb, "%s    json.push_str(\"null\");\n", indent)
			fmt.Fprintf(&w.sb, "%s}\n", indent)
		case ir.Bytes, ir.String:
			return fmt.Errorf("rust codegen: raw text must be emitted as a JSON sequence")
		default:
			fmt.Fprintf(&w.sb, "%sjson.push_str(&%s.to_string());\n", indent, access)
		}
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
		fmt.Fprintf(&w.sb, "%sjson.push_str(&raw.to_string());\n", indent)
	case ir.KindStruct:
		fmt.Fprintf(&w.sb, "%sjson.push_str(&%s.to_json());\n", indent, access)
	default:
		return fmt.Errorf("rust codegen: unsupported JSON element kind %d", fieldType.Kind)
	}
	return nil
}

func (w *writer) writeMatchJSON(field *ir.Field, access, indent string) error {
	match, ok := w.matchFor[field]
	if !ok {
		return fmt.Errorf("rust codegen: missing generated match type for %s", field.Name)
	}
	fmt.Fprintf(&w.sb, "%smatch %s {\n", indent, access)
	for _, variant := range match.variants {
		fmt.Fprintf(
			&w.sb,
			"%s    %s::%s(payload) => {\n",
			indent,
			match.name,
			variant.name,
		)
		if err := w.writeJSONValue(variant.fieldType, "", "payload", indent+"        "); err != nil {
			return err
		}
		fmt.Fprintf(&w.sb, "%s    }\n", indent)
	}
	fmt.Fprintf(&w.sb, "%s}\n", indent)
	return nil
}

func crossRefs(structure *ir.Struct) []*ir.Field {
	refs := make([]*ir.Field, 0)
	for _, field := range structure.Fields {
		if field.CrossRef != "" {
			refs = append(refs, field)
		}
	}
	return refs
}
