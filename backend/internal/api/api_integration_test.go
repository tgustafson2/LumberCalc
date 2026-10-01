package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"lumbercalc/backend/internal/dbtest"
	"lumbercalc/backend/internal/store"
)

func TestGetMeReturnsTheUserRowID(t *testing.T) {
	pool := dbtest.Migrated(t)
	const clerk = "user_row_id"
	st := store.New(pool)
	h, err := New(Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticate: func(*http.Request) (store.ClerkUserID, error) {
			return mustClerk(t, clerk), nil
		},
		EnsureUser: st.EnsureUser,
		DisplayName: func(context.Context, store.ClerkUserID) (string, error) {
			return "Ada", nil
		},
		Save:   st.Save,
		Get:    st.Get,
		List:   st.List,
		Delete: st.Delete,
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := call(h, http.MethodGet, "/v1/me", "Bearer token")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body Me
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var rowID string
	err = pool.QueryRow(context.Background(), `SELECT id::text FROM users WHERE clerk_user_id = $1`, clerk).Scan(&rowID)
	if err != nil {
		t.Fatal(err)
	}
	if body.UserId != rowID {
		t.Fatalf("user_id = %s, row = %s", body.UserId, rowID)
	}
	if rowID == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("row id is the zero uuid")
	}
}
