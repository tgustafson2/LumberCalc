package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"lumbercalc/backend/internal/solver"
	"lumbercalc/backend/internal/store"
)

func TestOptimizeReturnsResolvedFlags(t *testing.T) {
	off := mustFlags(t, "", "nominal")
	on := mustFlags(t, "0.125", "actual")
	zero := solver.Length{}
	actual := mustBasis(t, "actual")
	nominal := mustBasis(t, "nominal")
	kerf := mustKerf(t, "0.125")
	tests := []struct {
		name     string
		body     string
		flags    store.OptimizeFlags
		want     store.FlagChoice
		wantBody string
	}{
		{
			name:     "omitted",
			body:     "",
			flags:    off,
			want:     store.FlagChoice{},
			wantBody: `{"useKerf":false,"kerfInches":null,"dimensionBasis":"nominal"}`,
		},
		{
			name:     "whitespace",
			body:     " \n\t",
			flags:    off,
			want:     store.FlagChoice{},
			wantBody: `{"useKerf":false,"kerfInches":null,"dimensionBasis":"nominal"}`,
		},
		{
			name:     "empty object",
			body:     `{}`,
			flags:    off,
			want:     store.FlagChoice{},
			wantBody: `{"useKerf":false,"kerfInches":null,"dimensionBasis":"nominal"}`,
		},
		{
			name:     "basis overlay",
			body:     `{"dimensionBasis":"actual"}`,
			flags:    on,
			want:     store.FlagChoice{Basis: actual},
			wantBody: `{"useKerf":true,"kerfInches":0.125,"dimensionBasis":"actual"}`,
		},
		{
			name:     "kerf off",
			body:     `{"useKerf":false}`,
			flags:    off,
			want:     store.FlagChoice{Kerf: &zero},
			wantBody: `{"useKerf":false,"kerfInches":null,"dimensionBasis":"nominal"}`,
		},
		{
			name:     "kerf off null inches",
			body:     `{"useKerf":false,"kerfInches":null}`,
			flags:    off,
			want:     store.FlagChoice{Kerf: &zero},
			wantBody: `{"useKerf":false,"kerfInches":null,"dimensionBasis":"nominal"}`,
		},
		{
			name:     "kerf on",
			body:     `{"useKerf":true,"kerfInches":0.125}`,
			flags:    on,
			want:     store.FlagChoice{Kerf: kerf},
			wantBody: `{"useKerf":true,"kerfInches":0.125,"dimensionBasis":"actual"}`,
		},
		{
			name:     "both axes",
			body:     `{"useKerf":false,"dimensionBasis":"nominal"}`,
			flags:    off,
			want:     store.FlagChoice{Kerf: &zero, Basis: nominal},
			wantBody: `{"useKerf":false,"kerfInches":null,"dimensionBasis":"nominal"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got store.FlagChoice
			h := optimizeHandler(t, func(_ context.Context, _ store.UserID, id store.DesignID, choice store.FlagChoice) (store.OptimizeInput, error) {
				if id.String() != designID {
					t.Fatalf("id = %s", id)
				}
				got = choice
				return store.OptimizeInput{Document: store.Document{}, Flags: tt.flags}, nil
			})
			rr := callJSON(h, http.MethodPost, "/v1/designs/"+designID+"/optimize", tt.body)
			assertStatus(t, rr, http.StatusOK, "")
			if strings.TrimSpace(rr.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s", rr.Body.String())
			}
			if !sameChoice(got, tt.want) {
				t.Fatalf("choice %s, want %s", choiceText(got), choiceText(tt.want))
			}
		})
	}
}

func TestOptimizeRejectsABadBody(t *testing.T) {
	h := optimizeHandler(t, func(context.Context, store.UserID, store.DesignID, store.FlagChoice) (store.OptimizeInput, error) {
		t.Fatal("get optimize input was called")
		return store.OptimizeInput{}, nil
	})
	bodies := []string{
		`null`,
		`{}{}`,
		`{"nope":true}`,
		`[]`,
		`1`,
		`"nominal"`,
		`{"kerfInches":0.125}`,
		`{"useKerf":null}`,
		`{"useKerf":true}`,
		`{"useKerf":true,"kerfInches":null}`,
		`{"useKerf":true,"kerfInches":0}`,
		`{"useKerf":true,"kerfInches":"0.125"}`,
		`{"useKerf":false,"kerfInches":0.125}`,
		`{"useKerf":"false"}`,
		`{"dimensionBasis":null}`,
		`{"dimensionBasis":1}`,
		`{"dimensionBasis":"rough"}`,
	}
	for _, body := range bodies {
		t.Run(body, func(t *testing.T) {
			rr := callJSON(h, http.MethodPost, "/v1/designs/"+designID+"/optimize", body)
			assertStatus(t, rr, http.StatusBadRequest, "bad request")
		})
	}
}

func TestOptimizeMissingDesign(t *testing.T) {
	h := optimizeHandler(t, func(context.Context, store.UserID, store.DesignID, store.FlagChoice) (store.OptimizeInput, error) {
		return store.OptimizeInput{}, store.ErrNotFound
	})
	rr := callJSON(h, http.MethodPost, "/v1/designs/"+designID+"/optimize", "")
	assertStatus(t, rr, http.StatusNotFound, "not found")

	h = optimizeHandler(t, func(context.Context, store.UserID, store.DesignID, store.FlagChoice) (store.OptimizeInput, error) {
		t.Fatal("get optimize input was called")
		return store.OptimizeInput{}, nil
	})
	rr = callJSON(h, http.MethodPost, "/v1/designs/not-a-uuid/optimize", "")
	assertStatus(t, rr, http.StatusNotFound, "not found")
}

func TestOptimizeSettingsErrorStays500(t *testing.T) {
	h := optimizeHandler(t, func(context.Context, store.UserID, store.DesignID, store.FlagChoice) (store.OptimizeInput, error) {
		return store.OptimizeInput{}, fmt.Errorf("user_settings: %w", store.ErrBadKerf)
	})
	rr := callJSON(h, http.MethodPost, "/v1/designs/"+designID+"/optimize", "")
	assertStatus(t, rr, http.StatusInternalServerError, "internal server error")
}

func optimizeHandler(
	t *testing.T,
	get func(context.Context, store.UserID, store.DesignID, store.FlagChoice) (store.OptimizeInput, error),
) http.Handler {
	t.Helper()
	h, err := New(Deps{
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticate: okAuth,
		EnsureUser:   okEnsure,
		DisplayName:  okName,
		Save: func(context.Context, store.UserID, store.Draft) (store.Design, error) {
			t.Fatal("save was called")
			return store.Design{}, nil
		},
		Get: func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
			t.Fatal("get was called")
			return store.Design{}, nil
		},
		List: func(context.Context, store.UserID) ([]store.DesignSummary, error) {
			t.Fatal("list was called")
			return nil, nil
		},
		Delete: func(context.Context, store.UserID, store.DesignID) error {
			t.Fatal("delete was called")
			return nil
		},
		Copy: func(context.Context, store.UserID, store.DesignID) (store.Design, error) {
			t.Fatal("copy was called")
			return store.Design{}, nil
		},
		Publish: func(context.Context, store.UserID, store.DesignID, store.PublicName) (store.PatternID, error) {
			t.Fatal("publish was called")
			return store.PatternID{}, nil
		},
		Unpublish: func(context.Context, store.UserID, store.DesignID) (store.PatternID, error) {
			t.Fatal("unpublish was called")
			return store.PatternID{}, nil
		},
		CreateMaterial:     idleCreateMaterial,
		ListMaterials:      idleListMaterials,
		UpdateMaterial:     idleUpdateMaterial,
		DeleteMaterial:     idleDeleteMaterial,
		CreatePrice:        idleCreatePrice,
		ListPrices:         idleListPrices,
		UpdatePrice:        idleUpdatePrice,
		DeletePrice:        idleDeletePrice,
		GetOptimizeInput:   get,
		PublicName:         idlePublicName,
		ListPublicPatterns: idleListPublicPatterns,
		GetPattern:         idleGetPattern,
		AddPattern:         idleAddPattern,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func idleGetOptimizeInput(t *testing.T) func(context.Context, store.UserID, store.DesignID, store.FlagChoice) (store.OptimizeInput, error) {
	return func(context.Context, store.UserID, store.DesignID, store.FlagChoice) (store.OptimizeInput, error) {
		t.Fatal("get optimize input was called")
		return store.OptimizeInput{}, nil
	}
}

func mustFlags(t *testing.T, kerf, basis string) store.OptimizeFlags {
	t.Helper()
	var length solver.Length
	if kerf != "" {
		parsed, err := solver.ParseLength(kerf)
		if err != nil {
			t.Fatal(err)
		}
		length = parsed
	}
	flags, err := store.NewOptimizeFlags(length, *mustBasis(t, basis))
	if err != nil {
		t.Fatal(err)
	}
	return flags
}

func mustBasis(t *testing.T, raw string) *store.DimensionBasis {
	t.Helper()
	basis, err := store.ParseDimensionBasis(raw)
	if err != nil {
		t.Fatal(err)
	}
	return &basis
}

func mustKerf(t *testing.T, raw string) *solver.Length {
	t.Helper()
	length, err := solver.ParseLength(raw)
	if err != nil {
		t.Fatal(err)
	}
	return &length
}

func sameChoice(got, want store.FlagChoice) bool {
	if (got.Kerf == nil) != (want.Kerf == nil) || (got.Kerf != nil && *got.Kerf != *want.Kerf) {
		return false
	}
	if (got.Basis == nil) != (want.Basis == nil) || (got.Basis != nil && *got.Basis != *want.Basis) {
		return false
	}
	return true
}

func choiceText(c store.FlagChoice) string {
	kerf := "nil"
	if c.Kerf != nil {
		kerf = c.Kerf.String()
	}
	basis := "nil"
	if c.Basis != nil {
		basis = c.Basis.String()
	}
	return "kerf " + kerf + " basis " + basis
}
