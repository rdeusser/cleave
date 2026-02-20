# Cleave Language Specification

Cleave is a declarative language for describing binary data formats. A `.clv` file specifies the layout of binary structures — field names, types, byte order, array lengths — and a cleave compiler generates parser code that can read raw bytes into structured objects.

This document specifies every syntactic and semantic rule in the language. It is intended to be sufficient for implementing a cleave parser generator targeting any language.

## 1. Lexical Structure

### 1.1 Encoding

Source files are UTF-8 encoded.

### 1.2 Whitespace

Spaces, tabs, carriage returns, and newlines are whitespace. Whitespace separates tokens but is otherwise insignificant.

### 1.3 Comments

Line comments start with `//` and extend to the end of the line:

```clv
// This is a line comment
```

Block comments start with `/*` and end with `*/`:

```clv
/* This is a block comment */
```

Block comments do not nest. An unterminator block comment is an error.

### 1.4 Identifiers

An identifier starts with a letter (`a`–`z`, `A`–`Z`) or underscore (`_`), followed by zero or more letters, digits, or underscores.

```
Ident = [a-zA-Z_][a-zA-Z0-9_]*
```

The identifier `_` (a single underscore) is valid and has special meaning in union/match cases where it denotes the default case.

### 1.5 Keywords

The following identifiers are reserved keywords and cannot be used as names:

```
package   struct   enum   format   union   match   option   true   false
```

### 1.6 Integer Literals

Integer literals support three bases:

| Format | Example | Description |
|--------|---------|-------------|
| Decimal | `42`, `0` | One or more decimal digits |
| Hexadecimal | `0xFF`, `0x0A` | `0x` or `0X` prefix followed by one or more hex digits |
| Binary | `0b1010`, `0B110` | `0b` or `0B` prefix followed by one or more binary digits |

A hex or binary prefix with no digits following it is an error (`0x` alone is illegal).

Negative integers are not lexical tokens. Negation is handled syntactically: a `-` token followed by an integer literal. This occurs in enum variant values, union/match case values, and array dimension expressions.

### 1.7 String Literals

String literals are enclosed in double quotes. Backslash escapes are supported (`\"` to include a literal quote). Strings cannot span multiple lines — a newline before the closing quote is an error.

```clv
"hello"
"with \"escape\""
""
```

### 1.8 Punctuation

| Token | Symbol |
|-------|--------|
| Semicolon | `;` |
| Colon | `:` |
| Comma | `,` |
| Assign | `=` |
| Fat arrow | `=>` |
| Minus | `-` |
| Left brace | `{` |
| Right brace | `}` |
| Left bracket | `[` |
| Right bracket | `]` |
| Dot | `.` |
| Left paren | `(` |
| Right paren | `)` |

Note: `=` followed immediately by `>` is lexed as the single token `=>`, not two separate tokens.

### 1.9 Token Precedence

When scanning, the lexer applies these rules in order:

1. Skip whitespace
2. `//` starts a line comment
3. `/*` starts a block comment
4. `"` starts a string literal
5. Digit starts a number literal
6. Letter or `_` starts an identifier (resolved to keyword if applicable)
7. Everything else is punctuation or illegal

## 2. Grammar

The grammar is designed for recursive descent parsing with single-token lookahead. One disambiguation rule requires two-token lookahead (see section 2.7).

```ebnf
File         = PackageDecl { ImportDecl } { TopLevelDecl } .
PackageDecl  = "package" IDENT ";" .
ImportDecl   = "import" STRING ";" .
TopLevelDecl = StructDecl | EnumDecl | FormatDecl | UnionDecl .
```

### 2.1 Enum Declarations

An enum maps symbolic names to integer values with an explicit backing type.

```ebnf
EnumDecl    = "enum" IDENT ":" TypeRef "{" { EnumVariant } "}" .
EnumVariant = IDENT "=" Expression ";" .
```

The backing type must be an integer primitive type (see section 3.1). Every variant must have an explicit integer value.

Example:

```clv
enum ColorType : u8 {
    Grayscale = 0;
    RGB = 2;
    Palette = 3;
}
```

