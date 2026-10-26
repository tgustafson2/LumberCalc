package store_test

import (
	"context"
	"errors"
	"testing"

	"lumbercalc/backend/internal/dbtest"
	"lumbercalc/backend/internal/solver"
	"lumbercalc/backend/internal/store"
)

func TestGetOptimizeInputResolvesFlags(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_optimize_flags")
	name, err := store.ParseName("Bench")
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.Save(ctx, owner, store.CreateDraft{
		Name:     name,
		Document: store.Document{},
	})
	if err != nil {
		t.Fatal(err)
	}

	in, err := s.GetOptimizeInput(ctx, owner, empty.ID, store.FlagChoice{})
	if err != nil {
		t.Fatal(err)
	}
	if !in.Flags.Kerf().IsZero() || in.Flags.Basis().String() != "nominal" {
		t.Fatalf("schema defaults: kerf %s basis %s", in.Flags.Kerf(), in.Flags.Basis())
	}
	if in.Design != empty.ID || in.Version != empty.Version {
		t.Fatalf("design %s version %d, saved %s version %d", in.Design, in.Version, empty.ID, empty.Version)
	}
	assertDocument(t, in.Document, store.Document{})

	if _, err := pool.Exec(ctx, `
		UPDATE user_settings
		SET default_use_kerf = true, default_kerf_in = 0.0625
		WHERE user_id = $1
	`, owner.String()); err != nil {
		t.Fatal(err)
	}
	actual, err := store.ParseDimensionBasis("actual")
	if err != nil {
		t.Fatal(err)
	}
	in, err = s.GetOptimizeInput(ctx, owner, empty.ID, store.FlagChoice{Basis: &actual})
	if err != nil {
		t.Fatal(err)
	}
	if in.Flags.Kerf().String() != "0.0625" || in.Flags.Basis() != actual {
		t.Fatalf("overlay: kerf %s basis %s", in.Flags.Kerf(), in.Flags.Basis())
	}

	off := solver.Length{}
	in, err = s.GetOptimizeInput(ctx, owner, empty.ID, store.FlagChoice{Kerf: &off})
	if err != nil {
		t.Fatal(err)
	}
	if !in.Flags.Kerf().IsZero() {
		t.Fatalf("kerf off kept %s", in.Flags.Kerf())
	}
	if in.Flags.Basis().String() != "nominal" {
		t.Fatalf("basis = %s, want the settings basis", in.Flags.Basis())
	}

	missing, err := store.ParseDesignID("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetOptimizeInput(ctx, owner, missing, store.FlagChoice{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing design err = %v, want ErrNotFound", err)
	}
}

func TestParseKerf(t *testing.T) {
	for _, raw := range []string{"0", "-1", "0.0000001"} {
		if _, err := store.ParseKerf(raw); !errors.Is(err, store.ErrBadKerf) {
			t.Fatalf("ParseKerf(%q) err = %v, want ErrBadKerf", raw, err)
		}
	}
	eighth, err := store.ParseKerf("0.125")
	if err != nil {
		t.Fatal(err)
	}
	same, err := store.ParseKerf("1.25e-1")
	if err != nil {
		t.Fatal(err)
	}
	if eighth != same {
		t.Fatalf("ParseKerf(0.125) = %s, ParseKerf(1.25e-1) = %s", eighth, same)
	}
}

func TestNewOptimizeFlagsRejectsAZeroBasis(t *testing.T) {
	if _, err := store.NewOptimizeFlags(solver.Length{}, store.DimensionBasis{}); !errors.Is(err, store.ErrBadDimensionBasis) {
		t.Fatalf("err = %v", err)
	}
	basis, err := store.ParseDimensionBasis("nominal")
	if err != nil {
		t.Fatal(err)
	}
	flags, err := store.NewOptimizeFlags(solver.Length{}, basis)
	if err != nil {
		t.Fatal(err)
	}
	if !flags.Kerf().IsZero() || flags.Basis() != basis {
		t.Fatalf("kerf %s basis %s", flags.Kerf(), flags.Basis())
	}
}

func TestParseDimensionBasis(t *testing.T) {
	for _, raw := range []string{"Actual", ""} {
		if _, err := store.ParseDimensionBasis(raw); !errors.Is(err, store.ErrBadDimensionBasis) {
			t.Fatalf("ParseDimensionBasis(%q) err = %v, want ErrBadDimensionBasis", raw, err)
		}
	}
}
