package explore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WriteTo copies an entry's data to the file dst.
func WriteTo(e *Entry, dst string) error {
	if e.Open == nil {
		return errors.New("this item has no data")
	}
	rc, err := e.Open()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		rc.Close()
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		rc.Close()
		return err
	}
	_, err = io.Copy(f, rc)
	if cerr := rc.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
		return err
	}
	if !e.ModTime.IsZero() {
		os.Chtimes(dst, e.ModTime, e.ModTime)
	}
	return nil
}

// SafeJoin joins an archive path onto dir, refusing anything that would
// escape it ("zip slip") and replacing characters Windows forbids.
func SafeJoin(dir, p string) (string, error) {
	parts := strings.Split(CleanPath(p), "/")
	for i, s := range parts {
		parts[i] = SafeName(s)
	}
	out := filepath.Join(append([]string{dir}, parts...)...)
	rel, err := filepath.Rel(dir, out)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe path %q", p)
	}
	return out, nil
}

// SafeName makes one path element valid on every platform.
func SafeName(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case c < 0x20, strings.ContainsRune(`<>:"/\|?*`, c):
			b.WriteRune('_')
		default:
			b.WriteRune(c)
		}
	}
	out := strings.TrimRight(b.String(), ". ")
	switch strings.ToUpper(strings.SplitN(out, ".", 2)[0]) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "LPT1", "LPT2", "LPT3":
		out = "_" + out
	}
	if out == "" || out == ".." {
		out = "_"
	}
	return out
}
