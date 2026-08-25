package main

import (
	"github.com/alecthomas/kong"

	"github.com/rdeusser/cleave/internal/ir"
	"github.com/rdeusser/cleave/internal/lexer"
	"github.com/rdeusser/cleave/internal/resolver"
)

type CLI struct {
	Parse    ParseCmd    `cmd:"" help:"Validate a .clv spec file."`
	Generate GenerateCmd `cmd:"" help:"Generate parser code from a .clv spec."`
	Fmt      FmtCmd      `cmd:"" help:"Format a .clv file in place."`
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

  # Format a spec file in place:
  cleave fmt format.clv

  # Check formatting in CI (exits 1 if unformatted):
  cleave fmt --check format.clv`),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)
	ctx.FatalIfErrorf(ctx.Run())
}

func compileFile(path string) (*ir.Package, []byte, []string) {
	file, src, err := resolver.Resolve(path)
	if err != nil {
		return nil, src, []string{err.Error()}
	}

	lex := lexer.New(path, src)
	pkg, lowerErrs := ir.Lower(file, lex.Position)
	var errs []string
	for _, e := range lowerErrs {
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		return nil, src, errs
	}

	return pkg, src, nil
}
