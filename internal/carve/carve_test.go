package carve

import (
	"bytes"
	"math/rand"
	"os"
	"testing"
)

func noise(n int, seed int64) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(b)
	return b
}

// Three-letter signatures appear by chance in compressed data; they
// must not turn into "Flash movies" (or swallow the rest of the file).
func TestRandomSWFSignaturesIgnored(t *testing.T) {
	data := noise(1<<20, 1)
	for i := 1000; i < len(data)-64; i += 40000 {
		copy(data[i:], "CWS")
		data[i+3] = 10 // a plausible version byte
		copy(data[i+100:], "FWS\x09")
	}
	for _, h := range Scan(bytes.NewReader(data), int64(len(data)), Options{}) {
		if h.Ext == "swf" {
			t.Fatalf("false SWF at 0x%X", h.Off)
		}
	}
}

func TestRealSWFFound(t *testing.T) {
	swf, err := os.ReadFile("testdata/movie.swf")
	if err != nil {
		t.Fatal(err)
	}
	data := append(append(noise(5000, 2), swf...), noise(3000, 3)...)
	hits := Scan(bytes.NewReader(data), int64(len(data)), Options{})
	for _, h := range hits {
		if h.Ext == "swf" && h.Off == 5000 {
			return
		}
	}
	t.Fatalf("embedded SWF not found: %+v", hits)
}
