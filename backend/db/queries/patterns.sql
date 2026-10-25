-- name: UpsertPatternFromDesign :one
INSERT INTO patterns (
    source_design_id,
    publisher_id,
    name,
    description,
    schema_version,
    document,
    is_public
)
SELECT d.id, d.owner_id, d.name, d.description, d.schema_version, d.document, true
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
    updated_at = now()
RETURNING id;
