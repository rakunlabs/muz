package muz

import (
	"context"
	"fmt"
	"io/fs"
	"os"
)

type status string

const (
	StatusStart   status = "start"
	StatusProcess status = "process"
	StatusEnd     status = "end"
)

type Muzo struct {
	FilePath string `cfg:"filename" json:"filename"`

	embedPath fs.FS `cfg:"-" json:"-"`
}

func (d *Muzo) ReadFile() ([]byte, error) {
	if d.embedPath != nil {
		return fs.ReadFile(d.embedPath, d.FilePath)
	}

	return os.ReadFile(d.FilePath)
}

func (d *Muzo) Open() (fs.File, error) {
	if d.embedPath != nil {
		return d.embedPath.Open(d.FilePath)
	}

	return os.Open(d.FilePath)
}

// /////////////////////////////////

type Migrate struct {
	// Path to the directory containing migration files.
	//  - Default: "./migrations"
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

func (m *Migrate) setDefaults() error {
	if m.Path == "" {
		m.Path = "./migrations"
	}

	return nil
}

func (m *Migrate) Migrate(ctx context.Context, apply func(ctx context.Context, status status, data *Muzo) error) error {
	if err := m.setDefaults(); err != nil {
		return err
	}

	if err := apply(ctx, StatusStart, nil); err != nil {
		return fmt.Errorf("migrate start: %w", err)
	}

	for info, err := range m.iterMigrationInfo() {
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		for _, file := range info.Files {
			data := &Muzo{
				FilePath:  file,
				embedPath: m.EmbedPath,
			}

			if err := apply(ctx, StatusProcess, data); err != nil {
				return err
			}
		}
	}

	if err := apply(ctx, StatusEnd, nil); err != nil {
		return fmt.Errorf("migrate end: %w", err)
	}

	return nil
}
