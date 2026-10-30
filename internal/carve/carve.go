// Package carve finds files embedded anywhere inside another file by
// their signatures and works out where each one ends, the way forensic
// carvers do. It is AnyFile's last resort when no format-aware explorer
// understands a file, and a "deep scan" on demand for any file.
package carve

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"debug/elf"
	"debug/pe"
	"encoding/binary"
	"io"
	"sort"

	"github.com/xmhrnx/anyfile/internal/settings"
)

// Hit is one embedded object.
type Hit struct {
	Off    int64
	Len    int64 // bytes; exact unless Approx
	Approx bool  // the end could not be determined; Len runs to the next object
	Ext    string
	Label  string
}

type spec struct {
	magic     string
	at        int // offset of magic from the object start
	ext       string
	label     string
	container bool // compressed or structured payload: don't look inside
	extent    func(r io.ReaderAt, off, size int64) (int64, bool)
}

var specs = []spec{
	{"\xFF\xD8\xFF", 0, "jpg", "JPEG image", false, jpegExtent},
	{"\x89PNG\r\n\x1A\n", 0, "png", "PNG image", true, pngExtent},
	{"GIF89a", 0, "gif", "GIF image", true, gifExtent},
	{"GIF87a", 0, "gif", "GIF image", true, gifExtent},
	{"BM", 0, "bmp", "BMP image", true, bmpExtent},
	{"%PDF-", 0, "pdf", "PDF document", true, pdfExtent},
	{"PK\x03\x04", 0, "zip", "ZIP archive", true, zipExtent},
	{"7z\xBC\xAF\x27\x1C", 0, "7z", "7-Zip archive", true, sevenZipExtent},
	{"Rar!\x1A\x07", 0, "rar", "RAR archive", true, nil},
	{"MSCF\x00\x00\x00\x00", 0, "cab", "Cabinet archive", true, cabExtent},
	{"\x1F\x8B\x08", 0, "gz", "GZIP data", true, gzipExtent},
	{"BZh91AY&SY", 0, "bz2", "BZIP2 data", true, nil},
	{"\xFD7zXZ\x00", 0, "xz", "XZ data", true, nil},
	{"\x28\xB5\x2F\xFD", 0, "zst", "Zstandard data", true, nil},
	{"\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1", 0, "ole", "OLE compound file", true, nil},
	{"MZ", 0, "exe", "Windows executable", false, peExtent},
	{"\x7FELF", 0, "elf", "ELF executable", false, elfExtent},
	{"RIFF", 0, "riff", "RIFF media", true, riffExtent},
	{"OggS\x00\x02", 0, "ogg", "Ogg media", true, oggExtent},
	{"fLaC", 0, "flac", "FLAC audio", true, nil},
	{"ftyp", 4, "mp4", "MPEG-4 media", true, mp4Extent},
	{"SQLite format 3\x00", 0, "sqlite", "SQLite database", true, sqliteExtent},
	{"wOFF", 0, "woff", "WOFF font", true, woffExtent},
	{"wOF2", 0, "woff2", "WOFF2 font", true, woffExtent},
	{"OTTO", 0, "otf", "OpenType font", true, sfntExtent},
	{"\x00\x01\x00\x00\x00", 0, "ttf", "TrueType font", true, sfntExtent},
	{"FWS", 0, "swf", "Flash movie", true, swfExtent},
	{"CWS", 0, "swf", "Flash movie", true, swfExtent},
	{"-----BEGIN ", 0, "pem", "PEM block", false, pemExtent},
	{"\x00\x00\x01\x00", 0, "ico", "Windows icon", true, icoExtent},
}

// Options bound a scan.
type Options struct {
	Limit    int   // maximum hits (default 500)
	SkipSelf bool  // drop an object at offset 0 spanning the whole input
	MinSize  int64 // smallest object to report (default 48)
}

type cand struct {
	off int64
	s   *spec
}

