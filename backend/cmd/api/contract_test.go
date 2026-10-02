package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"lumbercalc/backend/internal/api"
	"lumbercalc/backend/internal/clerkauth"
	"lumbercalc/backend/internal/server"
	"lumbercalc/backend/internal/store"
)

type exchange struct {
	name         string
	checks       []server.Check
	auth         func(*http.Request) (store.ClerkUserID, error)
	ensure       func(context.Context, store.ClerkUserID) (store.UserID, error)
	display      func(context.Context, store.ClerkUserID) (string, error)
	path         string
	body         string
	validRequest bool
	status       int
	save         func(context.Context, store.UserID, store.Draft) (store.Design, error)
	get          func(context.Context, store.UserID, store.DesignID) (store.Design, error)
	list         func(context.Context, store.UserID) ([]store.DesignSummary, error)
	delete       func(context.Context, store.UserID, store.DesignID) error
	copy         func(context.Context, store.UserID, store.DesignID) (store.Design, error)
}

var exchanges = map[string][]exchange{
	"getHealth": {{
		name: "ok",
		checks: []server.Check{{
			Name:  "postgres",
			Probe: func(context.Context) error { return errProbe },
		}},
	}},
	"getReadiness": {
		{
			name: "ready",
			checks: []server.Check{{
				Name:  "postgres",
				Probe: func(context.Context) error { return nil },
			}},
		},
		{
			name: "a probe fails",
			checks: []server.Check{{
				Name:  "postgres",
				Probe: func(context.Context) error { return errProbe },
			}},
		},
	},
	"getMe": {
		{
			name:    "session",
			auth:    func(*http.Request) (store.ClerkUserID, error) { return sessionClerk, nil },
			display: func(context.Context, store.ClerkUserID) (string, error) { return "Ada", nil },
		},
		{
			name: "no session",
			auth: func(*http.Request) (store.ClerkUserID, error) {
				return store.ClerkUserID{}, clerkauth.ErrUnauthenticated
			},
		},
		{
			name: "jwks down",
			auth: func(*http.Request) (store.ClerkUserID, error) {
				return store.ClerkUserID{}, errProbe
			},
		},
	},
	"createDesign": {
		{
			name:         "created",
			path:         "/v1/designs",
			body:         `{"name":"Bench"}`,
			validRequest: true,
			status:       http.StatusCreated,
			auth:         func(*http.Request) (store.ClerkUserID, error) { return sessionClerk, nil },
			save: func(context.Context, store.UserID, store.Draft) (store.Design, error) {
				return sampleDesign(1), nil
			},
		},
		{
			name:   "blank name",
			path:   "/v1/designs",
			body:   `{"name":""}`,
			status: http.StatusBadRequest,
			auth:   func(*http.Request) (store.ClerkUserID, error) { return sessionClerk, nil },
		},
		{
			name:         "unknown material",
			path:         "/v1/designs",
			body:         `{"name":"Bench","document":{"schemaVersion":1,"units":"in","pieces":[{"id":"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee","name":"stud","materialId":"11111111-2222-4333-8444-555555555555","roughCut":false,"lengthIn":8,"continuity":{"x":true,"y":false,"z":false},"transform":{"positionIn":[0,0,0],"rotationDeg":[0,0,0]}}],"connections":[]}}`,
			validRequest: true,
			status:       http.StatusConflict,
			auth:         func(*http.Request) (store.ClerkUserID, error) { return sessionClerk, nil },
			save: func(context.Context, store.UserID, store.Draft) (store.Design, error) {
				return store.Design{}, store.ErrMaterialMissing
			},
		},
	},
	"listDesigns": {{
		name:         "live designs",
		path:         "/v1/designs",
		validRequest: true,
		status:       http.StatusOK,
		auth:         func(*http.Request) (store.ClerkUserID, error) { return sessionClerk, nil },
		list: func(context.Context, store.UserID) ([]store.DesignSummary, error) {
			return []store.DesignSummary{sampleDesign(1).DesignSummary}, nil
		},
	}},
	"getDesign": {{
		name:         "found",
		path:         "/v1/designs/00000000-0000-4000-8000-000000000001",
		validRequest: true,
		status:       http.StatusOK,
		auth:         func(*http.Request) (store.ClerkUserID, error) { return sessionClerk, nil },
		get: func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
			return sampleDesign(1), nil
		},
	}},
	"replaceDesign": {{
		name:         "replaced",
		path:         "/v1/designs/00000000-0000-4000-8000-000000000001",
		body:         `{"name":"Bench","version":1,"document":{"schemaVersion":1,"units":"in","pieces":[],"connections":[]}}`,
		validRequest: true,
		status:       http.StatusOK,
		auth:         func(*http.Request) (store.ClerkUserID, error) { return sessionClerk, nil },
		save: func(context.Context, store.UserID, store.Draft) (store.Design, error) {
			return sampleDesign(2), nil
		},
	}},
	"deleteDesign": {{
		name:         "deleted",
		path:         "/v1/designs/00000000-0000-4000-8000-000000000001",
		validRequest: true,
		status:       http.StatusNoContent,
		auth:         func(*http.Request) (store.ClerkUserID, error) { return sessionClerk, nil },
		delete: func(context.Context, store.UserID, store.DesignID) error {
			return nil
		},
	}},
	"copyDesign": {
		{
			name:         "copied",
			path:         "/v1/designs/00000000-0000-4000-8000-000000000001/copy",
			validRequest: true,
			status:       http.StatusCreated,
			auth:         func(*http.Request) (store.ClerkUserID, error) { return sessionClerk, nil },
			copy: func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
				return sampleDesign(1), nil
			},
		},
		{
			name:         "body",
			path:         "/v1/designs/00000000-0000-4000-8000-000000000001/copy",
			body:         `{}`,
			validRequest: false,
			status:       http.StatusBadRequest,
			auth:         func(*http.Request) (store.ClerkUserID, error) { return sessionClerk, nil },
		},
	},
}

