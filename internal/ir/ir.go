package ir

var primitiveNames = [...]string{
	U8:     "u8",
	U16:    "u16",
	U32:    "u32",
	U64:    "u64",
	I8:     "i8",
	I16:    "i16",
	I32:    "i32",
	I64:    "i64",
	F32:    "f32",
	F64:    "f64",
	Bytes:  "bytes",
	String: "string",
}

var primitiveByName = map[string]PrimitiveType{
	"u8": U8, "u16": U16, "u32": U32, "u64": U64,
	"i8": I8, "i16": I16, "i32": I32, "i64": I64,
	"f32": F32, "f64": F64,
	"bytes": Bytes, "string": String,
}

type Package struct {
	Name    string
	Format  *Format
	Enums   []*Enum
	Unions  []*Union
	Structs []*Struct
}

type Format struct {
	Name      string
	Title     string
	Extension string
	Endian    Endian
	Root      string
}

type Struct struct {
	Name    string
	Fields  []*Field
	Options StructOptions
}

type StructOptions struct {
	Endian Endian
}

type Field struct {
	Name        string
	Type        FieldType
	Match       *MatchSpec
	Options     FieldOptions
	Condition   *ExprNode        // [if = "..."] — conditional parsing
	Validations []ValidationRule // [(builtin).cel = { ... }] — validation rules
	Encoding    string
	CrossRef    string // [ref = Struct.field] — metadata-only cross-format reference
}

func (f *Field) EffectiveEndian(structEndian Endian) Endian {
	if f.Options.Endian != nil {
		return *f.Options.Endian
	}
	return structEndian
}

type ValidationRule struct {
	ID         string
	Message    string
	Expression *ExprNode
}

type FieldOptions struct {
	Endian *Endian
}

type Union struct {
	Name        string
	BackingType PrimitiveType
	Cases       []MatchCase
	Default     *FieldType
}

type MatchSpec struct {
	TagField string
	Cases    []MatchCase // valued cases only, never default
	Default  *FieldType  // nil if no _ case
}

type MatchCase struct {
	Value int64
	Type  FieldType
}

type FieldType struct {
	Kind      TypeKind
	Primitive PrimitiveType
	Ref       string // enum or struct name
	Array     ArraySpec
}

type TypeKind int

const (
	KindPrimitive TypeKind = iota
	KindEnum
	KindStruct
	KindMatch
)

type ArraySpec struct {
	Kind      ArrayKind
	FixedSize int64
	LengthRef string // field name for length-referenced arrays
	CountRef  string // dotted field path for count-referenced arrays
	Sentinel  int64  // terminator byte for sentinel-terminator arrays
}

type ArrayKind int

const (
	NotArray        ArrayKind = iota
	FixedSize                 // bytes[8]
	LengthRef                 // bytes[length]
	RestArray                 // [rest = true]
	Terminator                // [terminator = 0x00]
	CountRef                  // [count = header.record_count]
	FixedTerminator           // string[64, terminator = 0x00]
)

type Enum struct {
	Name        string
	BackingType PrimitiveType
	Variants    []EnumVariant
}

type EnumVariant struct {
	Name  string
	Value int64
}

type PrimitiveType int

const (
	U8 PrimitiveType = iota
	U16
	U32
	U64
	I8
	I16
	I32
	I64
	F32
	F64
	Bytes
	String
)

func LookupPrimitive(name string) (PrimitiveType, bool) {
	p, ok := primitiveByName[name]
	return p, ok
}

func (p PrimitiveType) String() string { return primitiveNames[p] }

func (p PrimitiveType) Size() int {
	switch p {
	case U8, I8:
		return 1
	case U16, I16:
		return 2
	case U32, I32, F32:
		return 4
	case U64, I64, F64:
		return 8
	default:
		return 0
	}
}

func (p PrimitiveType) IsInteger() bool {
	switch p {
	case U8, U16, U32, U64, I8, I16, I32, I64:
		return true
	default:
		return false
	}
}

type Endian int

const (
	LittleEndian Endian = iota
	BigEndian
)

func (e Endian) String() string {
	if e == BigEndian {
		return "big"
	}
	return "little"
}
