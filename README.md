# cleave

Cleave reads `.clv` spec files that describe binary formats and generates parser code in Python or C++. You write the struct layout once, and cleave gives you code that can parse raw bytes into objects, serialize back, and dump to JSON.

```clv
package png;

enum ColorType : u8 {
    Grayscale      = 0;
    RGB            = 2;
    Palette        = 3;
    GrayscaleAlpha = 4;
    RGBA           = 6;
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

struct PngFile {
    header Header;
    chunks Chunk [rest = true];
}
```

## Install

```
go install github.com/rdeusser/cleave/cmd/cleave@latest
```

## Usage

```sh
# Check that a spec parses without errors:
cleave parse format.clv

# Generate a Python parser:
cleave generate --lang python --out gen/ format.clv

# Generate C++ (produces a .h and .cpp):
cleave generate --lang cpp --out gen/ format.clv

# Format a spec file in place:
cleave fmt format.clv

# Check formatting in CI (exits 1 if not formatted):
cleave fmt --check format.clv
```

## Language

Fields are written name-before-type, Go-style. Semicolons terminate everything. Types include `u8`–`u64`, `i8`–`i64`, `f32`, `f64`, `bytes`, and `string`. Arrays can be fixed-size (`bytes[8]`), length-referenced (`bytes[length]`), count-referenced (`Chunk [count = header.n]`), sentinel-terminated (`[terminator = 0x00]`), or consume the rest of the input (`[rest = true]`).

Enums, unions, inline match expressions, conditional fields (`[if = "flags & 0x01 != 0"]`), CEL validation rules, imports with diamond dedup, and per-field/per-struct endian overrides are all supported.

The full spec is in [docs/cleave-language-spec.md](docs/cleave-language-spec.md).

## Pipeline

```
.clv source → lexer → parser → AST → IR lowering → codegen → .py / .h+.cpp
```

## Tests

```
go test ./...
```

Test specs live in `testdata/`. Golden files for codegen output are in `testdata/golden/`.
