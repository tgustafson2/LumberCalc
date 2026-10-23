package store_test

import (
	"context"
	"errors"
	"testing"

	"lumbercalc/backend/internal/dbtest"
	"lumbercalc/backend/internal/store"
)

func TestParseCents(t *testing.T) {
	for _, raw := range []string{"0", "1", "2147483647", " 12 "} {
		got, err := store.ParseCents(raw)
		if err != nil {
			t.Fatalf("ParseCents(%q) err = %v", raw, err)
		}
		if raw == "0" && got.Int32() != 0 {
			t.Fatalf("ParseCents(0) = %d", got.Int32())
		}
		if raw == "2147483647" && got.Int32() != 2147483647 {
			t.Fatalf("ParseCents(max) = %d", got.Int32())
		}
	}
	for _, raw := range []string{"", " ", "-1", "1.5", "1e2", "2147483648", "100.0", "1.0"} {
		_, err := store.ParseCents(raw)
		if !errors.Is(err, store.ErrBadCents) {
			t.Fatalf("ParseCents(%q) err = %v, want ErrBadCents", raw, err)
		}
	}
}

func TestParsePriceID(t *testing.T) {
	id, err := store.ParsePriceID("AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE")
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee" {
		t.Fatalf("id = %s", id)
	}
	_, err = store.ParsePriceID("not-a-uuid")
	if !errors.Is(err, store.ErrBadPriceID) {
		t.Fatalf("err = %v", err)
	}
}

