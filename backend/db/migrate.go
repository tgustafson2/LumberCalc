package db

import (
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var files embed.FS

type migration struct {
	version int
	name    string
	sql     string
	sha256  [32]byte
}

type applied struct {
	version int
	name    string
	sha256  [32]byte
}

var embedded = mustLoad(files)

var fileName = regexp.MustCompile(`^(\d{3})_[a-z0-9_]+\.sql$`)

func mustLoad(fsys fs.FS) []migration {
	ms, err := load(fsys)
	if err != nil {
		panic(err)
	}
	return ms
}

func load(fsys fs.FS) ([]migration, error) {
	paths, err := fs.Glob(fsys, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	ms := make([]migration, 0, len(paths))
	for _, p := range paths {
		base := path.Base(p)
		m := fileName.FindStringSubmatch(base)
		if m == nil {
			return nil, fmt.Errorf("migration file %s: want NNN_name.sql", base)
		}
		version, _ := strconv.Atoi(m[1])
		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{
			version: version,
			name:    base[:len(base)-len(".sql")],
			sql:     string(body),
			sha256:  sha256.Sum256(body),
		})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for i, m := range ms {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration %s: want version %03d", m.name, i+1)
		}
	}
	return ms, nil
}

// plan is the single definition of "current". Migrate applies its result and
// CheckApplied requires it to be empty. want is contiguous from 1 and have is
// sorted by version, so a healthy have is a prefix of want.
func plan(want []migration, have []applied) ([]migration, error) {
	for i, h := range have {
		if h.version > len(want) {
			return nil, fmt.Errorf("database has migration %03d %s, this binary knows only up to %03d", h.version, h.name, len(want))
		}
		w := want[i]
		if h.version != w.version {
			return nil, fmt.Errorf("database is missing migration %s but has %03d %s", w.name, h.version, h.name)
		}
		if h.name != w.name || h.sha256 != w.sha256 {
			return nil, fmt.Errorf("migration %s was changed after it was applied as %s", w.name, h.name)
		}
	}
	return want[len(have):], nil
}

// Migrate applies every pending migration in one transaction and returns the
// names it applied. Postgres DDL is transactional, so a failure leaves the
// database as it was. Concurrent runs serialize on the advisory lock, and the
// loser finds nothing pending.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, createLedger); err != nil {
		return nil, err
	}
	have, err := readApplied(ctx, tx)
	if err != nil {
		return nil, err
	}
	pending, err := plan(embedded, have)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(pending))
	for _, m := range pending {
		// No arguments, so pgx uses the simple protocol, which allows many statements.
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			return nil, fmt.Errorf("apply %s: %w", m.name, err)
		}
		if _, err := tx.Exec(ctx, insertApplied, m.version, m.name, m.sha256[:]); err != nil {
			return nil, err
		}
		names = append(names, m.name)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return names, nil
}

var errNotMigrated = errors.New("applied_migrations does not exist, run cmd/migrate")

// CheckApplied returns nil only when the database holds exactly the embedded
// migrations. It is the readyz probe, so the error is for logs only.
func CheckApplied(ctx context.Context, pool *pgxpool.Pool) error {
	have, err := readApplied(ctx, pool)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		return errNotMigrated
	}
	if err != nil {
		return err
	}
	pending, err := plan(embedded, have)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		return fmt.Errorf("%d migrations pending, first is %s", len(pending), pending[0].name)
	}
	return nil
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func readApplied(ctx context.Context, q querier) ([]applied, error) {
	rows, err := q.Query(ctx, "SELECT version, name, sha256 FROM applied_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (applied, error) {
		var a applied
		var sum []byte
		if err := row.Scan(&a.version, &a.name, &sum); err != nil {
			return a, err
		}
		copy(a.sha256[:], sum)
		return a, nil
	})
}

const lockKey int64 = 0x4c756d626572

const createLedger = `
CREATE TABLE IF NOT EXISTS applied_migrations (
    version    integer     PRIMARY KEY,
    name       text        NOT NULL,
    sha256     bytea       NOT NULL CHECK (octet_length(sha256) = 32),
    applied_at timestamptz NOT NULL DEFAULT now()
)`

const insertApplied = `INSERT INTO applied_migrations (version, name, sha256) VALUES ($1, $2, $3)`
