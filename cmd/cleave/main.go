package main

import (
	"errors"
	"io"
	"os"

	"github.com/alecthomas/kong"

	"github.com/rdeusser/cleave/internal/ir"
	"github.com/rdeusser/cleave/internal/lexer"
	"github.com/rdeusser/cleave/internal/resolver"
)

type CLI struct {
	Parse    ParseCmd    `cmd:"" help:"Validate a .clv spec file."`
	Generate GenerateCmd `cmd:"" help:"Generate parser code from a .clv spec."`
	Fmt      FmtCmd      `cmd:"" help:"Format .clv files."`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("cleave"),
		kong.Description(`A declarative binary format parser generator.

Cleave reads .clv spec files that describe binary formats and generates
parser code in Python, C++, or Rust. The generated code can parse raw bytes into
structured data, serialize back to bytes, and convert to JSON.

Examples:
  # Validate a spec file:
  cleave parse format.clv

  # Generate a Python parser in the current directory:
  cleave generate --lang python format.clv

  # Generate a C++ parser into a specific directory:
  cleave generate --lang cpp --out src/generated format.clv

  # Generate a Rust 2024 crate:
  cleave generate --lang rust --out generated format.clv

  # Generate one Rust source file without Cargo metadata:
  cleave generate --lang rust --no-cargo --out generated format.clv

  # Print a spec file in canonical form:
  cleave fmt format.clv

  # Format the spec files under the current directory in place:
  cleave fmt -w .

  # List the spec files whose formatting would change:
  cleave fmt -l .

  # Check formatting in CI (exits 1 if any file would change):
  cleave fmt --check .`),
		kong.BindFor[io.Writer](os.Stdout),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)
	ctx.FatalIfErrorf(ctx.Run())
}

func compileFile(path string) (*ir.Package, []byte, error) {
	file, src, err := resolver.Resolve(path)
	if err != nil {
		return nil, src, err
	}

	lex := lexer.New(path, src)
	pkg, lowerErrs := ir.Lower(file, lex.Position)
	if len(lowerErrs) > 0 {
		errs := make([]error, len(lowerErrs))
		for i, e := range lowerErrs {
			errs[i] = e
		}
		return nil, src, errors.Join(errs...)
	}

	return pkg, src, nil
}
