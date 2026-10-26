package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const maxBody = 1 << 20

var errBadBody = errors.New("bad body")

func readBody(body io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxBody {
		return nil, errBadBody
	}
	return raw, nil
}

func decodeJSON(body io.Reader, dest any) error {
	raw, err := readBody(body)
	if err != nil {
		return err
	}
	return decodeObject(raw, dest)
}

func decodeOptionalJSON(body io.Reader, dest any) error {
	raw, err := readBody(body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if jsonNull(raw) {
		return errBadBody
	}
	return decodeObject(raw, dest)
}

func decodeObject(raw []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil || dec.More() {
		return errBadBody
	}
	return nil
}

func rejectRequestBody(body io.Reader) error {
	raw, err := readBody(body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) != 0 {
		return errBadBody
	}
	return nil
}
