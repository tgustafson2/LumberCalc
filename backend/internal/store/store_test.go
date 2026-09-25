package store_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/internal/dbtest"
	"lumbercalc/backend/internal/store"
)

func TestParseClerkUserID(t *testing.T) {
	tests := []struct {
		raw string
		err error
	}{
		{raw: "user_2abcDEF", err: nil},
		{raw: "sess-shaped|subject", err: nil},
		{raw: strings.Repeat("x", 64), err: nil},
		{raw: "", err: store.ErrEmptyClerkUserID},
		{raw: strings.Repeat("x", 65), err: store.ErrClerkUserIDTooLong},
	}
	for _, tt := range tests {
		got, err := store.ParseClerkUserID(tt.raw)
		if !errors.Is(err, tt.err) {
			t.Fatalf("ParseClerkUserID(%q) err = %v, want %v", tt.raw, err, tt.err)
		}
		if err == nil && got.String() != tt.raw {
			t.Fatalf("ParseClerkUserID(%q) = %q", tt.raw, got.String())
		}
	}
}

type row struct {
	userID   string
	users    int
	settings int
	useKerf  bool
	kerfIn   string
	basis    string
}

func lookup(t *testing.T, pool *pgxpool.Pool, clerk string) row {
	t.Helper()
	var r row
	err := pool.QueryRow(context.Background(), `
		SELECT
			coalesce((SELECT id::text FROM users WHERE clerk_user_id = $1), ''),
			(SELECT count(*) FROM users WHERE clerk_user_id = $1),
			(SELECT count(*) FROM user_settings s JOIN users u ON u.id = s.user_id WHERE u.clerk_user_id = $1),
			coalesce((SELECT default_use_kerf FROM user_settings s JOIN users u ON u.id = s.user_id WHERE u.clerk_user_id = $1), false),
			coalesce((SELECT default_kerf_in::text FROM user_settings s JOIN users u ON u.id = s.user_id WHERE u.clerk_user_id = $1), ''),
			coalesce((SELECT default_dimension_basis FROM user_settings s JOIN users u ON u.id = s.user_id WHERE u.clerk_user_id = $1), '')
	`, clerk).Scan(&r.userID, &r.users, &r.settings, &r.useKerf, &r.kerfIn, &r.basis)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// warm opens every pool connection up front. Otherwise dialing serializes the
// callers and the first one commits before the rest reach the insert.
func warm(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	conns := make([]*pgxpool.Conn, pool.Config().MaxConns)
	for i := range conns {
		c, err := pool.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		conns[i] = c
	}
	for _, c := range conns {
		c.Release()
	}
}

func TestEnsureUserConcurrentFirstRequests(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	clerk, err := store.ParseClerkUserID("user_2abc")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	warm(t, pool)

	ids := make([]store.UserID, 32)
	errs := make([]error, 32)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ids[i], errs[i] = s.EnsureUser(ctx, clerk)
		}(i)
	}
	close(start)
	wg.Wait()

	got := lookup(t, pool, "user_2abc")
	want := row{userID: got.userID, users: 1, settings: 1, useKerf: false, kerfIn: "0.125", basis: "nominal"}
	if got != want {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
	if len(got.userID) != 36 {
		t.Fatalf("users.id = %q, want a uuid", got.userID)
	}
	for i := range ids {
		if errs[i] != nil {
			t.Fatalf("call %d: %v", i, errs[i])
		}
		if ids[i].String() != got.userID {
			t.Fatalf("call %d returned %s, users.id is %s", i, ids[i], got.userID)
		}
	}

	again, err := s.EnsureUser(ctx, clerk)
	if err != nil {
		t.Fatal(err)
	}
	if again.String() != got.userID {
		t.Fatalf("second call returned %s, want %s", again, got.userID)
	}
	if after := lookup(t, pool, "user_2abc"); after != want {
		t.Fatalf("rows after second call = %+v, want %+v", after, want)
	}
}

func TestEnsureUserHealsMissingSettings(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	clerk, _ := store.ParseClerkUserID("user_orphan")
	ctx := context.Background()

	var id string
	if err := pool.QueryRow(ctx, "INSERT INTO users (clerk_user_id) VALUES ('user_orphan') RETURNING id::text").Scan(&id); err != nil {
		t.Fatal(err)
	}

	got, err := s.EnsureUser(ctx, clerk)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != id {
		t.Fatalf("EnsureUser = %s, want existing %s", got, id)
	}
	want := row{userID: id, users: 1, settings: 1, useKerf: false, kerfIn: "0.125", basis: "nominal"}
	if r := lookup(t, pool, "user_orphan"); r != want {
		t.Fatalf("rows = %+v, want %+v", r, want)
	}
}

func TestEnsureUserDistinctSubjects(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	a, _ := store.ParseClerkUserID("user_a")
	b, _ := store.ParseClerkUserID("user_b")

	idA, err := s.EnsureUser(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	idB, err := s.EnsureUser(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if idA == idB {
		t.Fatalf("user_a and user_b share id %s", idA)
	}
	if r := lookup(t, pool, "user_a"); r.userID != idA.String() {
		t.Fatalf("user_a users.id = %s, EnsureUser = %s", r.userID, idA)
	}
	if r := lookup(t, pool, "user_b"); r.userID != idB.String() {
		t.Fatalf("user_b users.id = %s, EnsureUser = %s", r.userID, idB)
	}
}
