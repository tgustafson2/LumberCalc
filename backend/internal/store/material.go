package store

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"lumbercalc/backend/internal/store/internal/q"
)

type MaterialID struct{ v [16]byte }

// ParseMaterialID accepts the same UUID text as a design id.
func ParseMaterialID(raw string) (MaterialID, error) {
	if !uuidPattern.MatchString(raw) {
		return MaterialID{}, ErrBadMaterialID
	}
	var id MaterialID
	if err := decodeUUID(raw, &id.v); err != nil {
		return MaterialID{}, ErrBadMaterialID
	}
	return id, nil
}

func (id MaterialID) String() string { return formatUUID(id.v) }

// Inches is one JSON number token. float64 cannot hold every inch length.
type Inches struct{ token string }

// ParseInches accepts a JSON number token and rejects a value that is not greater than zero.
func ParseInches(raw string) (Inches, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed[0] == '"' {
		return Inches{}, ErrBadInches
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	var token json.Number
	if err := dec.Decode(&token); err != nil {
		return Inches{}, ErrBadInches
	}
	var extra struct{}
	if err := dec.Decode(&extra); err != io.EOF {
		return Inches{}, ErrBadInches
	}
	rat, ok := ratFromJSONNumber(token.String())
	if !ok || rat.Sign() <= 0 {
		return Inches{}, ErrBadInches
	}
	return Inches{token: token.String()}, nil
}

func (n Inches) String() string { return n.token }

func (n Inches) pg() (pgtype.Numeric, error) {
	var num pgtype.Numeric
	if err := num.Scan(n.token); err != nil {
		return pgtype.Numeric{}, err
	}
	return num, nil
}

type Material struct {
	ID                 MaterialID
	Name               Name
	Species            *string
	Grade              *string
	System             bool
	NominalThicknessIn Inches
	NominalWidthIn     Inches
	ActualThicknessIn  Inches
	ActualWidthIn      Inches
	RoughThicknessIn   Inches
	RoughWidthIn       Inches
	UpdatedAt          time.Time
}

// MaterialInput is one create or full replace. Species and grade store SQL null when blank.
type MaterialInput struct {
	Name               Name
	Species            string
	Grade              string
	NominalThicknessIn Inches
	NominalWidthIn     Inches
	ActualThicknessIn  Inches
	ActualWidthIn      Inches
	RoughThicknessIn   Inches
	RoughWidthIn       Inches
}

var (
	ErrMaterialNotFound = errors.New("material not found")
	ErrBadMaterialID    = errors.New("material id is not a uuid")
	ErrNotOwner         = errors.New("material is not owned by the caller")
	ErrNameTaken        = errors.New("material name is already used")
	ErrMaterialInUse    = errors.New("material is in use")
	ErrBadInches        = errors.New("material inches are not a positive number")
)

// ListMaterials returns system rows and the caller's rows.
// An owner with no visible rows gets an empty slice, so the list encodes as [].
func (s *Store) ListMaterials(ctx context.Context, owner UserID) ([]Material, error) {
	rows, err := s.q.ListVisibleMaterials(ctx, uuidPG(owner.v))
	if err != nil {
		return nil, err
	}
	out := make([]Material, len(rows))
	for i, row := range rows {
		item, err := materialFromRow(row)
		if err != nil {
			return nil, err
		}
		out[i] = item
	}
	return out, nil
}

// CreateMaterial inserts a custom row owned by the caller.
func (s *Store) CreateMaterial(ctx context.Context, owner UserID, in MaterialInput) (Material, error) {
	params, err := in.insertParams(owner)
	if err != nil {
		return Material{}, err
	}
	row, err := s.q.InsertMaterial(ctx, params)
	if err != nil {
		return Material{}, mapMaterialWriteErr(err)
	}
	return materialFromRow(row)
}

// UpdateMaterial replaces one custom row.
// The owner is read before the write so a missing id stays not found and any other row stays not-owner.
func (s *Store) UpdateMaterial(ctx context.Context, owner UserID, id MaterialID, in MaterialInput) (Material, error) {
	var row q.Material
	err := s.withTx(ctx, func(qtx *q.Queries) error {
		current, err := qtx.GetMaterialOwner(ctx, uuidPG(id.v))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMaterialNotFound
		}
		if err != nil {
			return err
		}
		if err := requireOwner(current.OwnerID, owner); err != nil {
			return err
		}
		params, err := in.updateParams(owner, id)
		if err != nil {
			return err
		}
		row, err = qtx.UpdateOwnedMaterial(ctx, params)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMaterialNotFound
		}
		return mapMaterialWriteErr(err)
	})
	if err != nil {
		return Material{}, err
	}
	return materialFromRow(row)
}

// DeleteMaterial removes one custom row.
// A price or a design usage blocks the delete. A second delete of a missing id returns not found.
func (s *Store) DeleteMaterial(ctx context.Context, owner UserID, id MaterialID) error {
	return s.withTx(ctx, func(qtx *q.Queries) error {
		current, err := qtx.GetMaterialOwner(ctx, uuidPG(id.v))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMaterialNotFound
		}
		if err != nil {
			return err
		}
		if err := requireOwner(current.OwnerID, owner); err != nil {
			return err
		}
		n, err := qtx.DeleteOwnedMaterial(ctx, q.DeleteOwnedMaterialParams{
			ID:      uuidPG(id.v),
			OwnerID: uuidPG(owner.v),
		})
		if err != nil {
			return mapMaterialWriteErr(err)
		}
		if n == 0 {
			return ErrMaterialNotFound
		}
		return nil
	})
}

