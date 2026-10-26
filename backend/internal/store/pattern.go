package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"lumbercalc/backend/internal/store/internal/q"
)

// ErrPatternNotFound is a missing pattern, a pattern the viewer cannot see, and a malformed id.
var ErrPatternNotFound = errors.New("pattern not found")

type PatternID struct{ v [16]byte }

func (id PatternID) String() string { return formatUUID(id.v) }

func ParsePatternID(raw string) (PatternID, error) {
	if !uuidPattern.MatchString(raw) {
		return PatternID{}, ErrPatternNotFound
	}
	var id PatternID
	if err := decodeUUID(raw, &id.v); err != nil {
		return PatternID{}, ErrPatternNotFound
	}
	return id, nil
}

// PublicName is a publisher label that is safe to show to every signed-in user.
// The zero value is no label.
type PublicName struct{ s string }

// ParsePublicName returns the zero PublicName for a blank name, an email-shaped name,
// or a name equal to the Clerk subject.
func ParsePublicName(raw string, subject ClerkUserID) PublicName {
	s := strings.TrimSpace(raw)
	if s == "" || strings.Contains(s, "@") || s == subject.s {
		return PublicName{}
	}
	return PublicName{s: s}
}

func (n PublicName) Value() (string, bool) { return n.s, n.s != "" }

type PatternSummary struct {
	ID          PatternID
	Name        Name
	Description string
	Publisher   PublicName
	PublishedAt time.Time
}

type Pattern struct {
	PatternSummary
	Public   bool
	Document PublicDocument
}

// ListPublicPatterns returns every public pattern, newest first.
func (s *Store) ListPublicPatterns(ctx context.Context) ([]PatternSummary, error) {
	rows, err := s.q.ListPublicPatterns(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PatternSummary, len(rows))
	for i, row := range rows {
		item, err := patternSummaryFromRow(row.ID, row.Name, row.Description, row.PublisherName, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out[i] = item
	}
	return out, nil
}

// GetPattern returns a public pattern, or a private pattern to its publisher.
func (s *Store) GetPattern(ctx context.Context, viewer UserID, id PatternID) (Pattern, error) {
	row, err := s.q.GetVisiblePattern(ctx, q.GetVisiblePatternParams{
		ID:       uuidPG(id.v),
		ViewerID: uuidPG(viewer.v),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Pattern{}, ErrPatternNotFound
	}
	if err != nil {
		return Pattern{}, err
	}
	return patternFromRow(row)
}

func patternFromRow(row q.GetVisiblePatternRow) (Pattern, error) {
	summary, err := patternSummaryFromRow(row.ID, row.Name, row.Description, row.PublisherName, row.UpdatedAt)
	if err != nil {
		return Pattern{}, err
	}
	doc, err := parseStored("pattern", row.Document, row.SchemaVersion)
	if err != nil {
		return Pattern{}, err
	}
	system := make(map[string]struct{}, len(row.SystemMaterialIds))
	for _, id := range row.SystemMaterialIds {
		if !id.Valid {
			return Pattern{}, errors.New("system material id is null")
		}
		system[formatUUID(id.Bytes)] = struct{}{}
	}
	return Pattern{
		PatternSummary: summary,
		Public:         row.IsPublic,
		Document:       publicDocument(doc, system),
	}, nil
}

func patternSummaryFromRow(id pgtype.UUID, name, description string, publisher pgtype.Text, updated pgtype.Timestamptz) (PatternSummary, error) {
	if !id.Valid || !updated.Valid {
		return PatternSummary{}, errors.New("pattern row is missing id or updated_at")
	}
	var label PublicName
	if publisher.Valid {
		label = PublicName{s: publisher.String}
	}
	return PatternSummary{
		ID:          PatternID{v: id.Bytes},
		Name:        Name{s: name},
		Description: description,
		Publisher:   label,
		PublishedAt: updated.Time,
	}, nil
}

func publicNamePG(n PublicName) pgtype.Text {
	s, ok := n.Value()
	return pgtype.Text{String: s, Valid: ok}
}
