package api

import (
	"errors"
	"net/http"

	"lumbercalc/backend/internal/server"
	"lumbercalc/backend/internal/store"
)

func (a *API) listPatterns(w http.ResponseWriter, r *http.Request, _ Caller) {
	rows, err := a.d.ListPublicPatterns(r.Context())
	if err != nil {
		a.writePatternErr(w, r, err)
		return
	}
	out := make([]PatternSummary, len(rows))
	for i, row := range rows {
		out[i] = toPatternSummary(row)
	}
	server.WriteJSON(w, http.StatusOK, out)
}

func (a *API) getPattern(w http.ResponseWriter, r *http.Request, c Caller) {
	id, err := store.ParsePatternID(r.PathValue("id"))
	if err != nil {
		a.writePatternErr(w, r, err)
		return
	}
	p, err := a.d.GetPattern(r.Context(), c.UserID(), id)
	if err != nil {
		a.writePatternErr(w, r, err)
		return
	}
	summary := toPatternSummary(p.PatternSummary)
	server.WriteJSON(w, http.StatusOK, Pattern{
		Id:            summary.Id,
		Name:          summary.Name,
		Description:   summary.Description,
		PublisherName: summary.PublisherName,
		PublishedAt:   summary.PublishedAt,
		IsPublic:      p.Public,
		Document:      p.Document,
	})
}

func (a *API) writePatternErr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrPatternNotFound) {
		server.WriteError(w, http.StatusNotFound)
		return
	}
	a.d.Log.ErrorContext(r.Context(), "pattern", "err", err)
	server.WriteError(w, http.StatusInternalServerError)
}

func toPatternSummary(p store.PatternSummary) PatternSummary {
	var publisher *string
	if name, ok := p.Publisher.Value(); ok {
		publisher = &name
	}
	return PatternSummary{
		Id:            p.ID.String(),
		Name:          p.Name.String(),
		Description:   p.Description,
		PublisherName: publisher,
		PublishedAt:   p.PublishedAt,
	}
}
