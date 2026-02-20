package muz

import (
	"context"
	"io/fs"
	"iter"
)

// /////////////////////////////////

type Migrate struct {
	// Path to the directory containing migration files.
	//  - Default: "migrations"
	Path string `cfg:"path" json:"path"`
	// FS if set, use this embedded filesystem instead of reading from Path.
	FS fs.FS `cfg:"-" json:"-"`

	// Order of directory names to apply migrations from.
	//  - Default: []string{}
	//  - If empty, all directories are applied in alphabetical order.
	//  - If set, give priority to the listed directories in the specified order.
	//    Directories not listed will be applied afterwards in alphabetical order.
	Order []string `cfg:"order" json:"order"`
	// Skip patterns to ignore during migration (supports glob patterns).
	//  - Default: []string{}
	//  - Supports glob patterns using doublestar syntax:
	//    - /test/** matches test directory and all contents recursively
	//    - /test/* matches only direct children of test (files and immediate subdirectories)
	//    - **/*.sql matches all .sql files in any directory
	//  - Can skip both files and directories.
	//  - Paths should be given in /test/dir1 format, relative to the migration path.
	Skip []string `cfg:"skip" json:"skip"`

	// Extension of migration files.
	//  - Default: none (all files are considered)
	//  - Only files with this extension will be considered as migration files.
	Extension string `cfg:"extension" json:"extension"`

	// Values is a map of key-value pairs for template substitution in migration files.
	//  - Default: nil (no substitution is performed)
	//  - When set, file contents read via ReadFile will have $KEY or ${KEY} references
	//    expanded using os.Expand with only these values.
	//  - Real environment variables are NOT expanded; only keys present in this map are resolved.
	//  - Unknown keys expand to an empty string.
	Values map[string]string `cfg:"values" json:"values"`
}

func (m Migrate) Migrations() iter.Seq2[*Muzo, error] {
	return m.iterMigrationInfo()
}

func (m Migrate) Migrate(ctx context.Context, driver Driver) (err error) {
	if err := driver.Start(ctx); err != nil {
		return err
	}

	defer driver.End(ctx, err)

	for info, err := range m.Migrations() {
		if err != nil {
			return err
		}

		if err := driver.Process(ctx, info); err != nil {
			return err
		}
	}

	return nil
}
