// Package cfb reads Microsoft Compound File Binary (OLE2 structured
// storage) files: legacy Office documents, MSI installers, Outlook
// messages, thumbs.db and Office embedded objects.
package cfb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

const (
	freeSect   = 0xFFFFFFFF
	endOfChain = 0xFFFFFFFE
	noStream   = 0xFFFFFFFF
)

// Signature is the CFB magic.
const Signature = "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"

// Entry is one storage or stream.
type Entry struct {
	Name     string // decoded display name
	RawName  string // name as stored
	Path     string // slash-joined path from the root
	IsStream bool
	Size     int64
	CLSID    [16]byte
	start    uint32
	children []int
}

// File is an open compound file.
type File struct {
	r         io.ReaderAt
	size      int64
	secSize   int
	miniSize  int
	cutoff    int64
	fat       []uint32
	miniFAT   []uint32
	dir       []*Entry
	ministrm  []byte
	miniStart uint32
	miniLen   int64
	loaded    bool
}

// Open parses the header, FAT and directory.
func Open(r io.ReaderAt, size int64) (*File, error) {
	h := make([]byte, 512)
	if _, err := r.ReadAt(h, 0); err != nil {
		return nil, err
	}
	if string(h[:8]) != Signature {
		return nil, errors.New("cfb: bad signature")
	}
	shift := binary.LittleEndian.Uint16(h[0x1E:])
	mshift := binary.LittleEndian.Uint16(h[0x20:])
	if shift != 9 && shift != 12 {
		return nil, fmt.Errorf("cfb: unsupported sector shift %d", shift)
	}
	f := &File{r: r, size: size, secSize: 1 << shift, miniSize: 1 << mshift}
	f.cutoff = int64(binary.LittleEndian.Uint32(h[0x38:]))
	nFAT := binary.LittleEndian.Uint32(h[0x2C:])
	dirStart := binary.LittleEndian.Uint32(h[0x30:])
	miniFATStart := binary.LittleEndian.Uint32(h[0x3C:])
	difatStart := binary.LittleEndian.Uint32(h[0x44:])
	nDIFAT := binary.LittleEndian.Uint32(h[0x48:])

	// Collect FAT sector numbers from the header DIFAT and its chain.
	var fatSecs []uint32
	for i := 0; i < 109 && uint32(len(fatSecs)) < nFAT; i++ {
		s := binary.LittleEndian.Uint32(h[0x4C+4*i:])
		if s == freeSect {
			break
		}
		fatSecs = append(fatSecs, s)
	}
	per := f.secSize/4 - 1
	sec := difatStart
	for n := uint32(0); n < nDIFAT && sec < endOfChain-2 && uint32(len(fatSecs)) < nFAT; n++ {
		b, err := f.sector(sec)
		if err != nil {
			return nil, err
		}
		for i := 0; i < per && uint32(len(fatSecs)) < nFAT; i++ {
			s := binary.LittleEndian.Uint32(b[4*i:])
			if s == freeSect {
				continue
			}
			fatSecs = append(fatSecs, s)
		}
		sec = binary.LittleEndian.Uint32(b[4*per:])
	}
	if len(fatSecs) == 0 {
		return nil, errors.New("cfb: no FAT")
	}
	maxSecs := int(size/int64(f.secSize)) + 1
	for _, s := range fatSecs {
		b, err := f.sector(s)
		if err != nil {
			return nil, err
		}
		for i := 0; i < f.secSize/4; i++ {
			f.fat = append(f.fat, binary.LittleEndian.Uint32(b[4*i:]))
		}
		if len(f.fat) > maxSecs*2+1024 {
			break
		}
	}

	// Directory.
	dirData, err := f.chain(dirStart, -1)
	if err != nil {
		return nil, err
	}
	for off := 0; off+128 <= len(dirData); off += 128 {
		e := dirData[off : off+128]
		nlen := int(binary.LittleEndian.Uint16(e[64:]))
		typ := e[66]
		ent := &Entry{}
		if nlen >= 2 && nlen <= 64 {
			u := make([]uint16, nlen/2-1)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(e[2*i:])
			}
			ent.RawName = string(utf16.Decode(u))
		}
		ent.IsStream = typ == 2
		copy(ent.CLSID[:], e[80:96])
		ent.start = binary.LittleEndian.Uint32(e[116:])
		ent.Size = int64(binary.LittleEndian.Uint64(e[120:]))
		if f.secSize == 512 {
			ent.Size &= 0xFFFFFFFF
		}
		if typ == 0 {
			ent.RawName = ""
		}
		ent.Name = DisplayName(ent.RawName)
		f.dir = append(f.dir, ent)
	}
	if len(f.dir) == 0 {
		return nil, errors.New("cfb: empty directory")
	}
	f.linkTree(dirData)
	f.miniStart = f.dir[0].start
	f.miniLen = f.dir[0].Size

	if miniFATStart < endOfChain-2 {
		mf, err := f.chain(miniFATStart, -1)
		if err == nil {
			for i := 0; i+4 <= len(mf); i += 4 {
				f.miniFAT = append(f.miniFAT, binary.LittleEndian.Uint32(mf[i:]))
			}
		}
	}
	return f, nil
}

