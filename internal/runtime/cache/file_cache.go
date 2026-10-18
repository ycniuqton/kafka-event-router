// Package cache is the worker's on-disk config cache: the last explicit assignment set and immutable
// version documents, so runners keep going while the control plane is unreachable. Never secrets.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eventrouter/router/internal/runtime/domain"
)

// FileName names the cache files.
type FileName string

const (
	AssignmentsFile FileName = "assignments.json"
	VersionsDir     FileName = "versions"
	VersionSuffix   FileName = ".json"
	TempSuffix      FileName = ".tmp"
)

// FileMode is the permission of cache files and directories.
type FileMode os.FileMode

const (
	DirMode      FileMode = 0o700
	FileModeFile FileMode = 0o600
)

// ErrMiss is returned when nothing is cached.
var ErrMiss = errors.New("cache: miss")

// FileConfigCache writes atomically (temp file + rename) under one directory.
type FileConfigCache struct {
	dir string
}

// NewFileConfigCache creates the directory layout.
func NewFileConfigCache(dir string) (*FileConfigCache, error) {
	if err := os.MkdirAll(filepath.Join(dir, string(VersionsDir)), os.FileMode(DirMode)); err != nil {
		return nil, fmt.Errorf("cache: %w", err)
	}
	return &FileConfigCache{dir: dir}, nil
}

// SaveAssignments stores the last explicit answer of the control plane.
func (c *FileConfigCache) SaveAssignments(set domain.AssignmentSet) error {
	raw, err := json.Marshal(set)
	if err != nil {
		return err
	}
	return c.write(filepath.Join(c.dir, string(AssignmentsFile)), raw)
}

// LoadAssignments returns the cached assignment set.
func (c *FileConfigCache) LoadAssignments() (domain.AssignmentSet, error) {
	raw, err := c.read(filepath.Join(c.dir, string(AssignmentsFile)))
	if err != nil {
		return domain.AssignmentSet{}, err
	}
	var set domain.AssignmentSet
	return set, json.Unmarshal(raw, &set)
}

// SaveVersion stores an immutable version document.
func (c *FileConfigCache) SaveVersion(ref domain.VersionRef, doc []byte) error {
	return c.write(c.versionPath(ref), doc)
}

// LoadVersion returns a cached version document.
func (c *FileConfigCache) LoadVersion(ref domain.VersionRef) ([]byte, error) {
	return c.read(c.versionPath(ref))
}

func (c *FileConfigCache) versionPath(ref domain.VersionRef) string {
	return filepath.Join(c.dir, string(VersionsDir), filepath.Base(ref.VersionID)+string(VersionSuffix))
}

func (c *FileConfigCache) write(path string, raw []byte) error {
	tmp := path + string(TempSuffix)
	if err := os.WriteFile(tmp, raw, os.FileMode(FileModeFile)); err != nil {
		return fmt.Errorf("cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("cache: %w", err)
	}
	return nil
}

func (c *FileConfigCache) read(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrMiss
	}
	return raw, err
}
