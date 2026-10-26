package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/internal/dbtest"
	"lumbercalc/backend/internal/store"
)

func TestPublishInsertsOnePattern(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
	oak := insertSystemMaterial(t, pool, "test oak")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	sourceDoc := testJoinedDocument(t, pine, oak)
	created, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        name,
		Description: "cut list",
		Document:    sourceDoc,
	})
	if err != nil {
		t.Fatal(err)
	}
	before := readDesignClock(t, pool, created.ID)

	patternID, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	if patternCount(t, pool, created.ID) != 1 {
		t.Fatalf("patterns = %d, want 1", patternCount(t, pool, created.ID))
	}
	row := mustPattern(t, pool, created.ID)
	if patternID.String() != row.id {
		t.Fatalf("id = %s, row = %s", patternID, row.id)
	}
	if row.sourceID != created.ID.String() || row.publisherID != owner.String() {
		t.Fatalf("source = %s, publisher = %s", row.sourceID, row.publisherID)
	}
	if row.name != "Bench" || row.description != "cut list" {
		t.Fatalf("name = %q, description = %q", row.name, row.description)
	}
	if row.schemaVersion != before.schemaVersion {
		t.Fatalf("schema_version = %d, design = %d", row.schemaVersion, before.schemaVersion)
	}
	if !row.sameDocument {
		t.Fatal("pattern document differs from the design document")
	}
	pieces, connections := documentIDs(t, row.document)
	if !sameStrings(pieces, []string{
		"00000000-0000-4000-8000-0000000000a1",
		"00000000-0000-4000-8000-0000000000a2",
	}) {
		t.Fatalf("piece ids = %v", pieces)
	}
	if !sameStrings(connections, []string{"joint-1"}) {
		t.Fatalf("connection ids = %v", connections)
	}
	if !row.isPublic {
		t.Fatal("is_public = false")
	}
	after := readDesignClock(t, pool, created.ID)
	if after.version != before.version || !after.updatedAt.Equal(before.updatedAt) {
		t.Fatalf("version %d updated_at %s, before version %d updated_at %s", after.version, after.updatedAt, before.version, before.updatedAt)
	}
}

func TestPublishAgainKeepsTheID(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
	oak := insertSystemMaterial(t, pool, "test oak")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        name,
		Description: "cut list",
		Document:    testJoinedDocument(t, pine, oak),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	stored := mustPattern(t, pool, created.ID)
	tag, err := pool.Exec(ctx, `
		UPDATE patterns
		SET updated_at = created_at - interval '1 day',
		    schema_version = 2
		WHERE id = $1
	`, first.String())
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated rows = %d", tag.RowsAffected())
	}
	backdated := mustPattern(t, pool, created.ID)

	nextName, err := store.ParseName("Stool")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:          created.ID,
		Name:        nextName,
		Description: "again",
		Document:    testDocument(t, pine),
		Base:        created.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	versionAfterSave := readDesignClock(t, pool, created.ID).version

	second, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("id = %s, want %s", second, first)
	}
	if patternCount(t, pool, created.ID) != 1 {
		t.Fatalf("patterns = %d, want 1", patternCount(t, pool, created.ID))
	}
	row := mustPattern(t, pool, created.ID)
	if row.id != first.String() {
		t.Fatalf("row id = %s, want %s", row.id, first)
	}
	if row.name != saved.Name.String() || row.description != saved.Description {
		t.Fatalf("name = %q, description = %q", row.name, row.description)
	}
	designSchema := readDesignClock(t, pool, created.ID).schemaVersion
	if row.schemaVersion != designSchema {
		t.Fatalf("schema_version = %d, design = %d", row.schemaVersion, designSchema)
	}
	if !row.sameDocument {
		t.Fatal("pattern document differs from the design document")
	}
	pieces, _ := documentIDs(t, row.document)
	if !sameStrings(pieces, []string{"00000000-0000-4000-8000-000000000001"}) {
		t.Fatalf("piece ids = %v", pieces)
	}
	if !row.createdAt.Equal(stored.createdAt) {
		t.Fatalf("created_at = %s, want %s", row.createdAt, stored.createdAt)
	}
	if !row.updatedAt.After(backdated.updatedAt) {
		t.Fatalf("updated_at = %s, previous = %s", row.updatedAt, backdated.updatedAt)
	}
	if !row.isPublic {
		t.Fatal("is_public = false")
	}
	if got := readDesignClock(t, pool, created.ID).version; got != versionAfterSave {
		t.Fatalf("version = %d, want %d", got, versionAfterSave)
	}
}