// Scan searches r for embedded objects.
func Scan(r io.ReaderAt, size int64, opt Options) []Hit {
	if opt.Limit == 0 {
		opt.Limit = 500
	}
	if opt.MinSize == 0 {
		opt.MinSize = 48
	}
	const chunk = 8 << 20
	const overlap = 32
	buf := make([]byte, chunk+overlap)
	var cands []cand
	for base := int64(0); base < size; base += chunk {
		n, _ := r.ReadAt(buf[:min(int64(len(buf)), size-base)], base)
		b := buf[:n]
		for i := range specs {
			s := &specs[i]
			m := []byte(s.magic)
			for j := 0; ; {
				k := bytes.Index(b[j:], m)
				if k < 0 {
					break
				}
				pos := int64(j + k)
				j += k + 1
				if pos >= chunk && base+chunk < size {
					break // the next chunk sees it
				}
				start := base + pos - int64(s.at)
				if start < 0 {
					continue
				}
				cands = append(cands, cand{start, s})
			}
		}
		if len(cands) > opt.Limit*50 {
			break
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].off < cands[j].off })

	var hits []Hit
	var covered int64 // end of the last container: hits inside it are noise
	for _, c := range cands {
		if c.off < covered {
			continue
		}
		var n int64 = -1
		if c.s.extent != nil {
			l, ok := c.s.extent(r, c.off, size)
			if !ok {
				continue
			}
			n = l
		} else if !plausible(r, c) {
			continue
		}
		h := Hit{Off: c.off, Len: n, Ext: c.s.ext, Label: c.s.label}
		if n < 0 {
			h.Approx = true
		} else if n < opt.MinSize && c.s.ext != "pem" {
			continue
		}
		// Only an object with a known end hides what lies inside it; an
		// open-ended one must not swallow the rest of the file.
		if c.s.container && n > 0 {
			covered = c.off + n
		}
		if len(hits) > 0 && hits[len(hits)-1].Off == h.Off {
			continue
		}
		hits = append(hits, h)
		if len(hits) >= opt.Limit {
			break
		}
	}
	// Close open-ended objects at the next object or the end.
	for i := range hits {
		if hits[i].Len < 0 {
			end := size
			if i+1 < len(hits) {
				end = hits[i+1].Off
			}
			hits[i].Len = end - hits[i].Off
		}
		if hits[i].Off+hits[i].Len > size {
			hits[i].Len = size - hits[i].Off
		}
	}
	if opt.SkipSelf && len(hits) > 0 && hits[0].Off == 0 && hits[0].Len >= size-16 {
		hits = hits[1:]
	}
	return hits
}

// plausible guards signatures with no length logic against noise.
func plausible(r io.ReaderAt, c cand) bool {
	h := make([]byte, 0x40)
	n, _ := r.ReadAt(h, c.off)
	h = h[:n]
	switch c.s.ext {
	case "rar":
		return n >= 8 && (h[6] == 0 || h[6] == 1)
	case "ole":
		return n >= 0x20 && (h[0x1E] == 9 || h[0x1E] == 12) && h[0x18] == 0x3E
	case "flac":
		// The first metadata block must be a 34-byte STREAMINFO.
		return n >= 8 && h[4]&0x7F == 0 && h[5] == 0 && h[6] == 0 && h[7] == 34
	case "zst":
		// Frame header descriptor: the reserved bit is always clear.
		return n >= 5 && h[4]&0x08 == 0
	case "xz":
		// Stream flags: a zero byte, a known check type, then CRC32.
		return n >= 12 && h[6] == 0 && (h[7] == 0 || h[7] == 1 || h[7] == 4 || h[7] == 10)
	}
	return true
}

func readAt(r io.ReaderAt, off int64, n int) []byte {
	b := make([]byte, n)
	k, _ := r.ReadAt(b, off)
	return b[:k]
}

func jpegExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	br := bufio.NewReaderSize(io.NewSectionReader(r, off, size-off), 64<<10)
	var pos int64
	next := func() (byte, bool) {
		c, err := br.ReadByte()
		pos++
		return c, err == nil
	}
	skip := func(n int64) bool {
		k, err := br.Discard(int(n))
		pos += int64(k)
		return err == nil
	}
	if !skip(2) {
		return 0, false
	}
	first := true
	for {
		c, ok := next()
		if !ok || c != 0xFF {
			return 0, false
		}
		m, ok := next()
		for ok && m == 0xFF {
			m, ok = next()
		}
		if !ok {
			return 0, false
		}
		if first {
			// APPn, DQT, SOF, DHT or COM must open a real JPEG.
			if !(m >= 0xE0 && m <= 0xEF || m == 0xDB || m == 0xC0 || m == 0xC2 || m == 0xC4 || m == 0xFE) {
				return 0, false
			}
			first = false
		}
		switch {
		case m == 0xD9:
			return pos, true
		case m == 0x01 || m >= 0xD0 && m <= 0xD7:
			continue
		}
		hi, ok1 := next()
		lo, ok2 := next()
		if !ok1 || !ok2 {
			return 0, false
		}
		l := int64(hi)<<8 | int64(lo)
		if l < 2 || !skip(l-2) {
			return 0, false
		}
		if m != 0xDA {
			continue
		}
		// Entropy-coded data runs until a marker other than a stuffed
		// zero or a restart marker.
		for {
			c, ok := next()
			if !ok {
				return 0, false
			}
			if c != 0xFF {
				continue
			}
			d, ok := next()
			if !ok {
				return 0, false
			}
			for d == 0xFF {
				if d, ok = next(); !ok {
					return 0, false
				}
			}
			if d == 0 || d >= 0xD0 && d <= 0xD7 {
				continue
			}
			if d == 0xD9 {
				return pos, true
			}
			// Another segment (progressive scans): rewind into the loop.
			hi, ok1 := next()
			lo, ok2 := next()
			if !ok1 || !ok2 {
				return 0, false
			}
			l := int64(hi)<<8 | int64(lo)
			if l < 2 || !skip(l-2) {
				return 0, false
			}
			if d != 0xDA {
				break
			}
		}
	}
}

func pngExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	p := off + 8
	for i := 0; i < 1<<20; i++ {
		h := readAt(r, p, 8)
		if len(h) < 8 {
			return 0, false
		}
		n := int64(binary.BigEndian.Uint32(h))
		for _, c := range h[4:8] {
			if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
				return 0, false
			}
		}
		p += 12 + n
		if p > size {
			return 0, false
		}
		if string(h[4:8]) == "IEND" {
			return p - off, true
		}
	}
	return 0, false
}

func gifExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	br := bufio.NewReader(io.NewSectionReader(r, off, size-off))
	var pos int64
	read := func(n int) []byte {
		b := make([]byte, n)
		k, _ := io.ReadFull(br, b)
		pos += int64(k)
		if k < n {
			return nil
		}
		return b
	}
	subBlocks := func() bool {
		for {
			l := read(1)
			if l == nil {
				return false
			}
			if l[0] == 0 {
				return true
			}
			if read(int(l[0])) == nil {
				return false
			}
		}
	}
	h := read(13)
	if h == nil {
		return 0, false
	}
	if h[10]&0x80 != 0 && read(3*(1<<(h[10]&7+1))) == nil {
		return 0, false
	}
	for {
		t := read(1)
		if t == nil {
			return 0, false
		}
		switch t[0] {
		case 0x3B:
			return pos, true
		case 0x21:
			if read(1) == nil || !subBlocks() {
				return 0, false
			}
		case 0x2C:
			d := read(9)
			if d == nil {
				return 0, false
			}
			if d[8]&0x80 != 0 && read(3*(1<<(d[8]&7+1))) == nil {
				return 0, false
			}
			if read(1) == nil || !subBlocks() {
				return 0, false
			}
		default:
			return 0, false
		}
	}
}

func bmpExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 30)
	if len(h) < 30 {
		return 0, false
	}
	n := int64(binary.LittleEndian.Uint32(h[2:]))
	data := binary.LittleEndian.Uint32(h[10:])
	dib := binary.LittleEndian.Uint32(h[14:])
	planes := binary.LittleEndian.Uint16(h[26:])
	bpp := binary.LittleEndian.Uint16(h[28:])
	switch dib {
	case 40, 52, 56, 108, 124:
	default:
		return 0, false
	}
	if binary.LittleEndian.Uint32(h[6:]) != 0 || planes != 1 || int64(data) >= n || n > size-off || n < 64 {
		return 0, false
	}
	switch bpp {
	case 1, 4, 8, 16, 24, 32:
	default:
		return 0, false
	}
	return n, true
}

func pdfExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	var last int64 = -1
	const step = 1 << 20
	for p := off + 5; p < size; p += step - 8 {
		b := readAt(r, p, step)
		if i := bytes.Index(b, []byte("%PDF-")); i >= 0 {
			b = b[:i]
			if j := bytes.LastIndex(b, []byte("%%EOF")); j >= 0 {
				last = p + int64(j) + 5
			}
			break
		}
		if j := bytes.LastIndex(b, []byte("%%EOF")); j >= 0 {
			last = p + int64(j) + 5
		}
		if len(b) < step {
			break
		}
	}
	if last < 0 {
		return -1, true
	}
	// Keep the end-of-line after %%EOF.
	t := readAt(r, last, 2)
	for _, c := range t {
		if c == '\r' || c == '\n' {
			last++
		} else {
			break
		}
	}
	return last - off, true
}

func zipExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 30)
	if len(h) < 30 {
		return 0, false
	}
	nameLen := binary.LittleEndian.Uint16(h[26:])
	if ver := binary.LittleEndian.Uint16(h[4:]); ver > 63 || nameLen == 0 || nameLen > 1024 {
		return 0, false
	}
	// The right end-of-central-directory record is the one whose
	// directory ends exactly where it starts; earlier ones may belong to
	// archives stored inside this one.
	const step = 1 << 20
	var first int64 = -1
	for p := off; p < size; p += step - 22 {
		b := readAt(r, p, step)
		for j := 0; ; {
			i := bytes.Index(b[j:], []byte("PK\x05\x06"))
			if i < 0 {
				break
			}
			at := p + int64(j+i)
			j += i + 1
			e := readAt(r, at, 22)
			if len(e) < 22 {
				continue
			}
			end := at + 22 + int64(binary.LittleEndian.Uint16(e[20:]))
			if first < 0 {
				first = end
			}
			cdSize := int64(binary.LittleEndian.Uint32(e[12:]))
			cdOff := int64(binary.LittleEndian.Uint32(e[16:]))
			if off+cdOff+cdSize == at || cdOff == 0xFFFFFFFF {
				return end - off, true
			}
		}
		if len(b) < step {
			break
		}
	}
	if first > 0 {
		return first - off, true
	}
	return -1, true
}

func sevenZipExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 32)
	if len(h) < 32 {
		return 0, false
	}
	n := 32 + int64(binary.LittleEndian.Uint64(h[12:])) + int64(binary.LittleEndian.Uint64(h[20:]))
	if n <= 32 || n > size-off {
		return -1, true
	}
	return n, true
}

func cabExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 12)
	if len(h) < 12 {
		return 0, false
	}
	n := int64(binary.LittleEndian.Uint32(h[8:]))
	if n < 36 || n > size-off {
		return 0, false
	}
	return n, true
}

// countingReader feeds compress/flate a ByteReader so the gzip reader
// doesn't buffer ahead, making the consumed count exact.
type countingReader struct {
	r *bufio.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	// Byte-at-a-time keeps the count exact; flate prefers ReadByte anyway.
	if len(p) == 0 {
		return 0, nil
	}
	b, err := c.r.ReadByte()
	if err != nil {
		return 0, err
	}
	p[0] = b
	c.n++
	return 1, nil
}

func (c *countingReader) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.n++
	}
	return b, err
}

func gzipExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 10)
	if len(h) < 10 || h[3]&0xE0 != 0 {
		return 0, false
	}
	cr := &countingReader{r: bufio.NewReaderSize(io.NewSectionReader(r, off, size-off), 64<<10)}
	zr, err := gzip.NewReader(cr)
	if err != nil {
		return 0, false
	}
	zr.Multistream(false)
	limit := settings.MB(settings.Get().MaxDecompressMB)
	n, err := io.Copy(io.Discard, io.LimitReader(zr, limit))
	if err != nil || n == 0 {
		return 0, false
	}
	if n >= limit {
		return -1, true
	}
	return cr.n, true
}

func peExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 0x40)
	if len(h) < 0x40 {
		return 0, false
	}
	lfanew := int64(binary.LittleEndian.Uint32(h[0x3C:]))
	if lfanew < 0x40 || lfanew > 4096 {
		return 0, false
	}
	if string(readAt(r, off+lfanew, 4)) != "PE\x00\x00" {
		return 0, false
	}
	f, err := pe.NewFile(io.NewSectionReader(r, off, size-off))
	if err != nil {
		return 0, false
	}
	var end int64
	for _, s := range f.Sections {
		if e := int64(s.Offset) + int64(s.Size); e > end {
			end = e
		}
	}
	var dirs []pe.DataDirectory
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		dirs = oh.DataDirectory[:oh.NumberOfRvaAndSizes]
	case *pe.OptionalHeader64:
		dirs = oh.DataDirectory[:oh.NumberOfRvaAndSizes]
	}
	if len(dirs) > 4 && dirs[4].VirtualAddress != 0 {
		if e := int64(dirs[4].VirtualAddress) + int64(dirs[4].Size); e > end && e <= size-off {
			end = e
		}
	}
	if end == 0 || end > size-off {
		return 0, false
	}
	return end, true
}

func elfExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	f, err := elf.NewFile(io.NewSectionReader(r, off, size-off))
	if err != nil {
		return 0, false
	}
	var end int64
	for _, s := range f.Sections {
		if s.Type != elf.SHT_NOBITS {
			if e := int64(s.Offset + s.FileSize); e > end {
				end = e
			}
		}
	}
	for _, p := range f.Progs {
		if e := int64(p.Off + p.Filesz); e > end {
			end = e
		}
	}
	h := readAt(r, off, 64)
	if len(h) >= 64 {
		if h[4] == 2 { // 64-bit
			shoff := int64(f.ByteOrder.Uint64(h[0x28:]))
			e := shoff + int64(f.ByteOrder.Uint16(h[0x3A:]))*int64(f.ByteOrder.Uint16(h[0x3C:]))
			end = max(end, e)
		} else {
			shoff := int64(f.ByteOrder.Uint32(h[0x20:]))
			e := shoff + int64(f.ByteOrder.Uint16(h[0x2E:]))*int64(f.ByteOrder.Uint16(h[0x30:]))
			end = max(end, e)
		}
	}
	if end <= 0 || end > size-off {
		return 0, false
	}
	return end, true
}

func riffExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 12)
	if len(h) < 12 {
		return 0, false
	}
	for _, c := range h[8:12] {
		if c < 0x20 || c > 0x7E {
			return 0, false
		}
	}
	n := int64(binary.LittleEndian.Uint32(h[4:])) + 8
	if n < 12 || n > size-off {
		return 0, false
	}
	return n, true
}

func oggExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	p := off
	for {
		h := readAt(r, p, 27)
		if len(h) < 27 || string(h[:4]) != "OggS" {
			break
		}
		segs := readAt(r, p+27, int(h[26]))
		if len(segs) < int(h[26]) {
			break
		}
		n := int64(27 + len(segs))
		for _, s := range segs {
			n += int64(s)
		}
		p += n
		if p >= size {
			p = size
			break
		}
	}
	if p == off {
		return 0, false
	}
	return p - off, true
}

func mp4Extent(r io.ReaderAt, off, size int64) (int64, bool) {
	p := off
	for p+8 <= size {
		h := readAt(r, p, 16)
		if len(h) < 8 {
			break
		}
		n := int64(binary.BigEndian.Uint32(h))
		for _, c := range h[4:8] {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == ' ' || c == '-') {
				goto done
			}
		}
		switch n {
		case 0:
			n = size - p
		case 1:
			if len(h) < 16 {
				goto done
			}
			n = int64(binary.BigEndian.Uint64(h[8:]))
		}
		if n < 8 || p+n > size {
			goto done
		}
		p += n
	}
done:
	if p-off < 16 {
		return 0, false
	}
	return p - off, true
}

func sqliteExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 100)
	if len(h) < 100 {
		return 0, false
	}
	ps := int64(binary.BigEndian.Uint16(h[16:]))
	if ps == 1 {
		ps = 65536
	}
	pages := int64(binary.BigEndian.Uint32(h[28:]))
	if ps < 512 || ps&(ps-1) != 0 {
		return 0, false
	}
	if pages == 0 || ps*pages > size-off {
		return -1, true
	}
	return ps * pages, true
}

func woffExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 12)
	if len(h) < 12 {
		return 0, false
	}
	n := int64(binary.BigEndian.Uint32(h[8:]))
	if n < 44 || n > size-off {
		return 0, false
	}
	return n, true
}

func sfntExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 12)
	if len(h) < 12 {
		return 0, false
	}
	n := int(binary.BigEndian.Uint16(h[4:]))
	if n < 4 || n > 64 {
		return 0, false
	}
	dir := readAt(r, off+12, 16*n)
	if len(dir) < 16*n {
		return 0, false
	}
	var end int64
	for i := 0; i < n; i++ {
		e := dir[16*i : 16*i+16]
		for _, c := range e[:4] {
			if c < 0x20 || c > 0x7E {
				return 0, false
			}
		}
		t := int64(binary.BigEndian.Uint32(e[8:])) + int64(binary.BigEndian.Uint32(e[12:]))
		end = max(end, t)
	}
	if end > size-off || end < 64 {
		return 0, false
	}
	return end, true
}

// swfExtent validates SWF headers. A three-letter signature turns up
// by chance in any large binary, so the movie header itself must make
// sense: a version Flash actually used, a sane length and, for
// zlib-compressed movies, a stream that really inflates.
func swfExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 10)
	if len(h) < 10 || h[3] < 1 || h[3] > 45 {
		return 0, false
	}
	n := int64(binary.LittleEndian.Uint32(h[4:]))
	if n < 21 || n > 1<<30 {
		return 0, false
	}
	if h[0] == 'C' {
		if h[8]&0x0F != 8 || (uint16(h[8])<<8|uint16(h[9]))%31 != 0 {
			return 0, false
		}
		zr, err := zlib.NewReader(io.NewSectionReader(r, off+8, size-off-8))
		if err != nil {
			return 0, false
		}
		body := make([]byte, 17)
		if _, err := io.ReadFull(zr, body); err != nil || !validRect(body) {
			return 0, false
		}
		return -1, true // the length is the uncompressed size
	}
	if n > size-off || !validRect(readAt(r, off+8, 17)) {
		return 0, false
	}
	return n, true
}

// validRect checks the stage rectangle that opens every SWF body: the
// field width, then xmin/ymin at zero and a plausible size.
func validRect(b []byte) bool {
	if len(b) < 2 {
		return false
	}
	nbits := int(b[0] >> 3)
	if nbits < 2 || nbits > 31 || (5+4*nbits+7)/8 > len(b) {
		return false
	}
	bit := 5
	field := func() uint64 {
		var v uint64
		for i := 0; i < nbits; i++ {
			v = v<<1 | uint64(b[bit/8]>>(7-bit%8)&1)
			bit++
		}
		return v
	}
	xmin, xmax, ymin, ymax := field(), field(), field(), field()
	return xmin == 0 && ymin == 0 && xmax > 0 && ymax > 0 && xmax < 20*8192 && ymax < 20*8192
}

func pemExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	b := readAt(r, off, 64<<10)
	i := bytes.Index(b, []byte("-----END "))
	if i < 0 {
		return 0, false
	}
	j := bytes.Index(b[i+9:], []byte("-----"))
	if j < 0 {
		return 0, false
	}
	return int64(i + 9 + j + 5), true
}

func icoExtent(r io.ReaderAt, off, size int64) (int64, bool) {
	h := readAt(r, off, 6)
	if len(h) < 6 {
		return 0, false
	}
	n := int(binary.LittleEndian.Uint16(h[4:]))
	if n == 0 || n > 64 {
		return 0, false
	}
	dir := readAt(r, off+6, 16*n)
	if len(dir) < 16*n {
		return 0, false
	}
	var end int64
	for i := 0; i < n; i++ {
		e := dir[16*i:]
		if e[3] != 0 || binary.LittleEndian.Uint16(e[4:]) > 1 {
			return 0, false
		}
		sz := int64(binary.LittleEndian.Uint32(e[8:]))
		o := int64(binary.LittleEndian.Uint32(e[12:]))
		switch binary.LittleEndian.Uint16(e[6:]) { // bit count
		case 0, 1, 4, 8, 16, 24, 32:
		default:
			return 0, false
		}
		if o < int64(6+16*n) || sz < 40 || o+sz > size-off {
			return 0, false
		}
		// Each image is a PNG or a BITMAPINFOHEADER.
		img := readAt(r, off+o, 8)
		if string(img) != "\x89PNG\r\n\x1A\n" && (len(img) < 4 || binary.LittleEndian.Uint32(img) != 40) {
			return 0, false
		}
		end = max(end, o+sz)
	}
	if end > size-off {
		return 0, false
	}
	return end, true
}