func (in MaterialInput) insertParams(owner UserID) (q.InsertMaterialParams, error) {
	sizes, err := in.sizes()
	if err != nil {
		return q.InsertMaterialParams{}, err
	}
	return q.InsertMaterialParams{
		OwnerID:            uuidPG(owner.v),
		Name:               in.Name.String(),
		Species:            nullText(in.Species),
		Grade:              nullText(in.Grade),
		NominalThicknessIn: sizes[0],
		NominalWidthIn:     sizes[1],
		ActualThicknessIn:  sizes[2],
		ActualWidthIn:      sizes[3],
		RoughThicknessIn:   sizes[4],
		RoughWidthIn:       sizes[5],
	}, nil
}

func (in MaterialInput) updateParams(owner UserID, id MaterialID) (q.UpdateOwnedMaterialParams, error) {
	sizes, err := in.sizes()
	if err != nil {
		return q.UpdateOwnedMaterialParams{}, err
	}
	return q.UpdateOwnedMaterialParams{
		Name:               in.Name.String(),
		Species:            nullText(in.Species),
		Grade:              nullText(in.Grade),
		NominalThicknessIn: sizes[0],
		NominalWidthIn:     sizes[1],
		ActualThicknessIn:  sizes[2],
		ActualWidthIn:      sizes[3],
		RoughThicknessIn:   sizes[4],
		RoughWidthIn:       sizes[5],
		ID:                 uuidPG(id.v),
		OwnerID:            uuidPG(owner.v),
	}, nil
}

func (in MaterialInput) sizes() ([6]pgtype.Numeric, error) {
	fields := [6]Inches{
		in.NominalThicknessIn,
		in.NominalWidthIn,
		in.ActualThicknessIn,
		in.ActualWidthIn,
		in.RoughThicknessIn,
		in.RoughWidthIn,
	}
	var out [6]pgtype.Numeric
	for i, field := range fields {
		num, err := field.pg()
		if err != nil {
			return out, err
		}
		out[i] = num
	}
	return out, nil
}

func materialFromRow(row q.Material) (Material, error) {
	if !row.ID.Valid || !row.UpdatedAt.Valid {
		return Material{}, errors.New("material row is missing id or updated_at")
	}
	name, err := ParseName(row.Name)
	if err != nil {
		return Material{}, err
	}
	nominalT, err := inchesFromNumeric(row.NominalThicknessIn)
	if err != nil {
		return Material{}, err
	}
	nominalW, err := inchesFromNumeric(row.NominalWidthIn)
	if err != nil {
		return Material{}, err
	}
	actualT, err := inchesFromNumeric(row.ActualThicknessIn)
	if err != nil {
		return Material{}, err
	}
	actualW, err := inchesFromNumeric(row.ActualWidthIn)
	if err != nil {
		return Material{}, err
	}
	roughT, err := inchesFromNumeric(row.RoughThicknessIn)
	if err != nil {
		return Material{}, err
	}
	roughW, err := inchesFromNumeric(row.RoughWidthIn)
	if err != nil {
		return Material{}, err
	}
	return Material{
		ID:                 MaterialID{v: row.ID.Bytes},
		Name:               name,
		Species:            textPtr(row.Species),
		Grade:              textPtr(row.Grade),
		System:             !row.OwnerID.Valid,
		NominalThicknessIn: nominalT,
		NominalWidthIn:     nominalW,
		ActualThicknessIn:  actualT,
		ActualWidthIn:      actualW,
		RoughThicknessIn:   roughT,
		RoughWidthIn:       roughW,
		UpdatedAt:          row.UpdatedAt.Time,
	}, nil
}

func requireOwner(owner pgtype.UUID, caller UserID) error {
	if !owner.Valid || owner.Bytes != caller.v {
		return ErrNotOwner
	}
	return nil
}

func nullText(raw string) pgtype.Text {
	s := strings.TrimSpace(raw)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	s := value.String
	return &s
}

func inchesFromNumeric(n pgtype.Numeric) (Inches, error) {
	if !n.Valid || n.NaN || n.InfinityModifier != pgtype.Finite {
		return Inches{}, errors.New("material inches are not a finite number")
	}
	text, err := n.Value()
	if err != nil {
		return Inches{}, err
	}
	raw, ok := text.(string)
	if !ok {
		return Inches{}, errors.New("material inches are not text")
	}
	return ParseInches(raw)
}

func mapMaterialWriteErr(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505":
		return ErrNameTaken
	case "23001":
		return ErrMaterialInUse
	default:
		return err
	}
}

func ratFromJSONNumber(token string) (*big.Rat, bool) {
	if rat, ok := new(big.Rat).SetString(token); ok {
		return rat, true
	}
	idx := strings.IndexAny(token, "eE")
	if idx <= 0 || idx == len(token)-1 {
		return nil, false
	}
	mant, ok := new(big.Rat).SetString(token[:idx])
	if !ok {
		return nil, false
	}
	exp, err := strconv.Atoi(token[idx+1:])
	if err != nil {
		return nil, false
	}
	shift := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(absInt(exp))), nil)
	scale := new(big.Rat).SetInt(shift)
	if exp < 0 {
		mant.Quo(mant, scale)
	} else {
		mant.Mul(mant, scale)
	}
	return mant, true
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