func TestSaveAfterPublishLeavesThePatternDocument(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
	oak := insertSystemMaterial(t, pool, "test oak")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        name,
		Description: "cut list",
		Document:    testJoinedDocument(t, pine, oak),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, owner, created.ID, store.PublicName{}); err != nil {
		t.Fatal(err)
	}
	published := mustPattern(t, pool, created.ID)

	if _, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:          created.ID,
		Name:        name,
		Description: "cut list",
		Document:    testDocument(t, pine),
		Base:        created.Version,
	}); err != nil {
		t.Fatal(err)
	}

	row := mustPattern(t, pool, created.ID)
	if !patternDocumentEquals(t, pool, row.id, published.document) {
		t.Fatal("pattern document changed")
	}
	if row.sameDocument {
		t.Fatal("pattern document matches the design after the save")
	}
	pieces, connections := documentIDs(t, row.document)
	if !sameStrings(pieces, []string{
		"00000000-0000-4000-8000-0000000000a1",
		"00000000-0000-4000-8000-0000000000a2",
	}) {
		t.Fatalf("piece ids = %v", pieces)
	}
	if !sameStrings(connections, []string{"joint-1"}) {
		t.Fatalf("connection ids = %v", connections)
	}
	designPieces, _ := documentIDs(t, designDocument(t, pool, created.ID))
	if !sameStrings(designPieces, []string{"00000000-0000-4000-8000-000000000001"}) {
		t.Fatalf("design piece ids = %v", designPieces)
	}
}

func TestPublishEmptyDesignWritesNoPattern(t *testing.T) {
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
	before := readDesignClock(t, pool, created.ID)
	_, err = s.Publish(ctx, owner, created.ID, store.PublicName{})
	if !errors.Is(err, store.ErrEmptyDesign) {
		t.Fatalf("err = %v", err)
	}
	if n := patternCount(t, pool, created.ID); n != 0 {
		t.Fatalf("patterns = %d, want 0", n)
	}
	after := readDesignClock(t, pool, created.ID)
	if after.version != before.version {
		t.Fatalf("version = %d, want %d", after.version, before.version)
	}
}

