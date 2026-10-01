package store

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/internal/store/internal/q"
)

// UserID is the internal users.id. Only the store can build one, from a row it read.
type UserID struct{ v [16]byte }

func (id UserID) String() string { return formatUUID(id.v) }

func formatUUID(v [16]byte) string {
	var b [36]byte
	hex.Encode(b[0:8], v[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], v[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], v[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], v[8:10])
	b[23] = '-'
	hex.Encode(b[24:36], v[10:16])
	return string(b[:])
}

func decodeUUID(raw string, dst *[16]byte) error {
	compact := make([]byte, 0, 32)
	compact = append(compact, raw[0:8]...)
	compact = append(compact, raw[9:13]...)
	compact = append(compact, raw[14:18]...)
	compact = append(compact, raw[19:23]...)
	compact = append(compact, raw[24:36]...)
	_, err := hex.Decode(dst[:], compact)
	return err
}

// ClerkUserID is a verified Clerk subject. The only way to get one is ParseClerkUserID.
type ClerkUserID struct{ s string }

func (c ClerkUserID) String() string { return c.s }

const maxClerkUserIDLen = 64

var (
	ErrEmptyClerkUserID   = errors.New("clerk user id is empty")
	ErrClerkUserIDTooLong = errors.New("clerk user id is longer than 64 bytes")
)

// ParseClerkUserID accepts the sub claim of a verified Clerk JWT.
func ParseClerkUserID(raw string) (ClerkUserID, error) {
	if raw == "" {
		return ClerkUserID{}, ErrEmptyClerkUserID
	}
	if len(raw) > maxClerkUserIDLen {
		return ClerkUserID{}, ErrClerkUserIDTooLong
	}
	return ClerkUserID{s: raw}, nil
}

type Store struct {
	pool *pgxpool.Pool
	q    *q.Queries
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, q: q.New(pool)} }

// withTx runs fn in one transaction.
// Save uses it so the design row and its usage rows commit together.
func (s *Store) withTx(ctx context.Context, fn func(*q.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EnsureUser returns the internal id for a Clerk user and guarantees exactly one
// user_settings row with schema defaults. Safe under concurrent first requests
// for the same subject. The steady state is one read.
func (s *Store) EnsureUser(ctx context.Context, clerk ClerkUserID) (UserID, error) {
	id, err := s.q.FindUser(ctx, clerk.s)
	if errors.Is(err, pgx.ErrNoRows) {
		id, err = s.q.CreateUser(ctx, clerk.s)
	}
	if err != nil {
		return UserID{}, err
	}
	return fromPG(id), nil
}

func fromPG(u pgtype.UUID) UserID { return UserID{v: u.Bytes} }
