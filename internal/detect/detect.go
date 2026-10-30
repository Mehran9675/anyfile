package detect

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/mehran9675/anyfile/internal/settings"

	"github.com/mehran9675/anyfile/internal/carve"
	"github.com/mehran9675/anyfile/internal/cfb"
	"github.com/mehran9675/anyfile/internal/tools"
	"github.com/mehran9675/giode/i18n"
)

// Options tune Detect.
type Options struct {
	// Name is the logical file name (for archive members the on-disk
	// copy may be named differently). It only breaks ties.
	Name string
	// ForceTrID runs TrID even when the built-in engines are sure.
	ForceTrID bool
	// ForceMagika runs Magika even when the built-in engines are sure.
	ForceMagika bool
}

// How much of a file the built-in engines read, the confidence above
// which TrID and Magika are skipped, and when they run at all come from
// the user's settings. The 64 KiB default sample covers every fixed-offset probe
// (the deepest is the ISO 9660 descriptor at 0x8001-0x9006) and gives
// the text classifiers enough lines, in a single read.

// Detect identifies the file at p.
func Detect(p string, opt Options) *Result {
	r := &Result{}
	f, err := os.Open(p)
	if err != nil {
		r.set("Unreadable file", "", "", CatUnknown, 0, SrcHeuristic)
		r.note(err.Error())
		return r
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		r.set("Unreadable file", "", "", CatUnknown, 0, SrcHeuristic)
		return r
	}
	r.Size = st.Size()
	if opt.Name == "" {
		opt.Name = st.Name()
	}
	if r.Size == 0 {
		r.set("Empty file", "", "application/x-empty", CatEmpty, 1, SrcHeuristic)
		return r
	}
	cfg := settings.Get()
	head := make([]byte, min(int64(cfg.HeadKB)<<10, r.Size))
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	r.Entropy = entropy(f, r.Size, settings.MB(cfg.EntropySampleMB))
	r.detail("Entropy", fmt.Sprintf("%.2f bits/byte", r.Entropy))

	// 1. Built-in signatures and structural probes.
	builtin(p, head, opt.Name, r)

	// 2. Magika, only when needed (or asked for). It runs before TrID
	// because it is fast and strong on text and headerless formats.
	switch {
	case opt.ForceMagika, cfg.MagikaMode == settings.TrIDAlways:
		consultMagika(p, r)
	case cfg.MagikaMode == settings.TrIDNever:
	case r.Confidence < cfg.TrIDTrust, genericText(r):
		consultMagika(p, r)
	}

	// 3. TrID, only when still needed (or asked for).
	switch {
	case opt.ForceTrID, cfg.TrIDMode == settings.TrIDAlways:
		consultTrID(p, opt.Name, r)
	case cfg.TrIDMode == settings.TrIDNever:
	case r.Confidence < cfg.TrIDTrust:
		consultTrID(p, opt.Name, r)
	}

	// 4. Our own fallback when nobody could tell.
	if r.Label == "" {
		fallback(f, head, r)
	}
	if r.Source == "" {
		r.Source = SrcHeuristic
	}
	return r
}

// genericText reports a "Plain text" verdict: confident that the file
// is text, but not which kind. Magika is good at telling text formats
// apart (YAML, Markdown, code), so it is consulted; TrID isn't, being
// weak on text.
func genericText(r *Result) bool {
	return r.Category == CatText && r.Ext == "txt" && r.Source == SrcHeuristic
}

// builtin runs the signature table and the structural refiners.
func builtin(p string, h []byte, name string, r *Result) {
	switch {
	case bytes.HasPrefix(h, []byte(cfb.Signature)):
		r.set("OLE compound document", "ole", "application/x-ole-storage", CatData, 0.9, SrcSignature)
		refineOLE(p, r)
		return
	case bytes.HasPrefix(h, []byte("RIFF")) && refineRIFF(h, r):
		return
	case len(h) > 12 && string(h[4:8]) == "ftyp" && refineFtyp(h, r):
		return
	case bytes.HasPrefix(h, []byte("\x1A\x45\xDF\xA3")):
		refineEBML(h, r)
		return
	case bytes.HasPrefix(h, []byte("OggS")):
		refineOgg(h, r)
		return
	case bytes.HasPrefix(h, []byte("MZ")) && refinePE(p, h, r):
		return
	case bytes.HasPrefix(h, []byte("MZ")):
		r.set("MS-DOS executable", "exe", "application/x-dosexec", CatExecutable, 0.8, SrcSignature)
		return
	case len(h) >= 262 && string(h[257:262]) == "ustar":
		r.set("TAR archive", "tar", "application/x-tar", CatArchive, 0.97, SrcStructure)
		return
	case refineISO(h, r):
		return
	}
	if s := matchSig(h); s != nil {
		r.set(s.label, s.ext, s.mime, s.cat, s.conf, SrcSignature)
		switch s.ext {
		case "zip":
			refineZip(p, r)
		case "gz", "bz2", "xz", "zst":
			refineCompressed(p, r)
		case "elf":
			refineELF(p, r)
		case "class", "macho":
			refineMachO(p, h, r)
		}
		return
	}
	// MPEG audio frame sync / ADTS.
	if len(h) > 4 && h[0] == 0xFF && h[1]&0xE0 == 0xE0 {
		if h[1]&0xF6 == 0xF0 {
			r.set("AAC audio (ADTS)", "aac", "audio/aac", CatAudio, 0.7, SrcSignature)
			return
		}
		if (h[1]>>3)&3 != 1 && (h[1]>>1)&3 != 0 && h[2]>>4 != 15 {
			r.set("MPEG audio", "mp3", "audio/mpeg", CatAudio, 0.65, SrcSignature)
			return
		}
	}
	if refineMBR(h, r) {
		return
	}
	// A ZIP with a stub in front (self-extractors, some installers).
	if ti, ok := analyzeText(h); ok {
		classifyText(ti, name, len(h) == int(r.Size), r)
		return
	}
}

