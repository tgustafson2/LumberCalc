package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lumbercalc/backend/internal/store"
)

const (
	designID     = "00000000-0000-4000-8000-000000000001"
	otherID      = "00000000-0000-4000-8000-000000000002"
	emptyDesign  = `{"description":"","document":{"schemaVersion":1,"units":"in","pieces":[],"connections":[]},"id":"` + designID + `","name":"Bench","updatedAt":"2026-01-02T03:04:05Z","version":1}`
	secondDesign = `{"description":"cut list","document":{"schemaVersion":1,"units":"in","pieces":[],"connections":[]},"id":"` + designID + `","name":"Bench","updatedAt":"2026-01-02T03:04:05Z","version":2}`
)

func TestCreateDesignReturns201(t *testing.T) {
	var saved store.CreateDraft
	h := designHandler(t, func(_ context.Context, _ store.UserID, draft store.Draft) (store.Design, error) {
		created, ok := draft.(store.CreateDraft)
		if !ok {
			t.Fatalf("draft = %T", draft)
		}
		saved = created
		return benchDesign(t, created.Name, created.Description, 1, created.Document), nil
	}, nil, nil, nil)
	rr := callJSON(h, http.MethodPost, "/v1/designs", `{"name":"  Bench  "}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != "/v1/designs/"+designID {
		t.Fatalf("Location = %q", rr.Header().Get("Location"))
	}
	if strings.TrimSpace(rr.Body.String()) != emptyDesign {
		t.Fatalf("body = %s", rr.Body.String())
	}
	if saved.Name.String() != "Bench" || saved.Description != "" {
		t.Fatalf("saved name = %q description = %q", saved.Name.String(), saved.Description)
	}
}

func TestListDesignsReturnsSummaries(t *testing.T) {
	h := designHandler(t, nil, nil, func(context.Context, store.UserID) ([]store.DesignSummary, error) {
		first := benchDesign(t, mustName(t, "Bench"), "cut list", 2, store.Document{})
		second := benchDesign(t, mustName(t, "Stool"), "", 1, store.Document{})
		second.ID = mustDesignID(t, otherID)
		return []store.DesignSummary{first.DesignSummary, second.DesignSummary}, nil
	}, nil)
	rr := callJSON(h, http.MethodGet, "/v1/designs", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	const want = `[{"description":"cut list","id":"` + designID + `","name":"Bench","updatedAt":"2026-01-02T03:04:05Z","version":2},{"description":"","id":"` + otherID + `","name":"Stool","updatedAt":"2026-01-02T03:04:05Z","version":1}]`
	if strings.TrimSpace(rr.Body.String()) != want {
		t.Fatalf("body = %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "document") {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestGetDesignReturnsTheDocument(t *testing.T) {
	h := designHandler(t, nil, func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
		return benchDesign(t, mustName(t, "Bench"), "", 1, store.Document{}), nil
	}, nil, nil)
	rr := callJSON(h, http.MethodGet, "/v1/designs/"+designID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != emptyDesign {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestReplaceDesignIncrementsVersion(t *testing.T) {
	const body = `{"name":"Bench","description":"cut list","document":{"schemaVersion":1,"units":"in","pieces":[],"connections":[]}}`
	h := designHandler(t, func(_ context.Context, _ store.UserID, draft store.Draft) (store.Design, error) {
		replaced, ok := draft.(store.ReplaceDraft)
		if !ok {
			t.Fatalf("draft = %T", draft)
		}
		if replaced.ID.String() != designID {
			t.Fatalf("id = %s", replaced.ID)
		}
		return benchDesign(t, replaced.Name, replaced.Description, 2, replaced.Document), nil
	}, nil, nil, nil)
	rr := callJSON(h, http.MethodPut, "/v1/designs/"+designID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != secondDesign {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestDeleteDesignThenGetReturns404(t *testing.T) {
	deleted := false
	h := designHandler(t, func(context.Context, store.UserID, store.Draft) (store.Design, error) {
		if deleted {
			return store.Design{}, store.ErrNotFound
		}
		t.Fatal("save was called")
		return store.Design{}, nil
	}, func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
		if deleted {
			return store.Design{}, store.ErrNotFound
		}
		return benchDesign(t, mustName(t, "Bench"), "", 1, store.Document{}), nil
	}, nil, func(context.Context, store.UserID, store.DesignID) error {
		if deleted {
			return store.ErrNotFound
		}
		deleted = true
		return nil
	})
	gone := callJSON(h, http.MethodDelete, "/v1/designs/"+designID, "")
	if gone.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", gone.Code, gone.Body.String())
	}
	if strings.TrimSpace(gone.Body.String()) != "" {
		t.Fatalf("body = %s", gone.Body.String())
	}
	assertNotFound(t, callJSON(h, http.MethodGet, "/v1/designs/"+designID, ""))
	assertNotFound(t, callJSON(h, http.MethodPut, "/v1/designs/"+designID, `{"name":"Bench","document":{"schemaVersion":1,"units":"in","pieces":[],"connections":[]}}`))
	assertNotFound(t, callJSON(h, http.MethodDelete, "/v1/designs/"+designID, ""))
}

func TestDesignRejectsABadBody(t *testing.T) {
	var log bytes.Buffer
	h := designHandlerWithLog(t, &log, nil, nil, nil, nil)
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "blank name", method: http.MethodPost, path: "/v1/designs", body: `{"name":"  "}`},
		{name: "bad json", method: http.MethodPost, path: "/v1/designs", body: `{`},
		{name: "unknown key", method: http.MethodPost, path: "/v1/designs", body: `{"name":"Bench","extra":1}`},
		{name: "bad document", method: http.MethodPost, path: "/v1/designs", body: `{"name":"Bench","document":{}}`},
		{name: "null description", method: http.MethodPost, path: "/v1/designs", body: `{"name":"Bench","description":null}`},
		{name: "missing document", method: http.MethodPut, path: "/v1/designs/" + designID, body: `{"name":"Bench"}`},
		{name: "body too large", method: http.MethodPost, path: "/v1/designs", body: `{"name":"` + strings.Repeat("a", maxDesignBody) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := callJSON(h, tt.method, tt.path, tt.body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
			}
			if strings.TrimSpace(rr.Body.String()) != `{"error":"bad request"}` {
				t.Fatalf("body = %s", rr.Body.String())
			}
		})
	}
	if !strings.Contains(log.String(), "/schemaVersion") {
		t.Fatalf("log = %s", log.String())
	}
}

