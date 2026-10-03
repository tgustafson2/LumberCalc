package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
		Copy:   st.Copy,
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

func TestForeignAndDeletedDesignsMatchAMissingID(t *testing.T) {
	pool := dbtest.Migrated(t)
	st := store.New(pool)
	const (
		ownerClerk  = "user_owner"
		otherClerk  = "user_other"
		putBody     = `{"name":"Bench","version":1,"document":{"schemaVersion":1,"units":"in","pieces":[],"connections":[]}}`
		missingPath = "/v1/designs/00000000-0000-4000-8000-000000000099"
	)
	clerk := ownerClerk
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
		Copy:   st.Copy,
	})
	if err != nil {
		t.Fatal(err)
	}

	sameAs := func(got, want *httptest.ResponseRecorder) {
		t.Helper()
		gotBody := strings.TrimSpace(got.Body.String())
		wantBody := strings.TrimSpace(want.Body.String())
		gotLocation := got.Header().Get("Location")
		wantLocation := want.Header().Get("Location")
		if got.Code != want.Code || gotBody != wantBody || gotLocation != wantLocation {
			t.Fatalf("status = %d body = %s Location = %q, recorded status = %d body = %s Location = %q", got.Code, gotBody, gotLocation, want.Code, wantBody, wantLocation)
		}
	}
	readDesign := func(rr *httptest.ResponseRecorder) Design {
		t.Helper()
		var body Design
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	listIDs := func(rr *httptest.ResponseRecorder) []string {
		t.Helper()
		var rows []DesignSummary
		if err := json.Unmarshal(rr.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		ids := make([]string, len(rows))
		for i, row := range rows {
			ids[i] = row.Id
		}
		return ids
	}
	hasID := func(ids []string, id string) bool {
		for _, got := range ids {
			if got == id {
				return true
			}
		}
		return false
	}

	created := callJSON(h, http.MethodPost, "/v1/designs", `{"name":"Bench"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", created.Code, created.Body.String())
	}
	bench := readDesign(created)
	if bench.Version != 1 {
		t.Fatalf("version = %d, want 1", bench.Version)
	}

	clerk = otherClerk
	created = callJSON(h, http.MethodPost, "/v1/designs", `{"name":"Stool"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", created.Code, created.Body.String())
	}
	stool := readDesign(created)
	if stool.Version != 1 {
		t.Fatalf("version = %d, want 1", stool.Version)
	}

	clerk = ownerClerk
	copied := callJSON(h, http.MethodPost, "/v1/designs/"+bench.Id+"/copy", "")
	if copied.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", copied.Code, copied.Body.String())
	}
	clone := readDesign(copied)

	missing := callJSON(h, http.MethodGet, missingPath, "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", missing.Code, missing.Body.String())
	}
	if strings.TrimSpace(missing.Body.String()) != `{"error":"not found"}` {
		t.Fatalf("body = %s", missing.Body.String())
	}
	if missing.Header().Get("Location") != "" {
		t.Fatalf("Location = %q", missing.Header().Get("Location"))
	}

	clerk = otherClerk
	benchPath := "/v1/designs/" + bench.Id
	sameAs(callJSON(h, http.MethodGet, benchPath, ""), missing)
	sameAs(callJSON(h, http.MethodPut, benchPath, putBody), missing)
	sameAs(callJSON(h, http.MethodDelete, benchPath, ""), missing)
	sameAs(callJSON(h, http.MethodPost, benchPath+"/copy", ""), missing)

	clerk = ownerClerk
	got := callJSON(h, http.MethodGet, benchPath, "")
	if got.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", got.Code, got.Body.String())
	}
	if body := readDesign(got); body.Id != bench.Id || body.Version != 1 {
		t.Fatalf("id = %s version = %d", body.Id, body.Version)
	}

	listed := callJSON(h, http.MethodGet, "/v1/designs", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", listed.Code, listed.Body.String())
	}
	ids := listIDs(listed)
	if len(ids) != 2 || !hasID(ids, bench.Id) || !hasID(ids, clone.Id) || hasID(ids, stool.Id) {
		t.Fatalf("ids = %v", ids)
	}

	clerk = otherClerk
	listed = callJSON(h, http.MethodGet, "/v1/designs", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", listed.Code, listed.Body.String())
	}
	ids = listIDs(listed)
	if len(ids) != 1 || ids[0] != stool.Id {
		t.Fatalf("ids = %v, want [%s]", ids, stool.Id)
	}

	clerk = ownerClerk
	deleted := callJSON(h, http.MethodDelete, benchPath, "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
	sameAs(callJSON(h, http.MethodGet, benchPath, ""), missing)
	sameAs(callJSON(h, http.MethodPut, benchPath, putBody), missing)
	sameAs(callJSON(h, http.MethodDelete, benchPath, ""), missing)
	sameAs(callJSON(h, http.MethodPost, benchPath+"/copy", ""), missing)

	listed = callJSON(h, http.MethodGet, "/v1/designs", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", listed.Code, listed.Body.String())
	}
	ids = listIDs(listed)
	if len(ids) != 1 || !hasID(ids, clone.Id) || hasID(ids, bench.Id) {
		t.Fatalf("ids = %v", ids)
	}

	clerk = otherClerk
	listed = callJSON(h, http.MethodGet, "/v1/designs", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", listed.Code, listed.Body.String())
	}
	ids = listIDs(listed)
	if len(ids) != 1 || ids[0] != stool.Id {
		t.Fatalf("ids = %v, want [%s]", ids, stool.Id)
	}

	clerk = ownerClerk
	sameAs(callJSON(h, http.MethodGet, "/v1/designs/"+stool.Id, ""), missing)
}
