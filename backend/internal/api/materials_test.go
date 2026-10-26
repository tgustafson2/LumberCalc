package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"lumbercalc/backend/internal/store"
)

const materialJSON = `{"name":"Pine","species":"pine","grade":null,"nominalThicknessIn":2,"nominalWidthIn":4,"actualThicknessIn":1.5,"actualWidthIn":3.5,"roughThicknessIn":1.75,"roughWidthIn":3.5}`

func TestCreateMaterialReturns201(t *testing.T) {
	h := materialHandler(t, func(context.Context, store.UserID, store.MaterialInput) (store.Material, error) {
		return fixtureMaterial(t, false), nil
	}, nil, nil, nil)
	rr := callJSON(h, http.MethodPost, "/v1/materials", materialJSON)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body Material
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.System {
		t.Fatal("system = true")
	}
	if body.Name != "Pine" {
		t.Fatalf("name = %q", body.Name)
	}
}

func TestListMaterialsReturns200(t *testing.T) {
	h := materialHandler(t, nil, func(context.Context, store.UserID) ([]store.Material, error) {
		return []store.Material{fixtureMaterial(t, true), fixtureMaterial(t, false)}, nil
	}, nil, nil)
	rr := callJSON(h, http.MethodGet, "/v1/materials", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body []Material
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 2 || !body[0].System || body[1].System {
		t.Fatalf("body = %#v", body)
	}
}

func TestReplaceMaterialReturns200(t *testing.T) {
	h := materialHandler(t, nil, nil, func(context.Context, store.UserID, store.MaterialID, store.MaterialInput) (store.Material, error) {
		return fixtureMaterial(t, false), nil
	}, nil)
	rr := callJSON(h, http.MethodPut, "/v1/materials/"+materialFixtureID, materialJSON)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteMaterialReturns204(t *testing.T) {
	h := materialHandler(t, nil, nil, nil, func(context.Context, store.UserID, store.MaterialID) error {
		return nil
	})
	rr := callJSON(h, http.MethodDelete, "/v1/materials/"+materialFixtureID, "")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestMaterialRejectsABadBody(t *testing.T) {
	h := materialHandler(t, func(context.Context, store.UserID, store.MaterialInput) (store.Material, error) {
		t.Fatal("create was called")
		return store.Material{}, nil
	}, nil, nil, nil)
	rr := callJSON(h, http.MethodPost, "/v1/materials", `{"name":"Pine","ownerId":"nope"}`)
	assertStatus(t, rr, http.StatusBadRequest, "bad request")
	rr = callJSON(h, http.MethodPost, "/v1/materials", `{"name":"Pine","nominalThicknessIn":0,"nominalWidthIn":4,"actualThicknessIn":1.5,"actualWidthIn":3.5,"roughThicknessIn":1.75,"roughWidthIn":3.5}`)
	assertStatus(t, rr, http.StatusBadRequest, "bad request")
}

func TestMaterialForbiddenAndMissing(t *testing.T) {
	h := materialHandler(t, nil, nil, func(context.Context, store.UserID, store.MaterialID, store.MaterialInput) (store.Material, error) {
		return store.Material{}, store.ErrNotOwner
	}, func(context.Context, store.UserID, store.MaterialID) error {
		return store.ErrMaterialNotFound
	})
	rr := callJSON(h, http.MethodPut, "/v1/materials/"+materialFixtureID, materialJSON)
	assertStatus(t, rr, http.StatusForbidden, "forbidden")
	rr = callJSON(h, http.MethodPut, "/v1/materials/not-a-uuid", materialJSON)
	assertStatus(t, rr, http.StatusNotFound, "not found")
	rr = callJSON(h, http.MethodDelete, "/v1/materials/"+materialFixtureID, "")
	assertStatus(t, rr, http.StatusNotFound, "not found")
}

func TestMaterialConflicts(t *testing.T) {
	h := materialHandler(t, func(context.Context, store.UserID, store.MaterialInput) (store.Material, error) {
		return store.Material{}, store.ErrNameTaken
	}, nil, nil, func(context.Context, store.UserID, store.MaterialID) error {
		return store.ErrMaterialInUse
	})
	rr := callJSON(h, http.MethodPost, "/v1/materials", materialJSON)
	assertStatus(t, rr, http.StatusConflict, nameTakenMessage)
	rr = callJSON(h, http.MethodDelete, "/v1/materials/"+materialFixtureID, "")
	assertStatus(t, rr, http.StatusConflict, materialInUseMessage)
}

func assertStatus(t *testing.T, rr *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, status, rr.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != message {
		t.Fatalf("error = %q, want %q", body.Error, message)
	}
}

const materialFixtureID = "00000000-0000-4000-8000-000000000001"

func fixtureMaterial(t *testing.T, system bool) store.Material {
	t.Helper()
	name, err := store.ParseName("Pine")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.ParseMaterialID(materialFixtureID)
	if err != nil {
		t.Fatal(err)
	}
	inches := func(raw string) store.Inches {
		n, err := store.ParseInches(raw)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	return store.Material{
		ID:                 id,
		Name:               name,
		System:             system,
		NominalThicknessIn: inches("2"),
		NominalWidthIn:     inches("4"),
		ActualThicknessIn:  inches("1.5"),
		ActualWidthIn:      inches("3.5"),
		RoughThicknessIn:   inches("1.75"),
		RoughWidthIn:       inches("3.5"),
		UpdatedAt:          time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func materialHandler(
	t *testing.T,
	create func(context.Context, store.UserID, store.MaterialInput) (store.Material, error),
	list func(context.Context, store.UserID) ([]store.Material, error),
	update func(context.Context, store.UserID, store.MaterialID, store.MaterialInput) (store.Material, error),
	del func(context.Context, store.UserID, store.MaterialID) error,
) http.Handler {
	t.Helper()
	if create == nil {
		create = func(context.Context, store.UserID, store.MaterialInput) (store.Material, error) {
			t.Fatal("create material was called")
			return store.Material{}, nil
		}
	}
	if list == nil {
		list = func(context.Context, store.UserID) ([]store.Material, error) {
			t.Fatal("list materials was called")
			return nil, nil
		}
	}
	if update == nil {
		update = func(context.Context, store.UserID, store.MaterialID, store.MaterialInput) (store.Material, error) {
			t.Fatal("update material was called")
			return store.Material{}, nil
		}
	}
	if del == nil {
		del = func(context.Context, store.UserID, store.MaterialID) error {
			t.Fatal("delete material was called")
			return nil
		}
	}
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
		CreateMaterial:     create,
		ListMaterials:      list,
		UpdateMaterial:     update,
		DeleteMaterial:     del,
		CreatePrice:        idleCreatePrice,
		ListPrices:         idleListPrices,
		UpdatePrice:        idleUpdatePrice,
		DeletePrice:        idleDeletePrice,
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

func idleCreateMaterial(context.Context, store.UserID, store.MaterialInput) (store.Material, error) {
	return store.Material{}, store.ErrMaterialNotFound
}

func idleListMaterials(context.Context, store.UserID) ([]store.Material, error) {
	return []store.Material{}, nil
}

func idleUpdateMaterial(context.Context, store.UserID, store.MaterialID, store.MaterialInput) (store.Material, error) {
	return store.Material{}, store.ErrMaterialNotFound
}

func idleDeleteMaterial(context.Context, store.UserID, store.MaterialID) error {
	return store.ErrMaterialNotFound
}
