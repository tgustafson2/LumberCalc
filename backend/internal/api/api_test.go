package api

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

	"lumbercalc/backend/internal/clerkauth"
	"lumbercalc/backend/internal/store"
)

func TestGetMeWithoutASessionReturns401(t *testing.T) {
	h := newHandler(t, func(r *http.Request) (store.ClerkUserID, error) {
		if r.Header.Get("Authorization") == "" {
			return store.ClerkUserID{}, clerkauth.ErrUnauthenticated
		}
		t.Fatal("authenticate called with a header")
		return store.ClerkUserID{}, nil
	}, okEnsure, okName)
	rr := call(h, http.MethodGet, "/v1/me", "")
	assertUnauthorized(t, rr)
}

func TestGetMeWithARejectedSessionReturns401(t *testing.T) {
	h := newHandler(t, func(*http.Request) (store.ClerkUserID, error) {
		return store.ClerkUserID{}, clerkauth.ErrUnauthenticated
	}, okEnsure, okName)
	rr := call(h, http.MethodGet, "/v1/me", "Bearer token")
	assertUnauthorized(t, rr)
}

func TestGetMeReturns500WhenTheSessionCheckFailsForAServerReason(t *testing.T) {
	const sdk = "synthetic-sdk-failure"
	h := newHandler(t, func(*http.Request) (store.ClerkUserID, error) {
		return store.ClerkUserID{}, errors.New(sdk)
	}, okEnsure, okName)
	rr := call(h, http.MethodGet, "/v1/me", "Bearer token")
	assertInternal(t, rr, sdk)
}

func TestGetMeReturns500WhenSavingTheCallerFails(t *testing.T) {
	const dbErr = "synthetic-db-failure"
	var log bytes.Buffer
	nameCalled := false
	h := newHandlerWithLog(t, &log, func(*http.Request) (store.ClerkUserID, error) {
		return mustClerk(t, "user_123"), nil
	}, func(context.Context, store.ClerkUserID) (store.UserID, error) {
		return store.UserID{}, errors.New(dbErr)
	}, func(context.Context, store.ClerkUserID) (string, error) {
		nameCalled = true
		return "Ada", nil
	})
	rr := call(h, http.MethodGet, "/v1/me", "Bearer token")
	assertInternal(t, rr, dbErr)
	if nameCalled {
		t.Fatal("display name was called")
	}
	if !strings.Contains(log.String(), "ensure_user") || !strings.Contains(log.String(), dbErr) {
		t.Fatalf("log = %s", log.String())
	}
}

func TestGetMeReturnsTheCallerFromTheSession(t *testing.T) {
	h := newHandler(t, func(*http.Request) (store.ClerkUserID, error) {
		return mustClerk(t, "user_123"), nil
	}, func(context.Context, store.ClerkUserID) (store.UserID, error) {
		return store.UserID{}, nil
	}, func(context.Context, store.ClerkUserID) (string, error) {
		return "Ada Lovelace", nil
	})
	rr := call(h, http.MethodGet, "/v1/me", "Bearer token")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body Me
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.UserId != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("user_id = %s", body.UserId)
	}
	if body.ClerkUserId != "user_123" {
		t.Fatalf("clerk_user_id = %s", body.ClerkUserId)
	}
	if body.DisplayName != "Ada Lovelace" {
		t.Fatalf("display_name = %s", body.DisplayName)
	}
}