func TestDesignUnknownMaterialReturns409(t *testing.T) {
	h := designHandler(t, func(context.Context, store.UserID, store.Draft) (store.Design, error) {
		return store.Design{}, store.ErrMaterialMissing
	}, nil, nil, nil)
	rr := callJSON(h, http.MethodPost, "/v1/designs", `{"name":"Bench"}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != `{"error":"conflict"}` {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestDesignForeignIDReturns404(t *testing.T) {
	var got store.DesignID
	h := designHandler(t, nil, func(_ context.Context, _ store.UserID, id store.DesignID) (store.Design, error) {
		got = id
		return store.Design{}, store.ErrNotFound
	}, nil, nil)
	rr := callJSON(h, http.MethodGet, "/v1/designs/"+otherID, "")
	assertNotFound(t, rr)
	if got.String() != otherID {
		t.Fatalf("id = %s", got)
	}
	assertNotFound(t, callJSON(h, http.MethodGet, "/v1/designs/not-a-uuid", ""))
}

func designHandler(
	t *testing.T,
	save func(context.Context, store.UserID, store.Draft) (store.Design, error),
	get func(context.Context, store.UserID, store.DesignID) (store.Design, error),
	list func(context.Context, store.UserID) ([]store.DesignSummary, error),
	del func(context.Context, store.UserID, store.DesignID) error,
) http.Handler {
	t.Helper()
	return designHandlerWithLog(t, nil, save, get, list, del)
}

func designHandlerWithLog(
	t *testing.T,
	logBuf *bytes.Buffer,
	save func(context.Context, store.UserID, store.Draft) (store.Design, error),
	get func(context.Context, store.UserID, store.DesignID) (store.Design, error),
	list func(context.Context, store.UserID) ([]store.DesignSummary, error),
	del func(context.Context, store.UserID, store.DesignID) error,
) http.Handler {
	t.Helper()
	w := io.Writer(io.Discard)
	if logBuf != nil {
		w = logBuf
	}
	if save == nil {
		save = func(context.Context, store.UserID, store.Draft) (store.Design, error) {
			t.Fatal("save was called")
			return store.Design{}, nil
		}
	}
	if get == nil {
		get = func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
			t.Fatal("get was called")
			return store.Design{}, nil
		}
	}
	if list == nil {
		list = func(context.Context, store.UserID) ([]store.DesignSummary, error) {
			t.Fatal("list was called")
			return nil, nil
		}
	}
	if del == nil {
		del = func(context.Context, store.UserID, store.DesignID) error {
			t.Fatal("delete was called")
			return nil
		}
	}
	h, err := New(Deps{
		Log:          slog.New(slog.NewTextHandler(w, nil)),
		Authenticate: okAuth,
		EnsureUser:   okEnsure,
		DisplayName:  okName,
		Save:         save,
		Get:          get,
		List:         list,
		Delete:       del,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func benchDesign(t *testing.T, name store.Name, description string, version int32, doc store.Document) store.Design {
	t.Helper()
	return store.Design{
		DesignSummary: store.DesignSummary{
			ID:          mustDesignID(t, designID),
			Name:        name,
			Description: description,
			Version:     version,
			UpdatedAt:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		Document: doc,
	}
}

func mustName(t *testing.T, raw string) store.Name {
	t.Helper()
	name, err := store.ParseName(raw)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func mustDesignID(t *testing.T, raw string) store.DesignID {
	t.Helper()
	id, err := store.ParseDesignID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func callJSON(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}
