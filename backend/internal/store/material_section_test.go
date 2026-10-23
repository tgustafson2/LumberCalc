package store_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"lumbercalc/backend/internal/store"
)

func TestMaterialCrossSectionMatchesFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/cross_sections.json")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var cases []struct {
		Name     string `json:"name"`
		Material struct {
			ActualThicknessIn json.Number `json:"actualThicknessIn"`
			ActualWidthIn     json.Number `json:"actualWidthIn"`
			RoughThicknessIn  json.Number `json:"roughThicknessIn"`
			RoughWidthIn      json.Number `json:"roughWidthIn"`
		} `json:"material"`
		RoughCut *bool `json:"roughCut"`
		Want     struct {
			ThicknessIn json.Number `json:"thicknessIn"`
			WidthIn     json.Number `json:"widthIn"`
		} `json:"want"`
	}
	if err := dec.Decode(&cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("fixture is empty")
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			actualT := mustInches(t, c.Material.ActualThicknessIn.String())
			actualW := mustInches(t, c.Material.ActualWidthIn.String())
			roughT := mustInches(t, c.Material.RoughThicknessIn.String())
			roughW := mustInches(t, c.Material.RoughWidthIn.String())
			material := store.Material{
				ActualThicknessIn: actualT,
				ActualWidthIn:     actualW,
				RoughThicknessIn:  roughT,
				RoughWidthIn:      roughW,
			}
			roughCut := false
			if c.RoughCut != nil {
				roughCut = *c.RoughCut
			}
			got := material.CrossSection(roughCut)
			wantT := mustInches(t, c.Want.ThicknessIn.String())
			wantW := mustInches(t, c.Want.WidthIn.String())
			if got.ThicknessIn.String() != wantT.String() || got.WidthIn.String() != wantW.String() {
				t.Fatalf("section = %s x %s, want %s x %s", got.ThicknessIn, got.WidthIn, wantT, wantW)
			}
		})
	}
}
