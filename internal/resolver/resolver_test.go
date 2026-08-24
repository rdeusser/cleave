package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdeusser/cleave/internal/ast"
)

func TestResolveBasicImport(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "import", "main.clv")
	file, _, errs := Resolve(path)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if file.Package.Name.Name != "sox" {
		t.Errorf("package name: got %q, want %q", file.Package.Name.Name, "sox")
	}

	// SoxFile should be in Decls (entry file's own declarations).
	localStructNames := make(map[string]bool)
	for _, d := range file.Decls {
		if s, ok := d.(*ast.StructDecl); ok {
			localStructNames[s.Name.Name] = true
		}
	}
	if !localStructNames["SoxFile"] {
		t.Error("missing SoxFile from main.clv in Decls")
	}

	// SoxHeader should be in ImportedDecls (from common.clv).
	importedStructNames := make(map[string]bool)
	for _, d := range file.ImportedDecls {
		if s, ok := d.(*ast.StructDecl); ok {
			importedStructNames[s.Name.Name] = true
		}
	}
	if !importedStructNames["SoxHeader"] {
		t.Error("missing SoxHeader from common.clv in ImportedDecls")
	}
}

func TestResolveCircularImport(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "errors", "circular_import.clv")
	_, _, errs := Resolve(path)
	if len(errs) == 0 {
		t.Fatal("expected circular import error")
	}
	found := false
	for _, err := range errs {
		if strings.Contains(err.Error(), "circular") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected circular import error, got: %v", errs)
	}
}

func TestResolveMissingImport(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "errors", "missing_import.clv")
	_, _, errs := Resolve(path)
	if len(errs) == 0 {
		t.Fatal("expected missing import error")
	}
}

func TestResolveDiamondImport(t *testing.T) {
	// Diamond import graph:
	//   main.clv → a.clv → common.clv
	//   main.clv → b.clv → common.clv
	dir := t.TempDir()

	writeFile(t, dir, "common.clv", `package common;

struct Shared {
    x u32;
}
`)
	writeFile(t, dir, "a.clv", `package a;

import "common.clv";

struct AType {
    shared Shared;
}
`)
	writeFile(t, dir, "b.clv", `package b;

import "common.clv";

struct BType {
    shared Shared;
}
`)
	writeFile(t, dir, "main.clv", `package main;

import "a.clv";
import "b.clv";

struct Root {
    a AType;
    b BType;
}
`)

	file, _, errs := Resolve(filepath.Join(dir, "main.clv"))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	// Root should be in Decls (entry file's own).
	localCounts := make(map[string]int)
	for _, d := range file.Decls {
		if s, ok := d.(*ast.StructDecl); ok {
			localCounts[s.Name.Name]++
		}
	}
	if localCounts["Root"] != 1 {
		t.Errorf("Root struct count in Decls: got %d, want 1", localCounts["Root"])
	}

	// Shared, AType, BType should be in ImportedDecls. Shared should appear only once.
	importedCounts := make(map[string]int)
	for _, d := range file.ImportedDecls {
		if s, ok := d.(*ast.StructDecl); ok {
			importedCounts[s.Name.Name]++
		}
	}
	if importedCounts["Shared"] != 1 {
		t.Errorf("Shared struct count in ImportedDecls: got %d, want 1", importedCounts["Shared"])
	}
	if importedCounts["AType"] != 1 {
		t.Errorf("AType struct count in ImportedDecls: got %d, want 1", importedCounts["AType"])
	}
	if importedCounts["BType"] != 1 {
		t.Errorf("BType struct count in ImportedDecls: got %d, want 1", importedCounts["BType"])
	}
}

func TestResolveNoImports(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "simple.clv", `package simple;

struct Foo {
    x u32;
}
`)
	file, _, errs := Resolve(filepath.Join(dir, "simple.clv"))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if file.Package.Name.Name != "simple" {
		t.Errorf("package name: got %q, want %q", file.Package.Name.Name, "simple")
	}
	if len(file.Decls) != 1 {
		t.Errorf("expected 1 decl, got %d", len(file.Decls))
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
