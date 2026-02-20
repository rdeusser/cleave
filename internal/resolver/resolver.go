package resolver

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rdeusser/cleave/internal/ast"
	"github.com/rdeusser/cleave/internal/parser"
)

func Resolve(entryPath string) (*ast.File, []byte, []error) {
	r := &resolver{
		parsed:    make(map[string]*ast.File),
		resolving: make(map[string]bool),
	}

	absPath, err := filepath.Abs(entryPath)
	if err != nil {
		return nil, nil, []error{fmt.Errorf("cannot resolve path %s: %w", entryPath, err)}
	}

	entrySrc, errs := r.resolveAll(absPath, nil)
	if len(errs) > 0 {
		return nil, entrySrc, errs
	}

	// Collect imported declarations (for type lookup) separately from entry declarations.
	visited := make(map[string]bool)
	var importedDecls []ast.Decl
	r.collectImportedDecls(absPath, visited, &importedDecls)

	entryFile := r.parsed[absPath]
	merged := &ast.File{
		Package:       entryFile.Package,
		Imports:       entryFile.Imports,
		Decls:         entryFile.Decls,
		ImportedDecls: importedDecls,
		Comments:      entryFile.Comments,
	}

	return merged, entrySrc, nil
}

type resolver struct {
	parsed    map[string]*ast.File // absolute path → parsed file (own decls only)
	resolving map[string]bool      // absolute paths currently being resolved (cycle detection)
}

func (r *resolver) resolveAll(absPath string, stack []string) ([]byte, []error) {
	// Cycle detection: currently on the resolution stack.
	if r.resolving[absPath] {
		return nil, []error{fmt.Errorf("circular import detected: %s", formatCycle(stack, absPath))}
	}

	// Already fully resolved (diamond dedup).
	if _, ok := r.parsed[absPath]; ok {
		return nil, nil
	}

	r.resolving[absPath] = true
	defer func() { delete(r.resolving, absPath) }()

	src, err := os.ReadFile(absPath)
	if err != nil {
		return nil, []error{fmt.Errorf("cannot read %s: %w", absPath, err)}
	}

	p := parser.New(absPath, src)
	file, parseErrs := p.Parse()
	var errs []error
	for _, e := range parseErrs {
		errs = append(errs, e)
	}
	if len(errs) > 0 {
		return src, errs
	}

	// Recursively resolve imports before caching.
	dir := filepath.Dir(absPath)
	newStack := append(stack, absPath)

	for _, imp := range file.Imports {
		importPath := filepath.Join(dir, imp.Path.Value)
		importAbs, err := filepath.Abs(importPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot resolve import path %q: %w", imp.Path.Value, err))
			continue
		}

		_, importErrs := r.resolveAll(importAbs, newStack)
		if len(importErrs) > 0 {
			errs = append(errs, importErrs...)
		}
	}

	if len(errs) > 0 {
		return src, errs
	}

	// Store the file with only its own declarations after successful resolution.
	r.parsed[absPath] = file
	return src, nil
}

func (r *resolver) collectImportedDecls(entryPath string, visited map[string]bool, out *[]ast.Decl) {
	entryFile := r.parsed[entryPath]
	dir := filepath.Dir(entryPath)

	for _, imp := range entryFile.Imports {
		importPath := filepath.Join(dir, imp.Path.Value)
		importAbs, _ := filepath.Abs(importPath)
		r.collectAllDecls(importAbs, visited, out)
	}
}

func (r *resolver) collectAllDecls(absPath string, visited map[string]bool, out *[]ast.Decl) {
	if visited[absPath] {
		return
	}
	visited[absPath] = true

	file := r.parsed[absPath]

	dir := filepath.Dir(absPath)
	for _, imp := range file.Imports {
		importPath := filepath.Join(dir, imp.Path.Value)
		importAbs, _ := filepath.Abs(importPath)
		r.collectAllDecls(importAbs, visited, out)
	}

	*out = append(*out, file.Decls...)
}

func formatCycle(stack []string, target string) string {
	result := ""
	for _, s := range stack {
		result += filepath.Base(s) + " -> "
	}
	result += filepath.Base(target)
	return result
}
