package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/internal/dbtest"
	"lumbercalc/backend/internal/store"
)

func TestParseName(t *testing.T) {
	name, err := store.ParseName("  Bench  ")
	if err != nil {
		t.Fatal(err)
	}
	if name.String() != "Bench" {
		t.Fatalf("name = %q", name.String())
	}
	_, err = store.ParseName(" \t ")
	if !errors.Is(err, store.ErrBlankName) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseDesignID(t *testing.T) {
	id, err := store.ParseDesignID("AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE")
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee" {
		t.Fatalf("id = %s", id)
	}
	_, err = store.ParseDesignID("not-a-uuid")
	if !errors.Is(err, store.ErrBadDesignID) {
		t.Fatalf("err = %v", err)
	}
}

func TestSaveGetListDelete(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	other := testUser(t, s, "user_other")
	pine := insertMaterial(t, pool, owner, "pine")
	oak := insertMaterial(t, pool, owner, "oak")

	empty, err := s.List(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("list = %#v, want an empty slice", empty)
	}

	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.Save(ctx, owner, store.CreateDraft{
		Name:     name,
		Document: testDocument(t, pine, pine),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Version != 1 {
		t.Fatalf("version = %d, want 1", created.Version)
	}
	if created.Name.String() != "Bench" {
		t.Fatalf("name = %q", created.Name.String())
	}
	if created.Description != "" {
		t.Fatalf("description = %q", created.Description)
	}
	assertDocument(t, created.Document, testDocument(t, pine, pine))
	assertSchemaVersion(t, pool, created.ID, 1)
	if got := usageMaterialIDs(t, pool, created.ID); len(got) != 1 || got[0] != pine {
		t.Fatalf("usages = %v, want [%s]", got, pine)
	}

	got, err := s.Get(ctx, owner, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.ID != created.ID {
		t.Fatalf("get = version %d id %s", got.Version, got.ID)
	}
	assertDocument(t, got.Document, created.Document)

	_, err = s.Get(ctx, other, created.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign get err = %v", err)
	}

	secondName, err := store.ParseName("Stool")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        secondName,
		Description: "spare",
		Document:    store.Document{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if usages := usageMaterialIDs(t, pool, second.ID); len(usages) != 0 {
		t.Fatalf("empty document usages = %v", usages)
	}

	replaced, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:          created.ID,
		Name:        name,
		Description: "cut list",
		Document:    testDocument(t, oak, pine),
	})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Version != 2 {
		t.Fatalf("version = %d, want 2", replaced.Version)
	}
	if replaced.Description != "cut list" {
		t.Fatalf("description = %q", replaced.Description)
	}
	assertSchemaVersion(t, pool, created.ID, 1)
	wantUsages := []string{oak, pine}
	if oak > pine {
		wantUsages = []string{pine, oak}
	}
	if got := usageMaterialIDs(t, pool, created.ID); !sameStrings(got, wantUsages) {
		t.Fatalf("usages = %v, want %v", got, wantUsages)
	}

	_, err = s.Save(ctx, other, store.ReplaceDraft{
		ID:       created.ID,
		Name:     name,
		Document: store.Document{},
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign replace err = %v", err)
	}
	still, err := s.Get(ctx, owner, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if still.Version != 2 {
		t.Fatalf("version after foreign replace = %d, want 2", still.Version)
	}

	older := replaced.UpdatedAt.Add(-time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE designs SET updated_at = $2 WHERE id = $1`, second.ID.String(), older); err != nil {
		t.Fatal(err)
	}
	listed, err := s.List(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("list len = %d", len(listed))
	}
	if listed[0].ID != replaced.ID || listed[0].Version != 2 || listed[0].Description != "cut list" {
		t.Fatalf("list[0] = %+v", listed[0])
	}
	if listed[1].ID != second.ID || listed[1].Version != 1 || listed[1].Description != "spare" {
		t.Fatalf("list[1] = %+v", listed[1])
	}

	if err := s.Delete(ctx, other, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign delete err = %v", err)
	}
	if err := s.Delete(ctx, owner, created.ID); err != nil {
		t.Fatal(err)
	}
	if got := usageMaterialIDs(t, pool, created.ID); !sameStrings(got, wantUsages) {
		t.Fatalf("usages after delete = %v, want %v", got, wantUsages)
	}
	_, err = s.Get(ctx, owner, created.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get after delete err = %v", err)
	}
	_, err = s.Save(ctx, owner, store.ReplaceDraft{
		ID:       created.ID,
		Name:     name,
		Document: store.Document{},
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("replace after delete err = %v", err)
	}
	if err := s.Delete(ctx, owner, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete err = %v", err)
	}

	listed, err = s.List(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != second.ID {
		t.Fatalf("list after delete = %+v", listed)
	}

	missing, err := store.ParseDesignID("00000000-0000-4000-8000-000000000099")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(ctx, owner, missing)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing get err = %v", err)
	}
}

func TestSaveUnknownMaterialRollsBack(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertMaterial(t, pool, owner, "pine")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.Save(ctx, owner, store.CreateDraft{
		Name:     name,
		Document: testDocument(t, pine),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Save(ctx, owner, store.ReplaceDraft{
		ID:       created.ID,
		Name:     name,
		Document: testDocument(t, "99999999-9999-4999-8999-999999999999"),
	})
	if !errors.Is(err, store.ErrMaterialMissing) {
		t.Fatalf("err = %v", err)
	}
	got, err := s.Get(ctx, owner, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 {
		t.Fatalf("version = %d, want 1", got.Version)
	}
	assertDocument(t, got.Document, testDocument(t, pine))
	if usages := usageMaterialIDs(t, pool, created.ID); len(usages) != 1 || usages[0] != pine {
		t.Fatalf("usages = %v, want [%s]", usages, pine)
	}

	_, err = s.Save(ctx, owner, store.CreateDraft{
		Name:     name,
		Document: testDocument(t, "99999999-9999-4999-8999-999999999999"),
	})
	if !errors.Is(err, store.ErrMaterialMissing) {
		t.Fatalf("create err = %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM designs WHERE owner_id = $1`, owner.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("design rows = %d, want 1", n)
	}
}

func TestGetRejectsSchemaVersionMismatch(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.Save(ctx, owner, store.CreateDraft{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE designs SET schema_version = 2 WHERE id = $1`, created.ID.String()); err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(ctx, owner, created.ID)
	var docErr *store.DocumentError
	if err == nil || errors.Is(err, store.ErrNotFound) || errors.As(err, &docErr) {
		t.Fatalf("err = %v", err)
	}
	if err.Error() != "design schema_version 2 does not match document" {
		t.Fatalf("err = %v", err)
	}
}

func testUser(t *testing.T, s *store.Store, clerk string) store.UserID {
	t.Helper()
	id, err := store.ParseClerkUserID(clerk)
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.EnsureUser(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func insertMaterial(t *testing.T, pool *pgxpool.Pool, owner store.UserID, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO materials (
			owner_id, name,
			nominal_thickness_in, nominal_width_in,
			actual_thickness_in, actual_width_in,
			rough_thickness_in, rough_width_in
		) VALUES ($1, $2, 1, 4, 0.75, 3.5, 1, 4)
		RETURNING id::text
	`, owner.String(), name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testDocument(t *testing.T, materialIDs ...string) store.Document {
	t.Helper()
	pieces := make([]string, len(materialIDs))
	for i, id := range materialIDs {
		pieces[i] = fmt.Sprintf(`{
			"id":"00000000-0000-4000-8000-%012d",
			"name":"piece",
			"materialId":"%s",
			"lengthIn":8,
			"continuity":{"x":true,"y":false,"z":false},
			"transform":{"positionIn":[0,0,0],"rotationDeg":[0,0,0]}
		}`, i+1, id)
	}
	raw := `{"schemaVersion":1,"units":"in","pieces":[` + strings.Join(pieces, ",") + `],"connections":[]}`
	var doc store.Document
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func assertDocument(t *testing.T, got, want store.Document) {
	t.Helper()
	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotRaw) != string(wantRaw) {
		t.Fatalf("document = %s, want %s", gotRaw, wantRaw)
	}
}

func assertSchemaVersion(t *testing.T, pool *pgxpool.Pool, id store.DesignID, want int32) {
	t.Helper()
	var got int32
	err := pool.QueryRow(context.Background(), `SELECT schema_version FROM designs WHERE id = $1`, id.String()).Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("schema_version = %d, want %d", got, want)
	}
}

func usageMaterialIDs(t *testing.T, pool *pgxpool.Pool, id store.DesignID) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT material_id::text
		FROM design_material_usages
		WHERE design_id = $1
		ORDER BY material_id
	`, id.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var materialID string
		if err := rows.Scan(&materialID); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, materialID)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
