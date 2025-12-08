package search

import (
	"io/fs"
	"path/filepath"
)

// FindFiles returns up to limit relative file paths under root, skipping common ignores.
func FindFiles(root string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 200
	}
	paths := make([]string, 0, limit)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".idea" || name == "target" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, rel)
		if len(paths) >= limit {
			return fs.SkipAll
		}
		return nil
	})
	return paths, err
}
