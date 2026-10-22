// Package archive explores ZIP (and every ZIP-based format: OOXML,
// ODF, JAR, APK, EPUB...), TAR in any compression, single-stream
// compressors (gzip, bzip2, xz, zstd) and ar archives.
package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/mehran9675/anyfile/internal/codec"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/explore"
)

func init() {
	explore.Register(explore.Engine{Name: "ZIP", Priority: 10, Match: matchZip, List: listZip})
	explore.Register(explore.Engine{Name: "TAR", Priority: 11, Match: matchTar, List: listTar})
	explore.Register(explore.Engine{Name: "Decompressor", Priority: 12, Match: matchCompressed, List: listCompressed})
	explore.Register(explore.Engine{Name: "ar archive", Priority: 13, Match: func(d *detect.Result, h []byte) bool {
		return bytes.HasPrefix(h, []byte("!<arch>\n"))
	}, List: listAr})
}

// --- ZIP and every ZIP-based format ------------------------------------------

func matchZip(d *detect.Result, h []byte) bool {
	return bytes.HasPrefix(h, []byte("PK")) || strings.Contains(d.Label, "ZIP self-extracting")
}

func listZip(p string, d *detect.Result) (*explore.Listing, error) {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, err
	}
	l := &explore.Listing{Engine: "ZIP (built-in)"}
	l.OnClose(func() { zr.Close() })
	encrypted := 0
	for _, f := range zr.File {
		f := f
		e := &explore.Entry{
			Path:    f.Name,
			Size:    int64(f.UncompressedSize64),
			ModTime: f.Modified,
			Dir:     strings.HasSuffix(f.Name, "/"),
		}
		if f.Flags&1 != 0 {
			encrypted++
			e.Note = "encrypted"
		}
		if !e.Dir {
			e.Open = func() (io.ReadCloser, error) {
				if f.Flags&1 != 0 {
					return nil, errors.New("this entry is password-protected")
				}
				return f.Open()
			}
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			e.Note = "method " + strconv.Itoa(int(f.Method))
		}
		l.Entries = append(l.Entries, e)
	}
	if encrypted > 0 {
		l.Notes = append(l.Notes, strconv.Itoa(encrypted)+" entries are encrypted and cannot be extracted without the password.")
	}
	if zr.Comment != "" {
		l.Notes = append(l.Notes, "Archive comment: "+zr.Comment)
	}
	return l, nil
}

// --- TAR (plain and compressed) ----------------------------------------------

func matchTar(d *detect.Result, h []byte) bool {
	return d.Ext == "tar" || strings.HasPrefix(d.Ext, "tar.")
}

// decompressor opens a reader for a compressed stream by extension,
// with the decompression limits from the settings.
func decompressor(ext string, r io.Reader) (io.Reader, func(), error) {
	return codec.Open(ext, r)
}

func listTar(p string, d *detect.Result) (*explore.Listing, error) {
	comp := strings.TrimPrefix(strings.TrimPrefix(d.Ext, "tar"), ".")
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cr := &countReader{r: f}
	var src io.Reader = cr
	rd, done, err := decompressor(comp, src)
	if err != nil {
		return nil, err
	}
	defer done()
	tr := tar.NewReader(rd)
	l := &explore.Listing{Engine: "TAR (built-in)"}
	if comp != "" {
		l.Engine = "TAR + " + strings.ToUpper(comp) + " (built-in)"
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if len(l.Entries) == 0 {
				return nil, err
			}
			l.Notes = append(l.Notes, "Archive is truncated or damaged: "+err.Error())
			break
		}
		e := &explore.Entry{Path: h.Name, Size: h.Size, ModTime: h.ModTime, Dir: h.Typeflag == tar.TypeDir}
		switch h.Typeflag {
		case tar.TypeSymlink, tar.TypeLink:
			e.Note = "link → " + h.Linkname
			e.Size = 0
		case tar.TypeReg, tar.TypeRegA, tar.TypeGNUSparse:
			name := h.Name
			if comp == "" {
				// Uncompressed: the data sits at a known offset.
				off := cr.n // tar reads whole blocks and no further
				e.Open = explore.SectionOpener(explore.LazyFile(p, l), off, h.Size)
			} else {
				e.Open = func() (io.ReadCloser, error) { return reopenTarMember(p, comp, name) }
			}
		}
		l.Entries = append(l.Entries, e)
	}
	return l, nil
}

