package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"lumbercalc/backend/internal/server"
	"lumbercalc/backend/internal/store"
)

const (
	priceTakenMessage          = "price already exists"
	materialUnavailableMessage = "material not available"
)

func (a *API) createPrice(w http.ResponseWriter, r *http.Request, c Caller) {
	key, cents, err := decodePriceCreate(r.Body)
	if err != nil {
		a.writePriceErr(w, r, err)
		return
	}
	row, err := a.d.CreatePrice(r.Context(), c.UserID(), key, cents)
	if err != nil {
		a.writePriceErr(w, r, err)
		return
	}
	server.WriteJSON(w, http.StatusCreated, toPrice(row))
}

func (a *API) listPrices(w http.ResponseWriter, r *http.Request, c Caller) {
	rows, err := a.d.ListPrices(r.Context(), c.UserID())
	if err != nil {
		a.writePriceErr(w, r, err)
		return
	}
	out := make([]Price, len(rows))
	for i, row := range rows {
		out[i] = toPrice(row)
	}
	server.WriteJSON(w, http.StatusOK, out)
}

func (a *API) updatePrice(w http.ResponseWriter, r *http.Request, c Caller) {
	id, err := store.ParsePriceID(r.PathValue("id"))
	if err != nil {
		a.writePriceErr(w, r, err)
		return
	}
	cents, err := decodePriceUpdate(r.Body)
	if err != nil {
		a.writePriceErr(w, r, err)
		return
	}
	row, err := a.d.UpdatePrice(r.Context(), c.UserID(), id, cents)
	if err != nil {
		a.writePriceErr(w, r, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, toPrice(row))
}

func (a *API) deletePrice(w http.ResponseWriter, r *http.Request, c Caller) {
	id, err := store.ParsePriceID(r.PathValue("id"))
	if err != nil {
		a.writePriceErr(w, r, err)
		return
	}
	if err := a.d.DeletePrice(r.Context(), c.UserID(), id); err != nil {
		a.writePriceErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodePriceCreate(body io.Reader) (store.PriceKey, store.Cents, error) {
	var env PriceCreate
	if err := decodeJSON(body, &env); err != nil {
		return store.PriceKey{}, store.Cents{}, err
	}
	material, err := store.ParseMaterialID(env.MaterialId)
	if err != nil {
		return store.PriceKey{}, store.Cents{}, errBadBody
	}
	length, err := store.ParseInches(string(env.LengthIn))
	if err != nil {
		return store.PriceKey{}, store.Cents{}, err
	}
	cents, err := store.ParseCents(string(env.PriceCents))
	if err != nil {
		return store.PriceKey{}, store.Cents{}, err
	}
	return store.PriceKey{Material: material, Length: length}, cents, nil
}

func decodePriceUpdate(body io.Reader) (store.Cents, error) {
	var env PriceUpdate
	if err := decodeJSON(body, &env); err != nil {
		return store.Cents{}, err
	}
	return store.ParseCents(string(env.PriceCents))
}

func (a *API) writePriceErr(w http.ResponseWriter, r *http.Request, err error) {
	a.d.Log.ErrorContext(r.Context(), "price", "err", err)
	switch {
	case errors.Is(err, store.ErrPriceTaken):
		server.WriteJSON(w, http.StatusConflict, server.Error{Error: priceTakenMessage})
	case errors.Is(err, store.ErrMaterialUnusable):
		server.WriteJSON(w, http.StatusConflict, server.Error{Error: materialUnavailableMessage})
	default:
		server.WriteError(w, priceStatus(err))
	}
}

func priceStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrBadPriceID), errors.Is(err, store.ErrPriceNotFound):
		return http.StatusNotFound
	case errors.Is(err, errBadBody), errors.Is(err, store.ErrBadInches), errors.Is(err, store.ErrBadCents):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func toPrice(p store.Price) Price {
	return Price{
		Id:         p.ID.String(),
		MaterialId: p.Key.Material.String(),
		LengthIn:   json.Number(p.Key.Length.String()),
		PriceCents: json.Number(strconv.Itoa(int(p.Cents.Int32()))),
		UpdatedAt:  p.UpdatedAt,
	}
}