func TestGetMeActsOnTheSessionUserWhenTheRequestNamesSomeoneElse(t *testing.T) {
	const sessionUser = "user_alice"
	const otherUser = "user_bob"
	var saved store.ClerkUserID
	var named store.ClerkUserID
	h := newHandler(t, func(*http.Request) (store.ClerkUserID, error) {
		return mustClerk(t, sessionUser), nil
	}, func(_ context.Context, id store.ClerkUserID) (store.UserID, error) {
		saved = id
		return store.UserID{}, nil
	}, func(_ context.Context, id store.ClerkUserID) (string, error) {
		named = id
		return "Ada", nil
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/me?user_id="+otherUser, strings.NewReader(`{"user_id":"`+otherUser+`","clerk_user_id":"`+otherUser+`"}`))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if saved.String() != sessionUser {
		t.Fatalf("saved clerk id = %s, want %s", saved.String(), sessionUser)
	}
	if named.String() != sessionUser {
		t.Fatalf("name lookup clerk id = %s, want %s", named.String(), sessionUser)
	}
	var body Me
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ClerkUserId != sessionUser || strings.Contains(body.ClerkUserId, otherUser) {
		t.Fatalf("body clerk_user_id = %s", body.ClerkUserId)
	}
	if strings.Contains(rr.Body.String(), otherUser) {
		t.Fatalf("body included %s: %s", otherUser, rr.Body.String())
	}
}

func TestGetMeFallsBackToTheClerkIDWhenTheNameLookupFails(t *testing.T) {
	const lookupErr = "name lookup failed"
	var log bytes.Buffer
	h := newHandlerWithLog(t, &log, func(*http.Request) (store.ClerkUserID, error) {
		return mustClerk(t, "user_123"), nil
	}, okEnsure, func(context.Context, store.ClerkUserID) (string, error) {
		return "", errors.New(lookupErr)
	})
	rr := call(h, http.MethodGet, "/v1/me", "Bearer token")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body Me
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.DisplayName != "user_123" {
		t.Fatalf("display_name = %s", body.DisplayName)
	}
	if strings.Contains(rr.Body.String(), lookupErr) {
		t.Fatalf("body = %s", rr.Body.String())
	}
	if !strings.Contains(log.String(), "display_name") || !strings.Contains(log.String(), lookupErr) {
		t.Fatalf("log = %s", log.String())
	}
}

func TestGetMeFallsBackToTheClerkIDWhenTheNameIsBlank(t *testing.T) {
	h := newHandler(t, func(*http.Request) (store.ClerkUserID, error) {
		return mustClerk(t, "user_123"), nil
	}, okEnsure, func(context.Context, store.ClerkUserID) (string, error) {
		return "", nil
	})
	rr := call(h, http.MethodGet, "/v1/me", "Bearer token")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body Me
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.DisplayName != "user_123" {
		t.Fatalf("display_name = %s", body.DisplayName)
	}
}

func TestUnknownV1RouteWithoutASessionReturns401(t *testing.T) {
	h := newHandler(t, func(r *http.Request) (store.ClerkUserID, error) {
		if r.Header.Get("Authorization") == "" {
			return store.ClerkUserID{}, clerkauth.ErrUnauthenticated
		}
		t.Fatal("authenticate called with a header")
		return store.ClerkUserID{}, nil
	}, okEnsure, okName)
	rr := call(h, http.MethodGet, "/v1/nope", "")
	assertUnauthorized(t, rr)
}

func TestUnknownV1RouteWithASessionReturns404(t *testing.T) {
	h := newHandler(t, okAuth, okEnsure, okName)
	rr := call(h, http.MethodGet, "/v1/nope", "Bearer token")
	assertNotFound(t, rr)
}

func TestPostMeWithASessionReturns404(t *testing.T) {
	h := newHandler(t, okAuth, okEnsure, okName)
	rr := call(h, http.MethodPost, "/v1/me", "Bearer token")
	assertNotFound(t, rr)
}

func newHandler(
	t *testing.T,
	auth func(*http.Request) (store.ClerkUserID, error),
	ensure func(context.Context, store.ClerkUserID) (store.UserID, error),
	name func(context.Context, store.ClerkUserID) (string, error),
) http.Handler {
	t.Helper()
	return newHandlerWithLog(t, nil, auth, ensure, name)
}

func newHandlerWithLog(
	t *testing.T,
	logBuf *bytes.Buffer,
	auth func(*http.Request) (store.ClerkUserID, error),
	ensure func(context.Context, store.ClerkUserID) (store.UserID, error),
	name func(context.Context, store.ClerkUserID) (string, error),
) http.Handler {
	t.Helper()
	w := io.Writer(io.Discard)
	if logBuf != nil {
		w = logBuf
	}
	h, err := New(Deps{
		Log:          slog.New(slog.NewTextHandler(w, nil)),
		Authenticate: auth,
		EnsureUser:   ensure,
		DisplayName:  name,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func okAuth(*http.Request) (store.ClerkUserID, error) {
	id, err := store.ParseClerkUserID("user_123")
	if err != nil {
		panic(err)
	}
	return id, nil
}

func okEnsure(context.Context, store.ClerkUserID) (store.UserID, error) {
	return store.UserID{}, nil
}

func okName(context.Context, store.ClerkUserID) (string, error) {
	return "Ada", nil
}

func mustClerk(t *testing.T, raw string) store.ClerkUserID {
	t.Helper()
	id, err := store.ParseClerkUserID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func call(h http.Handler, method, path, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func assertUnauthorized(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q", rr.Header().Get("WWW-Authenticate"))
	}
	if strings.TrimSpace(rr.Body.String()) != `{"error":"unauthorized"}` {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func assertInternal(t *testing.T, rr *httptest.ResponseRecorder, absent string) {
	t.Helper()
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := strings.TrimSpace(rr.Body.String())
	if body != `{"error":"internal server error"}` {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, absent) {
		t.Fatalf("body leaked %q", absent)
	}
}

func assertNotFound(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != `{"error":"not found"}` {
		t.Fatalf("body = %s", rr.Body.String())
	}
}
