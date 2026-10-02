package store

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Document is a design document at schema version 1.
// The zero value is the empty document.
// Every other value came from UnmarshalJSON, so it obeys every rule that parse checks.
type Document struct {
	pieces      []piece
	connections []connection
}

// DocumentError is the first rule a document breaks.
// Pointer is an RFC 6901 JSON pointer. An empty pointer is the root.
type DocumentError struct {
	Pointer string
	Problem string
}

func (e *DocumentError) Error() string {
	if e.Pointer == "" {
		return e.Problem
	}
	return e.Pointer + " " + e.Problem
}

// jsonNumber is the JSON number token. float64 cannot hold every inch length.
type jsonNumber string

type piece struct {
	id         string
	name       string
	materialID string
	roughCut   bool
	lengthIn   jsonNumber
	continuity continuity
	transform  transform
}

type continuity struct {
	x, y, z bool
}

type transform struct {
	position [3]float64
	rotation [3]float64
}

type connection struct {
	id     string
	pieceA string
	pieceB string
	join   string
	faceA  string
	faceB  string
	waste  jsonNumber
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (d *Document) UnmarshalJSON(raw []byte) error {
	next, err := parseDocument(raw)
	if err != nil {
		return err
	}
	*d = next
	return nil
}

func (d Document) schemaVersion() int32 { return 1 }

// materialIDs returns each piece material id once, in first-seen order.
func (d Document) materialIDs() []materialID {
	ids := make([]materialID, 0, len(d.pieces))
	seen := make(map[string]struct{}, len(d.pieces))
	for _, p := range d.pieces {
		if _, ok := seen[p.materialID]; ok {
			continue
		}
		seen[p.materialID] = struct{}{}
		var id materialID
		// parsePiece already accepted this uuid. A failure here is a broken document.
		if err := decodeUUID(p.materialID, &id.v); err != nil {
			panic(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func (d Document) fork() (Document, error) {
	pieces := make([]piece, len(d.pieces))
	copy(pieces, d.pieces)
	taken := make(map[string]struct{}, len(pieces))
	for i := range pieces {
		taken[pieces[i].id] = struct{}{}
	}
	remap := make(map[string]string, len(pieces))
	for i := range pieces {
		next, err := mintID(taken)
		if err != nil {
			return Document{}, err
		}
		remap[pieces[i].id] = next
		pieces[i].id = next
	}
	connections := make([]connection, len(d.connections))
	copy(connections, d.connections)
	for i := range connections {
		next, err := newV4()
		if err != nil {
			return Document{}, err
		}
		connections[i].id = next
		connections[i].pieceA = remap[connections[i].pieceA]
		connections[i].pieceB = remap[connections[i].pieceB]
	}
	return Document{pieces: pieces, connections: connections}, nil
}

func mintID(taken map[string]struct{}) (string, error) {
	for {
		id, err := newV4()
		if err != nil {
			return "", err
		}
		if _, ok := taken[id]; ok {
			continue
		}
		taken[id] = struct{}{}
		return id, nil
	}
}

func newV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// Version 4 and the RFC 4122 variant.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b), nil
}

type materialID struct{ v [16]byte }

func (d Document) MarshalJSON() ([]byte, error) {
	body := wireDocument{
		SchemaVersion: int(d.schemaVersion()),
		Units:         "in",
		Pieces:        make([]wirePiece, len(d.pieces)),
		Connections:   make([]wireConnection, len(d.connections)),
	}
	for i, p := range d.pieces {
		body.Pieces[i] = wirePiece{
			ID:         p.id,
			Name:       p.name,
			MaterialID: p.materialID,
			RoughCut:   p.roughCut,
			LengthIn:   json.Number(p.lengthIn),
			Continuity: wireContinuity{X: p.continuity.x, Y: p.continuity.y, Z: p.continuity.z},
			Transform: wireTransform{
				PositionIn:  p.transform.position,
				RotationDeg: p.transform.rotation,
			},
		}
	}
	for i, c := range d.connections {
		body.Connections[i] = wireConnection{
			ID:       c.id,
			PieceAID: c.pieceA,
			PieceBID: c.pieceB,
			JoinType: c.join,
			FaceA:    c.faceA,
			FaceB:    c.faceB,
			WasteIn:  json.Number(c.waste),
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

type wireDocument struct {
	SchemaVersion int              `json:"schemaVersion"`
	Units         string           `json:"units"`
	Pieces        []wirePiece      `json:"pieces"`
	Connections   []wireConnection `json:"connections"`
}

type wirePiece struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	MaterialID string         `json:"materialId"`
	RoughCut   bool           `json:"roughCut"`
	LengthIn   json.Number    `json:"lengthIn"`
	Continuity wireContinuity `json:"continuity"`
	Transform  wireTransform  `json:"transform"`
}

type wireContinuity struct {
	X bool `json:"x"`
	Y bool `json:"y"`
	Z bool `json:"z"`
}

type wireTransform struct {
	PositionIn  [3]float64 `json:"positionIn"`
	RotationDeg [3]float64 `json:"rotationDeg"`
}

type wireConnection struct {
	ID       string      `json:"id"`
	PieceAID string      `json:"pieceAId"`
	PieceBID string      `json:"pieceBId"`
	JoinType string      `json:"joinType"`
	FaceA    string      `json:"faceA"`
	FaceB    string      `json:"faceB"`
	WasteIn  json.Number `json:"wasteIn"`
}

func parseDocument(raw []byte) (Document, error) {
	obj, err := decodeObject(raw)
	if err != nil {
		return Document{}, &DocumentError{Problem: "is not an object"}
	}
	if key, ok := unknownKey(obj, "schemaVersion", "units", "pieces", "connections"); ok {
		return Document{}, &DocumentError{Pointer: "/" + key, Problem: "is not a known field"}
	}
	version, ok := obj["schemaVersion"]
	if !ok {
		return Document{}, &DocumentError{Pointer: "/schemaVersion", Problem: "is required"}
	}
	if bytes.TrimSpace(version) == nil || string(bytes.TrimSpace(version)) != "1" {
		return Document{}, &DocumentError{Pointer: "/schemaVersion", Problem: "is not a known schema version"}
	}
	units, ok := obj["units"]
	if !ok {
		return Document{}, &DocumentError{Pointer: "/units", Problem: "is required"}
	}
	if string(bytes.TrimSpace(units)) != `"in"` {
		return Document{}, &DocumentError{Pointer: "/units", Problem: "is not a known unit"}
	}
	piecesRaw, ok := obj["pieces"]
	if !ok {
		return Document{}, &DocumentError{Pointer: "/pieces", Problem: "is required"}
	}
	pieceVals, err := decodeArray(piecesRaw)
	if err != nil {
		return Document{}, &DocumentError{Pointer: "/pieces", Problem: "is not an array"}
	}
	connectionsRaw, ok := obj["connections"]
	if !ok {
		return Document{}, &DocumentError{Pointer: "/connections", Problem: "is required"}
	}
	connectionVals, err := decodeArray(connectionsRaw)
	if err != nil {
		return Document{}, &DocumentError{Pointer: "/connections", Problem: "is not an array"}
	}

	doc := Document{
		pieces:      make([]piece, len(pieceVals)),
		connections: make([]connection, len(connectionVals)),
	}
	seen := map[string]int{}
	for i, rawPiece := range pieceVals {
		p, err := parsePiece(rawPiece, i, seen)
		if err != nil {
			return Document{}, err
		}
		doc.pieces[i] = p
	}
	for i, rawConnection := range connectionVals {
		c, err := parseConnection(rawConnection, i, seen)
		if err != nil {
			return Document{}, err
		}
		doc.connections[i] = c
	}
	return doc, nil
}

func parsePiece(raw []byte, index int, seen map[string]int) (piece, error) {
	at := "/pieces/" + strconv.Itoa(index)
	obj, err := decodeObject(raw)
	if err != nil {
		return piece{}, &DocumentError{Pointer: at, Problem: "is not an object"}
	}
	if key, ok := unknownKey(obj, "id", "name", "materialId", "roughCut", "lengthIn", "continuity", "transform"); ok {
		return piece{}, &DocumentError{Pointer: at + "/" + key, Problem: "is not a known field"}
	}
	id, err := requiredUUID(obj, at, "id")
	if err != nil {
		return piece{}, err
	}
	if first, ok := seen[id]; ok {
		return piece{}, &DocumentError{
			Pointer: at + "/id",
			Problem: "duplicates /pieces/" + strconv.Itoa(first) + "/id",
		}
	}
	seen[id] = index
	name, err := requiredString(obj, at, "name")
	if err != nil {
		return piece{}, err
	}
	materialID, err := requiredUUID(obj, at, "materialId")
	if err != nil {
		return piece{}, err
	}
	rough, err := optionalBool(obj, at, "roughCut")
	if err != nil {
		return piece{}, err
	}
	length, err := requiredNumber(obj, at, "lengthIn")
	if err != nil {
		return piece{}, err
	}
	flags, err := parseContinuity(obj, at)
	if err != nil {
		return piece{}, err
	}
	place, err := parseTransform(obj, at)
	if err != nil {
		return piece{}, err
	}
	return piece{
		id:         id,
		name:       name,
		materialID: materialID,
		roughCut:   rough,
		lengthIn:   length,
		continuity: flags,
		transform:  place,
	}, nil
}

func parseContinuity(obj map[string]json.RawMessage, at string) (continuity, error) {
	raw, ok := obj["continuity"]
	if !ok {
		return continuity{}, &DocumentError{Pointer: at + "/continuity", Problem: "is required"}
	}
	body, err := decodeObject(raw)
	if err != nil {
		return continuity{}, &DocumentError{Pointer: at + "/continuity", Problem: "is not an object"}
	}
	if key, ok := unknownKey(body, "x", "y", "z"); ok {
		return continuity{}, &DocumentError{Pointer: at + "/continuity/" + key, Problem: "is not a known field"}
	}
	x, err := requiredBool(body, at+"/continuity", "x")
	if err != nil {
		return continuity{}, err
	}
	y, err := requiredBool(body, at+"/continuity", "y")
	if err != nil {
		return continuity{}, err
	}
	z, err := requiredBool(body, at+"/continuity", "z")
	if err != nil {
		return continuity{}, err
	}
	return continuity{x: x, y: y, z: z}, nil
}

func parseTransform(obj map[string]json.RawMessage, at string) (transform, error) {
	raw, ok := obj["transform"]
	if !ok {
		return transform{}, &DocumentError{Pointer: at + "/transform", Problem: "is required"}
	}
	body, err := decodeObject(raw)
	if err != nil {
		return transform{}, &DocumentError{Pointer: at + "/transform", Problem: "is not an object"}
	}
	if key, ok := unknownKey(body, "positionIn", "rotationDeg"); ok {
		return transform{}, &DocumentError{Pointer: at + "/transform/" + key, Problem: "is not a known field"}
	}
	position, err := requiredVec3(body, at+"/transform", "positionIn")
	if err != nil {
		return transform{}, err
	}
	rotation, err := requiredVec3(body, at+"/transform", "rotationDeg")
	if err != nil {
		return transform{}, err
	}
	return transform{position: position, rotation: rotation}, nil
}

func parseConnection(raw []byte, index int, seen map[string]int) (connection, error) {
	at := "/connections/" + strconv.Itoa(index)
	obj, err := decodeObject(raw)
	if err != nil {
		return connection{}, &DocumentError{Pointer: at, Problem: "is not an object"}
	}
	if key, ok := unknownKey(obj, "id", "pieceAId", "pieceBId", "joinType", "faceA", "faceB", "wasteIn"); ok {
		return connection{}, &DocumentError{Pointer: at + "/" + key, Problem: "is not a known field"}
	}
	id, err := requiredString(obj, at, "id")
	if err != nil {
		return connection{}, err
	}
	if id == "" {
		return connection{}, &DocumentError{Pointer: at + "/id", Problem: "is empty"}
	}
	pieceA, err := requiredRef(obj, at, "pieceAId", seen)
	if err != nil {
		return connection{}, err
	}
	pieceB, err := requiredRef(obj, at, "pieceBId", seen)
	if err != nil {
		return connection{}, err
	}
	join, err := requiredString(obj, at, "joinType")
	if err != nil {
		return connection{}, err
	}
	switch join {
	case "butt", "lap", "fastener":
	default:
		return connection{}, &DocumentError{Pointer: at + "/joinType", Problem: "is not a known join type"}
	}
	faceA, err := requiredText(obj, at, "faceA")
	if err != nil {
		return connection{}, err
	}
	faceB, err := requiredText(obj, at, "faceB")
	if err != nil {
		return connection{}, err
	}
	waste, err := requiredNumber(obj, at, "wasteIn")
	if err != nil {
		return connection{}, err
	}
	return connection{
		id:     id,
		pieceA: pieceA,
		pieceB: pieceB,
		join:   join,
		faceA:  faceA,
		faceB:  faceB,
		waste:  waste,
	}, nil
}

func requiredRef(obj map[string]json.RawMessage, at, key string, seen map[string]int) (string, error) {
	id, err := requiredUUID(obj, at, key)
	if err != nil {
		return "", err
	}
	if _, ok := seen[id]; !ok {
		return "", &DocumentError{Pointer: at + "/" + key, Problem: "names no piece"}
	}
	return id, nil
}

func requiredUUID(obj map[string]json.RawMessage, at, key string) (string, error) {
	text, err := requiredString(obj, at, key)
	if err != nil {
		return "", err
	}
	if !uuidPattern.MatchString(text) {
		return "", &DocumentError{Pointer: at + "/" + key, Problem: "is not a uuid"}
	}
	return strings.ToLower(text), nil
}

func requiredString(obj map[string]json.RawMessage, at, key string) (string, error) {
	raw, ok := obj[key]
	if !ok {
		return "", &DocumentError{Pointer: at + "/" + key, Problem: "is required"}
	}
	text, err := decodeString(raw)
	if err != nil {
		return "", &DocumentError{Pointer: at + "/" + key, Problem: "is not a string"}
	}
	return text, nil
}

func requiredText(obj map[string]json.RawMessage, at, key string) (string, error) {
	text, err := requiredString(obj, at, key)
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", &DocumentError{Pointer: at + "/" + key, Problem: "is empty"}
	}
	return text, nil
}

func requiredBool(obj map[string]json.RawMessage, at, key string) (bool, error) {
	raw, ok := obj[key]
	if !ok {
		return false, &DocumentError{Pointer: at + "/" + key, Problem: "is required"}
	}
	value, err := decodeBool(raw)
	if err != nil {
		return false, &DocumentError{Pointer: at + "/" + key, Problem: "is not a boolean"}
	}
	return value, nil
}

func optionalBool(obj map[string]json.RawMessage, at, key string) (bool, error) {
	raw, ok := obj[key]
	if !ok {
		return false, nil
	}
	value, err := decodeBool(raw)
	if err != nil {
		return false, &DocumentError{Pointer: at + "/" + key, Problem: "is not a boolean"}
	}
	return value, nil
}

func requiredNumber(obj map[string]json.RawMessage, at, key string) (jsonNumber, error) {
	raw, ok := obj[key]
	if !ok {
		return "", &DocumentError{Pointer: at + "/" + key, Problem: "is required"}
	}
	token, err := decodeNumber(raw)
	if err != nil {
		return "", &DocumentError{Pointer: at + "/" + key, Problem: "is not a number"}
	}
	return token, nil
}

func requiredVec3(obj map[string]json.RawMessage, at, key string) ([3]float64, error) {
	raw, ok := obj[key]
	if !ok {
		return [3]float64{}, &DocumentError{Pointer: at + "/" + key, Problem: "is required"}
	}
	items, err := decodeArray(raw)
	if err != nil || len(items) != 3 {
		return [3]float64{}, &DocumentError{Pointer: at + "/" + key, Problem: "is not a 3-number array"}
	}
	var out [3]float64
	for i, item := range items {
		token, err := decodeNumber(item)
		if err != nil {
			return [3]float64{}, &DocumentError{Pointer: at + "/" + key + "/" + strconv.Itoa(i), Problem: "is not a number"}
		}
		out[i], err = json.Number(token).Float64()
		if err != nil {
			return [3]float64{}, &DocumentError{Pointer: at + "/" + key + "/" + strconv.Itoa(i), Problem: "is not a number"}
		}
	}
	return out, nil
}

func unknownKey(obj map[string]json.RawMessage, allowed ...string) (string, bool) {
	ok := map[string]struct{}{}
	for _, key := range allowed {
		ok[key] = struct{}{}
	}
	found := ""
	for key := range obj {
		if _, allowed := ok[key]; allowed {
			continue
		}
		if found == "" || key < found {
			found = key
		}
	}
	return found, found != ""
}

func decodeObject(raw []byte) (map[string]json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, io.ErrUnexpectedEOF
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	if err := rejectTail(dec); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, io.ErrUnexpectedEOF
	}
	return obj, nil
}

func decodeArray(raw []byte) ([]json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return nil, io.ErrUnexpectedEOF
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var items []json.RawMessage
	if err := dec.Decode(&items); err != nil {
		return nil, err
	}
	if err := rejectTail(dec); err != nil {
		return nil, err
	}
	if items == nil {
		return []json.RawMessage{}, nil
	}
	return items, nil
}

func decodeString(raw []byte) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '"' {
		return "", io.ErrUnexpectedEOF
	}
	var text string
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&text); err != nil {
		return "", err
	}
	if err := rejectTail(dec); err != nil {
		return "", err
	}
	return text, nil
}

func decodeBool(raw []byte) (bool, error) {
	switch string(bytes.TrimSpace(raw)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, io.ErrUnexpectedEOF
	}
}

func decodeNumber(raw []byte) (jsonNumber, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] == '"' {
		return "", io.ErrUnexpectedEOF
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var token json.Number
	if err := dec.Decode(&token); err != nil {
		return "", err
	}
	if err := rejectTail(dec); err != nil {
		return "", err
	}
	return jsonNumber(token), nil
}

func rejectTail(dec *json.Decoder) error {
	var extra struct{}
	err := dec.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return io.ErrUnexpectedEOF
	}
	return err
}