func TestPublishRejectsACustomMaterial(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
	custom := insertMaterial(t, pool, owner, "pine")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}

	first, err := s.Save(ctx, owner, store.CreateDraft{Name: name, Document: testDocument(t, pine, custom)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Publish(ctx, owner, first.ID, store.PublicName{})
	if !errors.Is(err, store.ErrCustomMaterial) || !strings.Contains(err.Error(), custom) {
		t.Fatalf("first publish err = %v, want ErrCustomMaterial with %s", err, custom)
	}
	if n := patternCount(t, pool, first.ID); n != 0 {
		t.Fatalf("patterns = %d, want 0", n)
	}

	created, err := s.Save(ctx, owner, store.CreateDraft{Name: name, Document: testDocument(t, pine)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, owner, created.ID, store.PublicName{}); err != nil {
		t.Fatal(err)
	}
	published := mustPattern(t, pool, created.ID)
	if _, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:       created.ID,
		Name:     name,
		Document: testDocument(t, pine, custom),
		Base:     created.Version,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, owner, created.ID, store.PublicName{}); !errors.Is(err, store.ErrCustomMaterial) {
		t.Fatalf("republish err = %v, want ErrCustomMaterial", err)
	}
	row := mustPattern(t, pool, created.ID)
	if !patternDocumentEquals(t, pool, row.id, published.document) || !row.updatedAt.Equal(published.updatedAt) {
		t.Fatalf("pattern changed: updated_at = %s, want %s", row.updatedAt, published.updatedAt)
	}

	const missing = "00000000-0000-4000-8000-0000000000ff"
	raw, err := json.Marshal(testDocument(t, pine, missing))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE designs SET document = $2 WHERE id = $1`, created.ID.String(), raw); err != nil {
		t.Fatal(err)
	}
	_, err = s.Publish(ctx, owner, created.ID, store.PublicName{})
	if !errors.Is(err, store.ErrCustomMaterial) || !strings.Contains(err.Error(), missing) {
		t.Fatalf("missing material err = %v, want ErrCustomMaterial with %s", err, missing)
	}
	if row := mustPattern(t, pool, created.ID); !patternDocumentEquals(t, pool, row.id, published.document) {
		t.Fatal("pattern document changed")
	}
}

func TestPublishEmptyAfterSaveLeavesThePattern(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
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
	patternID, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	published := mustPattern(t, pool, created.ID)

	if _, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:       created.ID,
		Name:     name,
		Document: store.Document{},
		Base:     created.Version,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = s.Publish(ctx, owner, created.ID, store.PublicName{})
	if !errors.Is(err, store.ErrEmptyDesign) {
		t.Fatalf("err = %v", err)
	}
	row := mustPattern(t, pool, created.ID)
	if row.id != patternID.String() || row.isPublic != published.isPublic {
		t.Fatalf("id = %s, is_public = %v", row.id, row.isPublic)
	}
	if !patternDocumentEquals(t, pool, row.id, published.document) {
		t.Fatal("pattern document changed")
	}
}

func TestPublishNotFoundLeavesThePattern(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	other := testUser(t, s, "user_other")
	pine := insertSystemMaterial(t, pool, "test pine")
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
	missing, err := store.ParseDesignID("00000000-0000-4000-8000-000000000099")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, owner, missing, store.PublicName{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}

	patternID, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	published := mustPattern(t, pool, created.ID)

	if _, err := s.Publish(ctx, other, created.ID, store.PublicName{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign err = %v", err)
	}
	if row := mustPattern(t, pool, created.ID); row.id != patternID.String() || !patternDocumentEquals(t, pool, row.id, published.document) || row.isPublic != published.isPublic {
		t.Fatalf("pattern after foreign publish = %+v", row)
	}

	if err := s.Delete(ctx, owner, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, owner, created.ID, store.PublicName{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted err = %v", err)
	}
	row := mustPattern(t, pool, created.ID)
	if row.id != patternID.String() || row.isPublic {
		t.Fatalf("id = %s, is_public = %v", row.id, row.isPublic)
	}
	if !patternDocumentEquals(t, pool, row.id, published.document) {
		t.Fatal("pattern document changed")
	}
}

func TestPublishSetsIsPublicTrue(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
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
	var seeded string
	err = pool.QueryRow(ctx, `
		INSERT INTO patterns (
			source_design_id, publisher_id, name, description, schema_version, document, is_public
		)
		SELECT id, owner_id, name, description, schema_version, document, false
		FROM designs
		WHERE id = $1
		RETURNING id::text
	`, created.ID.String()).Scan(&seeded)
	if err != nil {
		t.Fatal(err)
	}

	patternID, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	if patternID.String() != seeded {
		t.Fatalf("id = %s, want %s", patternID, seeded)
	}
	row := mustPattern(t, pool, created.ID)
	if row.id != seeded || !row.isPublic {
		t.Fatalf("id = %s, is_public = %v", row.id, row.isPublic)
	}
}

func TestPublishReturnsNotFoundWhenDeleteCommitsUnderTheLock(t *testing.T) {
	pool := dbtest.Migrated(t)
	if pool.Config().MaxConns < 2 {
		t.Fatalf("MaxConns = %d, want at least 2", pool.Config().MaxConns)
	}
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        name,
		Description: "cut list",
		Document:    testDocument(t, pine),
	})
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
		_, publishErr := s.Publish(ctx, owner, source.ID, store.PublicName{})
		errCh <- publishErr
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
			t.Fatalf("publish returned before it waited: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("publish did not wait on the source lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-errCh:
		t.Fatalf("publish returned before the delete committed: %v", err)
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
		t.Fatal("publish did not return")
	}
	if n := patternCount(t, pool, source.ID); n != 0 {
		t.Fatalf("patterns = %d, want 0", n)
	}
}

func TestUnpublishSetsIsPublicFalse(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        name,
		Description: "cut list",
		Document:    testDocument(t, pine),
	})
	if err != nil {
		t.Fatal(err)
	}
	patternID, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	before := mustPattern(t, pool, created.ID)
	clock := readDesignClock(t, pool, created.ID)

	got, err := s.Unpublish(ctx, owner, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != patternID {
		t.Fatalf("id = %s, want %s", got, patternID)
	}
	if n := patternCount(t, pool, created.ID); n != 1 {
		t.Fatalf("patterns = %d, want 1", n)
	}
	row := mustPattern(t, pool, created.ID)
	if row.isPublic {
		t.Fatal("is_public = true")
	}
	if row.id != before.id || row.sourceID != before.sourceID || row.publisherID != before.publisherID {
		t.Fatalf("id = %s source = %s publisher = %s", row.id, row.sourceID, row.publisherID)
	}
	if row.name != before.name || row.description != before.description || row.schemaVersion != before.schemaVersion {
		t.Fatalf("name = %q description = %q schema_version = %d", row.name, row.description, row.schemaVersion)
	}
	if !row.createdAt.Equal(before.createdAt) || !row.updatedAt.Equal(before.updatedAt) {
		t.Fatalf("created_at = %s updated_at = %s, before created_at = %s updated_at = %s", row.createdAt, row.updatedAt, before.createdAt, before.updatedAt)
	}
	if !patternDocumentEquals(t, pool, row.id, before.document) {
		t.Fatal("pattern document changed")
	}
	after := readDesignClock(t, pool, created.ID)
	if after.version != clock.version || after.schemaVersion != clock.schemaVersion || !after.updatedAt.Equal(clock.updatedAt) {
		t.Fatalf("version %d schema_version %d updated_at %s, before version %d schema_version %d updated_at %s", after.version, after.schemaVersion, after.updatedAt, clock.version, clock.schemaVersion, clock.updatedAt)
	}
}

func TestUnpublishAgainKeepsUpdatedAt(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
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
	patternID, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Unpublish(ctx, owner, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	published := mustPattern(t, pool, created.ID)
	tag, err := pool.Exec(ctx, `
		UPDATE patterns
		SET updated_at = created_at - interval '1 day'
		WHERE id = $1
	`, patternID.String())
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated rows = %d", tag.RowsAffected())
	}
	backdated := mustPattern(t, pool, created.ID)

	second, err := s.Unpublish(ctx, owner, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second != first || second != patternID {
		t.Fatalf("id = %s, want %s", second, patternID)
	}
	row := mustPattern(t, pool, created.ID)
	if row.isPublic {
		t.Fatal("is_public = true")
	}
	if !row.updatedAt.Equal(backdated.updatedAt) {
		t.Fatalf("updated_at = %s, want %s", row.updatedAt, backdated.updatedAt)
	}
	if !patternDocumentEquals(t, pool, row.id, published.document) {
		t.Fatal("pattern document changed")
	}
}

func TestUnpublishNotFoundLeavesThePattern(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	other := testUser(t, s, "user_other")
	pine := insertSystemMaterial(t, pool, "test pine")
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
	if _, err := s.Unpublish(ctx, owner, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unpublished err = %v", err)
	}
	if n := patternCount(t, pool, created.ID); n != 0 {
		t.Fatalf("patterns = %d, want 0", n)
	}
	missing, err := store.ParseDesignID("00000000-0000-4000-8000-000000000099")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unpublish(ctx, owner, missing); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}

	patternID, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	published := mustPattern(t, pool, created.ID)
	if !published.isPublic {
		t.Fatal("is_public = false")
	}
	if _, err := s.Unpublish(ctx, other, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign err = %v", err)
	}
	row := mustPattern(t, pool, created.ID)
	if row.id != patternID.String() || !row.isPublic || !patternDocumentEquals(t, pool, row.id, published.document) {
		t.Fatalf("id = %s, is_public = %v", row.id, row.isPublic)
	}

	if err := s.Delete(ctx, owner, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unpublish(ctx, owner, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted err = %v", err)
	}
	row = mustPattern(t, pool, created.ID)
	if row.id != patternID.String() || row.isPublic {
		t.Fatalf("id = %s, is_public = %v", row.id, row.isPublic)
	}
	if !patternDocumentEquals(t, pool, row.id, published.document) {
		t.Fatal("pattern document changed")
	}
}

func TestUnpublishEmptyDocumentHidesThePattern(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
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
	patternID, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	published := mustPattern(t, pool, created.ID)
	saved, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:       created.ID,
		Name:     name,
		Document: store.Document{},
		Base:     created.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := readDesignClock(t, pool, created.ID)

	got, err := s.Unpublish(ctx, owner, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != patternID {
		t.Fatalf("id = %s, want %s", got, patternID)
	}
	row := mustPattern(t, pool, created.ID)
	if row.isPublic {
		t.Fatal("is_public = true")
	}
	if !patternDocumentEquals(t, pool, row.id, published.document) {
		t.Fatal("pattern document changed")
	}
	if row.sameDocument {
		t.Fatal("pattern document matches the empty design")
	}
	after := readDesignClock(t, pool, created.ID)
	if after.version != clock.version || !after.updatedAt.Equal(clock.updatedAt) || after.version != saved.Version {
		t.Fatalf("version = %d updated_at = %s, saved version = %d", after.version, after.updatedAt, saved.Version)
	}
}

func TestUnpublishLeavesCopiedFromPatternID(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
	oak := insertSystemMaterial(t, pool, "test oak")
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
	patternID, err := s.Publish(ctx, owner, source.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	cloneName, err := store.ParseName("Stool")
	if err != nil {
		t.Fatal(err)
	}
	clone, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        cloneName,
		Description: "kept",
		Document:    testDocument(t, oak),
	})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := pool.Exec(ctx, `
		UPDATE designs
		SET copied_from_pattern_id = $2
		WHERE id = $1 AND copied_from_design_id IS NULL
	`, clone.ID.String(), patternID.String())
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated rows = %d", tag.RowsAffected())
	}
	beforeID, beforeDocument, beforeUpdated := readCopyColumns(t, pool, clone.ID)

	if _, err := s.Unpublish(ctx, owner, source.ID); err != nil {
		t.Fatal(err)
	}

	afterID, _, afterUpdated := readCopyColumns(t, pool, clone.ID)
	if afterID != beforeID || afterID != patternID.String() {
		t.Fatalf("copied_from_pattern_id = %s, want %s", afterID, patternID)
	}
	if !designDocumentEquals(t, pool, clone.ID, beforeDocument) {
		t.Fatal("clone document changed")
	}
	if !afterUpdated.Equal(beforeUpdated) {
		t.Fatalf("updated_at = %s, want %s", afterUpdated, beforeUpdated)
	}
}

func TestPublishAfterUnpublishRefreshesTheDocument(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
	oak := insertSystemMaterial(t, pool, "test oak")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        name,
		Description: "cut list",
		Document:    testDocument(t, pine),
	})
	if err != nil {
		t.Fatal(err)
	}
	patternID, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	published := mustPattern(t, pool, created.ID)
	nextName, err := store.ParseName("Stool")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.Save(ctx, owner, store.ReplaceDraft{
		ID:          created.ID,
		Name:        nextName,
		Description: "again",
		Document:    testDocument(t, oak),
		Base:        created.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := s.Unpublish(ctx, owner, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hidden != patternID {
		t.Fatalf("id = %s, want %s", hidden, patternID)
	}
	row := mustPattern(t, pool, created.ID)
	if row.isPublic {
		t.Fatal("is_public = true")
	}
	if !patternDocumentEquals(t, pool, row.id, published.document) {
		t.Fatal("pattern document changed")
	}

	again, err := s.Publish(ctx, owner, created.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	if again != patternID {
		t.Fatalf("id = %s, want %s", again, patternID)
	}
	row = mustPattern(t, pool, created.ID)
	if !row.isPublic {
		t.Fatal("is_public = false")
	}
	if !row.sameDocument {
		t.Fatal("pattern document differs from the design document")
	}
	if row.name != saved.Name.String() || row.description != saved.Description {
		t.Fatalf("name = %q description = %q", row.name, row.description)
	}
}

func TestUnpublishReturnsNotFoundWhenDeleteCommitsUnderTheLock(t *testing.T) {
	pool := dbtest.Migrated(t)
	if pool.Config().MaxConns < 2 {
		t.Fatalf("MaxConns = %d, want at least 2", pool.Config().MaxConns)
	}
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_owner")
	pine := insertSystemMaterial(t, pool, "test pine")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.Save(ctx, owner, store.CreateDraft{
		Name:        name,
		Description: "cut list",
		Document:    testDocument(t, pine),
	})
	if err != nil {
		t.Fatal(err)
	}
	patternID, err := s.Publish(ctx, owner, source.ID, store.PublicName{})
	if err != nil {
		t.Fatal(err)
	}
	published := mustPattern(t, pool, source.ID)

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
		_, unpublishErr := s.Unpublish(ctx, owner, source.ID)
		errCh <- unpublishErr
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
			t.Fatalf("unpublish returned before it waited: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("unpublish did not wait on the source lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-errCh:
		t.Fatalf("unpublish returned before the delete committed: %v", err)
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
		t.Fatal("unpublish did not return")
	}
	row := mustPattern(t, pool, source.ID)
	if row.id != patternID.String() || row.isPublic {
		t.Fatalf("id = %s, is_public = %v", row.id, row.isPublic)
	}
	if !patternDocumentEquals(t, pool, row.id, published.document) {
		t.Fatal("pattern document changed")
	}
}

func readCopyColumns(t *testing.T, pool *pgxpool.Pool, id store.DesignID) (patternID string, document []byte, updatedAt time.Time) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		SELECT copied_from_pattern_id::text, document, updated_at
		FROM designs
		WHERE id = $1
	`, id.String()).Scan(&patternID, &document, &updatedAt)
	if err != nil {
		t.Fatal(err)
	}
	return patternID, document, updatedAt
}

