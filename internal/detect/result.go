// Package detect identifies what a file actually contains, regardless
// of its name. It runs a layered pipeline:
//
//  1. built-in signatures and structural probes (fast, precise for the
//     formats it knows, and the only step for most files);
//  2. TrID, only when step 1 is not confident;
//  3. heuristics of its own when TrID is missing or fails: text and
//     encoding analysis, entropy and an embedded-object census.
package detect

// Category groups formats for display and for choosing explorers.
type Category string

const (
	CatArchive      Category = "Archive"
	CatCompressed   Category = "Compressed"
	CatDocument     Category = "Document"
	CatSpreadsheet  Category = "Spreadsheet"
	CatPresentation Category = "Presentation"
	CatImage        Category = "Image"
	CatAudio        Category = "Audio"
	CatVideo        Category = "Video"
	CatExecutable   Category = "Executable"
	CatFont         Category = "Font"
	CatDatabase     Category = "Database"
	CatText         Category = "Text"
	CatCode         Category = "Source code"
	CatDiskImage    Category = "Disk image"
	CatEmail        Category = "Email"
	CatCrypto       Category = "Certificate / key"
	CatData         Category = "Data"
	CatEncrypted    Category = "Encrypted / random"
	CatEmpty        Category = "Empty"
	CatUnknown      Category = "Unknown"
)

// Source says which engine produced the verdict.
type Source string

const (
	SrcSignature Source = "Signature"
	SrcStructure Source = "Structure"
	SrcTrID      Source = "TrID"
	SrcHeuristic Source = "Heuristic"
	SrcMagika    Source = "Magika"
)

// Candidate is one possible identification, as reported by TrID.
type Candidate struct {
	Percent  float64
	Ext      string
	Label    string
	Points   int // definition score
	Patterns int // how many byte patterns of the definition matched
}

// Detail is one labelled fact about the file.
type Detail struct {
	Key, Value string
}

// Result is the verdict for one file.
type Result struct {
	Label      string   // human readable format name
	Ext        string   // canonical extension without dot, "" if none
	MIME       string   // best-effort media type
	Category   Category // display group
	Confidence float64  // 0..1
	Source     Source

	Details    []Detail
	Candidates []Candidate // TrID's ranked list, when TrID ran
	Notes      []string

	Size        int64
	Entropy     float64 // bits per byte over a sample, 0..8
	TrIDRan     bool
	TrIDMissing bool

	Magika        *MagikaVerdict // Magika's answer, when it ran
	MagikaRan     bool
	MagikaMissing bool
}

func (r *Result) detail(k, v string) {
	if v != "" {
		r.Details = append(r.Details, Detail{k, v})
	}
}

func (r *Result) note(s string) { r.Notes = append(r.Notes, s) }

// set fills in the identification fields.
func (r *Result) set(label, ext, mime string, cat Category, conf float64, src Source) {
	r.Label, r.Ext, r.MIME, r.Category, r.Confidence, r.Source = label, ext, mime, cat, conf, src
}

// Detail returns the value recorded under key, or "".
func (r *Result) Detail(key string) string {
	for _, d := range r.Details {
		if d.Key == key {
			return d.Value
		}
	}
	return ""
}