// reopenTarMember streams a compressed tarball up to one member.
func reopenTarMember(p, comp, name string) (io.ReadCloser, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	rd, done, err := decompressor(comp, f)
	if err != nil {
		f.Close()
		return nil, err
	}
	tr := tar.NewReader(rd)
	for {
		h, err := tr.Next()
		if err != nil {
			done()
			f.Close()
			if err == io.EOF {
				err = errors.New("member not found")
			}
			return nil, err
		}
		if h.Name == name {
			return explore.NewReadCloser(tr, func() error { done(); return f.Close() }), nil
		}
	}
}

// countReader counts bytes read so uncompressed tar members can be
// addressed by offset.
type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// --- single-stream compressors -----------------------------------------------

func matchCompressed(d *detect.Result, h []byte) bool {
	switch d.Ext {
	case "gz", "bz2", "xz", "zst":
		return true
	}
	return false
}

func listCompressed(p string, d *detect.Result) (*explore.Listing, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	base := path.Base(strings.ReplaceAll(p, "\\", "/"))
	name := strings.TrimSuffix(base, path.Ext(base))
	if name == base || name == "" {
		name = base + ".out"
	}
	var size int64 = -1
	var mod time.Time
	if d.Ext == "gz" {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		if zr.Name != "" {
			name = zr.Name
		}
		mod = zr.ModTime
		zr.Close()
		if st, err := f.Stat(); err == nil && st.Size() >= 4 && st.Size() < 1<<32 {
			t := make([]byte, 4)
			f.ReadAt(t, st.Size()-4)
			size = int64(binary.LittleEndian.Uint32(t))
		}
	}
	ext := d.Ext
	e := &explore.Entry{Path: name, Size: size, ModTime: mod, Note: "decompressed " + strings.ToUpper(ext)}
	e.Open = func() (io.ReadCloser, error) {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		rd, done, err := decompressor(ext, f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return explore.NewReadCloser(rd, func() error { done(); return f.Close() }), nil
	}
	return &explore.Listing{Engine: strings.ToUpper(ext) + " (built-in)", Entries: []*explore.Entry{e}}, nil
}

// --- ar (.deb, .a, .lib) -------------------------------------------------------

func listAr(p string, d *detect.Result) (*explore.Listing, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	st, _ := f.Stat()
	size := st.Size()
	l := &explore.Listing{Engine: "ar (built-in)"}
	l.OnClose(func() { f.Close() })
	var longNames []byte
	off := int64(8)
	h := make([]byte, 60)
	for off+60 <= size {
		if _, err := f.ReadAt(h, off); err != nil {
			break
		}
		if string(h[58:60]) != "`\n" {
			break
		}
		name := strings.TrimRight(string(h[0:16]), " ")
		n, err := strconv.ParseInt(strings.TrimSpace(string(h[48:58])), 10, 64)
		if err != nil {
			break
		}
		mt, _ := strconv.ParseInt(strings.TrimSpace(string(h[16:28])), 10, 64)
		data := off + 60
		switch {
		case name == "//":
			longNames = make([]byte, n)
			f.ReadAt(longNames, data)
			name = ""
		case name == "/" || name == "/SYM64/" || name == "__.SYMDEF":
			name = "(symbol table)"
		case strings.HasPrefix(name, "#1/"): // BSD long name inline
			k, _ := strconv.Atoi(name[3:])
			b := make([]byte, k)
			f.ReadAt(b, data)
			name = strings.TrimRight(string(b), "\x00")
			data += int64(k)
			n -= int64(k)
		case strings.HasPrefix(name, "/") && longNames != nil:
			if k, err := strconv.Atoi(name[1:]); err == nil && k < len(longNames) {
				s := string(longNames[k:])
				if i := strings.IndexAny(s, "/\n"); i >= 0 {
					s = s[:i]
				}
				name = s
			}
		default:
			name = strings.TrimSuffix(name, "/")
		}
		if name != "" {
			e := &explore.Entry{Path: name, Size: n, Open: explore.SectionOpener(f, data, n)}
			if mt > 0 {
				e.ModTime = time.Unix(mt, 0)
			}
			l.Entries = append(l.Entries, e)
		}
		off = data + n
		if off%2 == 1 {
			off++
		}
	}
	return l, nil
}
