package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lumbercalc/backend/internal/api"
	"lumbercalc/backend/internal/clerkauth"
	"lumbercalc/backend/internal/server"
	"lumbercalc/backend/internal/store"
)

func TestHealthAndReadyStayPublicWhileV1RequiresASession(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	v1, err := api.New(api.Deps{
		Log: log,
		Authenticate: func(*http.Request) (store.ClerkUserID, error) {
			return store.ClerkUserID{}, clerkauth.ErrUnauthenticated
		},
		EnsureUser: func(context.Context, store.ClerkUserID) (store.UserID, error) {
			return store.UserID{}, nil
		},
		DisplayName: func(context.Context, store.ClerkUserID) (string, error) {
			return "", nil
		},
		Save: func(context.Context, store.UserID, store.Draft) (store.Design, error) {
			return store.Design{}, store.ErrNotFound
		},
		Get: func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
			return store.Design{}, store.ErrNotFound
		},
		List: func(context.Context, store.UserID) ([]store.DesignSummary, error) {
			return nil, store.ErrNotFound
		},
		Delete: func(context.Context, store.UserID, store.DesignID) error {
			return store.ErrNotFound
		},
		Copy: func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
			return store.Design{}, store.ErrNotFound
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := server.New(server.Deps{
		Log: log,
		Ready: []server.Check{{
			Name:  "ok",
			Probe: func(context.Context) error { return nil },
		}},
		V1: v1,
	})
	if err != nil {
		t.Fatal(err)
	}

	health := httptest.NewRecorder()
	h.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, body = %s", health.Code, health.Body.String())
	}

	ready := httptest.NewRecorder()
	h.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("readyz status = %d, body = %s", ready.Code, ready.Body.String())
	}

	me := httptest.NewRecorder()
	h.ServeHTTP(me, httptest.NewRequest(http.MethodGet, "/v1/me", nil))
	if me.Code != http.StatusUnauthorized {
		t.Fatalf("me status = %d, body = %s", me.Code, me.Body.String())
	}
	if strings.TrimSpace(me.Body.String()) != `{"error":"unauthorized"}` {
		t.Fatalf("me body = %s", me.Body.String())
	}
}
