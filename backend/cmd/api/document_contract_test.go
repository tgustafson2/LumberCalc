package main

import (
	"encoding/json"
	"testing"

	"lumbercalc/backend/internal/store"
)

func TestDesignDocumentAgreesWithTheSchema(t *testing.T) {
	doc := loadSpec(t)
	ref := doc.Components.Schemas["DesignDocument"]
	if ref == nil || ref.Value == nil {
		t.Fatal("DesignDocument schema is missing")
	}
	schema := ref.Value
	tests := []struct {
		name    string
		raw     string
		schema  bool
		goParse bool
	}{
		{
			name:    "empty document",
			raw:     `{"schemaVersion":1,"units":"in","pieces":[],"connections":[]}`,
			schema:  true,
			goParse: true,
		},
		{
			name: "duplicate piece id",
			raw: `{
				"schemaVersion": 1,
				"units": "in",
				"pieces": [` + contractPiece("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee") + `,` + contractPiece("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee") + `],
				"connections": []
			}`,
			schema:  true,
			goParse: false,
		},
		{
			name:    "unknown field",
			raw:     `{"schemaVersion":1,"units":"in","pieces":[],"connections":[],"quaternion":[0,0,0,1]}`,
			schema:  false,
			goParse: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var value any
			if err := json.Unmarshal([]byte(tt.raw), &value); err != nil {
				t.Fatal(err)
			}
			err := schema.VisitJSON(value)
			if tt.schema && err != nil {
				t.Fatalf("schema: %v", err)
			}
			if !tt.schema && err == nil {
				t.Fatal("schema accepted the document")
			}
			err = json.Unmarshal([]byte(tt.raw), &store.Document{})
			if tt.goParse && err != nil {
				t.Fatalf("document: %v", err)
			}
			if !tt.goParse && err == nil {
				t.Fatal("document accepted the value")
			}
		})
	}
}

func contractPiece(id string) string {
	return `{
		"id": "` + id + `",
		"name": "stud",
		"materialId": "11111111-2222-4333-8444-555555555555",
		"roughCut": false,
		"lengthIn": 8,
		"continuity": {"x": true, "y": false, "z": false},
		"transform": {"positionIn": [0, 0, 0], "rotationDeg": [0, 0, 0]}
	}`
}
