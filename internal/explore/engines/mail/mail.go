// Package mail explores MIME messages: .eml files, MHTML web archives
// and mbox mailboxes. Bodies and attachments become entries, decoded
// from their transfer encodings.
package mail

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path"
	"strings"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/explore"
)

func init() {
	explore.Register(explore.Engine{Name: "Email", Priority: 50, Match: func(d *detect.Result, h []byte) bool {
		switch d.Ext {
		case "eml", "mht", "mhtml", "mbox":
			return true
		}
		return d.Category == detect.CatEmail && d.Ext != "msg"
	}, List: listMail})
}

const maxMessage = 512 << 20

func listMail(p string, d *detect.Result) (*explore.Listing, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	if len(data) > maxMessage {
		return nil, fmt.Errorf("message larger than %d MB", maxMessage>>20)
	}
	l := &explore.Listing{Engine: "MIME (built-in)"}
	if d.Ext == "mbox" {
		l.Engine = "mbox (built-in)"
		for i, msg := range splitMbox(data) {
			subj := "message"
			if m, err := mail.ReadMessage(bytes.NewReader(msg)); err == nil {
				if s := decodeHeader(m.Header.Get("Subject")); s != "" {
					subj = s
				}
			}
			name := fmt.Sprintf("%04d - %s.eml", i+1, explore.SafeName(truncate(subj, 60)))
			l.Entries = append(l.Entries, &explore.Entry{Path: "Messages/" + name, Size: int64(len(msg)), Open: explore.BytesOpener(msg)})
		}
		return l, nil
	}
	m, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var hdr strings.Builder
	for _, k := range []string{"From", "To", "Cc", "Date", "Subject", "Message-Id"} {
		if v := m.Header.Get(k); v != "" {
			// Tab-separated so the summary isn't itself mistaken for a message.
			fmt.Fprintf(&hdr, "%-10s\t%s\r\n", k, decodeHeader(v))
		}
	}
	if hdr.Len() > 0 {
		b := []byte(hdr.String())
		l.Entries = append(l.Entries, &explore.Entry{Path: "Headers.txt", Size: int64(len(b)), Note: "summary of the main headers", Open: explore.BytesOpener(b)})
	}
	w := &walker{l: l, mht: d.Ext == "mht" || d.Ext == "mhtml"}
	w.part(textprotoHeader(m.Header), m.Body, "")
	return l, nil
}

type header interface{ Get(string) string }

type textprotoHeader mail.Header

func (h textprotoHeader) Get(k string) string { return mail.Header(h).Get(k) }

type walker struct {
	l     *explore.Listing
	mht   bool
	n     int
	names map[string]int
}

func (w *walker) part(h header, body io.Reader, dir string) {
	ct := h.Get("Content-Type")
	if ct == "" {
		ct = "text/plain"
	}
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil {
		mt = "application/octet-stream"
	}
	if strings.HasPrefix(mt, "multipart/") && params["boundary"] != "" {
		mr := multipart.NewReader(body, params["boundary"])
		for {
			p, err := mr.NextRawPart()
			if err != nil {
				break
			}
			w.part(p.Header, p, dir)
		}
		return
	}
	data, _ := io.ReadAll(decodeBody(h.Get("Content-Transfer-Encoding"), body))
	w.n++
	name, note := w.nameFor(h, mt, params)
	if mt == "message/rfc822" {
		note = "attached message"
	}
	sub := "Attachments"
	if w.mht {
		sub = "Resources"
	}
	full := path.Join(sub, name)
	if strings.HasPrefix(name, "Body") {
		full = name
	}
	w.l.Entries = append(w.l.Entries, &explore.Entry{Path: w.unique(full), Size: int64(len(data)), Note: note, Open: explore.BytesOpener(data)})
}

func (w *walker) nameFor(h header, mt string, params map[string]string) (name, note string) {
	dec := new(mime.WordDecoder)
	if _, dp, err := mime.ParseMediaType(h.Get("Content-Disposition")); err == nil && dp["filename"] != "" {
		name = dp["filename"]
	} else if params["name"] != "" {
		name = params["name"]
	} else if loc := h.Get("Content-Location"); loc != "" {
		name = path.Base(strings.SplitN(strings.SplitN(loc, "?", 2)[0], "#", 2)[0])
		note = loc
	}
	if s, err := dec.DecodeHeader(name); err == nil {
		name = s
	}
	disp := strings.ToLower(h.Get("Content-Disposition"))
	if name == "" || name == "/" || name == "." {
		ext := extFor(mt)
		switch {
		case mt == "text/plain" && !strings.HasPrefix(disp, "attachment"):
			name = "Body (plain text).txt"
		case mt == "text/html" && !strings.HasPrefix(disp, "attachment"):
			name = "Body (HTML).html"
		default:
			name = fmt.Sprintf("part_%02d.%s", w.n, ext)
		}
	}
	if note == "" {
		note = mt
	}
	return explore.SafeName(name), note
}

func (w *walker) unique(p string) string {
	if w.names == nil {
		w.names = map[string]int{}
	}
	w.names[p]++
	if n := w.names[p]; n > 1 {
		ext := path.Ext(p)
		return fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(p, ext), n, ext)
	}
	return p
}

func decodeBody(enc string, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(enc)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, &stripSpace{r: r})
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	}
	return r
}

// stripSpace drops whitespace the base64 decoder would choke on
// (it only skips CR and LF).
type stripSpace struct{ r io.Reader }

func (s *stripSpace) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	j := 0
	for _, c := range p[:n] {
		if c != ' ' && c != '\t' {
			p[j] = c
			j++
		}
	}
	return j, err
}

func extFor(mt string) string {
	known := map[string]string{
		"text/plain": "txt", "text/html": "html", "text/css": "css", "image/jpeg": "jpg", "image/png": "png",
		"image/gif": "gif", "application/pdf": "pdf", "message/rfc822": "eml", "text/calendar": "ics",
		"application/javascript": "js", "text/javascript": "js", "image/svg+xml": "svg",
	}
	if e, ok := known[mt]; ok {
		return e
	}
	if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 {
		return strings.TrimPrefix(exts[0], ".")
	}
	return "bin"
}

func decodeHeader(s string) string {
	if d, err := new(mime.WordDecoder).DecodeHeader(s); err == nil {
		return d
	}
	return s
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// splitMbox cuts a mailbox at its "From " separator lines.
func splitMbox(data []byte) [][]byte {
	var out [][]byte
	var cur bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	started := false
	for sc.Scan() {
		line := sc.Bytes()
		if bytes.HasPrefix(line, []byte("From ")) {
			if started && cur.Len() > 0 {
				out = append(out, append([]byte(nil), cur.Bytes()...))
			}
			cur.Reset()
			started = true
			continue
		}
		if started {
			if bytes.HasPrefix(line, []byte(">From ")) {
				line = line[1:]
			}
			cur.Write(line)
			cur.WriteString("\r\n")
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.Bytes())
	}
	return out
}