func TestPrices(t *testing.T) {
	pool := dbtest.Migrated(t)
	s := store.New(pool)
	ctx := context.Background()
	owner := testUser(t, s, "user_price_owner")
	other := testUser(t, s, "user_price_other")

	none, err := s.ListPrices(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if none == nil || len(none) != 0 {
		t.Fatalf("empty list = %#v", none)
	}

	listed, err := s.ListMaterials(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	var system store.Material
	var sawSystem bool
	for _, row := range listed {
		if row.System {
			system = row
			sawSystem = true
			break
		}
	}
	if !sawSystem {
		t.Fatal("list has no system material")
	}

	zero, err := store.ParseCents("0")
	if err != nil {
		t.Fatal(err)
	}
	length96 := mustInches(t, "96")
	created, err := s.CreatePrice(ctx, owner, store.PriceKey{Material: system.ID, Length: length96}, zero)
	if err != nil {
		t.Fatalf("system price err = %v", err)
	}
	if created.Cents.Int32() != 0 {
		t.Fatalf("cents = %d, want 0", created.Cents.Int32())
	}
	if created.Key.Material != system.ID {
		t.Fatalf("material = %s, want %s", created.Key.Material, system.ID)
	}
	if created.UpdatedAt.IsZero() {
		t.Fatal("updated_at is zero")
	}

	length8 := mustInches(t, "8")
	eightCents, err := store.ParseCents("800")
	if err != nil {
		t.Fatal(err)
	}
	shorter, err := s.CreatePrice(ctx, owner, store.PriceKey{Material: system.ID, Length: length8}, eightCents)
	if err != nil {
		t.Fatal(err)
	}

	custom := mustCreate(t, s, owner, "Price Cedar")
	again, err := s.ListMaterials(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !materialListed(again, custom.ID) {
		t.Fatal("caller material is absent from ListMaterials")
	}
	customCents, err := store.ParseCents("1250")
	if err != nil {
		t.Fatal(err)
	}
	customPrice, err := s.CreatePrice(ctx, owner, store.PriceKey{Material: custom.ID, Length: length96}, customCents)
	if err != nil {
		t.Fatalf("custom price err = %v", err)
	}

	foreign := mustCreate(t, s, other, "Price Walnut")
	ownerList, err := s.ListMaterials(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if materialListed(ownerList, foreign.ID) {
		t.Fatal("foreign material is in ListMaterials")
	}
	_, err = s.CreatePrice(ctx, owner, store.PriceKey{Material: foreign.ID, Length: length96}, customCents)
	if !errors.Is(err, store.ErrMaterialUnusable) {
		t.Fatalf("foreign material err = %v", err)
	}

	missing, err := store.ParseMaterialID("00000000-0000-4000-8000-000000000099")
	if err != nil {
		t.Fatal(err)
	}
	if materialListed(ownerList, missing) {
		t.Fatal("missing material is in ListMaterials")
	}
	_, err = s.CreatePrice(ctx, owner, store.PriceKey{Material: missing, Length: length96}, customCents)
	if !errors.Is(err, store.ErrMaterialUnusable) {
		t.Fatalf("missing material err = %v", err)
	}

	_, err = s.CreatePrice(ctx, owner, store.PriceKey{Material: system.ID, Length: mustInches(t, "96.0")}, zero)
	if !errors.Is(err, store.ErrPriceTaken) {
		t.Fatalf("duplicate length err = %v", err)
	}
	otherSame, err := s.CreatePrice(ctx, other, store.PriceKey{Material: system.ID, Length: length96}, customCents)
	if err != nil {
		t.Fatalf("other user same key err = %v", err)
	}

	ownerPrices, err := s.ListPrices(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !priceBefore(ownerPrices, shorter.ID, created.ID) {
		t.Fatalf("list order = %v", priceIDs(ownerPrices))
	}
	otherPrices, err := s.ListPrices(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if priceListed(otherPrices, created.ID) || priceListed(otherPrices, shorter.ID) || priceListed(otherPrices, customPrice.ID) {
		t.Fatalf("other list = %v", priceIDs(otherPrices))
	}
	if !priceListed(otherPrices, otherSame.ID) {
		t.Fatal("other list misses its own price")
	}

	raised, err := store.ParseCents("599")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdatePrice(ctx, other, created.ID, raised)
	if !errors.Is(err, store.ErrPriceNotFound) {
		t.Fatalf("foreign update err = %v", err)
	}
	if err := s.DeletePrice(ctx, other, created.ID); !errors.Is(err, store.ErrPriceNotFound) {
		t.Fatalf("foreign delete err = %v", err)
	}
	unchanged := mustPrice(t, s, owner, created.ID)
	if unchanged.Cents.Int32() != 0 {
		t.Fatalf("cents after foreign write = %d, want 0", unchanged.Cents.Int32())
	}
	if unchanged.Key.Material != system.ID || unchanged.Key.Length.String() != created.Key.Length.String() {
		t.Fatalf("key changed to %s %s", unchanged.Key.Material, unchanged.Key.Length)
	}

	updated, err := s.UpdatePrice(ctx, owner, created.ID, raised)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Cents.Int32() != 599 {
		t.Fatalf("updated cents = %d", updated.Cents.Int32())
	}
	if updated.Key.Material != created.Key.Material || updated.Key.Length.String() != created.Key.Length.String() {
		t.Fatal("update changed the key")
	}

	if err := s.DeletePrice(ctx, owner, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePrice(ctx, owner, created.ID); !errors.Is(err, store.ErrPriceNotFound) {
		t.Fatalf("second delete err = %v", err)
	}

	if err := s.DeleteMaterial(ctx, owner, custom.ID); !errors.Is(err, store.ErrMaterialInUse) {
		t.Fatalf("priced material delete err = %v", err)
	}
}

func materialListed(rows []store.Material, id store.MaterialID) bool {
	for _, row := range rows {
		if row.ID == id {
			return true
		}
	}
	return false
}

func priceListed(rows []store.Price, id store.PriceID) bool {
	for _, row := range rows {
		if row.ID == id {
			return true
		}
	}
	return false
}

func priceBefore(rows []store.Price, first, second store.PriceID) bool {
	firstAt := -1
	secondAt := -1
	for i, row := range rows {
		if row.ID == first {
			firstAt = i
		}
		if row.ID == second {
			secondAt = i
		}
	}
	return firstAt >= 0 && secondAt >= 0 && firstAt < secondAt
}

func priceIDs(rows []store.Price) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.ID.String()
	}
	return out
}

func mustPrice(t *testing.T, s *store.Store, owner store.UserID, id store.PriceID) store.Price {
	t.Helper()
	rows, err := s.ListPrices(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("price %s is missing", id)
	return store.Price{}
}