### 2.2 Struct Declarations

A struct describes a sequence of fields read in order from a byte buffer.

```ebnf
StructDecl   = "struct" IDENT "{" { FieldDecl | OptionBlock } "}" .
FieldDecl    = IDENT ( TypeExpr [ FieldOptions ] | MatchExpr ) ";" .
TypeExpr     = TypeRef [ "[" Expression "]" ] .
TypeRef      = IDENT .
FieldOptions = "[" FieldOpt { "," FieldOpt } "]" .
FieldOpt     = FieldOptKey "=" Value .
FieldOptKey  = IDENT | "(" IDENT ")" "." IDENT .
```

A field declaration has its **name first, then its type** (Go-style ordering, not C-style). Either a type expression or a match expression follows the name, never both. Field options (in square brackets) only follow type expressions, never match expressions.

Example:

```clv
struct Chunk {
    length  u32;
    type    bytes[4];
    data    bytes[length];
    crc     u32;

    option (builtin) = {
        endian = big;
    };
}
```

### 2.3 Format Declarations

A format block provides file-level metadata. At most one format block is allowed per file.

```ebnf
FormatDecl = "format" IDENT "{" { FormatKV } "}" .
FormatKV   = IDENT "=" Value ";" .
```

Recognized keys:

| Key | Value type | Description |
|-----|-----------|-------------|
| `root` | Identifier | The struct that represents the top-level file structure. Required. |
| `title` | String | Human-readable format name |
| `extension` | String | File extension (e.g., `"png"`) |
| `endian` | `big` or `little` | Default byte order for all structs in the file |

Example:

```clv
format PNG {
    root = PngFile;
    title = "Portable Network Graphics";
    extension = "png";
    endian = big;
}
```

When a format block sets `endian`, that becomes the default for all structs unless overridden at the struct or field level.

### 2.4 Union Declarations

A union is a named, reusable discriminated mapping from integer tag values to types.

```ebnf
UnionDecl = "union" IDENT ":" TypeRef "{" UnionCase { UnionCase } "}" .
UnionCase = CaseValue "=>" TypeRef ";" .
CaseValue = INTEGER | "-" INTEGER | "_" .
```

The backing type (after `:`) must be an integer primitive and declares what type the tag field has on the wire. A union body must contain at least one case.

`_` is the default/wildcard case. At most one default case is allowed. Integer case values may be negative (using the `-` prefix). All case values must fit within the range of the backing type.

Example:

```clv
union ParamValue : u8 {
    0 => u32;
    1 => f32;
    _ => bytes[4];
}
```

A union is referenced from a struct field using the union name as the type and a `[tag = field]` option (see section 4.3).

### 2.5 Match Expressions (Inline Unions)

A match expression is an inline discriminated union defined directly on a field. It has the same case syntax as a union but is not reusable.

```ebnf
MatchExpr = "match" IDENT "{" UnionCase { UnionCase } "}" .
```

The identifier after `match` names the tag field (which must be a preceding integer field in the same struct). A match body must contain at least one case.

Example:

```clv
struct Event {
    type  u8;
    value match type {
        0 => u32;
        1 => f32;
        _ => bytes[4];
    };
}
```

### 2.6 Option Blocks

Option blocks provide struct-level configuration. They appear inside struct bodies.

```ebnf
OptionBlock = "option" "(" IDENT ")" "=" "{" { OptionKV } "}" ";" .
OptionKV    = IDENT "=" Value ";" .
```

The identifier in parentheses is the option namespace. Currently, the only recognized namespace is `builtin`.

Recognized `builtin` options:

| Key | Value | Description |
|-----|-------|-------------|
| `endian` | `big` or `little` | Byte order for all multi-byte fields in this struct |

Example:

```clv
option (builtin) = {
    endian = big;
};
```

### 2.7 Disambiguation: Array Dimension vs Field Options

After a type name, `[` can introduce either an array dimension (`bytes[8]`) or field options (`[rest = true]`). The parser disambiguates by peeking tokens after `[`:

