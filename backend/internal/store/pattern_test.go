package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/internal/dbtest"
	"lumbercalc/backend/internal/store"
)

func TestParsePatternID(t *testing.T) {
	for _, raw := range []string{"", "nope", "00000000-0000-4000-8000-00000000000z"} {
		if _, err := store.ParsePatternID(raw); !errors.Is(err, store.ErrPatternNotFound) {
			t.Fatalf("ParsePatternID(%q) err = %v, want ErrPatternNotFound", raw, err)
		}
	}
	id, err := store.ParsePatternID("00000000-0000-4000-8000-0000000000AB")
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != "00000000-0000-4000-8000-0000000000ab" {
		t.Fatalf("id = %s", id)
	}
}

func TestParsePublicName(t *testing.T) {
	subject, err := store.ParseClerkUserID("user_ada")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", "   ", "ada@example.com", "user_ada", " user_ada "} {
		if got, ok := store.ParsePublicName(raw, subject).Value(); ok {
			t.Fatalf("ParsePublicName(%q) = %q, want zero", raw, got)
		}
	}
	got, ok := store.ParsePublicName("  Ada Lovelace ", subject).Value()
	if !ok || got != "Ada Lovelace" {
		t.Fatalf("name = %q, %v", got, ok)
	}
}

func TestListPublicPatternsOmitsHiddenPatterns(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertMaterial(t, pool, owner, "pine")

	empty, err := s.ListPublicPatterns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("list = %#v, want an empty non-nil slice", empty)
	}

	shown := savedDesign(t, s, owner, "Bench", pine)
	unpublished := savedDesign(t, s, owner, "Stool", pine)
	deleted := savedDesign(t, s, owner, "Shelf", pine)
	shownID, err := s.Publish(ctx, owner, shown.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []store.Design{unpublished, deleted} {
		if _, err := s.Publish(ctx, owner, d.ID, store.PublicName{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Unpublish(ctx, owner, unpublished.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, owner, deleted.ID); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListPublicPatterns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != shownID || list[0].Name.String() != "Bench" {
		t.Fatalf("list = %+v, want only %s", list, shownID)
	}
}

func TestGetPatternHidesAPrivatePatternFromAStranger(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	stranger := testUser(t, s, "user_stranger")
	pine := insertMaterial(t, pool, owner, "pine")
	design := savedDesign(t, s, owner, "Bench", pine)
	patternID, err := s.Publish(ctx, owner, design.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}

	public, err := s.GetPattern(ctx, stranger, patternID)
	if err != nil {
		t.Fatal(err)
	}
	if !public.Public || public.ID != patternID {
		t.Fatalf("stranger got %+v", public.PatternSummary)
	}

	if _, err := s.Unpublish(ctx, owner, design.ID); err != nil {
		t.Fatal(err)
	}
	own, err := s.GetPattern(ctx, owner, patternID)
	if err != nil {
		t.Fatal(err)
	}
	if own.Public || own.ID != patternID || own.Name.String() != "Bench" {
		t.Fatalf("owner got public = %v, %+v", own.Public, own.PatternSummary)
	}
	if _, err := s.GetPattern(ctx, stranger, patternID); !errors.Is(err, store.ErrPatternNotFound) {
		t.Fatalf("stranger err = %v, want ErrPatternNotFound", err)
	}
	missing, err := store.ParsePatternID("00000000-0000-4000-8000-000000000099")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPattern(ctx, owner, missing); !errors.Is(err, store.ErrPatternNotFound) {
		t.Fatalf("missing err = %v, want ErrPatternNotFound", err)
	}
}

func TestGetPatternSendsOnlySystemMaterialIDs(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	stranger := testUser(t, s, "user_stranger")
	system := insertSystemMaterial(t, pool, "test 2x4")
	private := insertMaterial(t, pool, owner, "pine")
	design := savedDesign(t, s, owner, "Bench", system, private)
	patternID, err := s.Publish(ctx, owner, design.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetPattern(ctx, stranger, patternID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(got.Document)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Pieces []map[string]json.RawMessage `json:"pieces"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Pieces) != 2 {
		t.Fatalf("pieces = %s", raw)
	}
	if string(body.Pieces[0]["materialId"]) != `"`+system+`"` {
		t.Fatalf("system materialId = %s, want %q", body.Pieces[0]["materialId"], system)
	}
	if id, ok := body.Pieces[1]["materialId"]; !ok || string(id) != "null" {
		t.Fatalf("private materialId = %s, present = %v, want null", id, ok)
	}

	stored, err := s.Get(ctx, owner, design.ID)
	if err != nil {
		t.Fatal(err)
	}
	ownRaw, err := json.Marshal(stored.Document)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(ownRaw, &body); err != nil {
		t.Fatal(err)
	}
	if string(body.Pieces[1]["materialId"]) != `"`+private+`"` {
		t.Fatalf("design materialId = %s, want %q", body.Pieces[1]["materialId"], private)
	}
}

func TestPublishStoresThePublisherName(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	subject, err := store.ParseClerkUserID("user_owner")
	if err != nil {
		t.Fatal(err)
	}
	owner := testUser(t, s, subject.String())
	pine := insertMaterial(t, pool, owner, "pine")
	design := savedDesign(t, s, owner, "Bench", pine)

	patternID, err := s.Publish(ctx, owner, design.ID, store.ParsePublicName("  ", subject))
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := publisherNameColumn(t, pool, patternID); ok {
		t.Fatalf("publisher_name = %q, want NULL", name)
	}
	got, err := s.GetPattern(ctx, owner, patternID)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := got.Publisher.Value(); ok {
		t.Fatalf("publisher = %q, want zero", name)
	}

	again, err := s.Publish(ctx, owner, design.ID, store.ParsePublicName("Ada Lovelace", subject))
	if err != nil {
		t.Fatal(err)
	}
	if again != patternID {
		t.Fatalf("id = %s, want %s", again, patternID)
	}
	list, err := s.ListPublicPatterns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}
	if name, ok := list[0].Publisher.Value(); !ok || name != "Ada Lovelace" {
		t.Fatalf("publisher = %q, %v", name, ok)
	}
}

func TestDeleteSetsThePatternPrivateAndKeepsUpdatedAt(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertMaterial(t, pool, owner, "pine")
	design := savedDesign(t, s, owner, "Bench", pine)
	patternID, err := s.Publish(ctx, owner, design.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	before := mustPattern(t, pool, design.ID)

	if err := s.Delete(ctx, owner, design.ID); err != nil {
		t.Fatal(err)
	}

	after := mustPattern(t, pool, design.ID)
	if after.id != patternID.String() || after.isPublic {
		t.Fatalf("id = %s, is_public = %v", after.id, after.isPublic)
	}
	if !after.updatedAt.Equal(before.updatedAt) {
		t.Fatalf("updated_at = %s, want %s", after.updatedAt, before.updatedAt)
	}
	got, err := s.GetPattern(ctx, owner, patternID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Public {
		t.Fatal("owner sees the pattern as public")
	}
}

func savedDesign(t *testing.T, s *store.Store, owner store.UserID, title string, materialIDs ...string) store.Design {
	t.Helper()
	name, err := store.ParseName(title)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Save(context.Background(), owner, store.CreateDraft{Name: name, Document: testDocument(t, materialIDs...)})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func publisherNameColumn(t *testing.T, pool *pgxpool.Pool, id store.PatternID) (string, bool) {
	t.Helper()
	var name *string
	if err := pool.QueryRow(context.Background(), `SELECT publisher_name FROM patterns WHERE id = $1`, id.String()).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name == nil {
		return "", false
	}
	return *name, true
}
