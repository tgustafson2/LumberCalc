-- name: UpsertPatternFromDesign :one
INSERT INTO patterns (
    source_design_id,
    publisher_id,
    name,
    description,
    schema_version,
    document,
    is_public,
    publisher_name
)
SELECT d.id, d.owner_id, d.name, d.description, d.schema_version, d.document, true, sqlc.narg('publisher_name')::text
FROM designs d
WHERE d.id = sqlc.arg('id')
  AND d.owner_id = sqlc.arg('owner_id')
  AND d.deleted_at IS NULL
ON CONFLICT (source_design_id) DO UPDATE
SET name = excluded.name,
    description = excluded.description,
    schema_version = excluded.schema_version,
    document = excluded.document,
    is_public = true,
    publisher_name = excluded.publisher_name,
    updated_at = now()
RETURNING id;

-- name: SetPatternPrivate :one
UPDATE patterns
SET is_public = false
WHERE source_design_id = sqlc.arg('source_design_id')
RETURNING id;

-- name: GetDesignPublication :one
SELECT id, is_public
FROM patterns
WHERE source_design_id = sqlc.arg('source_design_id');

-- name: LockPublicPattern :one
-- FOR KEY SHARE blocks a delete of the row until commit, so the copied_from_pattern_id
-- foreign key cannot fail. It does not block publish or unpublish, which change no key column.
SELECT id, name, description, schema_version, document
FROM patterns
WHERE id = sqlc.arg('id')
  AND is_public
FOR KEY SHARE;

-- name: ListPublicPatterns :many
SELECT id, name, description, publisher_name, updated_at
FROM patterns
WHERE is_public
ORDER BY updated_at DESC, id;

-- name: GetVisiblePattern :one
SELECT p.id, p.name, p.description, p.publisher_name, p.updated_at,
       p.is_public, p.schema_version, p.document,
       ARRAY(
           SELECT m.id FROM materials m
           WHERE m.owner_id IS NULL
             AND m.id IN (
                 SELECT (piece->>'materialId')::uuid
                 FROM jsonb_array_elements(p.document->'pieces') AS piece
             )
       )::uuid[] AS system_material_ids
FROM patterns p
WHERE p.id = sqlc.arg('id')
  AND pattern_visible(p.is_public, p.publisher_id, sqlc.arg('viewer_id'));
