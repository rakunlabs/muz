package muz

import (
	"context"
	"database/sql"
	"fmt"
)

type Driver interface {
	Start(ctx context.Context) error
	Process(ctx context.Context, data *Muzo) error
	End(ctx context.Context, err error) error
}

// //////////////////////////////

type PostgresDriver struct {
	// DB is the database connection to use for migrations.
	DB *sql.DB
	// TableName is the name of the migration tracking table.
	TableName string

	// tx is the current transaction, if any.
	tx *sql.Tx
}

func (p *PostgresDriver) tableName() string {
	if p.TableName == "" {
		return "migrations"
	}

	return p.TableName
}

func (p *PostgresDriver) Start(ctx context.Context) error {
	var err error
	p.tx, err = p.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version integer NOT NULL,
			directory text NOT NULL,
			file_name text NOT NULL,
			processed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
			UNIQUE(version, directory)
		)
	`, p.tableName())

	_, err = p.tx.ExecContext(ctx, query)
	return err
}

func (p *PostgresDriver) Process(ctx context.Context, data *Muzo) error {
	directory := data.Dir
	version := 0

	// Get latest applied version for the directory
	query := fmt.Sprintf(`
		SELECT MAX(version) FROM %s WHERE directory = $1
	`, p.tableName())

	row := p.tx.QueryRowContext(ctx, query, directory)
	var latestVersion sql.NullInt64
	if err := row.Scan(&latestVersion); err != nil {
		return err
	}
	if latestVersion.Valid {
		version = int(latestVersion.Int64)
	}

	// Apply migrations in order
	for _, file := range data.Files {
		if file.Version <= version {
			continue // already applied
		}

		content, err := data.ReadFile(file.Path)
		if err != nil {
			return err
		}

		// Execute migration SQL
		if _, err := p.tx.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("applying migration %s: %w", file.Path, err)
		}

		// Record applied migration
		if _, err := p.tx.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO %s (version, directory, file_name)
			VALUES ($1, $2, $3)
		`, p.tableName()), file.Version, directory, file.Path); err != nil {
			return err
		}

		version = file.Version
	}

	return nil
}

func (p *PostgresDriver) End(ctx context.Context, err error) error {
	if p.tx != nil {
		if err != nil {
			return p.tx.Rollback()
		}

		return p.tx.Commit()
	}

	return nil
}

// //////////////////////////////
