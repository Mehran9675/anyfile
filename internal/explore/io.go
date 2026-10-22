package explore

import (
	"bytes"
	"io"
	"os"
	"sync"
)

// ReadCloser pairs a reader with a close function.
type ReadCloser struct {
	io.Reader
	CloseFn func() error
}

// NewReadCloser wraps r so that Close runs fn.
func NewReadCloser(r io.Reader, fn func() error) io.ReadCloser {
	return ReadCloser{Reader: r, CloseFn: fn}
}

// Close runs the close function.
func (r ReadCloser) Close() error {
	if r.CloseFn == nil {
		return nil
	}
	return r.CloseFn()
}

// BytesOpener serves an in-memory buffer as an entry's data.
func BytesOpener(b []byte) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
}

// SectionOpener serves a byte range of r as an entry's data.
func SectionOpener(r io.ReaderAt, off, n int64) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		return io.NopCloser(io.NewSectionReader(r, off, n)), nil
	}
}

// LazyFile returns a ReaderAt over p that opens the file on first use
// and closes it with l.
func LazyFile(p string, l *Listing) io.ReaderAt {
	return &lazyFile{path: p, l: l}
}

type lazyFile struct {
	path string
	l    *Listing
	once sync.Once
	f    *os.File
	err  error
}

func (z *lazyFile) ReadAt(b []byte, off int64) (int, error) {
	z.once.Do(func() {
		z.f, z.err = os.Open(z.path)
		if z.err == nil {
			f := z.f
			z.l.OnClose(func() { f.Close() })
		}
	})
	if z.err != nil {
		return 0, z.err
	}
	return z.f.ReadAt(b, off)
}
