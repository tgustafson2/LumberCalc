package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/internal/dbtest"
	"lumbercalc/backend/internal/store"
)

func TestAddPatternClonesTheSnapshot(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	stranger := testUser(t, s, "user_stranger")
	pine := insertSystemMaterial(t, pool, "test pine")
	oak := insertSystemMaterial(t, pool, "test oak")
	snapshot := testJoinedDocument(t, pine, oak)
	source, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        mustParseName(t, "Bench"),
		Description: "cut list",
		Document:    snapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	patternID, err := s.Publish(ctx, owner, source.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:       source.ID,
		Name:     mustParseName(t, "Stool"),
		Document: testDocument(t, oak),
		Base:     source.Version,
	}); err != nil {
		t.Fatal(err)
	}
	patternsBefore := allPatternCount(t, pool)

	clone, err := s.AddPattern(ctx, stranger, patternID)
	if err != nil {
		t.Fatal(err)
	}
	if clone.ID == source.ID || clone.Version != 1 || clone.Name.String() != "Bench" || clone.Description != "cut list" {
		t.Fatalf("clone id = %s version = %d name = %q description = %q", clone.ID, clone.Version, clone.Name, clone.Description)
	}
	got, err := s.Get(ctx, stranger, clone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, owner, clone.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("owner get err = %v, want ErrNotFound", err)
	}
	assertForkedDocument(t, snapshot, got.Document)
	if want, have := pieceShapes(t, snapshot), pieceShapes(t, got.Document); !sameStrings(have, want) {
		t.Fatalf("pieces = %v, want %v", have, want)
	}
	assertPatternLineage(t, pool, clone.ID, patternID)
	wantUsages := []string{pine, oak}
	sort.Strings(wantUsages)
	if usages := usageMaterialIDs(t, pool, clone.ID); !sameStrings(usages, wantUsages) {
		t.Fatalf("usages = %v, want %v", usages, wantUsages)
	}
	if n := priceCount(t, pool, stranger); n != 0 {
		t.Fatalf("stranger prices = %d, want 0", n)
	}
	if n := allPatternCount(t, pool); n != patternsBefore {
		t.Fatalf("patterns = %d, want %d", n, patternsBefore)
	}
}

