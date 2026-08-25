package main

import (
	"fmt"
	"os"
)

type ParseCmd struct {
	File string `arg:"" type:"existingfile" help:"Path to .clv file."`
}

func (cmd *ParseCmd) Run() error {
	_, _, err := compileFile(cmd.File)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return nil
}
