package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFmtCmd_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     FmtCmd
		wantErr string
	}{
		{
			name:    "unparseable spec reports the parse error",
			cmd:     FmtCmd{Check: true, File: filepath.Join("..", "..", "testdata", "errors", "missing_semicolon.clv")},
			wantErr: "expected ;",
		},
		{
			name:    "check reports an unformatted spec",
			cmd:     FmtCmd{Check: true, File: writeGenerateSpec(t)},
			wantErr: "not formatted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cmd.Run()
			if err == nil {
				t.Fatalf("Run() with %s error = nil, want one containing %q", tt.cmd.File, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Run() with %s error = %v, want one containing %q", tt.cmd.File, err, tt.wantErr)
			}
		})
	}
}
