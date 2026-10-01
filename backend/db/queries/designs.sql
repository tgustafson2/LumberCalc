-- name: InsertDesign :one
INSERT INTO designs (
    owner_id,
    name,
    description,
    schema_version,
    document
) VALUES (
    sqlc.arg('owner_id'),
    sqlc.arg('name'),
    sqlc.arg('description'),
    sqlc.arg('schema_version'),
    sqlc.arg('document')
)
RETURNING *;

-- name: UpdateLiveDesign :one
UPDATE designs
SET
    name = sqlc.arg('name'),
    description = sqlc.arg('description'),
    document = sqlc.arg('document'),
    schema_version = sqlc.arg('schema_version'),
    version = version + 1,
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND owner_id = sqlc.arg('owner_id')
  AND deleted_at IS NULL
RETURNING *;

-- name: GetLiveDesign :one
SELECT *
FROM designs
WHERE id = sqlc.arg('id')
  AND owner_id = sqlc.arg('owner_id')
  AND deleted_at IS NULL;

-- name: ListLiveDesigns :many
SELECT id, name, description, version, updated_at
FROM designs
WHERE owner_id = sqlc.arg('owner_id')
  AND deleted_at IS NULL
ORDER BY updated_at DESC, id DESC;

-- name: SoftDeleteDesign :execrows
UPDATE designs
SET deleted_at = now(),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND owner_id = sqlc.arg('owner_id')
  AND deleted_at IS NULL;

-- name: DeleteDesignUsages :exec
DELETE FROM design_material_usages
WHERE design_id = sqlc.arg('design_id');

-- name: InsertDesignUsage :exec
INSERT INTO design_material_usages (design_id, material_id)
VALUES (sqlc.arg('design_id'), sqlc.arg('material_id'));