- If the next token is `(`, it is a namespaced field option (e.g., `[(builtin).cel = ...]`)
- If the pattern is `IDENT =`, it is a simple field option list
- Otherwise, it is an array dimension

This requires up to two-token lookahead.

### 2.8 Expressions and Values

```ebnf
Expression = INTEGER | IDENT .
Value      = IDENT | DottedIdent | INTEGER | STRING | "true" | "false" | BlockExpr .
DottedIdent = IDENT { "." IDENT } .
BlockExpr  = "{" { BlockEntry } "}" .
BlockEntry = IDENT ":" Value .
```

Expressions appear in array dimensions and enum variant values. Values appear in option blocks, field options, and format entries — they are a superset of expressions that also allow strings, booleans, dotted identifiers, and block expressions. Block expressions use textproto-style `key: value` syntax (colons, no semicolons).

## 3. Type System

### 3.1 Primitive Types

| Type | Size | Description |
|------|------|-------------|
| `u8` | 1 byte | Unsigned 8-bit integer |
| `u16` | 2 bytes | Unsigned 16-bit integer |
| `u32` | 4 bytes | Unsigned 32-bit integer |
| `u64` | 8 bytes | Unsigned 64-bit integer |
| `i8` | 1 byte | Signed 8-bit integer |
| `i16` | 2 bytes | Signed 16-bit integer |
| `i32` | 4 bytes | Signed 32-bit integer |
| `i64` | 8 bytes | Signed 64-bit integer |
| `f32` | 4 bytes | IEEE 754 single-precision float |
| `f64` | 8 bytes | IEEE 754 double-precision float |
| `bytes` | variable | Raw byte sequence |
| `string` | variable | Byte sequence (semantically text) |

`bytes` and `string` are variable-length types. They must be used with an array specification (fixed size, length reference, terminator, or rest) to have a defined length at parse time. A bare `bytes` or `string` with no length specification is valid syntax but results in an empty read.

### 3.2 Named Types

A field's type can reference a previously declared enum, struct, or union by name. Forward references are supported — types can be referenced before their declaration in the file.

### 3.3 Array Specifications

Array behavior is specified either inline in the type expression or via field options:

| Notation | Source | Description |
|----------|--------|-------------|
| `type[N]` | Inline | Fixed-size: read exactly N elements |
| `type[field]` | Inline | Length-referenced: read `field` elements, where `field` is a preceding integer field |
| `[rest = true]` | Field option | Rest array: read elements until the end of the buffer |
| `[terminator = N]` | Field option | Sentinel-terminator: read until byte value N is encountered |
| `[count = ref]` | Field option | Count-referenced: read `ref` elements, where `ref` is a dotted field path |

For `bytes` and `string`, fixed-size and length-referenced arrays read that many raw bytes. For other primitives and struct types, they read that many instances of the element type.

### 3.4 Type Kinds

After semantic analysis, every field resolves to one of four kinds:

| Kind | Description |
|------|-------------|
| Primitive | A built-in type from section 3.1 |
| Enum | A reference to a declared enum |
| Struct | A reference to a declared struct |
| Match | A discriminated union (from inline match or union reference) |

## 4. Semantic Rules

### 4.1 Package Declaration

Every file must start with exactly one package declaration. The package name is used as the output filename stem.

### 4.2 Type Name Uniqueness

All type names (enums, unions, structs) share a single namespace. Duplicate names are an error.

### 4.3 Field Options

Field options are key-value pairs in square brackets after the type expression.

| Option | Value type | Description |
|--------|-----------|-------------|
| `rest` | `true` | Read remaining buffer as array of this type |
| `terminator` | Integer | Read until this byte value is encountered (sentinel is consumed but not included) |
| `count` | Identifier or dotted identifier | Read this many elements, referencing a field (possibly in a nested struct via dotted path like `header.n`) |
| `if` | String (CEL expression) | Conditional parsing — field is only parsed when expression is true. Cannot reference `this`. |
| `endian` | `big` or `little` | Override byte order for this specific field |
| `encoding` | String | Character encoding for string fields (e.g., `"CP949"`, `"SJIS"`) |
| `tag` | Identifier | When the type is a union, specifies which preceding field holds the discriminant |
| `ref` | Dotted identifier (`Struct.field`) | Metadata-only cross-format reference. Declares that this field's value indexes into the named struct's field. Both the struct and field must exist (resolved via imports). Does not affect parsing or serialization. |
| `(builtin).cel` | Block `{ id: message: expression: }` | Structured CEL validation rule (see section 5.7) |

