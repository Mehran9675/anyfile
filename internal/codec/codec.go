// Package codec opens the stream decompressors AnyFile uses, with the
// memory limits from the user's settings, and turns their limit errors
// into messages that say which setting to change.
package codec

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"github.com/mehran9675/anyfile/internal/settings"
)

// Open returns a decompressing reader for ext (gz, tgz, bz2, xz, zst)
// and a function releasing it. Other extensions pass r through.
func Open(ext string, r io.Reader) (io.Reader, func(), error) {
	switch ext {
	case "gz", "tgz":
		z, err := gzip.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return z, func() { z.Close() }, nil
	case "bz2":
		return bzip2.NewReader(r), func() {}, nil
	case "xz":
		z, err := xz.NewReader(bufio.NewReader(r))
		if err != nil {
			return nil, nil, err
		}
		return z, func() {}, nil
	case "zst":
		limit := uint64(settings.MB(settings.Get().MaxDecompressMB))
		z, err := zstd.NewReader(r, zstd.WithDecoderMaxWindow(limit), zstd.WithDecoderMaxMemory(limit*4))
		if err != nil {
			return nil, nil, err
		}
		return &limitErr{z}, z.Close, nil
	}
	return r, func() {}, nil
}

// limitErr rewrites a decoder's limit errors as a hint.
type limitErr struct{ r io.Reader }

func (l *limitErr) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	return n, Explain(err)
}

// Explain adds the setting to change to errors caused by AnyFile's
// decompression limits; other errors pass through unchanged.
func Explain(err error) error {
	if err == nil || err == io.EOF {
		return err
	}
	msg := err.Error()
	if errors.Is(err, zstd.ErrWindowSizeExceeded) || errors.Is(err, zstd.ErrDecoderSizeExceeded) ||
		strings.Contains(msg, "window size exceeded") || strings.Contains(msg, "exceeds configured limit") {
		return fmt.Errorf("%w: the data needs more memory than the decompression limit (%d MB); raise it in Settings",
			err, settings.Get().MaxDecompressMB)
	}
	return err
}
