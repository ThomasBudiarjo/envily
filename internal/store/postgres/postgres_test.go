package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/thomasbudiarjo/envily/internal/store"
	"github.com/thomasbudiarjo/envily/internal/store/postgres"
	"github.com/thomasbudiarjo/envily/internal/store/storetest"
)

// adminDSN points at a server where tests may create databases. It comes
// from ENVILY_TEST_POSTGRES_DSN, or from a container started by TestMain.
var adminDSN string

// TestMain runs the suite only when ENVILY_TEST_POSTGRES=1 (make test-pg).
// Without Docker the tests are skipped, except in CI where they must run.
func TestMain(m *testing.M) {
	if os.Getenv("ENVILY_TEST_POSTGRES") != "1" {
		os.Exit(m.Run()) // tests skip themselves
	}
	adminDSN = os.Getenv("ENVILY_TEST_POSTGRES_DSN")
	var container *tcpostgres.PostgresContainer
	if adminDSN == "" {
		ctx := context.Background()
		var err error
		container, err = tcpostgres.Run(ctx, "postgres:18-alpine",
			tcpostgres.WithDatabase("envily"), tcpostgres.WithUsername("envily"), tcpostgres.WithPassword("envily"),
			tcpostgres.BasicWaitStrategies())
		if err != nil {
			if os.Getenv("CI") != "" {
				fmt.Fprintln(os.Stderr, "postgres container:", err)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stderr, "skipping PostgreSQL tests: cannot start a container:", err)
			os.Exit(m.Run())
		}
		adminDSN, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	code := m.Run()
	if container != nil {
		_ = testcontainers.TerminateContainer(container)
	}
	os.Exit(code)
}

var dbCounter atomic.Int64

// newDatabase creates an empty database for one test and returns its DSN.
func newDatabase(t *testing.T) string {
	t.Helper()
	if adminDSN == "" {
		t.Skip("PostgreSQL tests need ENVILY_TEST_POSTGRES=1 and Docker or ENVILY_TEST_POSTGRES_DSN")
	}
	ctx := context.Background()
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("envily_test_%d_%d", os.Getpid(), dbCounter.Add(1))
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if a, err := sql.Open("pgx", adminDSN); err == nil {
			_, _ = a.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			a.Close()
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

// open returns a fresh migrated database so tests never share state.
func open(t *testing.T) store.DB {
	t.Helper()
	d, err := postgres.Open(context.Background(), newDatabase(t), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestStore(t *testing.T) { storetest.Run(t, open) }

func TestConcurrentMigrations(t *testing.T) {
	// Several instances starting at once must not race on migrations.
	dsn := newDatabase(t)
	type result struct {
		d   store.DB
		err error
	}
	results := make(chan result, 4)
	for i := 0; i < 4; i++ {
		go func() {
			d, err := postgres.Open(context.Background(), dsn, true, nil)
			results <- result{d, err}
		}()
	}
	var first store.DB
	for i := 0; i < 4; i++ {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		defer r.d.Close()
		first = r.d
	}
	if err := first.Backup(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "pg_dump") {
		t.Fatalf("backup error %v", err)
	}
}
