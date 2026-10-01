package store_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"lumbercalc/backend/internal/store"
)

const emptyDocument = `{"schemaVersion":1,"units":"in","pieces":[],"connections":[]}`

func TestDocumentRejects(t *testing.T) {
	pieceID := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	otherID := "11111111-2222-4333-8444-555555555555"
	tests := []struct {
		name string
		in   string
		want store.DocumentError
	}{
		{
			name: "unknown schema version",
			in:   `{"schemaVersion":2,"units":"in","pieces":[],"connections":[]}`,
			want: store.DocumentError{Pointer: "/schemaVersion", Problem: "is not a known schema version"},
		},
		{
			name: "pieces not an array",
			in:   `{"schemaVersion":1,"units":"in","pieces":{},"connections":[]}`,
			want: store.DocumentError{Pointer: "/pieces", Problem: "is not an array"},
		},
		{
			name: "connections not an array",
			in:   `{"schemaVersion":1,"units":"in","pieces":[],"connections":{}}`,
			want: store.DocumentError{Pointer: "/connections", Problem: "is not an array"},
		},
		{
			name: "duplicate piece id",
			in:   documentWithPieces(pieceID, pieceID),
			want: store.DocumentError{Pointer: "/pieces/1/id", Problem: "duplicates /pieces/0/id"},
		},
		{
			name: "duplicate piece id ignores case",
			in:   documentWithPieces(strings.ToUpper(pieceID), pieceID),
			want: store.DocumentError{Pointer: "/pieces/1/id", Problem: "duplicates /pieces/0/id"},
		},
		{
			name: "connection names no piece",
			in:   documentWithConnection(pieceID, otherID),
			want: store.DocumentError{Pointer: "/connections/0/pieceBId", Problem: "names no piece"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc store.Document
			err := json.Unmarshal([]byte(tt.in), &doc)
			var got *store.DocumentError
			if !errors.As(err, &got) || *got != tt.want {
				t.Fatalf("err = %v, want %v", err, &tt.want)
			}
		})
	}
}

func TestEmptyDocumentIsValid(t *testing.T) {
	var doc store.Document
	if err := json.Unmarshal([]byte(emptyDocument), &doc); err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != emptyDocument {
		t.Fatalf("marshal = %s, want %s", got, emptyDocument)
	}
}

func TestZeroDocumentMarshalsToTheEmptyDocument(t *testing.T) {
	got, err := json.Marshal(store.Document{})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != emptyDocument {
		t.Fatalf("marshal = %s, want %s", got, emptyDocument)
	}
}

func TestDocumentKeepsAnExactLength(t *testing.T) {
	const length = "9007199254740993"
	raw := `{
		"schemaVersion": 1,
		"units": "in",
		"pieces": [{
			"id": "AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE",
			"name": "stud",
			"materialId": "11111111-2222-4333-8444-555555555555",
			"lengthIn": ` + length + `,
			"continuity": {"x": true, "y": false, "z": false},
			"transform": {"positionIn": [0, 0, 0], "rotationDeg": [0, 90, 0]}
		}],
		"connections": []
	}`
	var doc store.Document
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), length) {
		t.Fatalf("marshal = %s, want digits %s", got, length)
	}
	const want = `{"schemaVersion":1,"units":"in","pieces":[{"id":"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee","name":"stud","materialId":"11111111-2222-4333-8444-555555555555","roughCut":false,"lengthIn":9007199254740993,"continuity":{"x":true,"y":false,"z":false},"transform":{"positionIn":[0,0,0],"rotationDeg":[0,90,0]}}],"connections":[]}`
	if string(got) != want {
		t.Fatalf("marshal = %s, want %s", got, want)
	}
}

func TestFailedUnmarshalLeavesTheDocument(t *testing.T) {
	var doc store.Document
	if err := json.Unmarshal([]byte(emptyDocument), &doc); err != nil {
		t.Fatal(err)
	}
	err := json.Unmarshal([]byte(`{"schemaVersion":2}`), &doc)
	if err == nil {
		t.Fatal("err = nil, want a document error")
	}
	got, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != emptyDocument {
		t.Fatalf("marshal = %s, want %s", got, emptyDocument)
	}
}

func TestDocumentAllowsASelfJoinAndANegativeLength(t *testing.T) {
	raw := documentWithConnection(
		"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
	)
	raw = strings.Replace(raw, `"lengthIn": 1`, `"lengthIn": -2`, 1)
	var doc store.Document
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
}

func documentWithPieces(first, second string) string {
	return `{
		"schemaVersion": 1,
		"units": "in",
		"pieces": [` + onePiece(first) + `,` + onePiece(second) + `],
		"connections": []
	}`
}

func documentWithConnection(pieceID, otherID string) string {
	return `{
		"schemaVersion": 1,
		"units": "in",
		"pieces": [` + onePiece(pieceID) + `],
		"connections": [{
			"id": "joint-1",
			"pieceAId": "` + pieceID + `",
			"pieceBId": "` + otherID + `",
			"joinType": "butt",
			"faceA": "end",
			"faceB": "end",
			"wasteIn": 0
		}]
	}`
}

func onePiece(id string) string {
	return `{
		"id": "` + id + `",
		"name": "stud",
		"materialId": "11111111-2222-4333-8444-555555555555",
		"lengthIn": 1,
		"continuity": {"x": true, "y": false, "z": false},
		"transform": {"positionIn": [0, 0, 0], "rotationDeg": [0, 0, 0]}
	}`
}
