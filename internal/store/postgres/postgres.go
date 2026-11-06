// Package postgres implements store.Store on PostgreSQL via pgx. The
// repository code in repos_gen.go is generated from store/sqlite/repos.go.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/thomasbudiarjo/envily/internal/store"
	"github.com/thomasbudiarjo/envily/internal/store/migrations"
	"github.com/thomasbudiarjo/envily/internal/store/postgres/db"
)

type DB struct {
	base
	conn *sql.DB
}

var _ store.DB = (*DB)(nil)

// Open connects to dsn (a postgres:// URL). Migrations run when migrate is
// set, under an advisory lock so concurrent instances do not race.
func Open(ctx context.Context, dsn string, migrate bool, log *slog.Logger) (*DB, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(10)
	conn.SetConnMaxIdleTime(5 * time.Minute)
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		// The error may contain the DSN host but pgx never includes the password.
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	if migrate {
		if err := Migrate(ctx, conn, log); err != nil {
			conn.Close()
			return nil, err
		}
	}
	q := db.New(conn)
	d := &DB{conn: conn}
	d.base = base{r: q, w: q, begin: func(ctx context.Context) (*sql.Tx, error) {
		return conn.BeginTx(ctx, nil)
	}}
	return d, nil
}

func Migrate(ctx context.Context, conn *sql.DB, log *slog.Logger) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	opts := []goose.ProviderOption{goose.WithDisableGlobalRegistry(true), goose.WithSessionLocker(locker)}
	if log != nil {
		opts = append(opts, goose.WithSlog(log))
	}
	p, err := goose.NewProvider(goose.DialectPostgres, conn, migrations.FS("postgres"), opts...)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	return nil
}

func (d *DB) Ping(ctx context.Context) error { return d.conn.PingContext(ctx) }
func (d *DB) Close() error                   { return d.conn.Close() }

func (d *DB) Backup(context.Context, string) error {
	return errors.New("backup is only supported for SQLite; use pg_dump for PostgreSQL")
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", store.ErrConflict, pe.ConstraintName)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: referenced row does not exist", store.ErrNotFound)
		}
	}
	return err
}
