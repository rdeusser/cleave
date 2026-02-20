package main

import (
	"fmt"
	"os"

	"github.com/rdeusser/cleave/internal/format"
	"github.com/rdeusser/cleave/internal/parser"
)

type FmtCmd struct {
	Check bool   `help:"Check formatting without modifying (exit 1 if unformatted)."`
	File  string `arg:"" type:"existingfile" help:"Path to .clv file."`
}

func (cmd *FmtCmd) Run() error {
	src, err := os.ReadFile(cmd.File)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", cmd.File, err)
	}

	p := parser.New(cmd.File, src)
	ast, parseErrs := p.Parse()
	if len(parseErrs) > 0 {
		for _, e := range parseErrs {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}

	formatted := format.Format(ast)

	if cmd.Check {
		if string(src) != formatted {
			fmt.Fprintf(os.Stderr, "%s: not formatted\n", cmd.File)
			os.Exit(1)
		}
		return nil
	}

	if err := os.WriteFile(cmd.File, []byte(formatted), 0644); err != nil {
		return fmt.Errorf("cannot write %s: %w", cmd.File, err)
	}

	return nil
}
