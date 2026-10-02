package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"lumbercalc/backend/internal/store/internal/q"
)

type DesignID struct{ v [16]byte }

// ParseDesignID accepts the same UUID text as a document id.
func ParseDesignID(raw string) (DesignID, error) {
	if !uuidPattern.MatchString(raw) {
		return DesignID{}, ErrBadDesignID
	}
	var id DesignID
	if err := decodeUUID(raw, &id.v); err != nil {
		return DesignID{}, ErrBadDesignID
	}
	return id, nil
}

func (id DesignID) String() string { return formatUUID(id.v) }

type Name struct{ s string }

// ParseName rejects a blank name.
// TrimSpace removes more characters than the SQL btrim check.
func ParseName(raw string) (Name, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Name{}, ErrBlankName
	}
	return Name{s: s}, nil
}

func (n Name) String() string { return n.s }

type DesignSummary struct {
	ID          DesignID
	Name        Name
	Description string
	Version     int32
	UpdatedAt   time.Time
}

type Design struct {
	DesignSummary
	Document Document
}

// Draft is one design write. CreateDraft and ReplaceDraft are the only drafts.
type Draft interface{ draftKind() }

type CreateDraft struct {
	Name        Name
	Description string
	Document    Document
}

type ReplaceDraft struct {
	ID          DesignID
	Name        Name
	Description string
	Document    Document
	Base        int32
}

func (CreateDraft) draftKind()  {}
func (ReplaceDraft) draftKind() {}

var (
	ErrNotFound        = errors.New("design not found")
	ErrBadDesignID     = errors.New("design id is not a uuid")
	ErrBlankName       = errors.New("design name is blank")
	ErrMaterialMissing = errors.New("design material is missing")
	ErrStale           = errors.New("stale version")
)

