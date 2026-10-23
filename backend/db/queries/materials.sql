-- name: ListVisibleMaterials :many
SELECT *
FROM materials
WHERE material_usable(owner_id, sqlc.arg('owner_id')::uuid)
ORDER BY name, id;

-- name: InsertMaterial :one
INSERT INTO materials (
    owner_id,
    name,
    species,
    grade,
    nominal_thickness_in,
    nominal_width_in,
    actual_thickness_in,
    actual_width_in,
    rough_thickness_in,
    rough_width_in
) VALUES (
    sqlc.arg('owner_id'),
    sqlc.arg('name'),
    sqlc.narg('species'),
    sqlc.narg('grade'),
    sqlc.arg('nominal_thickness_in'),
    sqlc.arg('nominal_width_in'),
    sqlc.arg('actual_thickness_in'),
    sqlc.arg('actual_width_in'),
    sqlc.arg('rough_thickness_in'),
    sqlc.arg('rough_width_in')
)
RETURNING *;

-- name: GetMaterialOwner :one
SELECT id, owner_id
FROM materials
WHERE id = sqlc.arg('id')
FOR UPDATE;

-- name: UpdateOwnedMaterial :one
UPDATE materials
SET
    name = sqlc.arg('name'),
    species = sqlc.narg('species'),
    grade = sqlc.narg('grade'),
    nominal_thickness_in = sqlc.arg('nominal_thickness_in'),
    nominal_width_in = sqlc.arg('nominal_width_in'),
    actual_thickness_in = sqlc.arg('actual_thickness_in'),
    actual_width_in = sqlc.arg('actual_width_in'),
    rough_thickness_in = sqlc.arg('rough_thickness_in'),
    rough_width_in = sqlc.arg('rough_width_in'),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND owner_id = sqlc.arg('owner_id')
RETURNING *;

-- name: DeleteOwnedMaterial :execrows
DELETE FROM materials
WHERE id = sqlc.arg('id')
  AND owner_id = sqlc.arg('owner_id');
