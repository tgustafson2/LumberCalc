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
	ErrNotFound    = errors.New("design not found")
	ErrBadDesignID = errors.New("design id is not a uuid")
	ErrBlankName   = errors.New("design name is blank")
	ErrStale       = errors.New("stale version")
	ErrEmptyDesign = errors.New("empty design")
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
		return insertUsages(ctx, qtx, owner, row.ID, doc.materialIDs())
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

// Publish refreshes the name, the document, and updated_at on a second call, and keeps the pattern id.
func (s *Store) Publish(ctx context.Context, owner UserID, id DesignID, publisher PublicName) (PatternID, error) {
	var patternID PatternID
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
		design, err := designFromRow(locked)
		if err != nil {
			return err
		}
		if design.Document.empty() {
			return ErrEmptyDesign
		}
		raw, err := qtx.UpsertPatternFromDesign(ctx, q.UpsertPatternFromDesignParams{
			PublisherName: publicNamePG(publisher),
			ID:            locked.ID,
			OwnerID:       locked.OwnerID,
		})
		if err != nil {
			return err
		}
		if !raw.Valid {
			return errors.New("pattern row is missing id")
		}
		patternID = PatternID{v: raw.Bytes}
		return nil
	})
	if err != nil {
		return PatternID{}, err
	}
	return patternID, nil
}

// Unpublish hides the pattern of one live design.
// An empty live document still hides the pattern.
// A second call keeps updated_at.
// A missing pattern is ErrNotFound.
func (s *Store) Unpublish(ctx context.Context, owner UserID, id DesignID) (PatternID, error) {
	var patternID PatternID
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
		raw, err := qtx.SetPatternPrivate(ctx, locked.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !raw.Valid {
			return errors.New("pattern row is missing id")
		}
		patternID = PatternID{v: raw.Bytes}
		return nil
	})
	if err != nil {
		return PatternID{}, err
	}
	return patternID, nil
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
		row, err = insertClone(ctx, qtx, owner, copyName(source.Name), source.Description, source.Document, fromDesign(source.ID))
		return err
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

// cloneSource is the one lineage column that a cloned design sets.
// fromDesign and fromPattern are the only sources, so a clone never sets both columns.
type cloneSource interface {
	copiedFrom() (design, pattern pgtype.UUID)
}

type fromDesign DesignID
type fromPattern PatternID

func (s fromDesign) copiedFrom() (design, pattern pgtype.UUID) {
	return uuidPG(s.v), pgtype.UUID{}
}

func (s fromPattern) copiedFrom() (design, pattern pgtype.UUID) {
	return pgtype.UUID{}, uuidPG(s.v)
}

// insertClone writes a version 1 design that owner owns, with new piece and connection ids,
// and its usage rows, in the transaction of qtx.
// A material that owner cannot use is ErrMaterialUnusable. The caller's transaction then rolls back.
func insertClone(ctx context.Context, qtx *q.Queries, owner UserID, name Name, description string, source Document, from cloneSource) (q.Design, error) {
	cloned, err := source.fork()
	if err != nil {
		return q.Design{}, err
	}
	raw, err := json.Marshal(cloned)
	if err != nil {
		return q.Design{}, err
	}
	design, pattern := from.copiedFrom()
	row, err := qtx.InsertCopiedDesign(ctx, q.InsertCopiedDesignParams{
		OwnerID:             uuidPG(owner.v),
		Name:                name.String(),
		Description:         description,
		SchemaVersion:       cloned.schemaVersion(),
		Document:            raw,
		CopiedFromDesignID:  design,
		CopiedFromPatternID: pattern,
	})
	if err != nil {
		return q.Design{}, err
	}
	if err := insertUsages(ctx, qtx, owner, row.ID, cloned.materialIDs()); err != nil {
		return q.Design{}, err
	}
	return row, nil
}

func insertUsages(ctx context.Context, qtx *q.Queries, owner UserID, designID pgtype.UUID, ids []MaterialID) error {
	for _, id := range ids {
		n, err := qtx.InsertDesignUsageForUsableMaterial(ctx, q.InsertDesignUsageForUsableMaterialParams{
			DesignID:   designID,
			MaterialID: uuidPG(id.v),
			OwnerID:    uuidPG(owner.v),
		})
		if err != nil {
			return usageErr(err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrMaterialUnusable, id)
		}
	}
	return nil
}

func designFromRow(row q.Design) (Design, error) {
	summary, err := summaryFromRow(row.ID, row.Name, row.Description, row.Version, row.UpdatedAt)
	if err != nil {
		return Design{}, err
	}
	doc, err := parseStored("design", row.Document, row.SchemaVersion)
	if err != nil {
		return Design{}, err
	}
	return Design{DesignSummary: summary, Document: doc}, nil
}

// parseStored parses a document that the store wrote earlier.
// A document that does not parse, or a schema_version that does not match, is a server fault.
// The error does not wrap DocumentError, because the handler maps that type to 400.
func parseStored(kind string, raw []byte, schemaVersion int32) (Document, error) {
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Document{}, fmt.Errorf("stored %s document is invalid: %v", kind, err)
	}
	if schemaVersion != doc.schemaVersion() {
		return Document{}, fmt.Errorf("%s schema_version %d does not match document", kind, schemaVersion)
	}
	return doc, nil
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
	// A delete that commits after this statement reads the material still fails the foreign key.
	// That race is the same error as a missing id.
	if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "design_material_usages_material_id_fkey" {
		return ErrMaterialUnusable
	}
	return err
}

func uuidPG(v [16]byte) pgtype.UUID {
	return pgtype.UUID{Bytes: v, Valid: true}
}