// consultTrID merges TrID's opinion into r.
func consultTrID(p, name string, r *Result) {
	cands, ok := runTrID(p)
	if !ok {
		r.TrIDMissing = !tools.TrID().Found()
		return
	}
	r.TrIDRan = true
	r.Candidates = cands
	if len(cands) == 0 {
		if r.Label != "" {
			r.note(i18n.Intl("TrID could not identify this file; the built-in analysis stands."))
		} else {
			r.note(i18n.Intl("TrID could not identify this file; AnyFile analysed it on its own."))
		}
		return
	}
	top := cands[0]
	// Prefer the candidate matching the file's own extension when it is
	// close to the top: TrID's generic definitions often tie.
	if e := extOf(name); e != "" {
		for _, c := range cands[1:] {
			if c.Ext == e && c.Percent >= top.Percent*0.6 {
				top = c
				break
			}
		}
	}
	// TrID's percentage is relative to its other guesses: a lone match
	// of a one-pattern definition scores 100% yet means little. Weigh it
	// by how many patterns matched.
	evidence := math.Min(1, float64(top.Patterns-1)/3)
	conf := math.Min(0.95, (0.2+top.Percent/100*0.6)*(0.4+0.6*evidence))
	if conf < 0.35 {
		r.note(i18n.Intl("TrID's best guess ({type}, {percent}%) rests on very little evidence and was not adopted.",
			"type", top.Label, "percent", fmt.Sprintf("%.0f", top.Percent)))
		return
	}
	if r.Label != "" && r.Confidence >= conf {
		// Built-in still wins, but TrID's ranking stays visible.
		return
	}
	if r.Label != "" {
		r.note(i18n.Intl("Built-in analysis suggested: {type}", "type", r.Label))
	}
	mime := r.MIME
	if r.Ext != top.Ext {
		mime = ""
	}
	r.set(top.Label, top.Ext, mime, tridCategory(top.Label, top.Ext), conf, SrcTrID)
}

// fallback is AnyFile's own analysis for files nobody recognised:
// entropy, strings and a census of embedded objects.
func fallback(f *os.File, h []byte, r *Result) {
	zeros := bytes.Count(h, []byte{0})
	strs := countStrings(h)
	hits := carve.Scan(f, r.Size, carve.Options{Limit: 50, SkipSelf: true})
	if len(hits) > 0 {
		var kinds []string
		seen := map[string]int{}
		for _, hit := range hits {
			seen[hit.Label]++
		}
		for k, v := range seen {
			kinds = append(kinds, fmt.Sprintf("%d× %s", v, k))
		}
		r.set("Binary container with embedded files", "bin", "application/octet-stream", CatData, 0.5, SrcHeuristic)
		r.note(i18n.Intl("Found {count} embedded objects: {kinds}. Explore to extract them.", "count", len(hits), "kinds", strings.Join(kinds, ", ")))
		if hits[0].Off == 0 {
			r.Label = "Unknown wrapper around " + hits[0].Label
		}
		return
	}
	switch {
	case zeros > len(h)*95/100:
		r.set("Zero-filled / sparse data", "bin", "application/octet-stream", CatData, 0.6, SrcHeuristic)
	case r.Entropy > 7.95:
		r.set("Encrypted or compressed data", "bin", "application/octet-stream", CatEncrypted, 0.45, SrcHeuristic)
		r.note(i18n.Intl("Near-maximal entropy with no recognisable structure: likely encrypted, compressed with an unknown scheme, or random."))
		if r.Magika != nil && r.Magika.RawLabel == "randombytes" {
			r.Confidence = 0.6
			r.note(i18n.Intl("Magika agrees: its model reads the content as random bytes ({percent}%).", "percent", fmt.Sprintf("%.0f", r.Magika.Score*100)))
		}
	case r.Entropy > 7.2:
		r.set("Compressed binary data", "bin", "application/octet-stream", CatData, 0.35, SrcHeuristic)
	default:
		r.set("Unknown binary data", "bin", "application/octet-stream", CatUnknown, 0.2, SrcHeuristic)
		if strs > 20 {
			r.note(i18n.Intl("Contains {count} readable strings in the first {size}; it may be a proprietary structured format.", "count", strs, "size", humanSize(int64(len(h)))))
		}
	}
}

// entropy samples up to 4 MiB spread over the file.
func entropy(f *os.File, size, sample int64) float64 {
	var counts [256]int64
	var total int64
	const block = 64 << 10
	blocks := int64(64)
	step := size / blocks
	if step < block {
		step = block
	}
	buf := make([]byte, block)
	for off := int64(0); off < size && total < sample; off += step {
		n, _ := f.ReadAt(buf, off)
		for _, c := range buf[:n] {
			counts[c]++
		}
		total += int64(n)
	}
	if total == 0 {
		return 0
	}
	var e float64
	for _, c := range counts {
		if c > 0 {
			p := float64(c) / float64(total)
			e -= p * math.Log2(p)
		}
	}
	return e
}

func countStrings(h []byte) int {
	n, run := 0, 0
	for _, c := range h {
		if c >= 0x20 && c < 0x7F {
			run++
			continue
		}
		if run >= 6 {
			n++
		}
		run = 0
	}
	return n
}