func sampleDesign(version int32) store.Design {
	id, err := store.ParseDesignID("00000000-0000-4000-8000-000000000001")
	if err != nil {
		panic(err)
	}
	name, err := store.ParseName("Bench")
	if err != nil {
		panic(err)
	}
	return store.Design{
		DesignSummary: store.DesignSummary{
			ID:          id,
			Name:        name,
			Description: "",
			Version:     version,
			UpdatedAt:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		},
	}
}

var (
	errProbe     = probeError("down")
	sessionClerk = mustClerk("user_123")
)

type probeError string

func (e probeError) Error() string { return string(e) }

func mustClerk(raw string) store.ClerkUserID {
	id, err := store.ParseClerkUserID(raw)
	if err != nil {
		panic(err)
	}
	return id
}

type operation struct {
	path   string
	method string
	op     *openapi3.Operation
}

func TestEveryOperationMatchesTheSpec(t *testing.T) {
	doc := loadSpec(t)
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]operation{}
	for _, path := range doc.Paths.InMatchingOrder() {
		item := doc.Paths.Find(path)
		for method, op := range item.Operations() {
			if op.OperationID == "" {
				t.Errorf("%s %s has no operationId", method, path)
				continue
			}
			if _, ok := byID[op.OperationID]; ok {
				t.Errorf("duplicate operationId %s", op.OperationID)
			}
			byID[op.OperationID] = operation{path: path, method: method, op: op}
			assertOperationRules(t, doc, path, op)
			if _, ok := exchanges[op.OperationID]; !ok {
				t.Errorf("operation %s has no case", op.OperationID)
			}
		}
	}
	for id, cases := range exchanges {
		op, ok := byID[id]
		if !ok {
			t.Errorf("case %s names no operation", id)
			continue
		}
		for _, c := range cases {
			t.Run(id+" "+c.name, func(t *testing.T) {
				assertExchange(t, router, op, c)
			})
		}
	}
}

func assertOperationRules(t *testing.T, doc *openapi3.T, path string, op *openapi3.Operation) {
	t.Helper()
	underV1 := strings.HasPrefix(path, "/v1/")
	if underV1 != (len(op.Tags) == 1 && op.Tags[0] == "v1") {
		t.Errorf("%s tag %v, path %s", op.OperationID, op.Tags, path)
	}
	if underV1 != bearerSecurity(doc, op) {
		t.Errorf("%s bearer security %v, path %s", op.OperationID, bearerSecurity(doc, op), path)
	}
	if !underV1 {
		return
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		if op.Responses == nil || op.Responses.Status(status) == nil {
			t.Errorf("%s does not document %d", op.OperationID, status)
		}
	}
}

