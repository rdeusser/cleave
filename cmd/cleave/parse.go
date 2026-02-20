package main

import (
	"fmt"
	"os"
)

type ParseCmd struct {
	File string `arg:"" type:"existingfile" help:"Path to .clv file."`
}

func (cmd *ParseCmd) Run() error {
	_, _, errs := compileFile(cmd.File)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}
	return nil
}
