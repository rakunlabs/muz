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

// Dialect represents the SQL dialect for different database systems.
type Dialect int

const (
	// DialectPostgres is the PostgreSQL dialect.
	DialectPostgres Dialect = iota
	// DialectMySQL is the MySQL dialect.
	DialectMySQL
	// DialectSQLite is the SQLite dialect.
	DialectSQLite
	// DialectMSSQL is the Microsoft SQL Server dialect.
	DialectMSSQL
)

// SQLDriver is a generic database driver that supports multiple SQL dialects.
type SQLDriver struct {
	// DB is the database connection to use for migrations.
	DB *sql.DB
	// Dialect specifies which SQL dialect to use.
	Dialect Dialect
	// Table is the name of the migration tracking table.
	Table string
	// Logger if set, used to log migration progress.
	Logger Logger

	// tx is the current transaction, if any.
	tx *sql.Tx
}

func (d *SQLDriver) tableName() string {
	if d.Table == "" {
		return "migrations"
	}

	return d.Table
}

// placeholder returns the appropriate placeholder for the given index (1-based).
func (d *SQLDriver) placeholder(index int) string {
	switch d.Dialect {
	case DialectPostgres:
		return fmt.Sprintf("$%d", index)
	case DialectMSSQL:
		return fmt.Sprintf("@p%d", index)
	default: // MySQL, SQLite use ?
		return "?"
	}
}

// createTableSQL returns the CREATE TABLE statement for the migration tracking table.
func (d *SQLDriver) createTableSQL() string {
	switch d.Dialect {
	case DialectPostgres:
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				version integer NOT NULL,
				directory text NOT NULL,
				file_name text NOT NULL,
				processed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
				UNIQUE(version, directory)
			)
		`, d.tableName())
	case DialectMySQL:
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				version int NOT NULL,
				directory varchar(255) NOT NULL,
				file_name varchar(255) NOT NULL,
				processed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
				UNIQUE KEY unique_version_directory (version, directory)
			)
		`, d.tableName())
	case DialectSQLite:
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				version integer NOT NULL,
				directory text NOT NULL,
				file_name text NOT NULL,
				processed_at datetime DEFAULT CURRENT_TIMESTAMP NOT NULL,
				UNIQUE(version, directory)
			)
		`, d.tableName())
	case DialectMSSQL:
		return fmt.Sprintf(`
			IF NOT EXISTS (SELECT * FROM sysobjects WHERE name='%s' AND xtype='U')
			CREATE TABLE %s (
				version int NOT NULL,
				directory nvarchar(255) NOT NULL,
				file_name nvarchar(255) NOT NULL,
				processed_at datetime2 DEFAULT GETDATE() NOT NULL,
				CONSTRAINT UQ_%s_version_directory UNIQUE(version, directory)
			)
		`, d.tableName(), d.tableName(), d.tableName())
	default:
		return ""
	}
}

func (d *SQLDriver) Start(ctx context.Context) error {
	var err error
	d.tx, err = d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	query := d.createTableSQL()

	if d.Logger != nil {
		d.Logger.Info("starting migration", "table", d.tableName(), "dialect", d.Dialect)
	}

	_, err = d.tx.ExecContext(ctx, query)
	return err
}

func (d *SQLDriver) Process(ctx context.Context, data *Muzo) error {
	directory := data.Dir
	version := 0

	// Get latest applied version for the directory
	query := fmt.Sprintf(`
		SELECT MAX(version) FROM %s WHERE directory = %s
	`, d.tableName(), d.placeholder(1))

	row := d.tx.QueryRowContext(ctx, query, directory)
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

		if d.Logger != nil {
			d.Logger.Info("applying migration", "version", file.Version, "directory", directory, "file", file.Path)
		}

		// Execute migration SQL
		if _, err := d.tx.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("applying migration %d - %s - %s: %w", file.Version, directory, file.Path, err)
		}

		// Record applied migration
		insertQuery := fmt.Sprintf(`
			INSERT INTO %s (version, directory, file_name)
			VALUES (%s, %s, %s)
		`, d.tableName(), d.placeholder(1), d.placeholder(2), d.placeholder(3))

		if _, err := d.tx.ExecContext(ctx, insertQuery, file.Version, directory, file.Path); err != nil {
			return err
		}

		version = file.Version
	}

	return nil
}

func (d *SQLDriver) End(ctx context.Context, err error) error {
	if d.tx != nil {
		if err != nil {
			return d.tx.Rollback()
		}

		if d.Logger != nil {
			d.Logger.Info("migrations applied successfully")
		}

		return d.tx.Commit()
	}

	return nil
}

// //////////////////////////////

// NewPostgresDriver creates a new SQLDriver configured for PostgreSQL.
func NewPostgresDriver(db *sql.DB, table string, logger Logger) *SQLDriver {
	return &SQLDriver{
		DB:      db,
		Dialect: DialectPostgres,
		Table:   table,
		Logger:  logger,
	}
}

// NewMySQLDriver creates a new SQLDriver configured for MySQL.
func NewMySQLDriver(db *sql.DB, table string, logger Logger) *SQLDriver {
	return &SQLDriver{
		DB:      db,
		Dialect: DialectMySQL,
		Table:   table,
		Logger:  logger,
	}
}

// NewSQLiteDriver creates a new SQLDriver configured for SQLite.
func NewSQLiteDriver(db *sql.DB, table string, logger Logger) *SQLDriver {
	return &SQLDriver{
		DB:      db,
		Dialect: DialectSQLite,
		Table:   table,
		Logger:  logger,
	}
}

// NewMSSQLDriver creates a new SQLDriver configured for Microsoft SQL Server.
func NewMSSQLDriver(db *sql.DB, table string, logger Logger) *SQLDriver {
	return &SQLDriver{
		DB:      db,
		Dialect: DialectMSSQL,
		Table:   table,
		Logger:  logger,
	}
}

// //////////////////////////////
