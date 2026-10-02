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

	next := testDocument(t, oak, pine)
	replaced, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:          created.ID,
		Name:        name,
		Description: "cut list",
		Document:    next,
		Base:        created.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Version != 2 {
		t.Fatalf("version = %d, want 2", replaced.Version)
	}
	assertDocument(t, replaced.Document, next)
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
		Base:     replaced.Version,
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
		Base:     replaced.Version,
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
		Base:     created.Version,
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

func TestReplaceStaleLeavesTheStoredDocument(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertMaterial(t, pool, owner, "pine")
	oak := insertMaterial(t, pool, owner, "oak")
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
	next := testDocument(t, oak)
	replaced, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:       created.ID,
		Name:     name,
		Document: next,
		Base:     created.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Version != 2 {
		t.Fatalf("version = %d, want 2", replaced.Version)
	}
	assertDocument(t, replaced.Document, next)

	_, err = s.Save(ctx, owner, store.ReplaceDraft{
		ID:       created.ID,
		Name:     name,
		Document: store.Document{},
		Base:     created.Version,
	})
	if !errors.Is(err, store.ErrStale) {
		t.Fatalf("err = %v, want ErrStale", err)
	}
	got, err := s.Get(ctx, owner, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 {
		t.Fatalf("version after stale = %d, want 2", got.Version)
	}
	assertDocument(t, got.Document, next)

	_, err = s.Save(ctx, owner, store.ReplaceDraft{
		ID:       created.ID,
		Name:     name,
		Document: store.Document{},
		Base:     0,
	})
	if !errors.Is(err, store.ErrStale) {
		t.Fatalf("base 0 err = %v, want ErrStale", err)
	}

	missing, err := store.ParseDesignID("00000000-0000-4000-8000-000000000099")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Save(ctx, owner, store.ReplaceDraft{
		ID:       missing,
		Name:     name,
		Document: store.Document{},
		Base:     1,
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing replace err = %v", err)
	}

	if err := s.Delete(ctx, owner, created.ID); err != nil {
		t.Fatal(err)
	}
	_, err = s.Save(ctx, owner, store.ReplaceDraft{
		ID:       created.ID,
		Name:     name,
		Document: store.Document{},
		Base:     replaced.Version,
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("replace after delete err = %v", err)
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

func TestCopy(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	other := testUser(t, s, "user_other")
	pine := insertMaterial(t, pool, owner, "pine")
	oak := insertMaterial(t, pool, owner, "oak")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	sourceDoc := testJoinedDocument(t, pine, oak)
	source, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        name,
		Description: "cut list",
		Document:    sourceDoc,
	})
	if err != nil {
		t.Fatal(err)
	}
	setSourcePattern(t, pool, source.ID)

	cloned, err := s.Copy(ctx, owner, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cloned.ID == source.ID {
		t.Fatalf("id = %s", cloned.ID)
	}
	if cloned.Version != 1 {
		t.Fatalf("version = %d, want 1", cloned.Version)
	}
	if cloned.Name.String() != "Bench copy" {
		t.Fatalf("name = %q", cloned.Name.String())
	}
	if cloned.Description != "cut list" {
		t.Fatalf("description = %q", cloned.Description)
	}
	assertCopyColumns(t, pool, cloned.ID, source.ID)
	assertSchemaVersion(t, pool, cloned.ID, 1)
	assertForkedDocument(t, sourceDoc, cloned.Document)
	wantUsages := []string{pine, oak}
	if wantUsages[0] > wantUsages[1] {
		wantUsages[0], wantUsages[1] = wantUsages[1], wantUsages[0]
	}
	if got := usageMaterialIDs(t, pool, cloned.ID); !sameStrings(got, wantUsages) {
		t.Fatalf("usages = %v, want %v", got, wantUsages)
	}
	if n := patternCount(t, pool, cloned.ID); n != 0 {
		t.Fatalf("patterns = %d, want 0", n)
	}
	if n := optimizationRunCount(t, pool, cloned.ID); n != 0 {
		t.Fatalf("optimization_runs = %d, want 0", n)
	}

	still, err := s.Get(ctx, owner, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if still.Version != 1 {
		t.Fatalf("source version = %d, want 1", still.Version)
	}
	assertDocument(t, still.Document, sourceDoc)

	again, err := s.Copy(ctx, owner, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID == cloned.ID || again.ID == source.ID {
		t.Fatalf("second id = %s", again.ID)
	}
	if again.Name.String() != "Bench copy" {
		t.Fatalf("second name = %q", again.Name.String())
	}
	assertCopyColumns(t, pool, again.ID, source.ID)
	kept, err := s.Get(ctx, owner, cloned.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertDocument(t, kept.Document, cloned.Document)

	_, err = s.Copy(ctx, other, source.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign copy err = %v", err)
	}
	still, err = s.Get(ctx, owner, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if still.Version != 1 {
		t.Fatalf("source version after foreign copy = %d", still.Version)
	}

	if err := s.Delete(ctx, owner, source.ID); err != nil {
		t.Fatal(err)
	}
	_, err = s.Copy(ctx, owner, source.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("copy after delete err = %v", err)
	}
	kept, err = s.Get(ctx, owner, cloned.ID)
	if err != nil {
		t.Fatal(err)
	}
	if kept.Version != 1 || kept.Name.String() != "Bench copy" {
		t.Fatalf("clone after source delete = version %d name %q", kept.Version, kept.Name.String())
	}
}

func TestCopyMissingMaterialLeavesNoRow(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertMaterial(t, pool, owner, "pine")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.Save(ctx, owner, store.CreateDraft{
		Name:     name,
		Document: testDocument(t, pine),
	})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := pool.Exec(ctx, `
		UPDATE designs
		SET document = jsonb_set(document, '{pieces,0,materialId}', to_jsonb($2::text))
		WHERE id = $1
	`, source.ID.String(), "99999999-9999-4999-8999-999999999999")
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated rows = %d", tag.RowsAffected())
	}
	_, err = s.Copy(ctx, owner, source.ID)
	if !errors.Is(err, store.ErrMaterialMissing) {
		t.Fatalf("err = %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM designs WHERE owner_id = $1`, owner.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("design rows = %d, want 1", n)
	}
	got, err := s.Get(ctx, owner, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 {
		t.Fatalf("version = %d, want 1", got.Version)
	}
}

func TestCopyReturnsNotFoundWhenDeleteCommitsUnderTheLock(t *testing.T) {
	pool := dbtest.Migrated(t)
	if pool.Config().MaxConns < 2 {
		t.Fatalf("MaxConns = %d, want at least 2", pool.Config().MaxConns)
	}
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.Save(ctx, owner, store.CreateDraft{Name: name, Description: "cut list"})
	if err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	var locked string
	err = tx.QueryRow(ctx, `
		SELECT id::text
		FROM designs
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, source.ID.String(), owner.String()).Scan(&locked)
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, copyErr := s.Copy(ctx, owner, source.ID)
		errCh <- copyErr
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		// A row lock wait is an ungranted transactionid lock on the holder.
		if err := tx.QueryRow(ctx, `
			SELECT count(*)
			FROM pg_locks waiter
			JOIN pg_locks holder
			  ON holder.locktype = 'transactionid'
			 AND holder.granted
			 AND holder.pid = pg_backend_pid()
			 AND waiter.locktype = 'transactionid'
			 AND NOT waiter.granted
			 AND waiter.transactionid = holder.transactionid
			 AND waiter.pid <> pg_backend_pid()
		`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case err := <-errCh:
			t.Fatalf("copy returned before it waited: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("copy did not wait on the source lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-errCh:
		t.Fatalf("copy returned before the delete committed: %v", err)
	default:
	}
	if _, err := tx.Exec(ctx, `UPDATE designs SET deleted_at = now(), updated_at = now() WHERE id = $1`, source.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("copy did not return")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM designs WHERE copied_from_design_id = $1`, source.ID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("clones = %d, want 0", n)
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

func testJoinedDocument(t *testing.T, materialA, materialB string) store.Document {
	t.Helper()
	raw := fmt.Sprintf(`{
		"schemaVersion": 1,
		"units": "in",
		"pieces": [
			{
				"id": "00000000-0000-4000-8000-0000000000a1",
				"name": "rail",
				"materialId": "%s",
				"lengthIn": 12,
				"continuity": {"x": true, "y": false, "z": false},
				"transform": {"positionIn": [1, 2, 3], "rotationDeg": [0, 0, 0]}
			},
			{
				"id": "00000000-0000-4000-8000-0000000000a2",
				"name": "stile",
				"materialId": "%s",
				"lengthIn": 36,
				"continuity": {"x": false, "y": true, "z": false},
				"transform": {"positionIn": [4, 5, 6], "rotationDeg": [0, 90, 0]}
			}
		],
		"connections": [{
			"id": "joint-1",
			"pieceAId": "00000000-0000-4000-8000-0000000000a1",
			"pieceBId": "00000000-0000-4000-8000-0000000000a2",
			"joinType": "butt",
			"faceA": "end",
			"faceB": "face",
			"wasteIn": 0.25
		}]
	}`, materialA, materialB)
	var doc store.Document
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func setSourcePattern(t *testing.T, pool *pgxpool.Pool, id store.DesignID) {
	t.Helper()
	var patternID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO patterns (source_design_id, publisher_id, name, schema_version, document)
		SELECT id, owner_id, name, schema_version, document
		FROM designs
		WHERE id = $1
		RETURNING id::text
	`, id.String()).Scan(&patternID)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := pool.Exec(context.Background(), `
		UPDATE designs
		SET copied_from_pattern_id = $2
		WHERE id = $1 AND copied_from_design_id IS NULL
	`, id.String(), patternID)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated rows = %d", tag.RowsAffected())
	}
	var got string
	if err := pool.QueryRow(context.Background(), `SELECT copied_from_pattern_id::text FROM designs WHERE id = $1`, id.String()).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != patternID {
		t.Fatalf("copied_from_pattern_id = %s, want %s", got, patternID)
	}
}

func assertCopyColumns(t *testing.T, pool *pgxpool.Pool, id, source store.DesignID) {
	t.Helper()
	var fromDesign *string
	var fromPattern *string
	var deleted *time.Time
	err := pool.QueryRow(context.Background(), `
		SELECT copied_from_design_id::text, copied_from_pattern_id::text, deleted_at
		FROM designs
		WHERE id = $1
	`, id.String()).Scan(&fromDesign, &fromPattern, &deleted)
	if err != nil {
		t.Fatal(err)
	}
	gotDesign := "<nil>"
	if fromDesign != nil {
		gotDesign = *fromDesign
	}
	gotPattern := "<nil>"
	if fromPattern != nil {
		gotPattern = *fromPattern
	}
	if fromDesign == nil || *fromDesign != source.String() || fromPattern != nil || deleted != nil {
		t.Fatalf("copied_from_design_id = %s, copied_from_pattern_id = %s, deleted_at set = %v", gotDesign, gotPattern, deleted != nil)
	}
}

func assertForkedDocument(t *testing.T, source, clone store.Document) {
	t.Helper()
	before := mustWireDoc(t, source)
	after := mustWireDoc(t, clone)
	if len(before.Pieces) != 2 || len(after.Pieces) != 2 || len(before.Connections) != 1 || len(after.Connections) != 1 {
		t.Fatalf("source pieces %d connections %d, clone pieces %d connections %d", len(before.Pieces), len(before.Connections), len(after.Pieces), len(after.Connections))
	}
	if after.Pieces[0].ID == before.Pieces[0].ID || after.Pieces[1].ID == before.Pieces[1].ID || after.Pieces[0].ID == after.Pieces[1].ID {
		t.Fatalf("piece ids = %s %s, source = %s %s", after.Pieces[0].ID, after.Pieces[1].ID, before.Pieces[0].ID, before.Pieces[1].ID)
	}
	if after.Connections[0].ID == before.Connections[0].ID {
		t.Fatalf("connection id = %s", after.Connections[0].ID)
	}
	if after.Connections[0].PieceA != after.Pieces[0].ID || after.Connections[0].PieceB != after.Pieces[1].ID {
		t.Fatalf("pieceA = %s, pieceB = %s, pieces = %s %s", after.Connections[0].PieceA, after.Connections[0].PieceB, after.Pieces[0].ID, after.Pieces[1].ID)
	}
	if after.Pieces[0].MaterialID != before.Pieces[0].MaterialID || after.Pieces[1].MaterialID != before.Pieces[1].MaterialID {
		t.Fatalf("materials = %s %s, source = %s %s", after.Pieces[0].MaterialID, after.Pieces[1].MaterialID, before.Pieces[0].MaterialID, before.Pieces[1].MaterialID)
	}
	if after.Pieces[0].Name != "rail" || after.Pieces[0].Transform.Position != [3]float64{1, 2, 3} {
		t.Fatalf("name = %s, position = %v", after.Pieces[0].Name, after.Pieces[0].Transform.Position)
	}
}

type wireDesignDoc struct {
	Pieces      []wireDesignPiece      `json:"pieces"`
	Connections []wireDesignConnection `json:"connections"`
}

type wireDesignPiece struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	MaterialID string `json:"materialId"`
	Transform  struct {
		Position [3]float64 `json:"positionIn"`
	} `json:"transform"`
}

type wireDesignConnection struct {
	ID     string `json:"id"`
	PieceA string `json:"pieceAId"`
	PieceB string `json:"pieceBId"`
}

func mustWireDoc(t *testing.T, doc store.Document) wireDesignDoc {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var wire wireDesignDoc
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	return wire
}

func patternCount(t *testing.T, pool *pgxpool.Pool, id store.DesignID) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM patterns WHERE source_design_id = $1`, id.String()).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func optimizationRunCount(t *testing.T, pool *pgxpool.Pool, id store.DesignID) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM optimization_runs WHERE design_id = $1`, id.String()).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
