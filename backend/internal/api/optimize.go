package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"lumbercalc/backend/internal/server"
	"lumbercalc/backend/internal/solver"
	"lumbercalc/backend/internal/store"
)

type optimizeEnvelope struct {
	UseKerf        json.RawMessage `json:"useKerf"`
	KerfInches     json.RawMessage `json:"kerfInches"`
	DimensionBasis json.RawMessage `json:"dimensionBasis"`
}

type optimizeFlags struct {
	UseKerf        bool         `json:"useKerf"`
	KerfInches     *json.Number `json:"kerfInches"`
	DimensionBasis string       `json:"dimensionBasis"`
}

func (a *API) optimizeDesign(w http.ResponseWriter, r *http.Request, c Caller) {
	id, err := store.ParseDesignID(r.PathValue("id"))
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	choice, err := decodeFlagChoice(r.Body)
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	in, err := a.d.GetOptimizeInput(r.Context(), c.UserID(), id, choice)
	if err != nil {
		a.writeDesignErr(w, r, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, flagsJSON(in.Flags))
}

func decodeFlagChoice(body io.Reader) (store.FlagChoice, error) {
	var env optimizeEnvelope
	if err := decodeOptionalJSON(body, &env); err != nil {
		return store.FlagChoice{}, err
	}
	kerf, err := kerfChoice(env.UseKerf, env.KerfInches)
	if err != nil {
		return store.FlagChoice{}, choiceErr(err)
	}
	basis, err := basisChoice(env.DimensionBasis)
	if err != nil {
		return store.FlagChoice{}, choiceErr(err)
	}
	return store.FlagChoice{Kerf: kerf, Basis: basis}, nil
}

func choiceErr(err error) error {
	if errors.Is(err, store.ErrBadKerf) || errors.Is(err, store.ErrBadDimensionBasis) {
		return errBadBody
	}
	return err
}

func kerfChoice(useKerf, kerfInches json.RawMessage) (*solver.Length, error) {
	if omitted(useKerf) {
		if !omitted(kerfInches) {
			return nil, errBadBody
		}
		return nil, nil
	}
	if jsonNull(useKerf) {
		return nil, errBadBody
	}
	var on bool
	if err := json.Unmarshal(useKerf, &on); err != nil {
		return nil, errBadBody
	}
	if !on {
		if omitted(kerfInches) || jsonNull(kerfInches) {
			return new(solver.Length), nil
		}
		return nil, errBadBody
	}
	length, err := kerfInchesChoice(kerfInches)
	if err != nil {
		return nil, err
	}
	return &length, nil
}

func kerfInchesChoice(raw json.RawMessage) (solver.Length, error) {
	if omitted(raw) || jsonNull(raw) {
		return solver.Length{}, errBadBody
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil || dec.More() {
		return solver.Length{}, errBadBody
	}
	num, ok := value.(json.Number)
	if !ok {
		return solver.Length{}, errBadBody
	}
	return store.ParseKerf(num.String())
}

func basisChoice(raw json.RawMessage) (*store.DimensionBasis, error) {
	if omitted(raw) {
		return nil, nil
	}
	if jsonNull(raw) {
		return nil, errBadBody
	}
	var token string
	if err := json.Unmarshal(raw, &token); err != nil {
		return nil, errBadBody
	}
	basis, err := store.ParseDimensionBasis(token)
	if err != nil {
		return nil, err
	}
	return &basis, nil
}

func omitted(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) == 0
}

func flagsJSON(flags store.OptimizeFlags) optimizeFlags {
	out := optimizeFlags{
		UseKerf:        !flags.Kerf().IsZero(),
		DimensionBasis: flags.Basis().String(),
	}
	if out.UseKerf {
		n := json.Number(flags.Kerf().String())
		out.KerfInches = &n
	}
	return out
}