func (s *Store) Save(ctx context.Context, owner UserID, draft Draft) (Design, error) {
	var row q.Design
	err := s.withTx(ctx, func(qtx *q.Queries) error {
		var doc Document
		switch d := draft.(type) {
		case CreateDraft:
			doc = d.Document
			raw, err := json.Marshal(doc)
			if err != nil {
				return err
			}
			row, err = qtx.InsertDesign(ctx, q.InsertDesignParams{
				OwnerID:       uuidPG(owner.v),
				Name:          d.Name.String(),
				Description:   d.Description,
				SchemaVersion: doc.schemaVersion(),
				Document:      raw,
			})
			if err != nil {
				return err
			}
		case ReplaceDraft:
			doc = d.Document
			raw, err := json.Marshal(doc)
			if err != nil {
				return err
			}
			row, err = qtx.UpdateLiveDesign(ctx, q.UpdateLiveDesignParams{
				Name:          d.Name.String(),
				Description:   d.Description,
				Document:      raw,
				SchemaVersion: doc.schemaVersion(),
				ID:            uuidPG(d.ID.v),
				OwnerID:       uuidPG(owner.v),
				BaseVersion:   d.Base,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				// A full Get unmarshals the document and can hide a stale write.
				exists, existsErr := qtx.LiveDesignExists(ctx, q.LiveDesignExistsParams{
					ID:      uuidPG(d.ID.v),
					OwnerID: uuidPG(owner.v),
				})
				if existsErr != nil {
					return existsErr
				}
				if !exists {
					return ErrNotFound
				}
				return ErrStale
			}
			if err != nil {
				return err
			}
		default:
			return errors.New("unknown design draft")
		}
		if err := qtx.DeleteDesignUsages(ctx, row.ID); err != nil {
			return err
		}
		return insertUsages(ctx, qtx, row.ID, doc.materialIDs())
	})
	if err != nil {
		return Design{}, err
	}
	return designFromRow(row)
}

func (s *Store) Get(ctx context.Context, owner UserID, id DesignID) (Design, error) {
	row, err := s.q.GetLiveDesign(ctx, q.GetLiveDesignParams{
		ID:      uuidPG(id.v),
		OwnerID: uuidPG(owner.v),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Design{}, ErrNotFound
	}
	if err != nil {
		return Design{}, err
	}
	return designFromRow(row)
}

// List returns the caller's live designs, newest first.
// An owner with no live designs gets an empty slice, so the list encodes as [].
func (s *Store) List(ctx context.Context, owner UserID) ([]DesignSummary, error) {
	rows, err := s.q.ListLiveDesigns(ctx, uuidPG(owner.v))
	if err != nil {
		return nil, err
	}
	out := make([]DesignSummary, len(rows))
	for i, row := range rows {
		item, err := summaryFromRow(row.ID, row.Name, row.Description, row.Version, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out[i] = item
	}
	return out, nil
}

// Delete soft-deletes one live design. A second delete returns ErrNotFound.
// Usage rows stay in place.
func (s *Store) Delete(ctx context.Context, owner UserID, id DesignID) error {
	n, err := s.q.SoftDeleteDesign(ctx, q.SoftDeleteDesignParams{
		ID:      uuidPG(id.v),
		OwnerID: uuidPG(owner.v),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Copy deep-clones one live design that this owner still has.
// A second call inserts another design. It does not return the first clone.
// A missing, foreign, or soft-deleted source is ErrNotFound.
// The clone's copied_from_design_id is the source.
// The clone's copied_from_pattern_id is null, including when the source points at a pattern.
// The source row is not updated.
func (s *Store) Copy(ctx context.Context, owner UserID, id DesignID) (Design, error) {
	var row q.Design
	err := s.withTx(ctx, func(qtx *q.Queries) error {
		locked, err := qtx.LockLiveDesign(ctx, q.LockLiveDesignParams{
			ID:      uuidPG(id.v),
			OwnerID: uuidPG(owner.v),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		source, err := designFromRow(locked)
		if err != nil {
			return err
		}
		cloned, err := source.Document.fork()
		if err != nil {
			return err
		}
		raw, err := json.Marshal(cloned)
		if err != nil {
			return err
		}
		row, err = qtx.InsertCopiedDesign(ctx, q.InsertCopiedDesignParams{
			OwnerID:            uuidPG(owner.v),
			Name:               copyName(source.Name).String(),
			Description:        source.Description,
			SchemaVersion:      cloned.schemaVersion(),
			Document:           raw,
			CopiedFromDesignID: uuidPG(source.ID.v),
		})
		if err != nil {
			return err
		}
		return insertUsages(ctx, qtx, row.ID, cloned.materialIDs())
	})
	if err != nil {
		return Design{}, err
	}
	return designFromRow(row)
}

// copyName is the only copy title. Names are not unique, so a repeated copy keeps this title.
func copyName(source Name) Name {
	return Name{s: source.s + " copy"}
}

func insertUsages(ctx context.Context, qtx *q.Queries, designID pgtype.UUID, ids []materialID) error {
	for _, id := range ids {
		err := qtx.InsertDesignUsage(ctx, q.InsertDesignUsageParams{
			DesignID:   designID,
			MaterialID: uuidPG(id.v),
		})
		if err != nil {
			return usageErr(err)
		}
	}
	return nil
}

func designFromRow(row q.Design) (Design, error) {
	summary, err := summaryFromRow(row.ID, row.Name, row.Description, row.Version, row.UpdatedAt)
	if err != nil {
		return Design{}, err
	}
	var doc Document
	if err := json.Unmarshal(row.Document, &doc); err != nil {
		// A stored document that does not parse is a server fault.
		// Do not wrap DocumentError. The handler maps that type to 400.
		return Design{}, fmt.Errorf("stored design document is invalid: %v", err)
	}
	if row.SchemaVersion != doc.schemaVersion() {
		return Design{}, fmt.Errorf("design schema_version %d does not match document", row.SchemaVersion)
	}
	return Design{DesignSummary: summary, Document: doc}, nil
}

func summaryFromRow(id pgtype.UUID, name, description string, version int32, updated pgtype.Timestamptz) (DesignSummary, error) {
	if !id.Valid || !updated.Valid {
		return DesignSummary{}, errors.New("design row is missing id or updated_at")
	}
	return DesignSummary{
		ID:          DesignID{v: id.Bytes},
		Name:        Name{s: name},
		Description: description,
		Version:     version,
		UpdatedAt:   updated.Time,
	}, nil
}

func usageErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "design_material_usages_material_id_fkey" {
		return ErrMaterialMissing
	}
	return err
}

func uuidPG(v [16]byte) pgtype.UUID {
	return pgtype.UUID{Bytes: v, Valid: true}
}
