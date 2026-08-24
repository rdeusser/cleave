package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateCmd_RunRustCrate(t *testing.T) {
	t.Parallel()

	spec := writeGenerateSpec(t)
	out := filepath.Join(t.TempDir(), "generated")
	cmd := GenerateCmd{Lang: "rust", Out: out, File: spec}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	manifest := readGeneratedFile(t, filepath.Join(out, "sample", "Cargo.toml"))
	if !strings.Contains(manifest, `edition = "2024"`) {
		t.Errorf("Cargo.toml does not select Rust 2024:\n%s", manifest)
	}
	source := readGeneratedFile(t, filepath.Join(out, "sample", "src", "lib.rs"))
	if !strings.Contains(source, "pub struct Packet") {
		t.Errorf("lib.rs does not declare Packet:\n%s", source)
	}
}

func TestGenerateCmd_RunRustWithoutCargo(t *testing.T) {
	t.Parallel()

	spec := writeGenerateSpec(t)
	out := filepath.Join(t.TempDir(), "generated")
	cmd := GenerateCmd{Lang: "rust", Out: out, File: spec, NoCargo: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	source := readGeneratedFile(t, filepath.Join(out, "sample.rs"))
	if !strings.Contains(source, "pub struct Packet") {
		t.Errorf("sample.rs does not declare Packet:\n%s", source)
	}
	if _, err := os.Stat(filepath.Join(out, "sample", "Cargo.toml")); !os.IsNotExist(err) {
		t.Errorf("Cargo.toml exists with --no-cargo; os.Stat error = %v", err)
	}
}

func TestGenerateCmd_NoCargoRequiresRust(t *testing.T) {
	t.Parallel()

	cmd := GenerateCmd{Lang: "python", NoCargo: true}
	err := cmd.Run()
	if err == nil || err.Error() != "--no-cargo requires --lang rust" {
		t.Fatalf("Run() error = %v, want --no-cargo requires --lang rust", err)
	}
}

func writeGenerateSpec(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "sample.clv")
	source := `package sample;

struct Packet {
    value u8;
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
	return path
}

func readGeneratedFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}
	return string(content)
}
