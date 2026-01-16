package muz

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestIterMigrationInfo(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, tempDir string)
		migrate   func(tempDir string) *Migrate
		want      []Muzo
		wantError bool
	}{
		{
			name: "basic single directory with files",
			setup: func(t *testing.T, tempDir string) {
				dir := filepath.Join(tempDir, "001_init")
				mustMkdir(t, dir)
				mustCreateFile(t, filepath.Join(dir, "001_create_users.sql"))
				mustCreateFile(t, filepath.Join(dir, "002_create_posts.sql"))
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{Path: tempDir}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "001_init", Files: []FileInfo{
					{Path: "001_create_users.sql", Version: 1},
					{Path: "002_create_posts.sql", Version: 2},
				}},
			},
		},
		{
			name: "multiple directories sorted alphabetically",
			setup: func(t *testing.T, tempDir string) {
				dirs := []string{"002_second", "001_first", "003_third"}
				for _, d := range dirs {
					dir := filepath.Join(tempDir, d)
					mustMkdir(t, dir)
					mustCreateFile(t, filepath.Join(dir, "001_migration.sql"))
				}
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{Path: tempDir}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "001_first", Files: []FileInfo{{Path: "001_migration.sql", Version: 1}}},
				{Dir: "002_second", Files: []FileInfo{{Path: "001_migration.sql", Version: 1}}},
				{Dir: "003_third", Files: []FileInfo{{Path: "001_migration.sql", Version: 1}}},
			},
		},
		{
			name: "custom order priority",
			setup: func(t *testing.T, tempDir string) {
				dirs := []string{"alpha", "beta", "gamma"}
				for _, d := range dirs {
					dir := filepath.Join(tempDir, d)
					mustMkdir(t, dir)
					mustCreateFile(t, filepath.Join(dir, "001_migration.sql"))
				}
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{
					Path:  tempDir,
					Order: []string{"gamma", "alpha"},
				}
			},
			want: []Muzo{
				{Dir: "gamma", Files: []FileInfo{{Path: "001_migration.sql", Version: 1}}},
				{Dir: "alpha", Files: []FileInfo{{Path: "001_migration.sql", Version: 1}}},
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "beta", Files: []FileInfo{{Path: "001_migration.sql", Version: 1}}},
			},
		},
		{
			name: "skip directories",
			setup: func(t *testing.T, tempDir string) {
				dirs := []string{"keep1", "skip_me", "keep2"}
				for _, d := range dirs {
					dir := filepath.Join(tempDir, d)
					mustMkdir(t, dir)
					mustCreateFile(t, filepath.Join(dir, "001_migration.sql"))
				}
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{
					Path: tempDir,
					Skip: []string{"/skip_me"},
				}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "keep1", Files: []FileInfo{{Path: "001_migration.sql", Version: 1}}},
				{Dir: "keep2", Files: []FileInfo{{Path: "001_migration.sql", Version: 1}}},
			},
		},
		{
			name: "extension filter",
			setup: func(t *testing.T, tempDir string) {
				dir := filepath.Join(tempDir, "migrations")
				mustMkdir(t, dir)
				mustCreateFile(t, filepath.Join(dir, "001_valid.sql"))
				mustCreateFile(t, filepath.Join(dir, "002_invalid.txt"))
				mustCreateFile(t, filepath.Join(dir, "003_also_valid.sql"))
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{
					Path:      tempDir,
					Extension: ".sql",
				}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "migrations", Files: []FileInfo{{Path: "001_valid.sql", Version: 1}, {Path: "003_also_valid.sql", Version: 3}}},
			},
		},
		{
			name: "files without leading number are excluded",
			setup: func(t *testing.T, tempDir string) {
				dir := filepath.Join(tempDir, "migrations")
				mustMkdir(t, dir)
				mustCreateFile(t, filepath.Join(dir, "001_valid.sql"))
				mustCreateFile(t, filepath.Join(dir, "readme.txt"))
				mustCreateFile(t, filepath.Join(dir, "no_number.sql"))
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{Path: tempDir}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "migrations", Files: []FileInfo{{Path: "001_valid.sql", Version: 1}}},
			},
		},
		{
			name: "files sorted by leading number",
			setup: func(t *testing.T, tempDir string) {
				dir := filepath.Join(tempDir, "migrations")
				mustMkdir(t, dir)
				mustCreateFile(t, filepath.Join(dir, "10_tenth.sql"))
				mustCreateFile(t, filepath.Join(dir, "2_second.sql"))
				mustCreateFile(t, filepath.Join(dir, "1_first.sql"))
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{Path: tempDir}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "migrations", Files: []FileInfo{{Path: "1_first.sql", Version: 1}, {Path: "2_second.sql", Version: 2}, {Path: "10_tenth.sql", Version: 10}}},
			},
		},
		{
			name: "nested directories",
			setup: func(t *testing.T, tempDir string) {
				parent := filepath.Join(tempDir, "parent")
				child := filepath.Join(parent, "child")
				mustMkdir(t, parent)
				mustMkdir(t, child)
				mustCreateFile(t, filepath.Join(parent, "001_parent.sql"))
				mustCreateFile(t, filepath.Join(child, "001_child.sql"))
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{Path: tempDir}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "parent", Files: []FileInfo{{Path: "001_parent.sql", Version: 1}}},
				{Dir: "parent/child", Files: []FileInfo{{Path: "001_child.sql", Version: 1}}},
			},
		},
		{
			name: "empty directory returns no files",
			setup: func(t *testing.T, tempDir string) {
				dir := filepath.Join(tempDir, "empty")
				mustMkdir(t, dir)
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{Path: tempDir}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "empty", Files: []FileInfo{}},
			},
		},
		{
			name: "root level files are included",
			setup: func(t *testing.T, tempDir string) {
				// Just create root-level files, no subdirectories
				mustCreateFile(t, filepath.Join(tempDir, "001_root.sql"))
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{Path: tempDir}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{{Path: "001_root.sql", Version: 1}}},
			},
		},
		{
			name: "skip nested directory and its children",
			setup: func(t *testing.T, tempDir string) {
				parent := filepath.Join(tempDir, "skip_parent")
				child := filepath.Join(parent, "child")
				keep := filepath.Join(tempDir, "keep")
				mustMkdir(t, parent)
				mustMkdir(t, child)
				mustMkdir(t, keep)
				mustCreateFile(t, filepath.Join(parent, "001_skip.sql"))
				mustCreateFile(t, filepath.Join(child, "001_skip_child.sql"))
				mustCreateFile(t, filepath.Join(keep, "001_keep.sql"))
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{
					Path: tempDir,
					Skip: []string{"/skip_parent"},
				}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "keep", Files: []FileInfo{{Path: "001_keep.sql", Version: 1}}},
			},
		},
		{
			name: "same number files sorted alphabetically",
			setup: func(t *testing.T, tempDir string) {
				dir := filepath.Join(tempDir, "migrations")
				mustMkdir(t, dir)
				mustCreateFile(t, filepath.Join(dir, "001_zebra.sql"))
				mustCreateFile(t, filepath.Join(dir, "001_alpha.sql"))
				mustCreateFile(t, filepath.Join(dir, "001_beta.sql"))
			},
			migrate: func(tempDir string) *Migrate {
				return &Migrate{Path: tempDir}
			},
			want: []Muzo{
				{Dir: ".", Files: []FileInfo{}},
				{Dir: "migrations", Files: []FileInfo{{Path: "001_alpha.sql", Version: 1}, {Path: "001_beta.sql", Version: 1}, {Path: "001_zebra.sql", Version: 1}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tt.setup(t, tempDir)

			m := tt.migrate(tempDir)

			var got []Muzo
			var gotError error

			for info, err := range m.iterMigrationInfo() {
				if err != nil {
					gotError = err
					break
				}
				got = append(got, *info)
			}

			if tt.wantError {
				if gotError == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if gotError != nil {
				t.Errorf("unexpected error: %v", gotError)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("got %d results, want %d", len(got), len(tt.want))
				t.Errorf("got: %+v", got)
				t.Errorf("want: %+v", tt.want)
				return
			}

			for i := range got {
				if got[i].Dir != tt.want[i].Dir {
					t.Errorf("result[%d].Dir = %q, want %q", i, got[i].Dir, tt.want[i].Dir)
				}
				if !slices.Equal(got[i].Files, tt.want[i].Files) {
					t.Errorf("result[%d].Files = %v, want %v", i, got[i].Files, tt.want[i].Files)
				}
			}
		})
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("failed to create directory %s: %v", path, err)
	}
}

func mustCreateFile(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file %s: %v", path, err)
	}
	f.Close()
}
