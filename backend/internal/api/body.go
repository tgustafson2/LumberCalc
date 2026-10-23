package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const maxBody = 1 << 20

var errBadBody = errors.New("bad body")

func decodeJSON(body io.Reader, dest any) error {
	raw, err := io.ReadAll(io.LimitReader(body, maxBody+1))
	if err != nil {
		return err
	}
	if len(raw) > maxBody {
		return errBadBody
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil || dec.More() {
		return errBadBody
	}
	return nil
}
