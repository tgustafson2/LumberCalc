package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/internal/dbtest"
	"lumbercalc/backend/internal/store"
)

func TestParseInches(t *testing.T) {
	for _, raw := range []string{"1.5", "2", " 3.5 ", "1e-1"} {
		got, err := store.ParseInches(raw)
		if err != nil {
			t.Fatalf("ParseInches(%q) err = %v", raw, err)
		}
		if got.String() == "" {
			t.Fatalf("ParseInches(%q) is empty", raw)
		}
	}
	for _, raw := range []string{"0", "0.0", "-1", "", "1/2", `"1"`, "NaN"} {
		_, err := store.ParseInches(raw)
		if !errors.Is(err, store.ErrBadInches) {
			t.Fatalf("ParseInches(%q) err = %v, want ErrBadInches", raw, err)
		}
	}
}

func TestParseMaterialID(t *testing.T) {
	id, err := store.ParseMaterialID("AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE")
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee" {
		t.Fatalf("id = %s", id)
	}
	_, err = store.ParseMaterialID("not-a-uuid")
	if !errors.Is(err, store.ErrBadMaterialID) {
		t.Fatalf("err = %v", err)
	}
}

func TestMaterials(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_material_owner")
	other := testUser(t, s, "user_material_other")

	empty, err := s.ListMaterials(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("list = %#v, want an empty slice", empty)
	}

	systemID := insertSystemMaterial(t, pool, "Doug Fir")
	otherRow, err := s.CreateMaterial(ctx, other, lumberInput(t, "Doug Fir", "oak"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateMaterial(ctx, owner, lumberInput(t, "Doug Fir", "  "))
	if err != nil {
		t.Fatal(err)
	}
	if created.System {
		t.Fatal("created row is system")
	}
	if created.Species != nil {
		t.Fatalf("species = %#v, want nil", created.Species)
	}
	if ownerOf(t, pool, created.ID.String()) != owner.String() {
		t.Fatalf("owner = %s", ownerOf(t, pool, created.ID.String()))
	}

	_, err = s.CreateMaterial(ctx, owner, lumberInput(t, "Doug Fir", "pine"))
	if !errors.Is(err, store.ErrNameTaken) {
		t.Fatalf("duplicate err = %v", err)
	}

	rows, err := s.ListMaterials(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.ID.String()] = true
	}
	if !seen[systemID] || !seen[created.ID.String()] || seen[otherRow.ID.String()] {
		t.Fatalf("list ids = %#v", seen)
	}

	system, err := store.ParseMaterialID(systemID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateMaterial(ctx, owner, system, lumberInput(t, "Doug Fir", "pine"))
	if !errors.Is(err, store.ErrNotOwner) {
		t.Fatalf("system update err = %v", err)
	}
	_, err = s.UpdateMaterial(ctx, owner, otherRow.ID, lumberInput(t, "Other", "pine"))
	if !errors.Is(err, store.ErrNotOwner) {
		t.Fatalf("foreign update err = %v", err)
	}
	missing, err := store.ParseMaterialID("00000000-0000-4000-8000-000000000099")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateMaterial(ctx, owner, missing, lumberInput(t, "Missing", "pine"))
	if !errors.Is(err, store.ErrMaterialNotFound) {
		t.Fatalf("missing update err = %v", err)
	}

	priced := mustCreate(t, s, owner, "Priced")
	if _, err := pool.Exec(ctx, `
		INSERT INTO prices (owner_id, material_id, length_in, price_cents)
		VALUES ($1, $2, 8, 100)
	`, owner.String(), priced.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMaterial(ctx, owner, priced.ID); !errors.Is(err, store.ErrMaterialInUse) {
		t.Fatalf("price delete err = %v", err)
	}

	used := mustCreate(t, s, owner, "Used")
	var designID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO designs (owner_id, name, schema_version, document)
		VALUES ($1, 'bench', 1, '{}')
		RETURNING id::text
	`, owner.String()).Scan(&designID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO design_material_usages (design_id, material_id)
		VALUES ($1, $2)
	`, designID, used.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMaterial(ctx, owner, used.ID); !errors.Is(err, store.ErrMaterialInUse) {
		t.Fatalf("usage delete err = %v", err)
	}

	free := mustCreate(t, s, owner, "Free")
	if err := s.DeleteMaterial(ctx, owner, free.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMaterial(ctx, owner, free.ID); !errors.Is(err, store.ErrMaterialNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}

func lumberInput(t *testing.T, name, species string) store.MaterialInput {
	t.Helper()
	parsed, err := store.ParseName(name)
	if err != nil {
		t.Fatal(err)
	}
	return store.MaterialInput{
		Name:               parsed,
		Species:            species,
		NominalThicknessIn: mustInches(t, "2"),
		NominalWidthIn:     mustInches(t, "4"),
		ActualThicknessIn:  mustInches(t, "1.5"),
		ActualWidthIn:      mustInches(t, "3.5"),
		RoughThicknessIn:   mustInches(t, "1.75"),
		RoughWidthIn:       mustInches(t, "3.5"),
	}
}

func mustInches(t *testing.T, raw string) store.Inches {
	t.Helper()
	n, err := store.ParseInches(raw)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func mustCreate(t *testing.T, s *store.Store, owner store.UserID, name string) store.Material {
	t.Helper()
	row, err := s.CreateMaterial(context.Background(), owner, lumberInput(t, name, "pine"))
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func insertSystemMaterial(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO materials (
			name,
			nominal_thickness_in, nominal_width_in,
			actual_thickness_in, actual_width_in,
			rough_thickness_in, rough_width_in
		) VALUES ($1, 2, 4, 1.5, 3.5, 1.75, 3.5)
		RETURNING id::text
	`, name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func ownerOf(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var owner string
	err := pool.QueryRow(context.Background(), `SELECT owner_id::text FROM materials WHERE id = $1`, id).Scan(&owner)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
