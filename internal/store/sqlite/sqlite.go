// Package sqlite implements store.Store on SQLite (modernc.org/sqlite, no CGO).
package sqlite

//go:generate sh -c "sed -e 's#internal/store/sqlite/db#internal/store/postgres/db#' -e 's/^package sqlite$/package postgres/' -e '1s#^#// Code generated from store/sqlite/repos.go by go generate; DO NOT EDIT.\\n\\n#' repos.go > ../postgres/repos_gen.go"

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/pressly/goose/v3"
	sqlite3 "modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"

	"github.com/thomasbudiarjo/envily/internal/store"
	"github.com/thomasbudiarjo/envily/internal/store/migrations"
	"github.com/thomasbudiarjo/envily/internal/store/sqlite/db"
)

// DB is an open SQLite database with a single-connection write pool and a
// multi-connection read pool.
type DB struct {
	base
	writer, reader *sql.DB
}

var _ store.DB = (*DB)(nil)

const pragmas = "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"

// dsn turns a file path (or a file: URI) into a DSN with Envily's pragmas.
func dsn(path string, readOnly bool) string {
	if !strings.HasPrefix(path, "file:") {
		path = "file:" + (&url.URL{Path: path}).EscapedPath()
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	d := path + sep + pragmas
	if readOnly {
		d += "&mode=ro"
	}
	return d
}

// Open opens the database at path. Migrations run when migrate is set.
func Open(ctx context.Context, path string, migrate bool, log *slog.Logger) (*DB, error) {
	// Create a new database file readable by its owner only.
	if !strings.HasPrefix(path, "file:") {
		f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o600) //nolint:gosec // operator-configured path
		if err != nil {
			return nil, fmt.Errorf("open sqlite database: %w", err)
		}
		f.Close()
	}
	writer, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	writer.SetConnMaxLifetime(0)
	if err := writer.PingContext(ctx); err != nil {
		writer.Close()
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if migrate {
		if err := Migrate(ctx, writer, log); err != nil {
			writer.Close()
			return nil, err
		}
	}
	reader, err := sql.Open("sqlite", dsn(path, true))
	if err != nil {
		writer.Close()
		return nil, err
	}
	reader.SetMaxOpenConns(max(4, runtime.NumCPU()))
	d := &DB{writer: writer, reader: reader}
	d.base = base{
		r: db.New(reader),
		w: db.New(writer),
		begin: func(ctx context.Context) (*sql.Tx, error) {
			return writer.BeginTx(ctx, nil)
		},
	}
	return d, nil
}

// Migrate applies all pending migrations.
func Migrate(ctx context.Context, conn *sql.DB, log *slog.Logger) error {
	opts := []goose.ProviderOption{goose.WithDisableGlobalRegistry(true)}
	if log != nil {
		opts = append(opts, goose.WithSlog(log))
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, conn, migrations.FS("sqlite"), opts...)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	return nil
}

func (d *DB) Ping(ctx context.Context) error {
	if err := d.writer.PingContext(ctx); err != nil {
		return err
	}
	return d.reader.PingContext(ctx)
}

func (d *DB) Close() error {
	return errors.Join(d.reader.Close(), d.writer.Close())
}

// Backup writes a consistent snapshot of the database to path, which must
// not exist yet.
func (d *DB) Backup(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	if _, err := d.writer.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	return os.Chmod(path, 0o600)
}

// mapErr converts driver errors to store errors.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	var se *sqlite3.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case sqlitelib.SQLITE_CONSTRAINT_UNIQUE, sqlitelib.SQLITE_CONSTRAINT_PRIMARYKEY:
			return fmt.Errorf("%w: %w", store.ErrConflict, err)
		case sqlitelib.SQLITE_CONSTRAINT_FOREIGNKEY:
			return fmt.Errorf("%w: referenced row does not exist", store.ErrNotFound)
		}
	}
	return err
}
