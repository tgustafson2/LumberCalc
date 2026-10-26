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
		PublicName: func(context.Context, store.ClerkUserID) (store.PublicName, error) {
			return store.PublicName{}, nil
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
		Publish: func(context.Context, store.UserID, store.DesignID, store.PublicName) (store.PatternID, error) {
			return store.PatternID{}, store.ErrNotFound
		},
		Unpublish: func(context.Context, store.UserID, store.DesignID) (store.PatternID, error) {
			return store.PatternID{}, store.ErrNotFound
		},
		ListPublicPatterns: func(context.Context) ([]store.PatternSummary, error) {
			return []store.PatternSummary{}, nil
		},
		GetPattern: func(context.Context, store.UserID, store.PatternID) (store.Pattern, error) {
			return store.Pattern{}, store.ErrPatternNotFound
		},
		AddPattern: func(context.Context, store.UserID, store.PatternID) (store.Design, error) {
			return store.Design{}, store.ErrPatternNotFound
		},
		CreateMaterial: func(context.Context, store.UserID, store.MaterialInput) (store.Material, error) {
			return store.Material{}, store.ErrMaterialNotFound
		},
		ListMaterials: func(context.Context, store.UserID) ([]store.Material, error) {
			return nil, store.ErrMaterialNotFound
		},
		UpdateMaterial: func(context.Context, store.UserID, store.MaterialID, store.MaterialInput) (store.Material, error) {
			return store.Material{}, store.ErrMaterialNotFound
		},
		DeleteMaterial: func(context.Context, store.UserID, store.MaterialID) error {
			return store.ErrMaterialNotFound
		},
		CreatePrice: func(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error) {
			return store.Price{}, store.ErrPriceNotFound
		},
		ListPrices: func(context.Context, store.UserID) ([]store.Price, error) {
			return nil, store.ErrPriceNotFound
		},
		UpdatePrice: func(context.Context, store.UserID, store.PriceID, store.Cents) (store.Price, error) {
			return store.Price{}, store.ErrPriceNotFound
		},
		DeletePrice: func(context.Context, store.UserID, store.PriceID) error {
			return store.ErrPriceNotFound
		},
		GetOptimizeInput: func(context.Context, store.UserID, store.DesignID, store.FlagChoice) (store.OptimizeInput, error) {
			t.Fatal("get optimize input was called")
			return store.OptimizeInput{}, nil
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
