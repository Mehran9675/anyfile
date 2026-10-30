// Package disc reads ISO 9660 disc images (with Joliet long names)
// natively; UDF-only images fall through to 7-Zip when it is installed.
package disc

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/xmhrnx/anyfile/internal/detect"
	"github.com/xmhrnx/anyfile/internal/explore"
)

func init() {
	explore.Register(explore.Engine{Name: "ISO 9660", Priority: 60, Match: func(d *detect.Result, h []byte) bool {
		return len(h) >= 0x8006 && string(h[0x8001:0x8006]) == "CD001"
	}, List: listISO})
}

const sector = 2048

type volume struct {
	rootLBA, rootLen uint32
	joliet           bool
}

func listISO(p string, d *detect.Result) (*explore.Listing, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	var primary, joliet *volume
	vd := make([]byte, sector)
	for s := int64(16); s < 64; s++ {
		if _, err := f.ReadAt(vd, s*sector); err != nil {
			break
		}
		if string(vd[1:6]) != "CD001" {
			break
		}
		v := &volume{rootLBA: binary.LittleEndian.Uint32(vd[156+2:]), rootLen: binary.LittleEndian.Uint32(vd[156+10:])}
		switch vd[0] {
		case 1:
			primary = v
		case 2:
			esc := string(vd[88:91])
			if esc == "%/@" || esc == "%/C" || esc == "%/E" {
				v.joliet = true
				joliet = v
			}
		case 255:
			s = 64
		}
	}
	v := joliet
	if v == nil {
		v = primary
	}
	if v == nil {
		f.Close()
		return nil, errors.New("no ISO 9660 volume descriptor")
	}
	l := &explore.Listing{Engine: "ISO 9660 (built-in)"}
	if v.joliet {
		l.Engine = "ISO 9660 + Joliet (built-in)"
	}
	l.OnClose(func() { f.Close() })
	seen := map[uint32]bool{}
	var walk func(lba, size uint32, dir string, depth int)
	walk = func(lba, size uint32, dir string, depth int) {
		if depth > 32 || seen[lba] || size > 64<<20 {
			return
		}
		seen[lba] = true
		buf := make([]byte, size)
		if _, err := f.ReadAt(buf, int64(lba)*sector); err != nil && err != io.EOF {
			return
		}
		for off := 0; off < len(buf); {
			n := int(buf[off])
			if n == 0 {
				off = (off/sector + 1) * sector // records never cross sectors
				continue
			}
			if off+n > len(buf) || n < 34 {
				break
			}
			r := buf[off : off+n]
			off += n
			nameLen := int(r[32])
			if 33+nameLen > len(r) {
				continue
			}
			raw := r[33 : 33+nameLen]
			if nameLen == 1 && (raw[0] == 0 || raw[0] == 1) {
				continue // "." and ".."
			}
			name := decodeName(raw, v.joliet)
			ext := binary.LittleEndian.Uint32(r[2:])
			length := binary.LittleEndian.Uint32(r[10:])
			isDir := r[25]&2 != 0
			full := path.Join(dir, explore.SafeName(name))
			t := recTime(r[18:25])
			if isDir {
				l.Entries = append(l.Entries, &explore.Entry{Path: full, Dir: true, Size: -1, ModTime: t})
				walk(ext, length, full, depth+1)
				continue
			}
			l.Entries = append(l.Entries, &explore.Entry{
				Path: full, Size: int64(length), ModTime: t,
				Open: explore.SectionOpener(f, int64(ext)*sector, int64(length)),
			})
		}
	}
	walk(v.rootLBA, v.rootLen, "", 0)
	if len(l.Entries) == 0 {
		l.Close()
		return nil, errors.New("empty ISO 9660 tree (the disc may be UDF only)")
	}
	return l, nil
}

func decodeName(b []byte, joliet bool) string {
	var s string
	if joliet {
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(b[2*i:])
		}
		s = string(utf16.Decode(u))
	} else {
		s = string(b)
	}
	if i := strings.LastIndex(s, ";"); i > 0 {
		s = s[:i]
	}
	return strings.TrimSuffix(s, ".")
}

func recTime(b []byte) time.Time {
	if len(b) < 7 || b[0] == 0 {
		return time.Time{}
	}
	loc := time.FixedZone("", int(int8(b[6]))*15*60)
	return time.Date(1900+int(b[0]), time.Month(b[1]), int(b[2]), int(b[3]), int(b[4]), int(b[5]), 0, loc)
}
