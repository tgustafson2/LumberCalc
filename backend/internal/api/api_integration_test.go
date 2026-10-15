package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lumbercalc/backend/internal/dbtest"
	"lumbercalc/backend/internal/store"
)

func TestGetMeReturnsTheUserRowID(t *testing.T) {
	pool := dbtest.Migrated(t)
	const clerk = "user_row_id"
	st := store.New(pool)
	h, err := New(Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticate: func(*http.Request) (store.ClerkUserID, error) {
			return mustClerk(t, clerk), nil
		},
		EnsureUser: st.EnsureUser,
		DisplayName: func(context.Context, store.ClerkUserID) (string, error) {
			return "Ada", nil
		},
		Save:           st.Save,
		Get:            st.Get,
		List:           st.List,
		Delete:         st.Delete,
		Copy:           st.Copy,
		CreateMaterial: st.CreateMaterial,
		ListMaterials:  st.ListMaterials,
		UpdateMaterial: st.UpdateMaterial,
		DeleteMaterial: st.DeleteMaterial,
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := call(h, http.MethodGet, "/v1/me", "Bearer token")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body Me
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var rowID string
	err = pool.QueryRow(context.Background(), `SELECT id::text FROM users WHERE clerk_user_id = $1`, clerk).Scan(&rowID)
	if err != nil {
		t.Fatal(err)
	}
	if body.UserId != rowID {
		t.Fatalf("user_id = %s, row = %s", body.UserId, rowID)
	}
	if rowID == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("row id is the zero uuid")
	}
}

