package muz

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

//go:embed testdata
var testMigrationsFS embed.FS

var DefaultPostgresImage = "postgres:15-alpine"

type testDB struct {
	db *sql.DB
}

func (tt *testDB) Close() {
	tt.db.Close()
}

func NewTestPostgresDB(t *testing.T) *testDB {
	t.Helper()

	image := DefaultPostgresImage
	if v := os.Getenv("TEST_IMAGE_POSTGRES"); v != "" {
		image = v
	}

	container, err := testcontainers.GenericContainer(t.Context(), testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_HOST_AUTH_METHOD": "trust",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		},
		Started: true,
	})

	if container == nil {
		t.Fatalf("could not create postgres container: %v", err)
	}

	port, err := container.MappedPort(t.Context(), "5432")
	if err != nil {
		t.Fatalf("could not get mapped port: %v", err)
	}

	host, err := container.Host(t.Context())
	if err != nil {
		t.Fatalf("could not get host: %v", err)
	}

	dsn := fmt.Sprintf("postgres://postgres@%s/postgres", net.JoinHostPort(host, port.Port()))
	t.Log("dsn", dsn)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("could not connect to postgres: %v", err)
	}

	if err := db.PingContext(t.Context()); err != nil {
		t.Fatalf("could not ping postgres: %v", err)
	}

	return &testDB{db: db}
}

func TestMuz(t *testing.T) {
	tt := NewTestPostgresDB(t)
	defer tt.Close()

	tt.TestMuz(t)
}

func TestMuzConcurrentWithLockKey(t *testing.T) {
	tt := NewTestPostgresDB(t)
	defer tt.Close()

	tt.TestMuzConcurrentWithLockKey(t)
}

func (tt *testDB) TestMuz(t *testing.T) {
	m := Migrate{
		Path: "testdata",
		FS:   testMigrationsFS,
	}

	driver := &SQLDriver{
		DB:      tt.db,
		Dialect: DialectPostgres,
		Table:   "muz_migrations",
		Logger:  slog.Default(),
	}

	if err := m.Migrate(t.Context(), driver); err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	// Verify that migrations were applied
	var count int
	err := tt.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM muz_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("could not query migrations table: %v", err)
	}

	expectedMigrations := 4 // Total number of migration files in testdata
	if count != expectedMigrations {
		t.Fatalf("expected %d migrations applied, got %d", expectedMigrations, count)
	}

	if err := m.Migrate(t.Context(), driver); err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	// Verify that no new migrations were applied
	var newCount int
	if err := tt.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM muz_migrations").Scan(&newCount); err != nil {
		t.Fatalf("could not query migrations table: %v", err)
	}

	if newCount != count {
		t.Fatalf("expected no new migrations applied, but count changed from %d to %d", count, newCount)
	}
}

func (tt *testDB) TestMuzConcurrentWithLockKey(t *testing.T) {
	m := Migrate{
		Path: "testdata",
		FS:   testMigrationsFS,
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	const workers = 4
	start := make(chan struct{})
	errCh := make(chan error, workers)

	for range workers {
		go func() {
			<-start

			driver := &SQLDriver{
				DB:      tt.db,
				Dialect: DialectPostgres,
				Table:   "muz_migrations_lock",
				LockKey: "muz:test:public:muz_migrations_lock",
			}

			errCh <- m.Migrate(ctx, driver)
		}()
	}

	close(start)

	var errs []error
	for range workers {
		if err := <-errCh; err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		t.Fatalf("concurrent migrations failed: %v", errs)
	}

	var migrationCount int
	if err := tt.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM muz_migrations_lock").Scan(&migrationCount); err != nil {
		t.Fatalf("could not query migrations table: %v", err)
	}

	if migrationCount != 4 {
		t.Fatalf("expected 4 migrations applied, got %d", migrationCount)
	}

	var userCount int
	if err := tt.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		t.Fatalf("could not query users table: %v", err)
	}

	if userCount != 3 {
		t.Fatalf("expected 3 users inserted, got %d", userCount)
	}
}
