package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"lumbercalc/backend/internal/server"
	"lumbercalc/backend/internal/store"
)

const (
	nameTakenMessage     = "name already used"
	materialInUseMessage = "material in use"
)

func (a *API) createMaterial(w http.ResponseWriter, r *http.Request, c Caller) {
	in, err := decodeMaterialInput(r.Body)
	if err != nil {
		a.writeMaterialErr(w, r, err)
		return
	}
	row, err := a.d.CreateMaterial(r.Context(), c.UserID(), in)
	if err != nil {
		a.writeMaterialErr(w, r, err)
		return
	}
	server.WriteJSON(w, http.StatusCreated, toMaterial(row))
}

func (a *API) listMaterials(w http.ResponseWriter, r *http.Request, c Caller) {
	rows, err := a.d.ListMaterials(r.Context(), c.UserID())
	if err != nil {
		a.writeMaterialErr(w, r, err)
		return
	}
	out := make([]Material, len(rows))
	for i, row := range rows {
		out[i] = toMaterial(row)
	}
	server.WriteJSON(w, http.StatusOK, out)
}

func (a *API) replaceMaterial(w http.ResponseWriter, r *http.Request, c Caller) {
	id, err := store.ParseMaterialID(r.PathValue("id"))
	if err != nil {
		a.writeMaterialErr(w, r, err)
		return
	}
	in, err := decodeMaterialInput(r.Body)
	if err != nil {
		a.writeMaterialErr(w, r, err)
		return
	}
	row, err := a.d.UpdateMaterial(r.Context(), c.UserID(), id, in)
	if err != nil {
		a.writeMaterialErr(w, r, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, toMaterial(row))
}

func (a *API) deleteMaterial(w http.ResponseWriter, r *http.Request, c Caller) {
	id, err := store.ParseMaterialID(r.PathValue("id"))
	if err != nil {
		a.writeMaterialErr(w, r, err)
		return
	}
	if err := a.d.DeleteMaterial(r.Context(), c.UserID(), id); err != nil {
		a.writeMaterialErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) writeMaterialErr(w http.ResponseWriter, r *http.Request, err error) {
	a.d.Log.ErrorContext(r.Context(), "material", "err", err)
	switch {
	case errors.Is(err, store.ErrNameTaken):
		server.WriteJSON(w, http.StatusConflict, server.Error{Error: nameTakenMessage})
	case errors.Is(err, store.ErrMaterialInUse):
		server.WriteJSON(w, http.StatusConflict, server.Error{Error: materialInUseMessage})
	default:
		server.WriteError(w, materialStatus(err))
	}
}

func materialStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrBadMaterialID), errors.Is(err, store.ErrMaterialNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrNotOwner):
		return http.StatusForbidden
	case errors.Is(err, store.ErrBlankName), errors.Is(err, store.ErrBadInches), errors.Is(err, errBadBody):
		return http.StatusBadRequest
	case errors.Is(err, store.ErrNameTaken), errors.Is(err, store.ErrMaterialInUse):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func toMaterial(m store.Material) Material {
	return Material{
		Id:                 m.ID.String(),
		Name:               m.Name.String(),
		Species:            m.Species,
		Grade:              m.Grade,
		System:             m.System,
		NominalThicknessIn: json.Number(m.NominalThicknessIn.String()),
		NominalWidthIn:     json.Number(m.NominalWidthIn.String()),
		ActualThicknessIn:  json.Number(m.ActualThicknessIn.String()),
		ActualWidthIn:      json.Number(m.ActualWidthIn.String()),
		RoughThicknessIn:   json.Number(m.RoughThicknessIn.String()),
		RoughWidthIn:       json.Number(m.RoughWidthIn.String()),
		UpdatedAt:          m.UpdatedAt,
	}
}

func decodeMaterialInput(body io.Reader) (store.MaterialInput, error) {
	var env MaterialWrite
	if err := decodeJSON(body, &env); err != nil {
		return store.MaterialInput{}, err
	}
	name, err := store.ParseName(env.Name)
	if err != nil {
		return store.MaterialInput{}, err
	}
	nominalT, err := store.ParseInches(string(env.NominalThicknessIn))
	if err != nil {
		return store.MaterialInput{}, err
	}
	nominalW, err := store.ParseInches(string(env.NominalWidthIn))
	if err != nil {
		return store.MaterialInput{}, err
	}
	actualT, err := store.ParseInches(string(env.ActualThicknessIn))
	if err != nil {
		return store.MaterialInput{}, err
	}
	actualW, err := store.ParseInches(string(env.ActualWidthIn))
	if err != nil {
		return store.MaterialInput{}, err
	}
	roughT, err := store.ParseInches(string(env.RoughThicknessIn))
	if err != nil {
		return store.MaterialInput{}, err
	}
	roughW, err := store.ParseInches(string(env.RoughWidthIn))
	if err != nil {
		return store.MaterialInput{}, err
	}
	return store.MaterialInput{
		Name:               name,
		Species:            deref(env.Species),
		Grade:              deref(env.Grade),
		NominalThicknessIn: nominalT,
		NominalWidthIn:     nominalW,
		ActualThicknessIn:  actualT,
		ActualWidthIn:      actualW,
		RoughThicknessIn:   roughT,
		RoughWidthIn:       roughW,
	}, nil
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
