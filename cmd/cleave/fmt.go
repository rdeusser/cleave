package main

import (
	"errors"
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
		errs := make([]error, len(parseErrs))
		for i, e := range parseErrs {
			errs[i] = e
		}
		return errors.Join(errs...)
	}

	formatted := format.Format(ast)

	if cmd.Check {
		if string(src) != formatted {
			return fmt.Errorf("%s: not formatted", cmd.File)
		}
		return nil
	}

	if err := os.WriteFile(cmd.File, []byte(formatted), 0644); err != nil {
		return fmt.Errorf("cannot write %s: %w", cmd.File, err)
	}

	return nil
}
