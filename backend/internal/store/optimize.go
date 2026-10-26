package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"lumbercalc/backend/internal/solver"
	"lumbercalc/backend/internal/store/internal/q"
)

type DimensionBasis struct{ s string }

var dimensionBases = [...]DimensionBasis{{s: "nominal"}, {s: "actual"}}

var (
	ErrBadKerf           = errors.New("kerf is not a positive length in whole microinches")
	ErrBadDimensionBasis = errors.New("dimension basis is not nominal or actual")
)

func ParseDimensionBasis(raw string) (DimensionBasis, error) {
	for _, basis := range dimensionBases {
		if raw == basis.s {
			return basis, nil
		}
	}
	return DimensionBasis{}, ErrBadDimensionBasis
}

func (b DimensionBasis) String() string { return b.s }

func ParseKerf(raw string) (solver.Length, error) {
	length, err := solver.ParseLength(raw)
	if err != nil || length.IsZero() {
		return solver.Length{}, ErrBadKerf
	}
	return length, nil
}

type OptimizeFlags struct {
	kerf  solver.Length
	basis DimensionBasis
}

func (f OptimizeFlags) Kerf() solver.Length { return f.kerf }

func (f OptimizeFlags) Basis() DimensionBasis { return f.basis }

func NewOptimizeFlags(kerf solver.Length, basis DimensionBasis) (OptimizeFlags, error) {
	if basis.s == "" {
		return OptimizeFlags{}, ErrBadDimensionBasis
	}
	return OptimizeFlags{kerf: kerf, basis: basis}, nil
}

type FlagChoice struct {
	Kerf  *solver.Length
	Basis *DimensionBasis
}

type OptimizeInput struct {
	Design   DesignID
	Version  int32
	Document Document
	Flags    OptimizeFlags
}

func (s *Store) GetOptimizeInput(ctx context.Context, owner UserID, id DesignID, choice FlagChoice) (OptimizeInput, error) {
	row, err := s.q.GetLiveDesign(ctx, q.GetLiveDesignParams{
		ID:      uuidPG(id.v),
		OwnerID: uuidPG(owner.v),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return OptimizeInput{}, ErrNotFound
	}
	if err != nil {
		return OptimizeInput{}, err
	}
	summary, doc, err := parseDesignRow(row)
	if err != nil {
		return OptimizeInput{}, err
	}
	settings, err := s.q.GetUserSettings(ctx, uuidPG(owner.v))
	if err != nil {
		return OptimizeInput{}, fmt.Errorf("user_settings: %w", err)
	}
	defaults, err := settingsFlags(settings)
	if err != nil {
		return OptimizeInput{}, err
	}
	return OptimizeInput{
		Design:   summary.ID,
		Version:  summary.Version,
		Document: doc,
		Flags:    choice.over(defaults),
	}, nil
}

func settingsFlags(row q.GetUserSettingsRow) (OptimizeFlags, error) {
	var kerf solver.Length
	if row.DefaultUseKerf {
		inches, err := inchesFromNumeric(row.DefaultKerfIn)
		if err != nil {
			return OptimizeFlags{}, fmt.Errorf("user_settings: %w", err)
		}
		kerf, err = ParseKerf(inches.String())
		if err != nil {
			// %v stops ErrBadKerf from matching a request error. The row is not a request.
			return OptimizeFlags{}, fmt.Errorf("user_settings: %v", err)
		}
	}
	basis, err := ParseDimensionBasis(row.DefaultDimensionBasis)
	if err != nil {
		return OptimizeFlags{}, fmt.Errorf("user_settings: %v", err)
	}
	return OptimizeFlags{kerf: kerf, basis: basis}, nil
}

func (c FlagChoice) over(defaults OptimizeFlags) OptimizeFlags {
	if c.Kerf != nil {
		defaults.kerf = *c.Kerf
	}
	if c.Basis != nil {
		defaults.basis = *c.Basis
	}
	return defaults
}
