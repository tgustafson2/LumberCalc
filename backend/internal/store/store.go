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

func (id UserID) String() string {
	var b [36]byte
	hex.Encode(b[0:8], id.v[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], id.v[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], id.v[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], id.v[8:10])
	b[23] = '-'
	hex.Encode(b[24:36], id.v[10:16])
	return string(b[:])
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

type Store struct{ q *q.Queries }

func New(pool *pgxpool.Pool) *Store { return &Store{q: q.New(pool)} }

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
