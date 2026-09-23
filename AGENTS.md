# Cleave

Declarative binary format parser generator. Reads `.clv` spec files and generates parser code in Python and C++.

## Pipeline

`.clv` → Lexer → Parser → AST → IR (Lower) → Code Generator → output files

## Package Dependency Graph

```
cmd/cleave → resolver, ir, codegen/*, format
resolver   → parser, ast
parser     → ast, token, lexer
ir         → ast, token, cel-go
codegen/*  → ir
format     → parser, ast, token
```

No cycles. `token` and `ast` are leaf packages.

## DSL Syntax

```clv
package png;

import "common.clv";

format PngFormat {
    title     = "PNG Image";
    extension = "png";
    endian    = big;
    root      = PngFile;
}

enum ColorType : u8 {
    Grayscale = 0;
    RGB = 2;
}

struct PngFile {
    header Header;
    chunks Chunk [rest = true];
}

struct Chunk {
    length u32;
    type   bytes[4];
    data   bytes[length];
    crc    u32;

    option (builtin) = {
        endian = big;
    };
}

struct Record {
    name    string[64, terminator = 0x00];
    version u16 [
        (builtin).cel = {
            id: "version.min"
            message: "version must be at least 1"
            expression: "this >= 1"
        }
    ];
}
```

Key rules:
- Field name before type (Go-style)
- Semicolons terminate all declarations
- Primitives: `u8`, `u16`, `u32`, `u64`, `i8`, `i16`, `i32`, `i64`, `f32`, `f64`, `bytes`, `string`
- Arrays: fixed `bytes[8]`, length-ref `bytes[length]`, rest `[rest = true]`, terminator `[terminator = 0x00]`, count `[count = header.n]`, fixed-terminated `string[64, terminator = 0x00]`
- String encoding: `string[32, encoding = "CP949"]`, `string [terminator = 0x00, encoding = "SJIS"]` — inline or field option, string fields only
- Conditional parsing (CEL syntax): `[if = "flags & 0x01 != 0"]`
- Structured validation: `[(builtin).cel = { id: "..." message: "..." expression: "this > 0" }]` — namespaced block with id, message, and CEL expression. Multiple blocks allowed, comma-separated.
- Imports: `import "common.clv";` — relative to importing file, diamond dedup, cycle detection
- Struct options: `option (builtin) = { endian = big; };`
- Format block: top-level metadata with root struct, endian default, title, extension
- Endian inheritance: field option > struct option > format default > little

## CLI

```
cleave parse <file.clv>
cleave generate --lang <python|cpp> --out <dir> <file.clv>
cleave fmt [-l] [-w] <path>... # --check for CI
```

Uses `alecthomas/kong` for CLI parsing.

## Code Conventions

- Procedural code generation via `strings.Builder` — no `text/template`
- Code generators use struct-based `writer` pattern: `type writer struct { sb strings.Builder; pkg *ir.Package }`
- Pure utility functions (type mapping) remain package-level
- Parser is recursive descent with one-token lookahead + peek buffer
- IR lowering does two-pass type resolution: collect names, then resolve references
- Resolver handles imports: recursive parsing, diamond dedup by absolute path, cycle detection
- Top-down ordering in `.clv` files: root/important structs first, leaf types last

## Testing

- `go test ./...` runs all tests
- `testdata/` contains `.clv` spec files and `golden/` output files
- `testdata/errors/` contains intentionally broken specs for error-path tests
- `testdata/import/` contains multi-file import test specs
- Golden files are written by test runs, not checked manually