// linkTree walks the red-black sibling trees and assigns paths.
func (f *File) linkTree(dirData []byte) {
	n := len(f.dir)
	link := func(i int, at int) uint32 { return binary.LittleEndian.Uint32(dirData[i*128+at:]) }
	seen := make([]bool, n)
	var siblings func(i uint32, out *[]int, depth int)
	siblings = func(i uint32, out *[]int, depth int) {
		if i == noStream || int(i) >= n || seen[i] || depth > 64 {
			return
		}
		seen[i] = true
		siblings(link(int(i), 68), out, depth+1)
		*out = append(*out, int(i))
		siblings(link(int(i), 72), out, depth+1)
	}
	var walk func(i int, prefix string, depth int)
	walk = func(i int, prefix string, depth int) {
		if depth > 32 {
			return
		}
		var kids []int
		siblings(link(i, 76), &kids, 0)
		f.dir[i].children = kids
		for _, k := range kids {
			p := f.dir[k].Name
			if prefix != "" {
				p = prefix + "/" + p
			}
			f.dir[k].Path = p
			if !f.dir[k].IsStream {
				walk(k, p, depth+1)
			}
		}
	}
	seen[0] = true
	walk(0, "", 0)
}

func (f *File) sector(n uint32) ([]byte, error) {
	off := int64(n+1) * int64(f.secSize)
	if off+int64(f.secSize) > f.size+int64(f.secSize) {
		return nil, fmt.Errorf("cfb: sector %d out of range", n)
	}
	b := make([]byte, f.secSize)
	k, err := f.r.ReadAt(b, off)
	if k == 0 && err != nil {
		return nil, err
	}
	return b, nil
}

// chain reads a FAT sector chain; limit < 0 reads it all.
func (f *File) chain(start uint32, limit int64) ([]byte, error) {
	var out []byte
	seen := map[uint32]bool{}
	for s := start; s < endOfChain-2; {
		if seen[s] {
			break
		}
		seen[s] = true
		b, err := f.sector(s)
		if err != nil {
			return out, err
		}
		out = append(out, b...)
		if limit >= 0 && int64(len(out)) >= limit {
			return out[:limit], nil
		}
		if int(s) >= len(f.fat) {
			break
		}
		s = f.fat[s]
	}
	if limit >= 0 && int64(len(out)) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *File) miniChain(start uint32, size int64) ([]byte, error) {
	if !f.loaded {
		f.loaded = true
		m, err := f.chain(f.miniStart, f.miniLen)
		if err != nil {
			return nil, err
		}
		f.ministrm = m
	}
	out := make([]byte, 0, size)
	seen := map[uint32]bool{}
	for s := start; s < endOfChain-2 && int64(len(out)) < size; {
		if seen[s] {
			break
		}
		seen[s] = true
		off := int(s) * f.miniSize
		if off+f.miniSize > len(f.ministrm) {
			break
		}
		out = append(out, f.ministrm[off:off+f.miniSize]...)
		if int(s) >= len(f.miniFAT) {
			break
		}
		s = f.miniFAT[s]
	}
	if int64(len(out)) > size {
		out = out[:size]
	}
	return out, nil
}

