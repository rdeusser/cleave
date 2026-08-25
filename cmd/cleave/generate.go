package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rdeusser/cleave/internal/codegen"
	"github.com/rdeusser/cleave/internal/codegen/cpp"
	"github.com/rdeusser/cleave/internal/codegen/python"
	"github.com/rdeusser/cleave/internal/codegen/rust"
)

type GenerateCmd struct {
	Lang    string `required:"" enum:"python,cpp,rust" help:"Output language (python, cpp, rust)."`
	Out     string `default:"." help:"Output directory."`
	NoCargo bool   `name:"no-cargo" help:"Generate one Rust source file without a Cargo crate."`
	File    string `arg:"" type:"existingfile" help:"Path to .clv file."`
}

func (cmd *GenerateCmd) Run() error {
	if cmd.NoCargo && cmd.Lang != "rust" {
		return errors.New("--no-cargo requires --lang rust")
	}

	pkg, _, err := compileFile(cmd.File)
	if err != nil {
		return err
	}

	var gen codegen.Generator
	switch cmd.Lang {
	case "python":
		gen = &python.Generator{}
	case "cpp":
		gen = &cpp.Generator{}
	case "rust":
		gen = &rust.Generator{NoCargo: cmd.NoCargo}
	default:
		return fmt.Errorf("unsupported output language %q", cmd.Lang)
	}

	outputs, err := gen.Generate(pkg)
	if err != nil {
		return fmt.Errorf("codegen: %w", err)
	}

	if err := os.MkdirAll(cmd.Out, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	for _, out := range outputs {
		path := filepath.Join(cmd.Out, out.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("cannot create output directory for %s: %w", path, err)
		}
		if err := os.WriteFile(path, out.Content, 0644); err != nil {
			return fmt.Errorf("cannot write %s: %w", path, err)
		}
	}

	return nil
}