The `tag` option is required when a field's type references a union declaration. It names the preceding field whose value determines which union case to use.

#### Namespaced Field Options

Field options can have namespaced keys using the syntax `(namespace).key`. Currently, the only recognized namespace is `builtin`. Namespaced options use block values with textproto-style `key: value` syntax (colons, no semicolons).

```clv
version u16 [
    (builtin).cel = {
        id: "version.min"
        message: "version must be at least 1"
        expression: "this >= 1"
    }
];
```

Multiple namespaced options are comma-separated:

```clv
hostname string[256, terminator = 0x00] [
    (builtin).cel = {
        id: "hostname.valid"
        message: "hostname must be valid"
        expression: "size(this) > 0"
    },
    (builtin).cel = {
        id: "hostname.notlocalhost"
        message: "localhost is not permitted"
        expression: "this != 'localhost'"
    }
];
```

### 4.4 Endianness Resolution

Byte order for multi-byte fields is resolved with this precedence (highest first):

1. Field-level `[endian = ...]` option
2. Struct-level `option (builtin) = { endian = ...; };`
3. Format-level `endian = ...` in the format block
4. Default: little-endian

Single-byte fields are unaffected by endianness.

### 4.5 Enum Rules

- Backing type must be an integer primitive
- Variant names must be unique within the enum
- Variant values must be integer literals

### 4.6 Struct Field Rules

- Field names must be unique within a struct
- Length-referenced array dimensions (`type[field]`) must reference a preceding integer field in the same struct
- Fields are parsed strictly in declaration order

### 4.7 Union and Match Rules

These rules apply to both top-level union declarations and inline match expressions:

1. **Tag type**: The tag field must be a preceding integer primitive field in the same struct
2. **Backing type match**: When using a named union via `[tag = field]`, the tag field's type must exactly match the union's declared backing type (e.g., if the union says `: u8`, the tag field must be `u8`)
3. **Case value range**: Every case value must fit within the range of the tag type (e.g., for `u8`: 0–255, for `i8`: -128–127)
4. **No duplicate cases**: Each integer case value may appear at most once
5. **At most one default**: The `_` (default) case may appear at most once
6. **At least one case**: A union or match body must contain at least one case
7. **Case types**: Each case maps to a type expression — any primitive, enum, struct, or `bytes[N]`
8. **Optional default**: A default case is not required. If absent, an unrecognized tag value at parse time is a runtime error
9. **Tag option required**: A field referencing a union type must include `[tag = field]`

### 4.8 Format Block Rules

- At most one format block per file
- The `root` property is required and must reference a declared struct name

## 5. Code Generation Semantics

This section defines what the generated parser code must do. The exact target language varies, but the semantics are invariant.

### 5.1 Parse Function

Every struct generates a parse function/method with this contract:

- **Input**: A byte buffer and a starting offset
- **Output**: A populated struct instance and the new offset (pointing to the first unconsumed byte)
- **Behavior**: Read fields in declaration order, advancing the offset after each read
- **Errors**: Throw/raise on buffer overflow, failed assertions, or unrecognized tag values (when no default case exists)

### 5.2 Primitive Parsing

| Type | Read |
|------|------|
| `u8`, `i8` | 1 byte, direct cast |
| `u16`, `i16` | 2 bytes, memcpy + endian swap if big-endian |
| `u32`, `i32`, `f32` | 4 bytes, memcpy + endian swap if big-endian |
| `u64`, `i64`, `f64` | 8 bytes, memcpy + endian swap if big-endian |

