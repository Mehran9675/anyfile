// Package blobs digs binary payloads out of text: data: URIs in HTML,
// SVG, CSS or JSON, long base64 runs, and files that are base64
// through and through.
package blobs

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"regexp"
	"strings"

	"github.com/xmhrnx/anyfile/internal/settings"

	"github.com/xmhrnx/anyfile/internal/detect"
	"github.com/xmhrnx/anyfile/internal/explore"
)

func init() {
	explore.Register(explore.Engine{Name: "Encoded blobs", Priority: 90, Match: match, List: list})
}

func match(d *detect.Result, h []byte) bool {
	if d.Size > settings.MB(settings.Get().MaxTextMB) || d.Detail("Encoding") == "" {
		return false // only text the detector decoded
	}
	switch d.Category {
	case detect.CatText, detect.CatCode, detect.CatData, detect.CatDocument, detect.CatImage:
	default:
		return false
	}
	// Claim the file only when its start shows encoded content, so plain
	// text doesn't look expandable.
	return d.Ext == "b64" || bytes.Contains(h, []byte(";base64,")) || reB64Run.Match(h)
}

var (
	reDataURI = regexp.MustCompile(`data:([\w.+-]+/[\w.+-]+)?((?:;[\w-]+=[^;,]*)*);base64,([A-Za-z0-9+/=\s\\]+)`)
	reB64Run  = regexp.MustCompile(`[A-Za-z0-9+/]{256,}={0,2}`)
)

func list(p string, d *detect.Result) (*explore.Listing, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	l := &explore.Listing{Engine: "Encoded blob scanner (built-in)"}
	n := 0
	add := func(data []byte, mt, origin string) {
		n++
		ext, label, _ := detect.SniffBytes(data)
		if mt != "" {
			if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 && (ext == "bin" || ext == "txt") {
				ext = strings.TrimPrefix(exts[0], ".")
			}
		}
		l.Entries = append(l.Entries, &explore.Entry{
			Path: fmt.Sprintf("Decoded/%03d.%s", n, ext),
			Size: int64(len(data)),
			Note: label + " · " + origin,
			Open: explore.BytesOpener(data),
		})
	}

	// The whole file is one base64 document.
	if d.Ext == "b64" {
		if b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(text), "")); err == nil {
			add(b, "", "whole file")
			return l, nil
		}
	}
	covered := make([][2]int, 0)
	for _, m := range reDataURI.FindAllStringSubmatchIndex(text, -1) {
		payload := strings.Join(strings.Fields(strings.ReplaceAll(text[m[6]:m[7]], "\\", "")), "")
		b, err := decodeLoose(payload)
		if err != nil || len(b) < 16 {
			continue
		}
		mt := ""
		if m[2] >= 0 {
			mt = text[m[2]:m[3]]
		}
		add(b, mt, "data: URI")
		covered = append(covered, [2]int{m[0], m[1]})
	}
	for _, m := range reB64Run.FindAllStringIndex(text, -1) {
		inside := false
		for _, c := range covered {
			if m[0] >= c[0] && m[1] <= c[1] {
				inside = true
				break
			}
		}
		if inside {
			continue
		}
		b, err := decodeLoose(text[m[0]:m[1]])
		if err != nil {
			continue
		}
		// Keep runs that decode to something recognisable; long runs of
		// letters are otherwise just as likely to be identifiers.
		if _, _, ok := detect.SniffBytes(b); !ok {
			continue
		}
		add(b, "", fmt.Sprintf("base64 at offset %d", m[0]))
	}
	return l, nil
}

func decodeLoose(s string) ([]byte, error) {
	s = strings.TrimRight(s, "=")
	if len(s)%4 == 1 {
		s = s[:len(s)-1] // a lone trailing character carries no full byte
	}
	return base64.RawStdEncoding.DecodeString(s)
}
