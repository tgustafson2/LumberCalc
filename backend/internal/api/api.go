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

// Deps supplies logging, the session check, the user row, the display name, and design writes.
// New rejects a nil logger or a nil callback.
type Deps struct {
	Log                *slog.Logger
	Authenticate       func(*http.Request) (store.ClerkUserID, error)
	EnsureUser         func(context.Context, store.ClerkUserID) (store.UserID, error)
	DisplayName        func(context.Context, store.ClerkUserID) (string, error)
	PublicName         func(context.Context, store.ClerkUserID) (store.PublicName, error)
	Save               func(context.Context, store.UserID, store.Draft) (store.Design, error)
	Get                func(context.Context, store.UserID, store.DesignID) (store.Design, error)
	List               func(context.Context, store.UserID) ([]store.DesignSummary, error)
	Delete             func(context.Context, store.UserID, store.DesignID) error
	Copy               func(context.Context, store.UserID, store.DesignID) (store.Design, error)
	Publish            func(context.Context, store.UserID, store.DesignID, store.PublicName) (store.PatternID, error)
	Unpublish          func(context.Context, store.UserID, store.DesignID) (store.PatternID, error)
	ListPublicPatterns func(context.Context) ([]store.PatternSummary, error)
	GetPattern         func(context.Context, store.UserID, store.PatternID) (store.Pattern, error)
	AddPattern         func(context.Context, store.UserID, store.PatternID) (store.Design, error)
	CreateMaterial     func(context.Context, store.UserID, store.MaterialInput) (store.Material, error)
	ListMaterials      func(context.Context, store.UserID) ([]store.Material, error)
	UpdateMaterial     func(context.Context, store.UserID, store.MaterialID, store.MaterialInput) (store.Material, error)
	DeleteMaterial     func(context.Context, store.UserID, store.MaterialID) error
	CreatePrice        func(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error)
	ListPrices         func(context.Context, store.UserID) ([]store.Price, error)
	UpdatePrice        func(context.Context, store.UserID, store.PriceID, store.Cents) (store.Price, error)
	DeletePrice        func(context.Context, store.UserID, store.PriceID) error
	GetOptimizeInput   func(context.Context, store.UserID, store.DesignID, store.FlagChoice) (store.OptimizeInput, error)
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
	if d.PublicName == nil {
		return nil, errors.New("public name is required")
	}
	if d.Save == nil {
		return nil, errors.New("save is required")
	}
	if d.Get == nil {
		return nil, errors.New("get is required")
	}
	if d.List == nil {
		return nil, errors.New("list is required")
	}
	if d.Delete == nil {
		return nil, errors.New("delete is required")
	}
	if d.Copy == nil {
		return nil, errors.New("copy is required")
	}
	if d.Publish == nil {
		return nil, errors.New("publish is required")
	}
	if d.Unpublish == nil {
		return nil, errors.New("unpublish is required")
	}
	if d.ListPublicPatterns == nil {
		return nil, errors.New("list public patterns is required")
	}
	if d.GetPattern == nil {
		return nil, errors.New("get pattern is required")
	}
	if d.AddPattern == nil {
		return nil, errors.New("add pattern is required")
	}
	if d.CreateMaterial == nil {
		return nil, errors.New("create material is required")
	}
	if d.ListMaterials == nil {
		return nil, errors.New("list materials is required")
	}
	if d.UpdateMaterial == nil {
		return nil, errors.New("update material is required")
	}
	if d.DeleteMaterial == nil {
		return nil, errors.New("delete material is required")
	}
	if d.CreatePrice == nil {
		return nil, errors.New("create price is required")
	}
	if d.ListPrices == nil {
		return nil, errors.New("list prices is required")
	}
	if d.UpdatePrice == nil {
		return nil, errors.New("update price is required")
	}
	if d.DeletePrice == nil {
		return nil, errors.New("delete price is required")
	}
	if d.GetOptimizeInput == nil {
		return nil, errors.New("get optimize input is required")
	}
	a := &API{d: d, mux: http.NewServeMux()}
	a.handle("GET /v1/me", a.me)
	a.handle("POST /v1/designs", a.createDesign)
	a.handle("GET /v1/designs", a.listDesigns)
	a.handle("GET /v1/designs/{id}", a.getDesign)
	a.handle("PUT /v1/designs/{id}", a.replaceDesign)
	a.handle("DELETE /v1/designs/{id}", a.deleteDesign)
	a.handle("POST /v1/designs/{id}/copy", a.copyDesign)
	a.handle("POST /v1/designs/{id}/publish", a.publishDesign)
	a.handle("POST /v1/designs/{id}/unpublish", a.unpublishDesign)
	a.handle("POST /v1/designs/{id}/optimize", a.optimizeDesign)
	a.handle("GET /v1/patterns", a.listPatterns)
	a.handle("GET /v1/patterns/{id}", a.getPattern)
	a.handle("POST /v1/patterns/{id}/add", a.addPattern)
	a.handle("POST /v1/materials", a.createMaterial)
	a.handle("GET /v1/materials", a.listMaterials)
	a.handle("PUT /v1/materials/{id}", a.replaceMaterial)
	a.handle("DELETE /v1/materials/{id}", a.deleteMaterial)
	a.handle("POST /v1/prices", a.createPrice)
	a.handle("GET /v1/prices", a.listPrices)
	a.handle("PUT /v1/prices/{id}", a.updatePrice)
	a.handle("DELETE /v1/prices/{id}", a.deletePrice)
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
