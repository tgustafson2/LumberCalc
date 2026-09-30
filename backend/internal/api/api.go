// Package api serves the authenticated /v1 routes.
// A path under /v1 checks the session before the handler runs, including paths with no route.
package api

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../openapi.yaml

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"lumbercalc/backend/internal/clerkauth"
	"lumbercalc/backend/internal/server"
	"lumbercalc/backend/internal/store"
)

// Caller is the signed-in user for one request.
// handle builds one only after Authenticate and EnsureUser both succeed.
type Caller struct {
	clerk store.ClerkUserID
	user  store.UserID
}

func (c Caller) ClerkID() store.ClerkUserID { return c.clerk }
func (c Caller) UserID() store.UserID       { return c.user }

// Deps supplies logging, the session check, the user row, and the display name.
// New rejects a nil logger or a nil callback.
type Deps struct {
	Log          *slog.Logger
	Authenticate func(*http.Request) (store.ClerkUserID, error)
	EnsureUser   func(context.Context, store.ClerkUserID) (store.UserID, error)
	DisplayName  func(context.Context, store.ClerkUserID) (string, error)
}

// API registers authenticated /v1 routes. New returns it as an http.Handler.
type API struct {
	d   Deps
	mux *http.ServeMux
}

type authedHandler func(http.ResponseWriter, *http.Request, Caller)

// New returns the /v1 handler.
// clerkauth.ErrUnauthenticated is 401 with a Bearer challenge.
// Any other error from Authenticate or EnsureUser is 500, and the response body omits the cause.
// GET /v1/me returns DisplayName, or the Clerk subject when that name is blank or the lookup fails.
// An unknown /v1 path with no session is 401. The same path with a session is 404.
func New(d Deps) (http.Handler, error) {
	if d.Log == nil {
		return nil, errors.New("log is required")
	}
	if d.Authenticate == nil {
		return nil, errors.New("authenticate is required")
	}
	if d.EnsureUser == nil {
		return nil, errors.New("ensure user is required")
	}
	if d.DisplayName == nil {
		return nil, errors.New("display name is required")
	}
	a := &API{d: d, mux: http.NewServeMux()}
	a.handle("GET /v1/me", a.me)
	a.handle("/v1/", a.notFound)
	return a.mux, nil
}

func (a *API) handle(pattern string, h authedHandler) {
	a.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		clerkID, err := a.d.Authenticate(r)
		if err != nil {
			if errors.Is(err, clerkauth.ErrUnauthenticated) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				server.WriteError(w, http.StatusUnauthorized)
				return
			}
			a.d.Log.ErrorContext(r.Context(), "authenticate")
			server.WriteError(w, http.StatusInternalServerError)
			return
		}
		userID, err := a.d.EnsureUser(r.Context(), clerkID)
		if err != nil {
			a.d.Log.ErrorContext(r.Context(), "ensure_user", "err", err)
			server.WriteError(w, http.StatusInternalServerError)
			return
		}
		h(w, r, Caller{clerk: clerkID, user: userID})
	})
}

// me reports the session user. It ignores any other identity in the request.
func (a *API) me(w http.ResponseWriter, r *http.Request, c Caller) {
	name, err := a.d.DisplayName(r.Context(), c.ClerkID())
	if err != nil {
		a.d.Log.ErrorContext(r.Context(), "display_name", "err", err)
		name = ""
	}
	if name == "" {
		name = c.ClerkID().String()
	}
	server.WriteJSON(w, http.StatusOK, Me{
		UserId:      c.UserID().String(),
		ClerkUserId: c.ClerkID().String(),
		DisplayName: name,
	})
}

func (a *API) notFound(w http.ResponseWriter, _ *http.Request, _ Caller) {
	server.WriteError(w, http.StatusNotFound)
}
