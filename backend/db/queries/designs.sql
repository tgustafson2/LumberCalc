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
  AND version = sqlc.arg('base_version')
RETURNING *;

-- name: LiveDesignExists :one
SELECT EXISTS (
    SELECT 1
    FROM designs
    WHERE id = sqlc.arg('id')
      AND owner_id = sqlc.arg('owner_id')
      AND deleted_at IS NULL
);

-- name: GetLiveDesign :one
SELECT *
FROM designs
WHERE id = sqlc.arg('id')
  AND owner_id = sqlc.arg('owner_id')
  AND deleted_at IS NULL;

-- name: LockLiveDesign :one
SELECT *
FROM designs
WHERE id = sqlc.arg('id')
  AND owner_id = sqlc.arg('owner_id')
  AND deleted_at IS NULL
FOR UPDATE;

-- name: InsertCopiedDesign :one
INSERT INTO designs (
    owner_id,
    name,
    description,
    schema_version,
    document,
    copied_from_design_id,
    copied_from_pattern_id
) VALUES (
    sqlc.arg('owner_id'),
    sqlc.arg('name'),
    sqlc.arg('description'),
    sqlc.arg('schema_version'),
    sqlc.arg('document'),
    sqlc.narg('copied_from_design_id'),
    sqlc.narg('copied_from_pattern_id')
)
RETURNING *;

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

-- name: InsertDesignUsageForUsableMaterial :execrows
INSERT INTO design_material_usages (design_id, material_id)
SELECT sqlc.arg('design_id')::uuid, m.id
FROM materials m
WHERE m.id = sqlc.arg('material_id')::uuid
  AND material_usable(m.owner_id, sqlc.arg('owner_id')::uuid);
