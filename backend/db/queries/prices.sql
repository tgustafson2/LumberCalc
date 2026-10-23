-- name: ListOwnedPrices :many
SELECT *
FROM prices
WHERE owner_id = sqlc.arg('owner_id')
ORDER BY material_id, length_in, id;

-- name: InsertPriceForUsableMaterial :one
INSERT INTO prices (owner_id, material_id, length_in, price_cents)
SELECT sqlc.arg('owner_id')::uuid, m.id, sqlc.arg('length_in')::numeric, sqlc.arg('price_cents')::integer
FROM materials m
WHERE m.id = sqlc.arg('material_id')
  AND material_usable(m.owner_id, sqlc.arg('owner_id')::uuid)
RETURNING *;

-- name: UpdateOwnedPriceCents :one
UPDATE prices
SET price_cents = sqlc.arg('price_cents'),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND owner_id = sqlc.arg('owner_id')
RETURNING *;

-- name: DeleteOwnedPrice :execrows
DELETE FROM prices
WHERE id = sqlc.arg('id')
  AND owner_id = sqlc.arg('owner_id');
