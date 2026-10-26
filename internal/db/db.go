// Package db opens SQLite or PostgreSQL and runs the migrations (goose) at start, so an upgrade is a new image.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB wraps *sql.DB with the dialect, so queries are written once with ? placeholders.
type DB struct {
	*sql.DB
	Postgres bool
}

func Open(url string, postgres bool) (*DB, error) {
	var (
		sqldb *sql.DB
		err   error
	)
	if postgres {
		sqldb, err = sql.Open("pgx", url)
	} else {
		// WAL: readers never wait for the writer; one writer at a time, so a single connection writes.
		sqldb, err = sql.Open("sqlite", "file:"+url+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)")
		if err == nil {
			sqldb.SetMaxOpenConns(1)
		}
	}
	if err != nil {
		return nil, err
	}
	if err := sqldb.Ping(); err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	d := &DB{DB: sqldb, Postgres: postgres}
	goose.SetBaseFS(migrations)
	goose.SetLogger(goose.NopLogger())
	dialect := "sqlite3"
	if postgres {
		dialect = "postgres"
	}
	if err := goose.SetDialect(dialect); err != nil {
		return nil, err
	}
	if err := goose.Up(sqldb, "migrations"); err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return d, nil
}

// Q rewrites ? placeholders to $1, $2 … for PostgreSQL.
func (d *DB) Q(query string) string {
	if !d.Postgres {
		return query
	}
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Querier is *sql.DB or *sql.Tx.
type Querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}
