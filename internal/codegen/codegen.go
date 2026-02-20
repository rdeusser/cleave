package codegen

import "github.com/rdeusser/cleave/internal/ir"

type Generator interface {
	Generate(pkg *ir.Package) ([]OutputFile, error)
}

type OutputFile struct {
	Name    string
	Content []byte
}
