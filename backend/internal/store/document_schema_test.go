package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	schemaSystemMaterial  = "11111111-2222-4333-8444-555555555555"
	schemaPrivateMaterial = "66666666-7777-4888-8999-aaaaaaaaaaaa"
)

func TestDocumentJSONMatchesTheOpenAPISchemas(t *testing.T) {
	spec := loadOpenAPI(t)
	var doc Document
	raw := `{"schemaVersion":1,"units":"in","pieces":[` +
		schemaPiece("00000000-0000-4000-8000-000000000001", schemaSystemMaterial) + `,` +
		schemaPiece("00000000-0000-4000-8000-000000000002", schemaPrivateMaterial) +
		`],"connections":[{"id":"joint-1","pieceAId":"00000000-0000-4000-8000-000000000001","pieceBId":"00000000-0000-4000-8000-000000000002","joinType":"butt","faceA":"end","faceB":"face","wasteIn":0}]}`
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	public := publicDocument(doc, map[string]struct{}{schemaSystemMaterial: {}})

	design := visitSchema(t, spec, "DesignDocument", doc)
	pattern := visitSchema(t, spec, "PatternDocument", public)

	if got := pieceMaterial(t, design, 1); got != schemaPrivateMaterial {
		t.Fatalf("design materialId = %v, want %s", got, schemaPrivateMaterial)
	}
	if got := pieceMaterial(t, pattern, 0); got != schemaSystemMaterial {
		t.Fatalf("pattern system materialId = %v, want %s", got, schemaSystemMaterial)
	}
	if got := pieceMaterial(t, pattern, 1); got != nil {
		t.Fatalf("pattern private materialId = %v, want null", got)
	}
}

func visitSchema(t *testing.T, spec *openapi3.T, name string, value any) map[string]any {
	t.Helper()
	ref := spec.Components.Schemas[name]
	if ref == nil || ref.Value == nil {
		t.Fatalf("%s schema is missing", name)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := ref.Value.VisitJSON(decoded); err != nil {
		t.Fatalf("%s rejects %s: %v", name, raw, err)
	}
	return decoded
}

func pieceMaterial(t *testing.T, doc map[string]any, index int) any {
	t.Helper()
	pieces, ok := doc["pieces"].([]any)
	if !ok || len(pieces) <= index {
		t.Fatalf("pieces = %v", doc["pieces"])
	}
	piece := pieces[index].(map[string]any)
	id, ok := piece["materialId"]
	if !ok {
		t.Fatalf("piece %d has no materialId key", index)
	}
	return id
}

func schemaPiece(id, materialID string) string {
	return `{"id":"` + id + `","name":"stud","materialId":"` + materialID + `","lengthIn":8,` +
		`"continuity":{"x":true,"y":false,"z":false},"transform":{"positionIn":[0,0,0],"rotationDeg":[0,0,0]}}`
}

func loadOpenAPI(t *testing.T) *openapi3.T {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("spec path")
	}
	spec, err := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(file), "..", "..", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return spec
}