func TestGetMaterialsReturnsTheSystemCatalog(t *testing.T) {
	pool := dbtest.Migrated(t)
	const clerk = "user_materials_catalog"
	st := store.New(pool)
	h, err := New(Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticate: func(*http.Request) (store.ClerkUserID, error) {
			return mustClerk(t, clerk), nil
		},
		EnsureUser: st.EnsureUser,
		DisplayName: func(context.Context, store.ClerkUserID) (string, error) {
			return "Ada", nil
		},
		Save:           st.Save,
		Get:            st.Get,
		List:           st.List,
		Delete:         st.Delete,
		Copy:           st.Copy,
		CreateMaterial: st.CreateMaterial,
		ListMaterials:  st.ListMaterials,
		UpdateMaterial: st.UpdateMaterial,
		DeleteMaterial: st.DeleteMaterial,
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := call(h, http.MethodGet, "/v1/materials", "Bearer token")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body []Material
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 17 {
		t.Fatalf("len = %d, want 17", len(body))
	}
	want := map[string][4]string{
		"1x2":  {"1", "2", "0.75", "1.5"},
		"1x3":  {"1", "3", "0.75", "2.5"},
		"1x4":  {"1", "4", "0.75", "3.5"},
		"1x6":  {"1", "6", "0.75", "5.5"},
		"1x8":  {"1", "8", "0.75", "7.25"},
		"1x10": {"1", "10", "0.75", "9.25"},
		"1x12": {"1", "12", "0.75", "11.25"},
		"2x2":  {"2", "2", "1.5", "1.5"},
		"2x3":  {"2", "3", "1.5", "2.5"},
		"2x4":  {"2", "4", "1.5", "3.5"},
		"2x6":  {"2", "6", "1.5", "5.5"},
		"2x8":  {"2", "8", "1.5", "7.25"},
		"2x10": {"2", "10", "1.5", "9.25"},
		"2x12": {"2", "12", "1.5", "11.25"},
		"4x4":  {"4", "4", "3.5", "3.5"},
		"4x6":  {"4", "6", "3.5", "5.5"},
		"6x6":  {"6", "6", "5.5", "5.5"},
	}
	quarter := big.NewRat(1, 4)
	index := map[string]int{}
	for i, row := range body {
		if !row.System {
			t.Fatalf("%s system = false", row.Name)
		}
		if row.Species != nil || row.Grade != nil {
			t.Fatalf("%s species = %#v grade = %#v, want nil", row.Name, row.Species, row.Grade)
		}
		size, ok := want[row.Name]
		if !ok {
			t.Fatalf("name = %q", row.Name)
		}
		if row.NominalThicknessIn.String() != size[0] || row.NominalWidthIn.String() != size[1] ||
			row.ActualThicknessIn.String() != size[2] || row.ActualWidthIn.String() != size[3] {
			t.Fatalf("%s nominal = %s x %s actual = %s x %s, want %s x %s and %s x %s",
				row.Name,
				row.NominalThicknessIn, row.NominalWidthIn,
				row.ActualThicknessIn, row.ActualWidthIn,
				size[0], size[1], size[2], size[3])
		}
		if row.Name == "2x4" && (row.RoughThicknessIn.String() != "1.75" || row.RoughWidthIn.String() != "3.5") {
			t.Fatalf("2x4 rough = %s x %s, want 1.75 x 3.5", row.RoughThicknessIn, row.RoughWidthIn)
		}
		actualT := inchesRat(t, row.ActualThicknessIn)
		roughT := inchesRat(t, row.RoughThicknessIn)
		if roughT.Cmp(new(big.Rat).Add(new(big.Rat).Set(actualT), quarter)) != 0 {
			t.Fatalf("%s rough thickness = %s, want actual %s + 0.25", row.Name, row.RoughThicknessIn, row.ActualThicknessIn)
		}
		if inchesRat(t, row.RoughWidthIn).Cmp(inchesRat(t, row.ActualWidthIn)) != 0 {
			t.Fatalf("%s rough width = %s, want actual width %s", row.Name, row.RoughWidthIn, row.ActualWidthIn)
		}
		if i > 0 && row.Name < body[i-1].Name {
			t.Fatalf("names = %s then %s", body[i-1].Name, row.Name)
		}
		index[row.Name] = i
		delete(want, row.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing = %v", want)
	}
	if index["1x10"] > index["1x2"] {
		t.Fatalf("1x10 index = %d, 1x2 index = %d", index["1x10"], index["1x2"])
	}
}

func inchesRat(t *testing.T, n json.Number) *big.Rat {
	t.Helper()
	rat, ok := new(big.Rat).SetString(n.String())
	if !ok {
		t.Fatalf("inches = %q", n.String())
	}
	return rat
}

func TestForeignAndDeletedDesignsMatchAMissingID(t *testing.T) {
	pool := dbtest.Migrated(t)
	st := store.New(pool)
	const (
		ownerClerk  = "user_owner"
		otherClerk  = "user_other"
		putBody     = `{"name":"Bench","version":1,"document":{"schemaVersion":1,"units":"in","pieces":[],"connections":[]}}`
		missingPath = "/v1/designs/00000000-0000-4000-8000-000000000099"
	)
	clerk := ownerClerk
	h, err := New(Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticate: func(*http.Request) (store.ClerkUserID, error) {
			return mustClerk(t, clerk), nil
		},
		EnsureUser: st.EnsureUser,
		DisplayName: func(context.Context, store.ClerkUserID) (string, error) {
			return "Ada", nil
		},
		Save:           st.Save,
		Get:            st.Get,
		List:           st.List,
		Delete:         st.Delete,
		Copy:           st.Copy,
		CreateMaterial: st.CreateMaterial,
		ListMaterials:  st.ListMaterials,
		UpdateMaterial: st.UpdateMaterial,
		DeleteMaterial: st.DeleteMaterial,
	})
	if err != nil {
		t.Fatal(err)
	}

	sameAs := func(got, want *httptest.ResponseRecorder) {
		t.Helper()
		gotBody := strings.TrimSpace(got.Body.String())
		wantBody := strings.TrimSpace(want.Body.String())
		gotLocation := got.Header().Get("Location")
		wantLocation := want.Header().Get("Location")
		if got.Code != want.Code || gotBody != wantBody || gotLocation != wantLocation {
			t.Fatalf("status = %d body = %s Location = %q, recorded status = %d body = %s Location = %q", got.Code, gotBody, gotLocation, want.Code, wantBody, wantLocation)
		}
	}
	readDesign := func(rr *httptest.ResponseRecorder) Design {
		t.Helper()
		var body Design
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	listIDs := func(rr *httptest.ResponseRecorder) []string {
		t.Helper()
		var rows []DesignSummary
		if err := json.Unmarshal(rr.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		ids := make([]string, len(rows))
		for i, row := range rows {
			ids[i] = row.Id
		}
		return ids
	}
	hasID := func(ids []string, id string) bool {
		for _, got := range ids {
			if got == id {
				return true
			}
		}
		return false
	}

	created := callJSON(h, http.MethodPost, "/v1/designs", `{"name":"Bench"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", created.Code, created.Body.String())
	}
	bench := readDesign(created)
	if bench.Version != 1 {
		t.Fatalf("version = %d, want 1", bench.Version)
	}

	clerk = otherClerk
	created = callJSON(h, http.MethodPost, "/v1/designs", `{"name":"Stool"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", created.Code, created.Body.String())
	}
	stool := readDesign(created)
	if stool.Version != 1 {
		t.Fatalf("version = %d, want 1", stool.Version)
	}

	clerk = ownerClerk
	copied := callJSON(h, http.MethodPost, "/v1/designs/"+bench.Id+"/copy", "")
	if copied.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", copied.Code, copied.Body.String())
	}
	clone := readDesign(copied)

	missing := callJSON(h, http.MethodGet, missingPath, "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", missing.Code, missing.Body.String())
	}
	if strings.TrimSpace(missing.Body.String()) != `{"error":"not found"}` {
		t.Fatalf("body = %s", missing.Body.String())
	}
	if missing.Header().Get("Location") != "" {
		t.Fatalf("Location = %q", missing.Header().Get("Location"))
	}

	clerk = otherClerk
	benchPath := "/v1/designs/" + bench.Id
	sameAs(callJSON(h, http.MethodGet, benchPath, ""), missing)
	sameAs(callJSON(h, http.MethodPut, benchPath, putBody), missing)
	sameAs(callJSON(h, http.MethodDelete, benchPath, ""), missing)
	sameAs(callJSON(h, http.MethodPost, benchPath+"/copy", ""), missing)

	clerk = ownerClerk
	got := callJSON(h, http.MethodGet, benchPath, "")
	if got.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", got.Code, got.Body.String())
	}
	if body := readDesign(got); body.Id != bench.Id || body.Version != 1 {
		t.Fatalf("id = %s version = %d", body.Id, body.Version)
	}

	listed := callJSON(h, http.MethodGet, "/v1/designs", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", listed.Code, listed.Body.String())
	}
	ids := listIDs(listed)
	if len(ids) != 2 || !hasID(ids, bench.Id) || !hasID(ids, clone.Id) || hasID(ids, stool.Id) {
		t.Fatalf("ids = %v", ids)
	}

	clerk = otherClerk
	listed = callJSON(h, http.MethodGet, "/v1/designs", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", listed.Code, listed.Body.String())
	}
	ids = listIDs(listed)
	if len(ids) != 1 || ids[0] != stool.Id {
		t.Fatalf("ids = %v, want [%s]", ids, stool.Id)
	}

	clerk = ownerClerk
	deleted := callJSON(h, http.MethodDelete, benchPath, "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
	sameAs(callJSON(h, http.MethodGet, benchPath, ""), missing)
	sameAs(callJSON(h, http.MethodPut, benchPath, putBody), missing)
	sameAs(callJSON(h, http.MethodDelete, benchPath, ""), missing)
	sameAs(callJSON(h, http.MethodPost, benchPath+"/copy", ""), missing)

	listed = callJSON(h, http.MethodGet, "/v1/designs", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", listed.Code, listed.Body.String())
	}
	ids = listIDs(listed)
	if len(ids) != 1 || !hasID(ids, clone.Id) || hasID(ids, bench.Id) {
		t.Fatalf("ids = %v", ids)
	}

	clerk = otherClerk
	listed = callJSON(h, http.MethodGet, "/v1/designs", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", listed.Code, listed.Body.String())
	}
	ids = listIDs(listed)
	if len(ids) != 1 || ids[0] != stool.Id {
		t.Fatalf("ids = %v, want [%s]", ids, stool.Id)
	}

	clerk = ownerClerk
	sameAs(callJSON(h, http.MethodGet, "/v1/designs/"+stool.Id, ""), missing)
}