// A nil operation security list inherits the document list.
// An empty list opts out.
func bearerSecurity(doc *openapi3.T, op *openapi3.Operation) bool {
	sec := doc.Security
	if op.Security != nil {
		sec = *op.Security
	}
	if len(sec) != 1 || len(sec[0]) != 1 || doc.Components == nil {
		return false
	}
	var scheme *openapi3.SecurityScheme
	for name := range sec[0] {
		ref := doc.Components.SecuritySchemes[name]
		if ref == nil {
			return false
		}
		scheme = ref.Value
	}
	return scheme != nil && scheme.Type == "http" && strings.EqualFold(scheme.Scheme, "bearer")
}

func assertExchange(t *testing.T, router routers.Router, op operation, c exchange) {
	t.Helper()
	h := newStack(t, c)
	path := op.path
	if c.path != "" {
		path = c.path
	}
	var payload *bytes.Reader
	var reqBody io.Reader
	if c.body != "" {
		payload = bytes.NewReader([]byte(c.body))
		reqBody = payload
	}
	req := httptest.NewRequest(op.method, path, reqBody)
	if c.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.validRequest {
		req.Header.Set("Authorization", "Bearer token")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if c.status != 0 && rr.Code != c.status {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, c.status, rr.Body.Bytes())
	}

	route, params, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if route.Operation.OperationID != op.op.OperationID {
		t.Fatalf("route %s, want %s", route.Operation.OperationID, op.op.OperationID)
	}
	if c.validRequest {
		if payload != nil {
			if _, err := payload.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
		}
		err = openapi3filter.ValidateRequest(context.Background(), &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: params,
			Route:      route,
			Options: &openapi3filter.Options{
				AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
			},
		})
		if err != nil {
			t.Fatalf("request: %v", err)
		}
	}
	body := bytes.Clone(rr.Body.Bytes())
	err = openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: params,
			Route:      route,
		},
		Status: rr.Code,
		Header: rr.Header(),
		Body:   io.NopCloser(bytes.NewReader(body)),
	})
	if err != nil {
		t.Fatalf("status %d body %s: %v", rr.Code, body, err)
	}
}

func newStack(t *testing.T, c exchange) http.Handler {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := c.auth
	if auth == nil {
		auth = func(*http.Request) (store.ClerkUserID, error) {
			return store.ClerkUserID{}, clerkauth.ErrUnauthenticated
		}
	}
	ensure := c.ensure
	if ensure == nil {
		ensure = func(context.Context, store.ClerkUserID) (store.UserID, error) {
			return store.UserID{}, nil
		}
	}
	display := c.display
	if display == nil {
		display = func(context.Context, store.ClerkUserID) (string, error) { return "Ada", nil }
	}
	save := c.save
	if save == nil {
		save = func(context.Context, store.UserID, store.Draft) (store.Design, error) {
			return store.Design{}, store.ErrNotFound
		}
	}
	get := c.get
	if get == nil {
		get = func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
			return store.Design{}, store.ErrNotFound
		}
	}
	list := c.list
	if list == nil {
		list = func(context.Context, store.UserID) ([]store.DesignSummary, error) {
			return nil, store.ErrNotFound
		}
	}
	deleteDesign := c.delete
	if deleteDesign == nil {
		deleteDesign = func(context.Context, store.UserID, store.DesignID) error {
			return store.ErrNotFound
		}
	}
	copyDesign := c.copy
	if copyDesign == nil {
		copyDesign = func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
			return store.Design{}, store.ErrNotFound
		}
	}
	checks := c.checks
	if len(checks) == 0 {
		checks = []server.Check{{Name: "postgres", Probe: func(context.Context) error { return nil }}}
	}
	v1, err := api.New(api.Deps{
		Log:          log,
		Authenticate: auth,
		EnsureUser:   ensure,
		DisplayName:  display,
		Save:         save,
		Get:          get,
		List:         list,
		Delete:       deleteDesign,
		Copy:         copyDesign,
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := server.New(server.Deps{Log: log, Ready: checks, V1: v1})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("spec path")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "openapi.yaml")
	doc, err := openapi3.NewLoader().LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return doc
}
