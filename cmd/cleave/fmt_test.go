package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFmtCmd_Run checks what Run prints, reports, and writes in each mode.
// Each case writes its files into a new working directory, so that it can name
// paths such as "." and compare printed paths exactly. The working directory
// belongs to the whole process, so the cases cannot run in parallel.
func TestFmtCmd_Run(t *testing.T) {
	const formatted = `package sample;

struct Packet {
    tag   u8;
    value u32;
}
`
	const unformatted = `package sample;
struct Packet {
  tag u8;
  value u32;
}
`
	const unparseable = `package sample;

struct Packet {
    value u8
}
`

	// tree holds two unformatted specs, one of them in a subdirectory, and a
	// formatted spec.
	tree := map[string]string{
		"a.clv":     unformatted,
		"b.clv":     formatted,
		"sub/c.clv": unformatted,
	}
	formattedTree := map[string]string{
		"a.clv":     formatted,
		"b.clv":     formatted,
		"sub/c.clv": formatted,
	}

	tests := []struct {
		name    string
		cmd     FmtCmd
		files   map[string]string // file contents by path
		links   map[string]string // symbolic link targets by link path
		want    map[string]string // file contents after Run, or nil if unchanged
		wantOut string
		wantErr string
	}{
		{
			name:    "prints the formatted source of a file",
			cmd:     FmtCmd{Paths: []string{"a.clv"}},
			files:   tree,
			wantOut: formatted,
		},
		{
			name:    "lists the unformatted files under the working directory",
			cmd:     FmtCmd{List: true, Paths: []string{"."}},
			files:   tree,
			wantOut: "a.clv\nsub/c.clv\n",
		},
		{
			name:  "writes the unformatted files",
			cmd:   FmtCmd{Write: true, Paths: []string{"."}},
			files: tree,
			want:  formattedTree,
		},
		{
			name:    "lists and writes the unformatted files",
			cmd:     FmtCmd{List: true, Write: true, Paths: []string{"."}},
			files:   tree,
			want:    formattedTree,
			wantOut: "a.clv\nsub/c.clv\n",
		},
		{
			name: "skips hidden files, other extensions, and hidden, node_modules, and testdata directories",
			cmd:  FmtCmd{List: true, Paths: []string{"."}},
			files: map[string]string{
				".a.clv":                 unformatted,
				".git/b.clv":             unformatted,
				"node_modules/c.clv":     unformatted,
				"testdata/d.clv":         unformatted,
				"sub/.cache/e.clv":       unformatted,
				"sub/node_modules/f.clv": unformatted,
				"sub/testdata/g.clv":     unformatted,
				"sub/h.txt":              unformatted,
				"sub/i.clv":              unformatted,
			},
			wantOut: "sub/i.clv\n",
		},
		{
			name: "searches a hidden directory named as an argument",
			cmd:  FmtCmd{List: true, Paths: []string{".specs"}},
			files: map[string]string{
				".specs/a.clv":      unformatted,
				".specs/.git/b.clv": unformatted,
			},
			wantOut: ".specs/a.clv\n",
		},
		{
			name: "searches a testdata directory named as an argument",
			cmd:  FmtCmd{List: true, Paths: []string{"testdata"}},
			files: map[string]string{
				"testdata/a.clv":              unformatted,
				"testdata/sub/b.clv":          unformatted,
				"testdata/sub/testdata/c.clv": unformatted,
			},
			wantOut: "testdata/a.clv\ntestdata/sub/b.clv\n",
		},
		{
			name: "formats a file named as an argument whatever its name",
			cmd:  FmtCmd{List: true, Paths: []string{"spec.txt", ".spec.clv"}},
			files: map[string]string{
				"spec.txt":  unformatted,
				".spec.clv": unformatted,
			},
			wantOut: "spec.txt\n.spec.clv\n",
		},
		{
			name: "follows a symbolic link named as an argument but no directory link inside it",
			cmd:  FmtCmd{List: true, Paths: []string{"link"}},
			files: map[string]string{
				"specs/a.clv": unformatted,
				"other/b.clv": unformatted,
			},
			links: map[string]string{
				"link":        "specs",
				"specs/other": "../other",
			},
			wantOut: "link/a.clv\n",
		},
		{
			name:    "check reports each unformatted file",
			cmd:     FmtCmd{Check: true, Paths: []string{"."}},
			files:   tree,
			wantErr: "a.clv: not formatted\nsub/c.clv: not formatted",
		},
		{
			name: "check accepts formatted files",
			cmd:  FmtCmd{Check: true, Paths: []string{"."}},
			files: map[string]string{
				"a.clv":     formatted,
				"sub/b.clv": formatted,
			},
		},
		{
			name: "reports a parse error and writes the other files",
			cmd:  FmtCmd{Write: true, Paths: []string{"."}},
			files: map[string]string{
				"a.clv": unparseable,
				"b.clv": unformatted,
			},
			want: map[string]string{
				"a.clv": unparseable,
				"b.clv": formatted,
			},
			wantErr: "expected ;",
		},
		{
			name:    "reports a missing path and lists the other paths",
			cmd:     FmtCmd{List: true, Paths: []string{"missing.clv", "a.clv"}},
			files:   tree,
			wantOut: "a.clv\n",
			wantErr: "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for path, content := range tt.files {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for link, target := range tt.links {
				if err := os.Symlink(target, link); err != nil {
					t.Fatal(err)
				}
			}

			var buf bytes.Buffer
			switch err := tt.cmd.Run(&buf); {
			case tt.wantErr == "":
				if err != nil {
					t.Errorf("Run() with %+v error = %v, want nil", tt.cmd, err)
				}
			case err == nil || !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("Run() with %+v error = %v, want one containing %q", tt.cmd, err, tt.wantErr)
			}
			if got := buf.String(); got != tt.wantOut {
				t.Errorf("Run() with %+v output =\n%s\nwant:\n%s", tt.cmd, got, tt.wantOut)
			}

			want := tt.want
			if want == nil {
				want = tt.files
			}
			for path, content := range want {
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != content {
					t.Errorf("%s after Run() with %+v =\n%s\nwant:\n%s", path, tt.cmd, got, content)
				}
			}
		})
	}
}
