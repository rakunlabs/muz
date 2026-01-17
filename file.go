package muz

import (
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type Muzo struct {
	Dir   string
	Files []FileInfo

	fs fs.FS
}

type FileInfo struct {
	Path    string
	Version int
}

func (d *Muzo) ReadFile(filePath string) ([]byte, error) {
	return fs.ReadFile(d.fs, filepath.Join(d.Dir, filePath))
}

func (d *Muzo) Open(filePath string) (fs.File, error) {
	return d.fs.Open(filepath.Join(d.Dir, filePath))
}

// iterMigrationInfo returns an iterator over the migration files.
// It yields slices of file paths grouped by directory, respecting Order and Skip settings.
func (m *Migrate) iterMigrationInfo() iter.Seq2[*Muzo, error] {
	return func(yield func(*Muzo, error) bool) {
		path := m.Path
		if path == "" {
			path = "migrations"
		}

		var fileSystem fs.FS
		if m.FS != nil {
			var err error
			fileSystem, err = fs.Sub(m.FS, path)
			if err != nil {
				yield(nil, err)
				return
			}
		} else {
			fileSystem = os.DirFS(path)
		}

		// Get all directories
		dirs, err := m.getMigrationDirs(fileSystem)
		if err != nil {
			yield(nil, err)
			return
		}

		// Sort directories according to Order preference
		dirs = m.sortDirs(dirs)

		// Iterate over each directory and yield migration files
		for _, dir := range dirs {
			files, err := m.getMigrationFiles(fileSystem, dir)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(&Muzo{
				Dir:   dir,
				Files: files,
				fs:    fileSystem,
			}, nil) {
				return
			}
		}
	}
}

// getMigrationDirs returns all directories in the migration path, excluding skipped ones.
func (m *Migrate) getMigrationDirs(fileSystem fs.FS) ([]string, error) {
	var dirs []string

	err := fs.WalkDir(fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		// Check if this entire directory subtree should be skipped
		if m.shouldSkipDir(path) {
			return fs.SkipDir
		}

		// Check if this specific directory matches a skip pattern
		// (but we still need to walk into it for potential child matches)
		if !m.shouldSkip(path) {
			dirs = append(dirs, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return dirs, nil
}

// sortDirs sorts directories according to the Order preference.
// Directories in Order come first in the specified order, followed by remaining directories alphabetically.
func (m *Migrate) sortDirs(dirs []string) []string {
	if len(m.Order) == 0 {
		slices.Sort(dirs)
		return dirs
	}

	// Create a map for quick lookup of order priority
	orderMap := make(map[string]int)
	for i, o := range m.Order {
		orderMap[strings.TrimPrefix(o, "/")] = i
	}

	slices.SortFunc(dirs, func(a, b string) int {
		aOrder, aHasOrder := orderMap[a]
		bOrder, bHasOrder := orderMap[b]

		if aHasOrder && bHasOrder {
			return aOrder - bOrder
		}
		if aHasOrder {
			return -1
		}
		if bHasOrder {
			return 1
		}
		return strings.Compare(a, b)
	})

	return dirs
}

// getMigrationFiles returns all files in the given directory, sorted alphabetically.
func (m *Migrate) getMigrationFiles(fileSystem fs.FS, dir string) ([]FileInfo, error) {
	entries, err := fs.ReadDir(fileSystem, dir)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Build the full path for skip pattern matching
		fullPath := name
		if dir != "." {
			fullPath = filepath.Join(dir, name)
		}

		// Check if this file should be skipped
		if m.shouldSkip(fullPath) {
			continue
		}

		if m.Extension != "" && !strings.HasSuffix(strings.ToLower(name), strings.ToLower(m.Extension)) {
			continue
		}

		// Only include files that start with a number
		if n, _ := extractLeadingNumber(name); n > 0 {
			files = append(files, FileInfo{
				Path:    name,
				Version: n,
			})
		}
	}

	sortMigrationFiles(files)

	return files, nil
}

// sortMigrationFiles sorts files by their leading number prefix, then alphabetically.
// Files like 001_xx, 01xyz, 1abvc are treated as having the same number (1).
// If no leading number exists, it defaults to 1.
func sortMigrationFiles(files []FileInfo) {
	slices.SortFunc(files, func(a, b FileInfo) int {
		aNum, aName := extractLeadingNumber(filepath.Base(a.Path))
		bNum, bName := extractLeadingNumber(filepath.Base(b.Path))

		if aNum != bNum {
			return aNum - bNum
		}
		return strings.Compare(aName, bName)
	})
}

// extractLeadingNumber extracts the leading number from a filename.
// Returns the number and the original filename for secondary sorting.
// If no leading number exists, returns 0 (for filtering out).
func extractLeadingNumber(filename string) (int, string) {
	var numStr string
	for _, r := range filename {
		if r >= '0' && r <= '9' {
			numStr += string(r)
		} else {
			break
		}
	}

	if numStr == "" {
		return 0, filename
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, filename
	}

	return num, filename
}

// shouldSkip checks if the given path should be skipped based on the skip patterns.
// Supports glob patterns using doublestar syntax:
//   - /test/** matches test directory and all contents recursively
//   - /test/* matches only direct children of test
//   - **/*.sql matches all .sql files in any directory
func (m *Migrate) shouldSkip(path string) bool {
	for _, skip := range m.Skip {
		pattern := strings.TrimPrefix(skip, "/")
		if matched, _ := doublestar.Match(pattern, path); matched {
			return true
		}
	}
	return false
}

// shouldSkipDir checks if a directory should be skipped entirely (including all children).
// This is used during directory walking to skip entire subtrees.
// A directory is fully skipped if:
//   - It matches a pattern like "test" or "test/**" exactly
//   - The pattern doesn't contain wildcards in a way that could match children differently
func (m *Migrate) shouldSkipDir(path string) bool {
	for _, skip := range m.Skip {
		pattern := strings.TrimPrefix(skip, "/")

		// Check for exact directory match (original behavior for backward compatibility)
		if pattern == path {
			return true
		}

		// Check for recursive glob pattern like "test/**"
		// If pattern is "dir/**", we can skip the entire dir
		if strings.HasSuffix(pattern, "/**") {
			basePattern := strings.TrimSuffix(pattern, "/**")
			if path == basePattern || strings.HasPrefix(path, basePattern+"/") {
				return true
			}
		}
	}
	return false
}
