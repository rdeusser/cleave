package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rdeusser/cleave/internal/format"
)

type FmtCmd struct {
	List  bool     `short:"l" help:"List files whose formatting would change."`
	Write bool     `short:"w" xor:"write" help:"Write the result to the source file instead of standard output."`
	Check bool     `xor:"write" help:"Report files whose formatting would change and exit 1 if there are any."`
	Paths []string `arg:"" name:"path" help:"Files or directories to format. A directory is searched recursively for .clv files. The search skips hidden files and the hidden, node_modules, and testdata directories inside it."`
}

func (cmd *FmtCmd) Run(w io.Writer) error {
	var errs []error
	for _, path := range cmd.Paths {
		info, err := os.Stat(path)
		switch {
		case err != nil:
			errs = append(errs, err)
		case info.IsDir():
			errs = append(errs, cmd.formatDir(w, path))
		default:
			// A file named on the command line is formatted whatever its name.
			errs = append(errs, cmd.formatFile(w, path))
		}
	}
	return errors.Join(errs...)
}

// formatDir formats each .clv file under dir. It skips hidden files and the
// hidden, node_modules, and testdata directories inside dir. A testdata
// directory holds test fixtures, which may be unformatted on purpose. It
// follows dir when dir is a symbolic link, but it does not follow symbolic
// links to directories inside dir. It records each failure and continues the
// walk.
func (cmd *FmtCmd) formatDir(w io.Writer, dir string) error {
	// WalkDir does not follow a symbolic link at its root. A trailing
	// separator makes the link resolve to the directory it names.
	sep := string(filepath.Separator)
	root := strings.TrimRight(dir, sep) + sep

	var errs []error
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}

		name := d.Name()
		hidden := strings.HasPrefix(name, ".")
		if d.IsDir() {
			switch {
			case path == root:
				// The root is searched whatever its name, so "." and a
				// testdata directory named on the command line work.
			case hidden, name == "node_modules", name == "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !hidden && strings.HasSuffix(name, ".clv") {
			errs = append(errs, cmd.formatFile(w, path))
		}
		return nil
	})
	return errors.Join(append(errs, err)...)
}

// formatFile formats the file at path. With none of -l, -w, and --check set, it
// prints the result. Otherwise it lists, reports, or writes the file when the
// result differs from the source.
func (cmd *FmtCmd) formatFile(w io.Writer, path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}

	formatted, err := format.Source(path, src)
	if err != nil {
		return err
	}
	if !cmd.List && !cmd.Write && !cmd.Check {
		_, err = w.Write(formatted)
		return err
	}
	if bytes.Equal(src, formatted) {
		return nil
	}

	if cmd.List {
		if _, err := fmt.Fprintln(w, path); err != nil {
			return err
		}
	}
	if cmd.Check {
		return fmt.Errorf("%s: not formatted", path)
	}
	if cmd.Write {
		if err := os.WriteFile(path, formatted, 0644); err != nil {
			return fmt.Errorf("cannot write %s: %w", path, err)
		}
	}
	return nil
}
