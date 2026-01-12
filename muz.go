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
	// EmbedPath if set, use this embedded filesystem instead of reading from Path.
	EmbedPath fs.FS `cfg:"-" json:"-"`

	// Order of directory names to apply migrations from.
	//  - Default: []string{}
	//  - If empty, all directories are applied in alphabetical order.
	//  - If set, give priority to the listed directories in the specified order.
	//    Directories not listed will be applied afterwards in alphabetical order.
	Order []string `cfg:"order" json:"order"`
	// Skip directories to ignore during migration.
	//  - Default: []string{}
	//  - Directories listed here will be skipped entirely.
	//  - Should be given /test/dir1 format, relative to the migration path.
	Skip []string `cfg:"skip" json:"skip"`

	// Extension of migration files.
	//  - Default: none (all files are considered)
	//  - Only files with this extension will be considered as migration files.
	Extension string `cfg:"extension" json:"extension"`
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
