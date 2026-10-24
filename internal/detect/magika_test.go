package detect

import (
	"os"
	"testing"
)

func TestParseMagika(t *testing.T) {
	b, err := os.ReadFile("testdata/magika_yaml.json") // recorded from magika 1.1.0
	if err != nil {
		t.Fatal(err)
	}
	v, ok, err := parseMagika(b)
	if !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if v.Label != "yaml" || v.Score < 0.5 || magikaCategory(v) != CatCode || len(v.Extensions) == 0 {
		t.Fatalf("parsed %+v", v)
	}
	if _, ok, _ := parseMagika([]byte(`[{"path":"x","result":{"status":"file_does_not_exist"}}]`)); ok {
		t.Fatal("an error status must not count as an answer")
	}
}

func TestMergeMagika(t *testing.T) {
	yaml := &MagikaVerdict{Label: "yaml", Description: "YAML source", Group: "code", Extensions: []string{"yml", "yaml"}, Score: 0.99}

	// A generic plain-text verdict gives way to a confident answer.
	r := &Result{}
	r.set("Plain text", "txt", "text/plain", CatText, 0.85, SrcHeuristic)
	mergeMagika(r, yaml)
	if r.Ext != "yml" || r.Source != SrcMagika {
		t.Fatalf("got %s .%s via %s", r.Label, r.Ext, r.Source)
	}

	// A signature beats Magika.
	r = &Result{}
	r.set("PNG image", "png", "image/png", CatImage, 0.98, SrcSignature)
	mergeMagika(r, yaml)
	if r.Ext != "png" {
		t.Fatalf("signature overridden: %s", r.Ext)
	}

	// Magika's own "plain text" fallback doesn't replace a specific
	// text format, and "unknown" changes nothing.
	r = &Result{}
	r.set("Comma-separated values", "csv", "text/csv", CatSpreadsheet, 0.8, SrcHeuristic)
	r.Category = CatText
	mergeMagika(r, &MagikaVerdict{Label: "txt", Description: "Generic text document", Group: "text", Score: 0.99})
	mergeMagika(r, &MagikaVerdict{Label: "unknown", RawLabel: "randombytes", Score: 0.99})
	if r.Ext != "csv" {
		t.Fatalf("got .%s", r.Ext)
	}
}
