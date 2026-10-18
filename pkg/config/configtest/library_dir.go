package configtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/eventrouter/router/pkg/config"
)

// LibraryFile names the parts of a library file name `<ref>@<version>.json`.
type LibraryFile string

const (
	LibraryFileExt       LibraryFile = ".json"
	LibraryFileSeparator LibraryFile = "@"
	LibraryTablesDir     LibraryFile = "tables"
)

// LibraryDir loads a library from a directory: transforms (`steps` arrays) as
// `<dir>/<ref>@<version>.json` and lookup tables (`[{key,value}]`) under `<dir>/tables/`.
type LibraryDir struct {
	Path string
}

// Load reads every library file into an in-memory resolver.
func (d LibraryDir) Load() (*InMemoryLibrary, error) {
	lib := NewInMemoryLibrary()
	err := d.each(d.Path, func(ref string, version int, raw []byte) error {
		var steps config.Steps
		if err := json.Unmarshal(raw, &steps); err != nil {
			return err
		}
		lib.PutTransform(ref, version, steps)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = d.each(filepath.Join(d.Path, string(LibraryTablesDir)), func(ref string, version int, raw []byte) error {
		var rows config.LookupRows
		if err := json.Unmarshal(raw, &rows); err != nil {
			return err
		}
		lib.PutLookupTable(ref, version, rows)
		return nil
	})
	return lib, err
}

// each calls load for every `<ref>@<version>.json` file directly in dir.
func (LibraryDir) each(dir string, load func(ref string, version int, raw []byte) error) error {
	files, err := filepath.Glob(filepath.Join(dir, "*"+string(LibraryFileExt)))
	if err != nil {
		return err
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), string(LibraryFileExt))
		ref, ver, ok := strings.Cut(name, string(LibraryFileSeparator))
		version, err := strconv.Atoi(ver)
		if !ok || err != nil {
			return fmt.Errorf("library file %q: want <ref>@<version>.json", file)
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if err := load(ref, version, raw); err != nil {
			return fmt.Errorf("library file %q: %w", file, err)
		}
	}
	return nil
}
