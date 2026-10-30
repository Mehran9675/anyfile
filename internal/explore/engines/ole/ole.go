// Package ole explores Compound File Binary containers: legacy Office
// documents, MSI packages, Outlook messages and OLE embedded objects,
// including files packaged into Office documents (Ole10Native).
package ole

import (
	"bytes"
	"io"
	"os"
	"path"
	"strings"

	"github.com/xmhrnx/anyfile/internal/cfb"
	"github.com/xmhrnx/anyfile/internal/detect"
	"github.com/xmhrnx/anyfile/internal/explore"
)

func init() {
	explore.Register(explore.Engine{Name: "OLE compound file", Priority: 20, Match: func(d *detect.Result, h []byte) bool {
		return bytes.HasPrefix(h, []byte(cfb.Signature))
	}, List: listCFB})
}

// --- OLE / CFB -----------------------------------------------------------------

func listCFB(p string, d *detect.Result) (*explore.Listing, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	st, _ := f.Stat()
	c, err := cfb.Open(f, st.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	l := &explore.Listing{Engine: "Compound File (built-in)"}
	l.OnClose(func() { f.Close() })
	for _, e := range c.Entries() {
		e := e
		ent := &explore.Entry{Path: e.Path, Dir: !e.IsStream, Size: e.Size}
		if e.IsStream {
			ent.Open = func() (io.ReadCloser, error) {
				b, err := c.Read(e)
				if err != nil {
					return nil, err
				}
				return io.NopCloser(bytes.NewReader(b)), nil
			}
			// Files packaged into Office documents.
			if strings.EqualFold(e.RawName, "\x01Ole10Native") {
				if b, err := c.Read(e); err == nil {
					if pkg, ok := cfb.ParseOle10Native(b); ok {
						name := path.Base(strings.ReplaceAll(pkg.Label, "\\", "/"))
						if name == "" || name == "." {
							name = "package.bin"
						}
						l.Entries = append(l.Entries, &explore.Entry{
							Path: path.Join(path.Dir(e.Path), "Packaged file", name),
							Size: int64(len(pkg.Data)),
							Note: "embedded from " + pkg.Source,
							Open: explore.BytesOpener(pkg.Data),
						})
					}
				}
			}
			if strings.EqualFold(e.Name, "EncryptedPackage") {
				ent.Note = "encrypted OOXML payload"
			}
		}
		l.Entries = append(l.Entries, ent)
	}
	return l, nil
}
