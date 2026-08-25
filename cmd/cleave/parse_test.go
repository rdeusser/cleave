package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCmd_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		file    string
		wantErr string
	}{
		{
			name: "valid spec",
			file: writeGenerateSpec(t),
		},
		{
			name:    "missing semicolon",
			file:    filepath.Join("..", "..", "testdata", "errors", "missing_semicolon.clv"),
			wantErr: "expected ;",
		},
		{
			name:    "circular import",
			file:    filepath.Join("..", "..", "testdata", "errors", "circular_import.clv"),
			wantErr: "circular import detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := ParseCmd{File: tt.file}
			err := cmd.Run()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Run() with %s error = %v, want nil", tt.file, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Run() with %s error = nil, want one containing %q", tt.file, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Run() with %s error = %v, want one containing %q", tt.file, err, tt.wantErr)
			}
		})
	}
}