func designDocumentEquals(t *testing.T, pool *pgxpool.Pool, id store.DesignID, document []byte) bool {
	t.Helper()
	var same bool
	err := pool.QueryRow(context.Background(), `
		SELECT document = $2::jsonb
		FROM designs
		WHERE id = $1
	`, id.String(), document).Scan(&same)
	if err != nil {
		t.Fatal(err)
	}
	return same
}

type patternRow struct {
	id            string
	sourceID      string
	publisherID   string
	name          string
	description   string
	schemaVersion int32
	document      []byte
	sameDocument  bool
	isPublic      bool
	createdAt     time.Time
	updatedAt     time.Time
}

func mustPattern(t *testing.T, pool *pgxpool.Pool, id store.DesignID) patternRow {
	t.Helper()
	var row patternRow
	err := pool.QueryRow(context.Background(), `
		SELECT p.id::text, p.source_design_id::text, p.publisher_id::text,
		       p.name, p.description, p.schema_version, p.document,
		       p.document = d.document, p.is_public, p.created_at, p.updated_at
		FROM patterns p
		JOIN designs d ON d.id = p.source_design_id
		WHERE p.source_design_id = $1
	`, id.String()).Scan(
		&row.id, &row.sourceID, &row.publisherID,
		&row.name, &row.description, &row.schemaVersion, &row.document,
		&row.sameDocument, &row.isPublic, &row.createdAt, &row.updatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func patternDocumentEquals(t *testing.T, pool *pgxpool.Pool, patternID string, document []byte) bool {
	t.Helper()
	var same bool
	err := pool.QueryRow(context.Background(), `
		SELECT document = $2::jsonb
		FROM patterns
		WHERE id = $1
	`, patternID, document).Scan(&same)
	if err != nil {
		t.Fatal(err)
	}
	return same
}

type designClock struct {
	version       int32
	schemaVersion int32
	updatedAt     time.Time
}

func readDesignClock(t *testing.T, pool *pgxpool.Pool, id store.DesignID) designClock {
	t.Helper()
	var clock designClock
	err := pool.QueryRow(context.Background(), `
		SELECT version, schema_version, updated_at
		FROM designs
		WHERE id = $1
	`, id.String()).Scan(&clock.version, &clock.schemaVersion, &clock.updatedAt)
	if err != nil {
		t.Fatal(err)
	}
	return clock
}

func designDocument(t *testing.T, pool *pgxpool.Pool, id store.DesignID) []byte {
	t.Helper()
	var raw []byte
	err := pool.QueryRow(context.Background(), `SELECT document FROM designs WHERE id = $1`, id.String()).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func documentIDs(t *testing.T, raw []byte) (pieces, connections []string) {
	t.Helper()
	var body struct {
		Pieces []struct {
			ID string `json:"id"`
		} `json:"pieces"`
		Connections []struct {
			ID string `json:"id"`
		} `json:"connections"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	pieces = make([]string, len(body.Pieces))
	for i, piece := range body.Pieces {
		pieces[i] = piece.ID
	}
	connections = make([]string, len(body.Connections))
	for i, connection := range body.Connections {
		connections[i] = connection.ID
	}
	return pieces, connections
}
