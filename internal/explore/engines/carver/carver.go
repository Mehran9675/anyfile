// Package carver is the last-resort engine: it finds files embedded
// anywhere inside another file by their signatures (see package carve)
// and serves each as an entry.
package carver

import (
	"fmt"
	"os"

	"github.com/mehran9675/anyfile/internal/carve"
	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/explore"
)

func init() {
	explore.Register(explore.Engine{Name: "Signature carver", Priority: 1000, Fallback: true, Match: autoCarve, List: carveFile})
}

// autoCarve decides whether carving runs without being asked.
func autoCarve(d *detect.Result, h []byte) bool {
	switch d.Category {
	case detect.CatUnknown, detect.CatData, detect.CatEncrypted, detect.CatDiskImage, detect.CatExecutable, detect.CatDatabase:
		return d.Size > 64
	}
	return false
}

// --- carving -------------------------------------------------------------------

func carveFile(p string, d *detect.Result) (*explore.Listing, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	st, _ := f.Stat()
	hits := carve.Scan(f, st.Size(), carve.Options{SkipSelf: true, Limit: 1000})
	if len(hits) == 0 {
		f.Close()
		return nil, explore.ErrNotContainer
	}
	l := &explore.Listing{Engine: "Signature carver (built-in)"}
	l.OnClose(func() { f.Close() })
	for i, h := range hits {
		e := &explore.Entry{
			Path: fmt.Sprintf("carved/%03d_at_0x%X.%s", i+1, h.Off, h.Ext),
			Size: h.Len,
			Note: h.Label,
			Open: explore.SectionOpener(f, h.Off, h.Len),
		}
		if h.Approx {
			e.Note += " · end estimated"
		}
		l.Entries = append(l.Entries, e)
	}
	l.Notes = append(l.Notes, "Objects were found by scanning for embedded signatures; boundaries marked \"end estimated\" may include trailing data.")
	return l, nil
}
