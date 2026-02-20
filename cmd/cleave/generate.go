package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rdeusser/cleave/internal/codegen"
	"github.com/rdeusser/cleave/internal/codegen/cpp"
	"github.com/rdeusser/cleave/internal/codegen/python"
)

type GenerateCmd struct {
	Lang string `required:"" enum:"python,cpp" help:"Output language (python, cpp)."`
	Out  string `default:"." help:"Output directory."`
	File string `arg:"" type:"existingfile" help:"Path to .clv file."`
}

func (cmd *GenerateCmd) Run() error {
	pkg, _, errs := compileFile(cmd.File)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}

	var gen codegen.Generator
	switch cmd.Lang {
	case "python":
		gen = &python.Generator{}
	case "cpp":
		gen = &cpp.Generator{}
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
		if err := os.WriteFile(path, out.Content, 0644); err != nil {
			return fmt.Errorf("cannot write %s: %w", path, err)
		}
	}

	return nil
}
