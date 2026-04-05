package fswalker

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// FileEntry represents a single file found during directory traversal.
type FileEntry struct {
	RelPath string // path relative to the walked directory
	AbsPath string // absolute path on disk
}

// Walk recursively collects all regular files under dir.
// It skips hidden files/directories (names starting with ".") and symlinks.
// Results are sorted by RelPath for deterministic output.
func Walk(dir string) ([]FileEntry, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving directory: %w", err)
	}

	var entries []FileEntry
	err = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		name := d.Name()
		if strings.HasPrefix(name, ".") && path != absDir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(absDir, path)
		if err != nil {
			return fmt.Errorf("computing relative path for %s: %w", path, err)
		}

		entries = append(entries, FileEntry{
			RelPath: rel,
			AbsPath: path,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory %s: %w", dir, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].RelPath < entries[j].RelPath
	})

	return entries, nil
}
