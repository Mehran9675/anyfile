package detect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/mehran9675/anyfile/internal/tools"
)

// MagikaVerdict is what Magika, Google's deep-learning content-type
// classifier, said about a file.
type MagikaVerdict struct {
	Label       string   // short label: "pdf", "pebin", "unknown"...
	Description string   // "PDF document"
	Group       string   // "document", "image", "executable"...
	MIME        string   // media type
	Extensions  []string // typical extensions, most common first
	IsText      bool
	Score       float64 // model confidence for the raw prediction, 0..1
	// The model's raw prediction, before Magika's own confidence
	// thresholds turn uncertain answers into "unknown" or "txt".
	RawLabel       string
	RawDescription string
}

// Known reports whether Magika gave a usable answer.
func (v *MagikaVerdict) Known() bool { return v != nil && v.Label != "" && v.Label != "unknown" }

type magikaType struct {
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Group       string   `json:"group"`
	MIME        string   `json:"mime_type"`
	Extensions  []string `json:"extensions"`
	IsText      bool     `json:"is_text"`
}

type magikaEntry struct {
	Path   string `json:"path"`
	Result struct {
		Status string `json:"status"`
		Value  struct {
			DL     magikaType `json:"dl"`
			Output magikaType `json:"output"`
			Score  float64    `json:"score"`
		} `json:"value"`
	} `json:"result"`
}

const magikaTimeout = 30 * time.Second

// runMagika asks the magika command about p. ok is false when Magika
// isn't installed or couldn't run.
func runMagika(p string) (v *MagikaVerdict, ok bool, err error) {
	t := tools.Magika()
	if !t.Found() {
		return nil, false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), magikaTimeout)
	defer cancel()
	cmd := tools.Command(t.Path, "--json", "--no-colors", p)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-ctx.Done():
		cmd.Process.Kill()
		return nil, false, fmt.Errorf("magika took longer than %s", magikaTimeout)
	case <-done:
	}
	return parseMagika(out.Bytes())
}

// parseMagika reads the --json output for a single file.
func parseMagika(b []byte) (*MagikaVerdict, bool, error) {
	var entries []magikaEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, false, fmt.Errorf("unreadable magika output: %w", err)
	}
	if len(entries) == 0 {
		return nil, false, fmt.Errorf("magika returned nothing")
	}
	e := entries[0]
	if e.Result.Status != "ok" {
		return nil, false, fmt.Errorf("magika: %s", e.Result.Status)
	}
	o, dl := e.Result.Value.Output, e.Result.Value.DL
	return &MagikaVerdict{
		Label: o.Label, Description: o.Description, Group: o.Group, MIME: o.MIME,
		Extensions: o.Extensions, IsText: o.IsText, Score: e.Result.Value.Score,
		RawLabel: dl.Label, RawDescription: dl.Description,
	}, true, nil
}

// magikaCategory maps Magika's groups (and a few labels) onto AnyFile's
// categories.
func magikaCategory(v *MagikaVerdict) Category {
	switch v.Label {
	case "xls", "xlsx", "xlsb", "ods", "csv", "tsv":
		return CatSpreadsheet
	case "ppt", "pptx", "odp":
		return CatPresentation
	case "eml", "outlook", "mbox":
		return CatEmail
	case "sqlite":
		return CatDatabase
	case "pem", "der", "pgp", "pkcs":
		return CatCrypto
	case "iso", "vhd", "vmdk", "qcow", "dmg", "squashfs":
		return CatDiskImage
	case "gzip", "bzip", "bzip3", "xz", "zstd", "lz4", "lzip", "lzma":
		return CatCompressed
	}
	switch v.Group {
	case "archive":
		return CatArchive
	case "audio":
		return CatAudio
	case "video":
		return CatVideo
	case "image":
		return CatImage
	case "document":
		return CatDocument
	case "executable":
		return CatExecutable
	case "font":
		return CatFont
	case "code":
		return CatCode
	case "text":
		return CatText
	case "application":
		return CatData
	}
	return CatUnknown
}

// magikaConfidence turns Magika's score into AnyFile's confidence
// scale, a little below its own so signatures and structure win ties.
func magikaConfidence(score float64) float64 { return math.Min(0.95, score*0.95) }

// consultMagika merges Magika's opinion into r.
func consultMagika(p string, r *Result) {
	v, ok, err := runMagika(p)
	if !ok {
		r.MagikaMissing = !tools.Magika().Found()
		if err != nil {
			r.note("Magika could not run: " + err.Error())
		}
		return
	}
	r.MagikaRan, r.Magika = true, v
	mergeMagika(r, v)
}

// mergeMagika adopts Magika's answer when it beats the current verdict.
func mergeMagika(r *Result, v *MagikaVerdict) {
	switch {
	case !v.Known():
		// An unknown answer leaves the verdict to AnyFile's own fallback,
		// which also looks for embedded files Magika can't see; its raw
		// guess is reported there as supporting evidence.
		return
	case v.Label == "txt" && r.Label != "" && (r.Category == CatText || r.Category == CatCode || r.Category == CatData):
		// "Plain text" is Magika's fallback; a specific text format
		// found by the built-in analysis is more useful.
		return
	}
	conf := magikaConfidence(v.Score)
	if r.Label != "" && r.Confidence >= conf {
		return // built-in still wins; Magika's view stays visible
	}
	if r.Label != "" {
		r.note("Built-in analysis suggested: " + r.Label)
	}
	ext := v.Label
	if len(v.Extensions) > 0 {
		ext = v.Extensions[0]
	}
	r.set(v.Description, ext, v.MIME, magikaCategory(v), conf, SrcMagika)
}
