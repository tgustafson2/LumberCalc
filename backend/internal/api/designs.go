package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"lumbercalc/backend/internal/server"
	"lumbercalc/backend/internal/store"
)

const maxDesignBody = 1 << 20

const staleVersionMessage = "stale version"

var errBadBody = errors.New("bad design body")

type designEnvelope struct {
	Name        json.RawMessage `json:"name"`
	Description json.RawMessage `json:"description"`
	Document    json.RawMessage `json:"document"`
}

type replaceEnvelope struct {
	designEnvelope
	Version json.RawMessage `json:"version"`
}

func (a *API) createDesign(w http.ResponseWriter, r *http.Request, c Caller) {
	var env designEnvelope
	if err := decodeDesignJSON(r.Body, &env); err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	name, description, doc, err := designFields(env.Name, env.Description, env.Document, false)
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
	var env replaceEnvelope
	if err := decodeDesignJSON(r.Body, &env); err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	name, description, doc, err := designFields(env.Name, env.Description, env.Document, true)
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	seen, err := seenVersion(env.Version)
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	design, err := a.d.Save(r.Context(), c.UserID(), store.ReplaceDraft{
		ID:          id,
		Name:        name,
		Description: description,
		Document:    doc,
		Base:        seen,
	})
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, toDesign(design))
}

func (a *API) copyDesign(w http.ResponseWriter, r *http.Request, c Caller) {
	id, err := store.ParseDesignID(r.PathValue("id"))
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	if err := rejectCopyBody(r.Body); err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	design, err := a.d.Copy(r.Context(), c.UserID(), id)
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/designs/"+design.ID.String())
	server.WriteJSON(w, http.StatusCreated, toDesign(design))
}

func rejectCopyBody(body io.Reader) error {
	raw, err := io.ReadAll(io.LimitReader(body, maxDesignBody+1))
	if err != nil {
		return err
	}
	if len(raw) > maxDesignBody || len(bytes.TrimSpace(raw)) != 0 {
		return errBadBody
	}
	return nil
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

func decodeDesignJSON(body io.Reader, dest any) error {
	raw, err := io.ReadAll(io.LimitReader(body, maxDesignBody+1))
	if err != nil {
		return err
	}
	if len(raw) > maxDesignBody {
		return errBadBody
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil || dec.More() {
		return errBadBody
	}
	return nil
}

func designFields(nameRaw, descriptionRaw, documentRaw json.RawMessage, documentRequired bool) (store.Name, string, store.Document, error) {
	if len(nameRaw) == 0 || jsonNull(nameRaw) {
		return store.Name{}, "", store.Document{}, errBadBody
	}
	var name string
	if err := json.Unmarshal(nameRaw, &name); err != nil {
		return store.Name{}, "", store.Document{}, errBadBody
	}
	parsed, err := store.ParseName(name)
	if err != nil {
		return store.Name{}, "", store.Document{}, err
	}
	description := ""
	if len(descriptionRaw) != 0 {
		if jsonNull(descriptionRaw) {
			return store.Name{}, "", store.Document{}, errBadBody
		}
		if err := json.Unmarshal(descriptionRaw, &description); err != nil {
			return store.Name{}, "", store.Document{}, errBadBody
		}
	}
	var doc store.Document
	if len(documentRaw) == 0 {
		if documentRequired {
			return store.Name{}, "", store.Document{}, errBadBody
		}
		return parsed, description, doc, nil
	}
	if jsonNull(documentRaw) {
		return store.Name{}, "", store.Document{}, errBadBody
	}
	if err := json.Unmarshal(documentRaw, &doc); err != nil {
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

func seenVersion(raw json.RawMessage) (int32, error) {
	token := bytes.TrimSpace(raw)
	if len(token) == 0 || bytes.Equal(token, []byte("null")) || !integerToken(token) {
		return 0, errBadBody
	}
	n, err := strconv.ParseInt(string(token), 10, 32)
	if err != nil || n < 1 {
		return 0, errBadBody
	}
	return int32(n), nil
}

func integerToken(token []byte) bool {
	if len(token) == 0 {
		return false
	}
	i := 0
	if token[0] == '-' {
		if len(token) == 1 {
			return false
		}
		i = 1
	}
	for ; i < len(token); i++ {
		if token[i] < '0' || token[i] > '9' {
			return false
		}
	}
	return true
}

func (a *API) writeDesignErr(w http.ResponseWriter, r *http.Request, err error) {
	a.d.Log.ErrorContext(r.Context(), "design", "err", err)
	if errors.Is(err, store.ErrStale) {
		server.WriteJSON(w, http.StatusConflict, server.Error{Error: staleVersionMessage})
		return
	}
	server.WriteError(w, designStatus(err))
}

func designStatus(err error) int {
	var docErr *store.DocumentError
	switch {
	case errors.Is(err, store.ErrBadDesignID), errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrBlankName), errors.Is(err, errBadBody), errors.As(err, &docErr):
		return http.StatusBadRequest
	case errors.Is(err, store.ErrStale), errors.Is(err, store.ErrMaterialMissing):
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
