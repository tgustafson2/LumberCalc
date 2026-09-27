package db_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"lumbercalc/backend/db"
	"lumbercalc/backend/internal/dbtest"
)

func TestMigrateAgainstPostgres(t *testing.T) {
	pool := dbtest.Fresh(t)
	ctx := context.Background()

	if err := db.CheckApplied(ctx, pool); err == nil || err.Error() != "applied_migrations does not exist, run cmd/migrate" {
		t.Fatalf("CheckApplied before migrate = %v", err)
	}

	results := make([][]string, 4)
	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = db.Migrate(ctx, pool)
		}(i)
	}
	wg.Wait()
	appliedBy := 0
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("run %d: %v", i, errs[i])
		}
		switch {
		case slices.Equal(results[i], []string{"001_init"}):
			appliedBy++
		case len(results[i]) == 0:
		default:
			t.Fatalf("run %d applied %v", i, results[i])
		}
	}
	if appliedBy != 1 {
		t.Fatalf("%d concurrent runs applied 001_init, want 1", appliedBy)
	}

	if err := db.CheckApplied(ctx, pool); err != nil {
		t.Fatalf("CheckApplied after migrate = %v", err)
	}

	var tables []string
	rows, err := pool.Query(ctx, `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"applied_migrations", "design_material_usages", "designs", "materials",
		"optimization_runs", "patterns", "prices", "user_settings", "users",
	}
	if !slices.Equal(tables, want) {
		t.Fatalf("tables = %v, want %v", tables, want)
	}

	if _, err := pool.Exec(ctx, "UPDATE applied_migrations SET sha256 = sha256(sha256) WHERE version = 1"); err != nil {
		t.Fatal(err)
	}
	const drift = "migration 001_init was changed after it was applied as 001_init"
	if err := db.CheckApplied(ctx, pool); err == nil || err.Error() != drift {
		t.Fatalf("CheckApplied after drift = %v, want %q", err, drift)
	}
	if _, err := db.Migrate(ctx, pool); err == nil || err.Error() != drift {
		t.Fatalf("Migrate after drift = %v, want %q", err, drift)
	}
}
