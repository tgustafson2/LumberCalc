package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"lumbercalc/backend/internal/store/internal/q"
)

type PriceID struct{ v [16]byte }

func ParsePriceID(raw string) (PriceID, error) {
	if !uuidPattern.MatchString(raw) {
		return PriceID{}, ErrBadPriceID
	}
	var id PriceID
	if err := decodeUUID(raw, &id.v); err != nil {
		return PriceID{}, ErrBadPriceID
	}
	return id, nil
}

func (id PriceID) String() string { return formatUUID(id.v) }

// Cents is a non-negative integer number of cents that fits the integer column.
type Cents struct{ v int32 }

// ParseCents accepts one JSON integer token from 0 to 2147483647.
func ParseCents(raw string) (Cents, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
	if err != nil || n < 0 {
		return Cents{}, ErrBadCents
	}
	return Cents{v: int32(n)}, nil
}

func (c Cents) Int32() int32 { return c.v }

// PriceKey is the unique key of a price inside one owner's list.
type PriceKey struct {
	Material MaterialID
	Length   Inches
}

type Price struct {
	ID        PriceID
	Key       PriceKey
	Cents     Cents
	UpdatedAt time.Time
}

var (
	ErrPriceNotFound    = errors.New("price not found")
	ErrBadPriceID       = errors.New("price id is not a uuid")
	ErrBadCents         = errors.New("price cents are not a non-negative integer")
	ErrPriceTaken       = errors.New("price already exists for this material and length")
	ErrMaterialUnusable = errors.New("material is not a system material or owned by the caller")
)

// ListPrices returns the caller's prices ordered by material, length, and id.
func (s *Store) ListPrices(ctx context.Context, owner UserID) ([]Price, error) {
	rows, err := s.q.ListOwnedPrices(ctx, uuidPG(owner.v))
	if err != nil {
		return nil, err
	}
	out := make([]Price, len(rows))
	for i, row := range rows {
		item, err := priceFromRow(row)
		if err != nil {
			return nil, err
		}
		out[i] = item
	}
	return out, nil
}

// CreatePrice inserts one price for a system material or a material the caller owns.
// A missing or foreign material is ErrMaterialUnusable. A repeated key is ErrPriceTaken.
func (s *Store) CreatePrice(ctx context.Context, owner UserID, key PriceKey, cents Cents) (Price, error) {
	length, err := key.Length.pg()
	if err != nil {
		return Price{}, err
	}
	row, err := s.q.InsertPriceForUsableMaterial(ctx, q.InsertPriceForUsableMaterialParams{
		OwnerID:    uuidPG(owner.v),
		MaterialID: uuidPG(key.Material.v),
		LengthIn:   length,
		PriceCents: cents.v,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Price{}, ErrMaterialUnusable
	}
	if err != nil {
		return Price{}, mapPriceWriteErr(err)
	}
	return priceFromRow(row)
}

// UpdatePrice sets the cents of one of the caller's prices.
// A missing id and another user's id are both ErrPriceNotFound.
func (s *Store) UpdatePrice(ctx context.Context, owner UserID, id PriceID, cents Cents) (Price, error) {
	row, err := s.q.UpdateOwnedPriceCents(ctx, q.UpdateOwnedPriceCentsParams{
		PriceCents: cents.v,
		ID:         uuidPG(id.v),
		OwnerID:    uuidPG(owner.v),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Price{}, ErrPriceNotFound
	}
	if err != nil {
		return Price{}, err
	}
	return priceFromRow(row)
}

// DeletePrice removes one of the caller's prices. A second delete is ErrPriceNotFound.
func (s *Store) DeletePrice(ctx context.Context, owner UserID, id PriceID) error {
	n, err := s.q.DeleteOwnedPrice(ctx, q.DeleteOwnedPriceParams{
		ID:      uuidPG(id.v),
		OwnerID: uuidPG(owner.v),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrPriceNotFound
	}
	return nil
}

func priceFromRow(row q.Price) (Price, error) {
	if !row.ID.Valid || !row.MaterialID.Valid || !row.UpdatedAt.Valid {
		return Price{}, errors.New("price row is missing id, material_id, or updated_at")
	}
	length, err := inchesFromNumeric(row.LengthIn)
	if err != nil {
		return Price{}, err
	}
	return Price{
		ID: PriceID{v: row.ID.Bytes},
		Key: PriceKey{
			Material: MaterialID{v: row.MaterialID.Bytes},
			Length:   length,
		},
		Cents:     Cents{v: row.PriceCents},
		UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func mapPriceWriteErr(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch {
	case pgErr.Code == "23505" && pgErr.ConstraintName == "prices_owner_id_material_id_length_in_key":
		return ErrPriceTaken
	case pgErr.Code == "23503" && pgErr.ConstraintName == "prices_material_id_fkey":
		return ErrMaterialUnusable
	default:
		return err
	}
}
