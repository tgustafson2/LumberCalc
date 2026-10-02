package store

import (
	"encoding/json"
	"testing"
)

func TestForkRewritesPieceAndConnectionIDs(t *testing.T) {
	const raw = `{
		"schemaVersion": 1,
		"units": "in",
		"pieces": [
			{
				"id": "AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE",
				"name": "rail",
				"materialId": "11111111-2222-4333-8444-555555555555",
				"lengthIn": 12,
				"continuity": {"x": true, "y": false, "z": false},
				"transform": {"positionIn": [1, 2, 3], "rotationDeg": [0, 0, 0]}
			},
			{
				"id": "00000000-0000-4000-8000-000000000002",
				"name": "stile",
				"materialId": "22222222-3333-4444-8555-666666666666",
				"roughCut": true,
				"lengthIn": 36,
				"continuity": {"x": false, "y": true, "z": false},
				"transform": {"positionIn": [4, 5, 6], "rotationDeg": [0, 90, 0]}
			}
		],
		"connections": [{
			"id": "joint-1",
			"pieceAId": "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
			"pieceBId": "00000000-0000-4000-8000-000000000002",
			"joinType": "lap",
			"faceA": "end",
			"faceB": "face",
			"wasteIn": 0.25
		}]
	}`
	var doc Document
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	forked, err := doc.fork()
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("source = %s, want %s", after, before)
	}
	if len(forked.pieces) != 2 || len(forked.connections) != 1 {
		t.Fatalf("pieces = %d, connections = %d", len(forked.pieces), len(forked.connections))
	}
	if forked.pieces[0].id == doc.pieces[0].id || forked.pieces[1].id == doc.pieces[1].id || forked.pieces[0].id == forked.pieces[1].id {
		t.Fatalf("piece ids = %s %s, source = %s %s", forked.pieces[0].id, forked.pieces[1].id, doc.pieces[0].id, doc.pieces[1].id)
	}
	if !uuidPattern.MatchString(forked.pieces[0].id) || !uuidPattern.MatchString(forked.pieces[1].id) {
		t.Fatalf("piece ids = %s %s", forked.pieces[0].id, forked.pieces[1].id)
	}
	if forked.connections[0].id == doc.connections[0].id || !uuidPattern.MatchString(forked.connections[0].id) {
		t.Fatalf("connection id = %s, source = %s", forked.connections[0].id, doc.connections[0].id)
	}
	if forked.connections[0].pieceA != forked.pieces[0].id || forked.connections[0].pieceB != forked.pieces[1].id {
		t.Fatalf("pieceA = %s, pieceB = %s, pieces = %s %s", forked.connections[0].pieceA, forked.connections[0].pieceB, forked.pieces[0].id, forked.pieces[1].id)
	}
	if forked.pieces[0].materialID != doc.pieces[0].materialID || forked.pieces[1].materialID != doc.pieces[1].materialID {
		t.Fatalf("materials = %s %s", forked.pieces[0].materialID, forked.pieces[1].materialID)
	}
	if forked.pieces[0].name != "rail" || forked.pieces[0].transform.position != [3]float64{1, 2, 3} {
		t.Fatalf("name = %s, position = %v", forked.pieces[0].name, forked.pieces[0].transform.position)
	}
	if forked.pieces[1].name != "stile" || forked.pieces[1].lengthIn != "36" || !forked.pieces[1].roughCut {
		t.Fatalf("name = %s, length = %s, roughCut = %v", forked.pieces[1].name, forked.pieces[1].lengthIn, forked.pieces[1].roughCut)
	}
	doc.pieces[0].name = "changed"
	doc.connections[0].join = "butt"
	if forked.pieces[0].name != "rail" || forked.connections[0].join != "lap" {
		t.Fatal("fork shares slice storage with the source")
	}
}
