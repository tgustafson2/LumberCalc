package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"lumbercalc/backend/internal/store"
)

const priceJSON = `{"materialId":"00000000-0000-4000-8000-000000000002","lengthIn":96,"priceCents":548}`

func TestCreatePriceReturns201(t *testing.T) {
	h := priceHandler(t, func(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error) {
		return fixturePrice(t), nil
	}, nil, nil, nil)
	rr := callJSON(h, http.MethodPost, "/v1/prices", priceJSON)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body Price
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.PriceCents.String() != "548" {
		t.Fatalf("priceCents = %s", body.PriceCents)
	}
	if body.Id != priceFixtureID {
		t.Fatalf("id = %s", body.Id)
	}
}

func TestListPricesReturns200(t *testing.T) {
	h := priceHandler(t, nil, func(context.Context, store.UserID) ([]store.Price, error) {
		return []store.Price{fixturePrice(t)}, nil
	}, nil, nil)
	rr := callJSON(h, http.MethodGet, "/v1/prices", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body []Price
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 || body[0].PriceCents.String() != "548" {
		t.Fatalf("body = %#v", body)
	}
}

func TestUpdatePriceReturns200(t *testing.T) {
	h := priceHandler(t, nil, nil, func(context.Context, store.UserID, store.PriceID, store.Cents) (store.Price, error) {
		return fixturePrice(t), nil
	}, nil)
	rr := callJSON(h, http.MethodPut, "/v1/prices/"+priceFixtureID, `{"priceCents":599}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestDeletePriceReturns204(t *testing.T) {
	h := priceHandler(t, nil, nil, nil, func(context.Context, store.UserID, store.PriceID) error {
		return nil
	})
	rr := callJSON(h, http.MethodDelete, "/v1/prices/"+priceFixtureID, "")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestPriceRejectsABadBody(t *testing.T) {
	h := priceHandler(t, func(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error) {
		t.Fatal("create was called")
		return store.Price{}, nil
	}, nil, func(context.Context, store.UserID, store.PriceID, store.Cents) (store.Price, error) {
		t.Fatal("update was called")
		return store.Price{}, nil
	}, nil)
	rr := callJSON(h, http.MethodPost, "/v1/prices", `{"materialId":"00000000-0000-4000-8000-000000000002","lengthIn":96,"priceCents":548,"ownerId":"nope"}`)
	assertStatus(t, rr, http.StatusBadRequest, "bad request")
	for _, body := range []string{
		`{"materialId":"00000000-0000-4000-8000-000000000002","lengthIn":96}`,
		`{"materialId":"00000000-0000-4000-8000-000000000002","lengthIn":96,"priceCents":null}`,
		`{"materialId":"00000000-0000-4000-8000-000000000002","lengthIn":96,"priceCents":-1}`,
		`{"materialId":"00000000-0000-4000-8000-000000000002","lengthIn":96,"priceCents":1.5}`,
		`{"materialId":"00000000-0000-4000-8000-000000000002","lengthIn":96,"priceCents":1e2}`,
		`{"materialId":"00000000-0000-4000-8000-000000000002","lengthIn":96,"priceCents":2147483648}`,
		`{"materialId":"00000000-0000-4000-8000-000000000002","lengthIn":0,"priceCents":548}`,
		`{"materialId":"not-a-uuid","lengthIn":96,"priceCents":548}`,
	} {
		rr = callJSON(h, http.MethodPost, "/v1/prices", body)
		assertStatus(t, rr, http.StatusBadRequest, "bad request")
	}
	rr = callJSON(h, http.MethodPut, "/v1/prices/"+priceFixtureID, `{"priceCents":599,"materialId":"00000000-0000-4000-8000-000000000002"}`)
	assertStatus(t, rr, http.StatusBadRequest, "bad request")
	rr = callJSON(h, http.MethodPut, "/v1/prices/"+priceFixtureID, `{"priceCents":599,"lengthIn":96}`)
	assertStatus(t, rr, http.StatusBadRequest, "bad request")
}

func TestPriceSentinels(t *testing.T) {
	taken := priceHandler(t, func(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error) {
		return store.Price{}, store.ErrPriceTaken
	}, nil, func(context.Context, store.UserID, store.PriceID, store.Cents) (store.Price, error) {
		return store.Price{}, store.ErrPriceNotFound
	}, nil)
	rr := callJSON(taken, http.MethodPost, "/v1/prices", priceJSON)
	assertStatus(t, rr, http.StatusConflict, priceTakenMessage)
	rr = callJSON(taken, http.MethodPut, "/v1/prices/not-a-uuid", `{"priceCents":599}`)
	assertStatus(t, rr, http.StatusNotFound, "not found")
	rr = callJSON(taken, http.MethodPut, "/v1/prices/"+priceFixtureID, `{"priceCents":599}`)
	assertStatus(t, rr, http.StatusNotFound, "not found")

	unusable := priceHandler(t, func(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error) {
		return store.Price{}, store.ErrMaterialUnusable
	}, nil, nil, nil)
	rr = callJSON(unusable, http.MethodPost, "/v1/prices", priceJSON)
	assertStatus(t, rr, http.StatusConflict, materialUnavailableMessage)
}

func TestPriceDoesNotMapMaterialSentinels(t *testing.T) {
	h := priceHandler(t, func(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error) {
		return store.Price{}, store.ErrNotOwner
	}, nil, nil, nil)
	rr := callJSON(h, http.MethodPost, "/v1/prices", priceJSON)
	assertStatus(t, rr, http.StatusInternalServerError, "internal server error")

	h = priceHandler(t, func(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error) {
		return store.Price{}, store.ErrNameTaken
	}, nil, nil, nil)
	rr = callJSON(h, http.MethodPost, "/v1/prices", priceJSON)
	assertStatus(t, rr, http.StatusInternalServerError, "internal server error")
}

const priceFixtureID = "00000000-0000-4000-8000-000000000001"

func fixturePrice(t *testing.T) store.Price {
	t.Helper()
	id, err := store.ParsePriceID(priceFixtureID)
	if err != nil {
		t.Fatal(err)
	}
	material, err := store.ParseMaterialID("00000000-0000-4000-8000-000000000002")
	if err != nil {
		t.Fatal(err)
	}
	length, err := store.ParseInches("96")
	if err != nil {
		t.Fatal(err)
	}
	cents, err := store.ParseCents("548")
	if err != nil {
		t.Fatal(err)
	}
	return store.Price{
		ID:        id,
		Key:       store.PriceKey{Material: material, Length: length},
		Cents:     cents,
		UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func priceHandler(
	t *testing.T,
	create func(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error),
	list func(context.Context, store.UserID) ([]store.Price, error),
	update func(context.Context, store.UserID, store.PriceID, store.Cents) (store.Price, error),
	del func(context.Context, store.UserID, store.PriceID) error,
) http.Handler {
	t.Helper()
	if create == nil {
		create = func(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error) {
			t.Fatal("create price was called")
			return store.Price{}, nil
		}
	}
	if list == nil {
		list = func(context.Context, store.UserID) ([]store.Price, error) {
			t.Fatal("list prices was called")
			return nil, nil
		}
	}
	if update == nil {
		update = func(context.Context, store.UserID, store.PriceID, store.Cents) (store.Price, error) {
			t.Fatal("update price was called")
			return store.Price{}, nil
		}
	}
	if del == nil {
		del = func(context.Context, store.UserID, store.PriceID) error {
			t.Fatal("delete price was called")
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
		Publish: func(context.Context, store.UserID, store.DesignID) (store.PatternID, error) {
			t.Fatal("publish was called")
			return store.PatternID{}, nil
		},
		CreateMaterial: idleCreateMaterial,
		ListMaterials:  idleListMaterials,
		UpdateMaterial: idleUpdateMaterial,
		DeleteMaterial: idleDeleteMaterial,
		CreatePrice:    create,
		ListPrices:     list,
		UpdatePrice:    update,
		DeletePrice:    del,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func idleCreatePrice(context.Context, store.UserID, store.PriceKey, store.Cents) (store.Price, error) {
	return store.Price{}, store.ErrPriceNotFound
}

func idleListPrices(context.Context, store.UserID) ([]store.Price, error) {
	return []store.Price{}, nil
}

func idleUpdatePrice(context.Context, store.UserID, store.PriceID, store.Cents) (store.Price, error) {
	return store.Price{}, store.ErrPriceNotFound
}

func idleDeletePrice(context.Context, store.UserID, store.PriceID) error {
	return store.ErrPriceNotFound
}