func TestAddPatternHidesAPrivatePattern(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	stranger := testUser(t, s, "user_stranger")
	pine := insertSystemMaterial(t, pool, "test pine")
	source := savedDesign(t, s, owner, "Bench", pine)
	patternID, err := s.Publish(ctx, owner, source.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unpublish(ctx, owner, source.ID); err != nil {
		t.Fatal(err)
	}
	missing, err := store.ParsePatternID("00000000-0000-4000-8000-000000000099")
	if err != nil {
		t.Fatal(err)
	}

	for _, call := range []struct {
		name   string
		caller store.UserID
		id     store.PatternID
	}{
		{"publisher", owner, patternID},
		{"stranger", stranger, patternID},
		{"missing", stranger, missing},
	} {
		if _, err := s.AddPattern(ctx, call.caller, call.id); !errors.Is(err, store.ErrPatternNotFound) {
			t.Fatalf("%s err = %v, want ErrPatternNotFound", call.name, err)
		}
	}
	if n := designCount(t, pool, owner); n != 1 {
		t.Fatalf("owner designs = %d, want 1", n)
	}
	if n := designCount(t, pool, stranger); n != 0 {
		t.Fatalf("stranger designs = %d, want 0", n)
	}
}

func TestAddPatternTwiceInsertsTwoDesigns(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	stranger := testUser(t, s, "user_stranger")
	pine := insertSystemMaterial(t, pool, "test pine")
	source := savedDesign(t, s, owner, "Bench", pine)
	patternID, err := s.Publish(ctx, owner, source.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}

	first, err := s.AddPattern(ctx, stranger, patternID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AddPattern(ctx, stranger, patternID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("second add returned the first design %s", first.ID)
	}
	if n := designCount(t, pool, stranger); n != 2 {
		t.Fatalf("stranger designs = %d, want 2", n)
	}
	assertPatternLineage(t, pool, first.ID, patternID)
	assertPatternLineage(t, pool, second.ID, patternID)
}

func TestRepublishAndUnpublishLeaveTheClone(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	stranger := testUser(t, s, "user_stranger")
	pine := insertSystemMaterial(t, pool, "test pine")
	oak := insertSystemMaterial(t, pool, "test oak")
	source := savedDesign(t, s, owner, "Bench", pine)
	patternID, err := s.Publish(ctx, owner, source.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	clone, err := s.AddPattern(ctx, stranger, patternID)
	if err != nil {
		t.Fatal(err)
	}
	_, beforeDocument, beforeUpdated := readCopyColumns(t, pool, clone.ID)
	assertCloneUnchanged := func(step string) {
		t.Helper()
		afterID, _, afterUpdated := readCopyColumns(t, pool, clone.ID)
		if afterID != patternID.String() || !designDocumentEquals(t, pool, clone.ID, beforeDocument) || !afterUpdated.Equal(beforeUpdated) {
			t.Fatalf("after %s: copied_from_pattern_id = %s, updated_at = %s, want %s", step, afterID, afterUpdated, beforeUpdated)
		}
		got, err := s.Get(ctx, stranger, clone.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Version != 1 {
			t.Fatalf("after %s: version = %d, want 1", step, got.Version)
		}
	}

	if _, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:       source.ID,
		Name:     source.Name,
		Document: testDocument(t, oak),
		Base:     source.Version,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, owner, source.ID, store.PublicName{}); err != nil {
		t.Fatal(err)
	}
	assertCloneUnchanged("republish")

	fresh, err := s.AddPattern(ctx, stranger, patternID)
	if err != nil {
		t.Fatal(err)
	}
	if usages := usageMaterialIDs(t, pool, fresh.ID); !sameStrings(usages, []string{oak}) {
		t.Fatalf("add after republish usages = %v, want [%s]", usages, oak)
	}

	if _, err := s.Unpublish(ctx, owner, source.ID); err != nil {
		t.Fatal(err)
	}
	assertCloneUnchanged("unpublish")
}

func TestAddPatternWithACustomMaterialInsertsNoDesign(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	stranger := testUser(t, s, "user_stranger")
	pine := insertSystemMaterial(t, pool, "test pine")
	custom := insertMaterial(t, pool, owner, "pine")
	source := savedDesign(t, s, owner, "Bench", pine)
	patternID, err := s.Publish(ctx, owner, source.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	writeLegacyPatternDocument(t, pool, patternID, testDocument(t, pine, custom))

	if _, err := s.AddPattern(ctx, stranger, patternID); !errors.Is(err, store.ErrMaterialUnusable) {
		t.Fatalf("stranger err = %v, want ErrMaterialUnusable", err)
	}
	if n := designCount(t, pool, stranger); n != 0 {
		t.Fatalf("stranger designs = %d, want 0", n)
	}

	clone, err := s.AddPattern(ctx, owner, patternID)
	if err != nil {
		t.Fatal(err)
	}
	wantUsages := []string{pine, custom}
	sort.Strings(wantUsages)
	if usages := usageMaterialIDs(t, pool, clone.ID); !sameStrings(usages, wantUsages) {
		t.Fatalf("publisher usages = %v, want %v", usages, wantUsages)
	}
}

func writeLegacyPatternDocument(t *testing.T, pool *pgxpool.Pool, id store.PatternID, doc store.Document) {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := pool.Exec(context.Background(), `UPDATE patterns SET document = $2 WHERE id = $1`, id.String(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated rows = %d", tag.RowsAffected())
	}
}

func mustParseName(t *testing.T, raw string) store.Name {
	t.Helper()
	name, err := store.ParseName(raw)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func pieceShapes(t *testing.T, doc store.Document) []string {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Pieces []struct {
			Name     string      `json:"name"`
			LengthIn json.Number `json:"lengthIn"`
		} `json:"pieces"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(body.Pieces))
	for i, p := range body.Pieces {
		out[i] = p.Name + " " + p.LengthIn.String()
	}
	return out
}

func assertPatternLineage(t *testing.T, pool *pgxpool.Pool, id store.DesignID, pattern store.PatternID) {
	t.Helper()
	var fromDesign, fromPattern *string
	err := pool.QueryRow(context.Background(), `
		SELECT copied_from_design_id::text, copied_from_pattern_id::text
		FROM designs
		WHERE id = $1
	`, id.String()).Scan(&fromDesign, &fromPattern)
	if err != nil {
		t.Fatal(err)
	}
	if fromDesign != nil || fromPattern == nil || *fromPattern != pattern.String() {
		t.Fatalf("copied_from_design_id = %v, copied_from_pattern_id = %v, want null and %s", fromDesign, fromPattern, pattern)
	}
}

func designCount(t *testing.T, pool *pgxpool.Pool, owner store.UserID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM designs WHERE owner_id = $1`, owner.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func priceCount(t *testing.T, pool *pgxpool.Pool, owner store.UserID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM prices WHERE owner_id = $1`, owner.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func allPatternCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM patterns`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
