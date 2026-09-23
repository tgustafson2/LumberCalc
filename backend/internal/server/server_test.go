package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthzIgnoresFailingProbe(t *testing.T) {
	h, err := New(Deps{
		Log: NewLogger(io.Discard, slog.LevelInfo),
		Ready: []Check{{
			Name: "postgres",
			Probe: func(context.Context) error {
				return errors.New("should not be called")
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	want := `{"status":"ok"}`
	got := strings.TrimSpace(rr.Body.String())
	if got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestReadyzOK(t *testing.T) {
	h, err := New(Deps{
		Log: NewLogger(io.Discard, slog.LevelInfo),
		Ready: []Check{{
			Name:  "postgres",
			Probe: func(context.Context) error { return nil },
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	want := `{"status":"ready","checks":[{"name":"postgres","ok":true}]}`
	got := strings.TrimSpace(rr.Body.String())
	if got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestReadyzNotReadyOmitsError(t *testing.T) {
	h, err := New(Deps{
		Log: NewLogger(io.Discard, slog.LevelInfo),
		Ready: []Check{{
			Name: "postgres",
			Probe: func(context.Context) error {
				return errors.New("password=secret-from-driver")
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rr.Code)
	}
	want := `{"status":"not_ready","checks":[{"name":"postgres","ok":false}]}`
	got := strings.TrimSpace(rr.Body.String())
	if got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
	if strings.Contains(got, "secret-from-driver") {
		t.Fatalf("body leaked probe error: %s", got)
	}
}

func TestRequestIDMinted(t *testing.T) {
	h, err := New(Deps{
		Log: NewLogger(io.Discard, slog.LevelInfo),
		Ready: []Check{{
			Name:  "postgres",
			Probe: func(context.Context) error { return nil },
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-Id", "client-picked")
	h.ServeHTTP(rr, req)
	got := rr.Header().Get("X-Request-Id")
	if got == "" {
		t.Fatal("missing X-Request-Id")
	}
	if got == "client-picked" {
		t.Fatalf("used inbound id %q", got)
	}
}

func TestNewRejectsEmptyReady(t *testing.T) {
	_, err := New(Deps{
		Log:   NewLogger(io.Discard, slog.LevelInfo),
		Ready: nil,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestV1KeepsFullPath(t *testing.T) {
	var got string
	h, err := New(Deps{
		Log: NewLogger(io.Discard, slog.LevelInfo),
		Ready: []Check{{
			Name:  "postgres",
			Probe: func(context.Context) error { return nil },
		}},
		V1: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rr.Code)
	}
	if got != "/v1/me" {
		t.Fatalf("path = %q, want /v1/me", got)
	}
}

func TestAccessLogIncludesRequestID(t *testing.T) {
	var buf bytes.Buffer
	h, err := New(Deps{
		Log: NewLogger(&buf, slog.LevelInfo),
		Ready: []Check{{
			Name:  "postgres",
			Probe: func(context.Context) error { return nil },
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(rr, req)
	id := rr.Header().Get("X-Request-Id")
	if id == "" {
		t.Fatal("missing X-Request-Id")
	}
	line := buf.String()
	if !strings.Contains(line, "request_id") {
		t.Fatalf("access log missing request_id: %s", line)
	}
	if !strings.Contains(line, id) {
		t.Fatalf("access log missing id %q: %s", id, line)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &payload); err != nil {
		t.Fatalf("access log not JSON: %v (%s)", err, line)
	}
}