For target languages with native endian-aware reads (like Python's `struct.unpack`), use the appropriate format prefix (`>` for big-endian, `<` for little-endian).

### 5.3 Bytes/String Parsing

| Array kind | Read behavior |
|-----------|---------------|
| Fixed size `bytes[N]` | Read N bytes from buffer |
| Length ref `bytes[field]` | Read `field` bytes from buffer (where `field` is a previously-parsed integer) |
| Terminator `[terminator = S]` | Read bytes one at a time until sentinel byte S is found. Consume the sentinel but do not include it in the result. |
| Rest `[rest = true]` | Read all remaining bytes |
| Bare `bytes` | Empty byte sequence |

### 5.4 Struct Field Parsing

Recurse: call the referenced struct's parse function with the current buffer and offset.

For array struct fields:
- **Rest array**: Loop calling parse until offset reaches the end of the buffer
- **Count ref**: Loop calling parse for the specified count

### 5.5 Enum Field Parsing

Read the backing integer type, then cast/convert to the enum type.

### 5.6 Match/Union Field Parsing

Given a match specification with tag field `T`, cases `[V1 => Type1, V2 => Type2, ...]`, and optional default `_ => DefaultType`:

```
if T == V1:
    parse as Type1
elif T == V2:
    parse as Type2
...
else:
    if default exists:
        parse as DefaultType
    else:
        raise error "unknown tag value"
```

Each case type is parsed using the same rules as a regular field of that type.

### 5.7 Validation

After parsing a field with one or more `(builtin).cel` validation rules, evaluate each rule's CEL expression with `this` bound to the parsed field value. If the expression evaluates to false, raise an error using the rule's `message` string.

A `(builtin).cel` block requires three fields:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | Unique identifier for this rule (e.g., `"version.min"`) |
| `message` | String | Error message to raise when validation fails |
| `expression` | String | CEL expression that must evaluate to true. `this` refers to the field's parsed value. |

Multiple validation rules are evaluated in declaration order. Each rule is independent — a failing rule raises an error immediately.

### 5.8 JSON Serialization

Every struct generates a `to_json` method that produces a JSON string representation:

| Field kind | JSON representation |
|-----------|---------------------|
| Integer primitive | JSON number |
| Float primitive | JSON number |
| Bytes/string (raw) | JSON array of integers (byte values) |
| String (terminator) | JSON string (decoded as UTF-8) |
| Enum | JSON string (variant name) |
| Struct | JSON object (recursive) |
| Array of structs | JSON array of objects |
| Match field | Determined by the actual parsed type at runtime |

### 5.9 Binary Serialization

Every struct generates a `to_bytes` method that serializes the struct back to its binary representation. The generated method has the signature `to_bytes() -> bytes` (Python) or `std::vector<uint8_t> to_bytes() const` (C++). Round-trip fidelity is the goal: `Struct.parse(s.to_bytes(), 0)` should produce an equivalent struct.

**Auto-computed length and count fields.** When a struct contains a length-referenced array (`bytes[field]`) or a same-struct count-referenced array (`[count = field]`), the length/count field's stored value is ignored on write. Instead, the actual size of the referenced data is written. This ensures the serialized binary is consistent even if the struct was constructed manually. Dotted count references (e.g., `[count = header.n]`) cross struct boundaries and cannot be auto-computed — the stored value is written as-is.

| Field kind | Write behavior |
|-----------|----------------|
| Scalar primitive | Pack with struct format and endian prefix |
| Auto-computed primitive | Pack actual length/count of referenced field |
| `bytes[N]` (fixed) | Write raw bytes |
| `bytes[field]` (length-ref) | Write raw bytes; length field auto-computed |
| `bytes` with `[terminator = S]` | Write raw bytes, then append sentinel byte S |
| `bytes` with `[rest = true]` | Write raw bytes |
| Bare `bytes` | Write raw bytes (empty) |
| Primitive array | Pack each element; terminator arrays append sentinel value |
| Enum | Cast to backing integer type, pack |
| Struct (single) | Recursively call `to_bytes()` |
| Struct (array) | Call `to_bytes()` on each element in order |
| Match/union | Dispatch by tag field value (Python) or variant index (C++) |

Validation rules are not re-evaluated on write. The struct is assumed to be correctly constructed.

## 6. Complete Example

This example shows a PNG file format specification using most language features:

```clv
package png;

// Color type values from the PNG specification.
enum ColorType : u8 {
    Grayscale = 0;
    RGB = 2;
    Palette = 3;
    GrayscaleAlpha = 4;
    RGBA = 6;
}

struct Header {
    signature bytes[8];

    option (builtin) = {
        endian = big;
    };
}

struct IHDRData {
    width             u32;
    height            u32;
    bit_depth         u8;
    color_type        ColorType;
    compression       u8;
    filter            u8;
    interlace         u8;

    option (builtin) = {
        endian = big;
    };
}

struct Chunk {
    length  u32;
    type    bytes[4];
    data    bytes[length];
    crc     u32;

    option (builtin) = {
        endian = big;
    };
}

struct PngFile {
    header  Header;
    chunks  Chunk [rest = true];

    option (builtin) = {
        endian = big;
    };
}
```

## 7. Union/Match Example

This example shows both inline match and named union usage:

```clv
package events;

// Reusable discriminated union for parameter values.
union ParamValue : u8 {
    0 => u32;
    1 => f32;
    _ => bytes[4];
}

struct InlineEvent {
    type  u8;
    // Inline match — type expression defined right here.
    value match type {
        0 => u32;
        1 => f32;
        _ => bytes[4];
    };
}

struct RefEvent {
    type  u8;
    // Named union reference — reuses the ParamValue definition.
    value ParamValue [tag = type];
}
```

Both `InlineEvent` and `RefEvent` produce identical parsing behavior. Use inline match for one-off cases; use named unions when the same tag-to-type mapping appears in multiple structs.

## 8. Validation Example

This example demonstrates CEL validation rules applied to a binary container format. It shows single-field validation, multiple rules on one field, compound boolean expressions, and validation combined with conditional parsing.

```clv
package validation;

struct Header {
    // Exact match: magic number must equal 0x434C4156 ("CLAV").
    magic   u32 [
        (builtin).cel = {
            id: "magic.check"
            message: "invalid magic number"
            expression: "this == 0x434C4156"
        }
    ];

    // Range: two rules on one field enforce min and max.
    version u16 [
        (builtin).cel = {
            id: "version.min"
            message: "version must be at least 1"
            expression: "this >= 1"
        },
        (builtin).cel = {
            id: "version.max"
            message: "version must be at most 100"
            expression: "this <= 100"
        }
    ];

    // Compound boolean: flags must be in [0, 7].
    flags u8 [
        (builtin).cel = {
            id: "flags.range"
            message: "flags must be between 0 and 7"
            expression: "this >= 0 && this <= 7"
        }
    ];

    option (builtin) = {
        endian = big;
    };
}

struct Record {
    name     string[64, terminator = 0x00];

    // Enum-like constraint without a dedicated enum type.
    priority u8 [
        (builtin).cel = {
            id: "priority.valid"
            message: "priority must be low (1), medium (2), or high (3)"
            expression: "this >= 1 && this <= 3"
        }
    ];

    // Validation and conditional parsing coexist on different fields.
    has_data u8;
    data     u32 [if = "has_data != 0"];
}
```

### Common validation patterns

**Exact value (magic numbers, signatures):**

```clv
magic u32 [
    (builtin).cel = {
        id: "magic"
        message: "not a valid file"
        expression: "this == 0x89504E47"
    }
];
```

**Minimum / maximum (version bounds, sizes):**

```clv
version u16 [
    (builtin).cel = {
        id: "version.min"
        message: "version too old"
        expression: "this >= 2"
    }
];
```

**Range (bitfields, enum-like values):**

```clv
flags u8 [
    (builtin).cel = {
        id: "flags.range"
        message: "flags out of range"
        expression: "this >= 0 && this <= 15"
    }
];
```

**Non-zero check:**

```clv
length u32 [
    (builtin).cel = {
        id: "length.nonzero"
        message: "length must not be zero"
        expression: "this != 0"
    }
];
```

**Multiple independent rules:**

```clv
port u16 [
    (builtin).cel = {
        id: "port.min"
        message: "port must be at least 1"
        expression: "this >= 1"
    },
    (builtin).cel = {
        id: "port.max"
        message: "port must be at most 65535"
        expression: "this <= 65535"
    }
];
```

### CEL expression reference

Validation expressions use [CEL (Common Expression Language)](https://github.com/google/cel-spec) syntax. The special identifier `this` refers to the field's parsed value.

| Category | Operators | Example |
|----------|-----------|---------|
| Comparison | `==`, `!=`, `<`, `<=`, `>`, `>=` | `this >= 1` |
| Logical | `&&`, `\|\|`, `!` | `this >= 0 && this <= 255` |
| Arithmetic | `+`, `-`, `*`, `/`, `%` | `this % 4 == 0` |

Hex literals (e.g., `0xFF`) are supported in expressions and are converted to decimal before CEL evaluation.

The `if` option also uses CEL syntax but cannot reference `this` (the field hasn't been parsed yet). It references other field names directly:

```clv
has_crc u8;
crc     u32 [if = "has_crc != 0"];
```

## 9. Error Reporting

All errors include the source location as `file:line:column`. Parse errors attempt recovery by synchronizing to the next semicolon, closing brace, or top-level keyword (`struct`, `enum`, `format`, `union`). A partial AST is returned alongside the error list, allowing tooling to report multiple errors in a single pass.

Semantic errors (from the lowering/IR phase) are also collected and reported with source locations. They cover:

- Duplicate type names
- Duplicate field names
- Unknown type references
- Invalid backing types (non-integer for enums/unions)
- Invalid length references (not a preceding integer field)
- Tag field validation failures
- Tag type mismatches against union backing types
- Case value range violations
- Duplicate case values

## 10. Formal Grammar Summary

For reference, the complete grammar in one block:

```ebnf
File         = PackageDecl { ImportDecl } { TopLevelDecl } .
PackageDecl  = "package" IDENT ";" .
ImportDecl   = "import" STRING ";" .
TopLevelDecl = StructDecl | EnumDecl | FormatDecl | UnionDecl .

EnumDecl     = "enum" IDENT ":" TypeRef "{" { EnumVariant } "}" .
EnumVariant  = IDENT "=" Expression ";" .

StructDecl   = "struct" IDENT "{" { FieldDecl | OptionBlock } "}" .
FieldDecl    = IDENT ( TypeExpr [ FieldOptions ] | MatchExpr ) ";" .
TypeExpr     = TypeRef [ "[" Expression "]" ] .
TypeRef      = IDENT .
FieldOptions = "[" FieldOpt { "," FieldOpt } "]" .
FieldOpt     = FieldOptKey "=" Value .
FieldOptKey  = IDENT | "(" IDENT ")" "." IDENT .

FormatDecl   = "format" IDENT "{" { FormatKV } "}" .
FormatKV     = IDENT "=" Value ";" .

UnionDecl    = "union" IDENT ":" TypeRef "{" UnionCase { UnionCase } "}" .
UnionCase    = CaseValue "=>" TypeExpr ";" .
CaseValue    = INTEGER | "-" INTEGER | "_" .

MatchExpr    = "match" IDENT "{" UnionCase { UnionCase } "}" .

OptionBlock  = "option" "(" IDENT ")" "=" "{" { OptionKV } "}" ";" .
OptionKV     = IDENT "=" Value ";" .

Expression   = INTEGER | IDENT .
Value        = IDENT | DottedIdent | INTEGER | STRING | "true" | "false" | BlockExpr .
DottedIdent  = IDENT { "." IDENT } .
BlockExpr    = "{" { BlockEntry } "}" .
BlockEntry   = IDENT ":" Value .
```

Terminals: `IDENT`, `INTEGER`, `STRING`, and all quoted keyword/punctuation tokens.
