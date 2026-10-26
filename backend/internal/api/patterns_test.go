package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"lumbercalc/backend/internal/store"
)

const patternID = "00000000-0000-4000-8000-0000000000aa"

func TestListPatternsReturnsAnEmptyArray(t *testing.T) {
	h := patternHandler(t, func(context.Context) ([]store.PatternSummary, error) {
		return []store.PatternSummary{}, nil
	}, nil)
	rr := callJSON(h, http.MethodGet, "/v1/patterns", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != "[]" {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestListPatternsWritesANullPublisherName(t *testing.T) {
	subject := mustClerk(t, "user_123")
	h := patternHandler(t, func(context.Context) ([]store.PatternSummary, error) {
		return []store.PatternSummary{
			samplePatternSummary(t, store.ParsePublicName("Ada Lovelace", subject)),
			samplePatternSummary(t, store.PublicName{}),
		}, nil
	}, nil)
	rr := callJSON(h, http.MethodGet, "/v1/patterns", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	want := `[` +
		`{"description":"","id":"` + patternID + `","name":"Bench","publishedAt":"2026-01-02T03:04:05Z","publisherName":"Ada Lovelace"},` +
		`{"description":"","id":"` + patternID + `","name":"Bench","publishedAt":"2026-01-02T03:04:05Z","publisherName":null}` +
		`]`
	if strings.TrimSpace(rr.Body.String()) != want {
		t.Fatalf("body = %s, want %s", rr.Body.String(), want)
	}
}

func TestGetPatternPassesTheCallerAndTheID(t *testing.T) {
	h := patternHandler(t, nil, func(_ context.Context, _ store.UserID, id store.PatternID) (store.Pattern, error) {
		if id.String() != patternID {
			t.Fatalf("id = %s", id)
		}
		return store.Pattern{PatternSummary: samplePatternSummary(t, store.PublicName{}), Public: false}, nil
	})
	rr := callJSON(h, http.MethodGet, "/v1/patterns/"+patternID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"isPublic":false`) || !strings.Contains(body, `"publisherName":null`) || !strings.Contains(body, `"document":{"schemaVersion":1`) {
		t.Fatalf("body = %s", body)
	}
}

func TestGetPatternReturns404ForAHiddenOrMalformedID(t *testing.T) {
	h := patternHandler(t, nil, func(context.Context, store.UserID, store.PatternID) (store.Pattern, error) {
		return store.Pattern{}, store.ErrPatternNotFound
	})
	assertNotFound(t, callJSON(h, http.MethodGet, "/v1/patterns/"+patternID, ""))

	h = patternHandler(t, nil, func(context.Context, store.UserID, store.PatternID) (store.Pattern, error) {
		t.Fatal("get pattern was called")
		return store.Pattern{}, nil
	})
	assertNotFound(t, callJSON(h, http.MethodGet, "/v1/patterns/not-a-uuid", ""))
}

func patternHandler(
	t *testing.T,
	list func(context.Context) ([]store.PatternSummary, error),
	get func(context.Context, store.UserID, store.PatternID) (store.Pattern, error),
) http.Handler {
	t.Helper()
	if list == nil {
		list = func(context.Context) ([]store.PatternSummary, error) {
			t.Fatal("list patterns was called")
			return nil, nil
		}
	}
	if get == nil {
		get = func(context.Context, store.UserID, store.PatternID) (store.Pattern, error) {
			t.Fatal("get pattern was called")
			return store.Pattern{}, nil
		}
	}
	h, err := New(Deps{
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticate: okAuth,
		EnsureUser:   okEnsure,
		DisplayName:  okName,
		PublicName:   idlePublicName,
		Save: func(context.Context, store.UserID, store.Draft) (store.Design, error) {
			return store.Design{}, store.ErrNotFound
		},
		Get: func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
			return store.Design{}, store.ErrNotFound
		},
		List: func(context.Context, store.UserID) ([]store.DesignSummary, error) {
			return []store.DesignSummary{}, nil
		},
		Delete: func(context.Context, store.UserID, store.DesignID) error {
			return store.ErrNotFound
		},
		Copy: func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
			return store.Design{}, store.ErrNotFound
		},
		Publish: func(context.Context, store.UserID, store.DesignID, store.PublicName) (store.PatternID, error) {
			return store.PatternID{}, store.ErrNotFound
		},
		Unpublish: func(context.Context, store.UserID, store.DesignID) (store.PatternID, error) {
			return store.PatternID{}, store.ErrNotFound
		},
		ListPublicPatterns: list,
		GetPattern:         get,
		CreateMaterial:     idleCreateMaterial,
		ListMaterials:      idleListMaterials,
		UpdateMaterial:     idleUpdateMaterial,
		DeleteMaterial:     idleDeleteMaterial,
		CreatePrice:        idleCreatePrice,
		ListPrices:         idleListPrices,
		UpdatePrice:        idleUpdatePrice,
		DeletePrice:        idleDeletePrice,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func samplePatternSummary(t *testing.T, publisher store.PublicName) store.PatternSummary {
	t.Helper()
	id, err := store.ParsePatternID(patternID)
	if err != nil {
		t.Fatal(err)
	}
	return store.PatternSummary{
		ID:          id,
		Name:        mustName(t, "Bench"),
		Publisher:   publisher,
		PublishedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func idlePublicName(context.Context, store.ClerkUserID) (store.PublicName, error) {
	return store.PublicName{}, nil
}

func idleListPublicPatterns(context.Context) ([]store.PatternSummary, error) {
	return []store.PatternSummary{}, nil
}

func idleGetPattern(context.Context, store.UserID, store.PatternID) (store.Pattern, error) {
	return store.Pattern{}, store.ErrPatternNotFound
}
