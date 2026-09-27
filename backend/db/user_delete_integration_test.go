package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/internal/dbtest"
)

func TestDeleteUserRemovesOwnedMaterialUse(t *testing.T) {
	pool := dbtest.Migrated(t)
	ctx := context.Background()

	owner := insertUser(t, pool, "user_owner")
	material := insertMaterial(t, pool, owner, "2x4")
	if _, err := pool.Exec(ctx, `
		INSERT INTO prices (owner_id, material_id, length_in, price_cents)
		VALUES ($1, $2, 96, 500)`, owner, material); err != nil {
		t.Fatal(err)
	}
	design := insertDesign(t, pool, owner, "bench")
	if _, err := pool.Exec(ctx, `
		INSERT INTO design_material_usages (design_id, material_id)
		VALUES ($1, $2)`, design, material); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, owner); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users WHERE id = $1) +
			(SELECT count(*) FROM materials WHERE id = $2) +
			(SELECT count(*) FROM prices WHERE material_id = $2) +
			(SELECT count(*) FROM designs WHERE id = $3) +
			(SELECT count(*) FROM design_material_usages WHERE design_id = $3)`,
		owner, material, design).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("rows left after user delete = %d, want 0", left)
	}

	owner = insertUser(t, pool, "user_material_owner")
	other := insertUser(t, pool, "user_other")
	material = insertMaterial(t, pool, owner, "shared")
	design = insertDesign(t, pool, other, "their bench")
	if _, err := pool.Exec(ctx, `
		INSERT INTO design_material_usages (design_id, material_id)
		VALUES ($1, $2)`, design, material); err != nil {
		t.Fatal(err)
	}

	_, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, owner)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23001" || pgErr.ConstraintName != "design_material_usages_material_id_fkey" {
		t.Fatalf("delete user with another user's usage = %v, want 23001 on design_material_usages_material_id_fkey", err)
	}
	_, err = pool.Exec(ctx, `DELETE FROM materials WHERE id = $1`, material)
	pgErr = nil
	if !errors.As(err, &pgErr) || pgErr.Code != "23001" || pgErr.ConstraintName != "design_material_usages_material_id_fkey" {
		t.Fatalf("delete referenced material = %v, want 23001 on design_material_usages_material_id_fkey", err)
	}
}

func insertUser(t *testing.T, pool *pgxpool.Pool, clerk string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		WITH u AS (
			INSERT INTO users (clerk_user_id) VALUES ($1) RETURNING id
		), s AS (
			INSERT INTO user_settings (user_id) SELECT id FROM u
		)
		SELECT id::text FROM u`, clerk).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func insertMaterial(t *testing.T, pool *pgxpool.Pool, owner, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO materials (
			owner_id, name,
			nominal_thickness_in, nominal_width_in,
			actual_thickness_in, actual_width_in,
			rough_thickness_in, rough_width_in)
		VALUES ($1, $2, 2, 4, 1.5, 3.5, 1.5, 3.5)
		RETURNING id::text`, owner, name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func insertDesign(t *testing.T, pool *pgxpool.Pool, owner, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO designs (owner_id, name, schema_version, document)
		VALUES ($1, $2, 1, '{}')
		RETURNING id::text`, owner, name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
