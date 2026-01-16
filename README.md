# muz 🍌

[![License](https://img.shields.io/github/license/rakunlabs/muz?color=red&style=flat-square)](https://raw.githubusercontent.com/rakunlabs/muz/main/LICENSE)
[![Coverage](https://img.shields.io/sonar/coverage/rakunlabs_muz?logo=sonarcloud&server=https%3A%2F%2Fsonarcloud.io&style=flat-square)](https://sonarcloud.io/summary/overall?id=rakunlabs_muz)
[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/rakunlabs/muz/test.yml?branch=main&logo=github&style=flat-square&label=ci)](https://github.com/rakunlabs/muz/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/rakunlabs/muz?style=flat-square)](https://goreportcard.com/report/github.com/rakunlabs/muz)
[![Go PKG](https://raw.githubusercontent.com/rakunlabs/.github/main/assets/badges/gopkg.svg)](https://pkg.go.dev/github.com/rakunlabs/muz)

Simple migration library for golang.

```sh
go get github.com/rakunlabs/muz
```

## Usage

Set a custom driver that implements the `muz.Driver` interface for your storage.  
Predefined drivers:  
__-__ `muz.PostgresDriver` - PostgreSQL

```go
//	"github.com/rakunlabs/muz"
//	_ "github.com/jackc/pgx/v5/stdlib"

// /////////////////////////////////////

//go:embed migrations
var migrationsFS embed.FS

func migration(ctx context.Context) error {
	db, err := sql.Open("pgx", "postgres://user:pass@localhost/dbname")
	if err != nil {
		return err
	}
	defer db.Close()

	m := muz.Migrate{
		Path:      "migrations", // directory inside the FS
		FS:        migrationsFS, // optional: if not set, uses os.DirFS
		Extension: ".sql", // optional: default not set and supports all files
		// Order: []string{"schema", "data"}, // optional: prioritize specific directories
		// Skip:  []string{"/test"},          // optional: skip directories
	}

	driver := &muz.PostgresDriver{
		DB:    db, // *sql.DB instance
		Table: "migrations", // migration tracking table name
		Logger: slog.Default(), // optional: logger instance
	}

	if err := m.Migrate(ctx, driver); err != nil {
		return err
	}

    return nil
}
```

### Migration Files Structure

Migration files should be named with a leading number prefix (e.g., `001_create_users.sql`, `2_add_index.sql`). Files are sorted by their numeric prefix and executed in order.

Example structure:

```
migrations/
├── 1_create_users.sql
├── 2_create_posts.sql
└── schema/
    ├── 1_tables.sql
    └── 2_indexes.sql
```
