# cleave

Cleave reads `.clv` spec files that describe binary formats. It generates parser code in Python, C++, or Rust. You write the struct layout once. Cleave generates code that parses bytes, serializes values, and emits JSON.

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

# Generate a Rust 2024 crate:
cleave generate --lang rust --out gen/ format.clv

# Generate one Rust source file without Cargo metadata:
cleave generate --lang rust --no-cargo --out gen/ format.clv

# Format a spec file in place:
cleave fmt format.clv

# Check formatting in CI (exits 1 if not formatted):
cleave fmt --check format.clv
```

## Language

Fields are written name-before-type, Go-style. Semicolons terminate everything. Types include `u8`–`u64`, `i8`–`i64`, `f32`, `f64`, `bytes`, and `string`. Arrays can be fixed-size (`bytes[8]`), length-referenced (`bytes[length]`), count-referenced (`Chunk [count = header.n]`), sentinel-terminated (`[terminator = 0x00]`), or consume the rest of the input (`[rest = true]`).

Enums, unions, inline match expressions, conditional fields (`[if = "flags & 0x01 != 0"]`), CEL validation rules, imports with diamond dedup, and endian overrides are supported.

The Rust generator creates `<package>/Cargo.toml` and `<package>/src/lib.rs`. It uses the Rust 2024 edition and targets the current stable toolchain. The manifest does not set `rust-version`. Add the generated crate as a path dependency:

```toml
[dependencies]
png = { path = "gen/png" }
```

Use `--no-cargo` to create `<package>.rs` instead. The generated source has no crate-level attributes. You can load it with `#[path]`:

```rust
#[path = "../gen/png.rs"]
pub mod png;
```

You can also load the source-only file with `include!`:

```rust
pub mod generated {
    include!(concat!(env!("CARGO_MANIFEST_DIR"), "/generated/png.rs"));
}
```

Each generated struct has public fields and these methods:

```rust
pub fn parse(buf: &[u8], offset: &mut usize) -> Result<Self, Error>;
pub fn to_bytes(&self) -> Result<Vec<u8>, Error>;
pub fn to_json(&self) -> String;
```

The generated `Error` type implements `Display` and `std::error::Error`.

The Rust crate has no dependencies unless the spec requires them. Text fields that use CP949, windows-949, or Shift-JIS add `encoding_rs`. CEL `matches()` expressions add `regex`.

The full spec is in [docs/cleave-language-spec.md](docs/cleave-language-spec.md).

## Pipeline

```
.clv source → lexer → parser → AST → IR lowering → codegen → .py / .h+.cpp / Rust crate
```

## Tests

```
go test ./...
```

Test specs live in `testdata/`. Golden files for codegen output are in `testdata/golden/`.