// Entries returns every storage and stream except the root.
func (f *File) Entries() []*Entry {
	var out []*Entry
	for _, e := range f.dir[1:] {
		if e.Path != "" {
			out = append(out, e)
		}
	}
	return out
}

// Root returns the root entry.
func (f *File) Root() *Entry { return f.dir[0] }

// Children returns the direct children of e (nil for the root's).
func (f *File) Children(e *Entry) []*Entry {
	var out []*Entry
	for _, k := range e.children {
		out = append(out, f.dir[k])
	}
	return out
}

// Read returns a stream's contents.
func (f *File) Read(e *Entry) ([]byte, error) {
	if !e.IsStream {
		return nil, errors.New("cfb: not a stream")
	}
	if e.Size == 0 {
		return nil, nil
	}
	if e.Size < f.cutoff {
		return f.miniChain(e.start, e.Size)
	}
	return f.chain(e.start, e.Size)
}

// Find returns the first entry whose display or raw name matches.
func (f *File) Find(name string) *Entry {
	for _, e := range f.dir[1:] {
		if strings.EqualFold(e.RawName, name) || strings.EqualFold(e.Name, name) {
			return e
		}
	}
	return nil
}

const msiAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz._"

// DisplayName makes a stored name presentable: MSI-compressed names
// are decoded and control characters (\x05SummaryInformation) shown
// as a bracketed number.
func DisplayName(raw string) string {
	var b strings.Builder
	for _, c := range raw {
		switch {
		case c >= 0x3800 && c < 0x4800:
			c -= 0x3800
			b.WriteByte(msiAlphabet[c&0x3F])
			b.WriteByte(msiAlphabet[(c>>6)&0x3F])
		case c >= 0x4800 && c < 0x4840:
			b.WriteByte(msiAlphabet[c-0x4800])
		case c == 0x4840:
			b.WriteByte('!')
		case c < 0x20:
			fmt.Fprintf(&b, "[%d]", c)
		case c == '/' || c == '\\':
			b.WriteByte('_')
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// Ole10Native is a file packaged into an OLE object (the classic
// "Package" embedding), as found in Office documents.
type Ole10Native struct {
	Label  string
	Source string
	Data   []byte
}

// ParseOle10Native decodes an \x01Ole10Native stream.
func ParseOle10Native(b []byte) (*Ole10Native, bool) {
	if len(b) < 10 {
		return nil, false
	}
	p := 6 // total size (4) + flags (2)
	cstr := func() (string, bool) {
		i := p
		for i < len(b) && b[i] != 0 {
			i++
		}
		if i >= len(b) {
			return "", false
		}
		s := string(b[p:i])
		p = i + 1
		return s, true
	}
	label, ok1 := cstr()
	src, ok2 := cstr()
	if !ok1 || !ok2 || p+8 > len(b) {
		return nil, false
	}
	p += 8                    // unknown + type
	if _, ok := cstr(); !ok { // temp path
		return nil, false
	}
	if p+4 > len(b) {
		return nil, false
	}
	n := int(binary.LittleEndian.Uint32(b[p:]))
	p += 4
	if n < 0 || p+n > len(b) {
		return nil, false
	}
	return &Ole10Native{Label: label, Source: src, Data: b[p : p+n]}, true
}
