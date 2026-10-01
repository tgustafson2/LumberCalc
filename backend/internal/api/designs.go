package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"lumbercalc/backend/internal/server"
	"lumbercalc/backend/internal/store"
)

const maxDesignBody = 1 << 20

var errBadBody = errors.New("bad design body")

func (a *API) createDesign(w http.ResponseWriter, r *http.Request, c Caller) {
	name, description, doc, err := readDesignBody(r.Body, false)
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	design, err := a.d.Save(r.Context(), c.UserID(), store.CreateDraft{
		Name:        name,
		Description: description,
		Document:    doc,
	})
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/designs/"+design.ID.String())
	server.WriteJSON(w, http.StatusCreated, toDesign(design))
}

func (a *API) listDesigns(w http.ResponseWriter, r *http.Request, c Caller) {
	rows, err := a.d.List(r.Context(), c.UserID())
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	out := make([]DesignSummary, len(rows))
	for i, row := range rows {
		out[i] = toSummary(row)
	}
	server.WriteJSON(w, http.StatusOK, out)
}

func (a *API) getDesign(w http.ResponseWriter, r *http.Request, c Caller) {
	id, err := store.ParseDesignID(r.PathValue("id"))
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	design, err := a.d.Get(r.Context(), c.UserID(), id)
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, toDesign(design))
}

func (a *API) replaceDesign(w http.ResponseWriter, r *http.Request, c Caller) {
	id, err := store.ParseDesignID(r.PathValue("id"))
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	name, description, doc, err := readDesignBody(r.Body, true)
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	design, err := a.d.Save(r.Context(), c.UserID(), store.ReplaceDraft{
		ID:          id,
		Name:        name,
		Description: description,
		Document:    doc,
	})
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, toDesign(design))
}

func (a *API) deleteDesign(w http.ResponseWriter, r *http.Request, c Caller) {
	id, err := store.ParseDesignID(r.PathValue("id"))
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	if err := a.d.Delete(r.Context(), c.UserID(), id); err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func readDesignBody(body io.Reader, documentRequired bool) (store.Name, string, store.Document, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxDesignBody+1))
	if err != nil {
		return store.Name{}, "", store.Document{}, err
	}
	if len(raw) > maxDesignBody {
		return store.Name{}, "", store.Document{}, errBadBody
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var env struct {
		Name        json.RawMessage `json:"name"`
		Description json.RawMessage `json:"description"`
		Document    json.RawMessage `json:"document"`
	}
	if err := dec.Decode(&env); err != nil || dec.More() {
		return store.Name{}, "", store.Document{}, errBadBody
	}
	if len(env.Name) == 0 || jsonNull(env.Name) {
		return store.Name{}, "", store.Document{}, errBadBody
	}
	var name string
	if err := json.Unmarshal(env.Name, &name); err != nil {
		return store.Name{}, "", store.Document{}, errBadBody
	}
	parsed, err := store.ParseName(name)
	if err != nil {
		return store.Name{}, "", store.Document{}, err
	}
	description := ""
	if len(env.Description) != 0 {
		if jsonNull(env.Description) {
			return store.Name{}, "", store.Document{}, errBadBody
		}
		if err := json.Unmarshal(env.Description, &description); err != nil {
			return store.Name{}, "", store.Document{}, errBadBody
		}
	}
	var doc store.Document
	if len(env.Document) == 0 {
		if documentRequired {
			return store.Name{}, "", store.Document{}, errBadBody
		}
		return parsed, description, doc, nil
	}
	if jsonNull(env.Document) {
		return store.Name{}, "", store.Document{}, errBadBody
	}
	if err := json.Unmarshal(env.Document, &doc); err != nil {
		var docErr *store.DocumentError
		if errors.As(err, &docErr) {
			return store.Name{}, "", store.Document{}, err
		}
		return store.Name{}, "", store.Document{}, errBadBody
	}
	return parsed, description, doc, nil
}

func jsonNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func (a *API) writeDesignErr(w http.ResponseWriter, r *http.Request, err error) {
	a.d.Log.ErrorContext(r.Context(), "design", "err", err)
	server.WriteError(w, designStatus(err))
}

func designStatus(err error) int {
	var docErr *store.DocumentError
	switch {
	case errors.Is(err, store.ErrBadDesignID), errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrBlankName), errors.Is(err, errBadBody), errors.As(err, &docErr):
		return http.StatusBadRequest
	case errors.Is(err, store.ErrMaterialMissing):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func toDesign(d store.Design) Design {
	return Design{
		Id:          d.ID.String(),
		Name:        d.Name.String(),
		Description: d.Description,
		Version:     int(d.Version),
		UpdatedAt:   d.UpdatedAt,
		Document:    d.Document,
	}
}

func toSummary(d store.DesignSummary) DesignSummary {
	return DesignSummary{
		Id:          d.ID.String(),
		Name:        d.Name.String(),
		Description: d.Description,
		Version:     int(d.Version),
		UpdatedAt:   d.UpdatedAt,
	}
}
