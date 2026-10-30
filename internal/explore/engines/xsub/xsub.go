// Package xsub explores Call of Duty XSUB stream packages ("KAPI"
// files): the streamed data of recent IW-engine games.
//
// Layout, as found in the files:
//
//	0x0000  "KAPI", u16 version, u16 flags, ..., u64 file size at 0x18
//	0x0788  u64 count, u64 data offset, u64 data size
//	0x07A0  u64 count, u64 index offset, u64 index size (count * 20)
//	index   count × { u64 key, u32 size<<1 | compressed, u32 offset>>7, u32 raw size }
//
// Uncompressed entries are mostly Opus audio: a table of u32 packet
// offsets (sometimes after a 32-byte header) followed by
// { u16 length, packet } records. AnyFile rewraps them as
// standard .opus files. Compressed entries are other assets: a block
// header with a table of Oodle chunks, decoded with the game's own
// oo2core library.
package xsub

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mehran9675/giode/i18n"
	"github.com/xmhrnx/anyfile/internal/detect"
	"github.com/xmhrnx/anyfile/internal/explore"
	"github.com/xmhrnx/anyfile/internal/oggopus"
	"github.com/xmhrnx/anyfile/internal/oodle"
)

func init() {
	explore.Register(explore.Engine{Name: "Call of Duty XSUB", Priority: 70, Match: func(d *detect.Result, h []byte) bool {
		return bytes.HasPrefix(h, []byte("KAPI"))
	}, List: list})
}

type entry struct {
	key        uint64
	off, size  int64
	compressed bool
	raw        int64
}

func readIndex(f *os.File, fileSize int64) ([]entry, error) {
	h := make([]byte, 0x7B8)
	if _, err := f.ReadAt(h, 0); err != nil {
		return nil, err
	}
	if string(h[:4]) != "KAPI" {
		return nil, errors.New("not an XSUB file")
	}
	count := binary.LittleEndian.Uint64(h[0x7A0:])
	ioff := int64(binary.LittleEndian.Uint64(h[0x7A8:]))
	isize := int64(binary.LittleEndian.Uint64(h[0x7B0:]))
	if count == 0 || count > 1<<24 || isize != int64(count)*20 || ioff+isize > fileSize {
		return nil, fmt.Errorf("unexpected XSUB index (count %d, offset 0x%X, size %d)", count, ioff, isize)
	}
	raw := make([]byte, isize)
	if _, err := f.ReadAt(raw, ioff); err != nil {
		return nil, err
	}
	out := make([]entry, 0, count)
	for i := int64(0); i < int64(count); i++ {
		b := raw[20*i:]
		sz := binary.LittleEndian.Uint32(b[8:])
		e := entry{
			key:        binary.LittleEndian.Uint64(b),
			size:       int64(sz >> 1),
			compressed: sz&1 != 0,
			off:        int64(binary.LittleEndian.Uint32(b[12:])) << 7,
			raw:        int64(binary.LittleEndian.Uint32(b[16:])),
		}
		if e.off+e.size > fileSize {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func list(p string, d *detect.Result) (*explore.Listing, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	st, _ := f.Stat()
	ents, err := readIndex(f, st.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	l := &explore.Listing{Engine: "XSUB (built-in)", Format: "Call of Duty XSUB stream package"}
	l.OnClose(func() { f.Close() })
	var audio, assets int
	for _, e := range ents {
		e := e
		name := fmt.Sprintf("%016x", e.key)
		if !e.compressed {
			head := make([]byte, min(e.raw, 1<<20))
			f.ReadAt(head, e.off)
			if _, n, ok := opusStream(head, e.raw); ok {
				audio++
				l.Entries = append(l.Entries, &explore.Entry{
					// The Ogg file is only built on demand; its size is
					// within a few percent of the stored stream.
					Path: "Audio/" + name + ".opus", Size: e.raw,
					Note: fmt.Sprintf("Opus · %d packets", n),
					Open: func() (io.ReadCloser, error) { return openOpus(f, e) },
				})
				continue
			}
			l.Entries = append(l.Entries, &explore.Entry{
				Path: "Raw/" + name + ".bin", Size: e.raw, Note: "stored",
				Open: explore.SectionOpener(f, e.off, e.raw),
			})
			continue
		}
		assets++
		l.Entries = append(l.Entries, &explore.Entry{
			Path: "Assets/" + name + ".bin", Size: e.raw, Note: "Oodle compressed",
			Open: func() (io.ReadCloser, error) { return openBlock(f, p, e) },
		})
	}
	if audio > 0 {
		l.Notes = append(l.Notes, i18n.Intl("{count} audio streams were stored as bare Opus packets; each is rewrapped as a standard .opus file when opened or extracted.", "count", audio))
	}
	if assets > 0 {
		if oodle.Find(p) == "" {
			l.Notes = append(l.Notes, i18n.Intl("{count} assets are Oodle compressed. {err}.", "count", assets, "err", oodle.ErrMissing))
		} else {
			l.Notes = append(l.Notes, i18n.Intl("{count} assets are Oodle compressed and are decompressed with the game's own oo2core library. Their formats are engine-internal.", "count", assets))
		}
	}
	return l, nil
}

// opusHeader is the size of the optional header some streams carry
// before their packet table: u64 0, i64 -4, u64 data size, u64 0.
const opusHeader = 0x20

// opusStream locates the packet table of an Opus entry, with or
// without the header, and returns where it starts and how many packets
// it lists.
func opusStream(b []byte, total int64) (base, n int, ok bool) {
	if n, ok := opusLayout(b, total); ok {
		return 0, n, true
	}
	if len(b) > opusHeader && binary.LittleEndian.Uint64(b) == 0 && int64(binary.LittleEndian.Uint64(b[8:])) == -4 {
		if n, ok := opusLayout(b[opusHeader:], total-opusHeader); ok {
			return opusHeader, n, true
		}
	}
	return 0, 0, false
}

// opusLayout finds where the packet offset table ends: the first
// position from which every packet's u16 length prefix matches the gap
// to the next offset. It returns the number of packets.
func opusLayout(b []byte, total int64) (int, bool) {
	if len(b) < 12 || binary.LittleEndian.Uint32(b) != 0 {
		return 0, false
	}
	off := func(i int) int64 { return int64(binary.LittleEndian.Uint32(b[4*i:])) }
	for n := 2; 4*n+3 <= len(b); n++ {
		if n > 2 && off(n-1) <= off(n-2) {
			return 0, false // the table must keep increasing until it ends
		}
		start := 4 * n
		if int64(start)+off(n-1) >= total || !packetsMatch(b, n) {
			continue
		}
		return n, true
	}
	return 0, false
}

// packetsMatch checks the table entries against the length prefixes of
// the packets that lie within b.
func packetsMatch(b []byte, n int) bool {
	start := 4 * n
	checked := 0
	for i := 0; i+1 < n; i++ {
		o := start + int(binary.LittleEndian.Uint32(b[4*i:]))
		next := start + int(binary.LittleEndian.Uint32(b[4*(i+1):]))
		if o+3 > len(b) {
			break
		}
		if int(binary.LittleEndian.Uint16(b[o:]))+2 != next-o || !oggopus.ValidTOC(b[o+2:]) {
			return false
		}
		checked++
	}
	return checked >= min(n-1, 8) && checked > 0
}

func openOpus(f *os.File, e entry) (io.ReadCloser, error) {
	b := make([]byte, e.raw)
	if _, err := f.ReadAt(b, e.off); err != nil {
		return nil, err
	}
	base, n, ok := opusStream(b, e.raw)
	if !ok {
		return nil, errors.New("not an Opus packet stream")
	}
	b = b[base:]
	data := b[4*n:]
	packets := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		o := int(binary.LittleEndian.Uint32(b[4*i:]))
		if o+2 > len(data) {
			break
		}
		l := int(binary.LittleEndian.Uint16(data[o:]))
		if o+2+l > len(data) {
			break
		}
		packets = append(packets, data[o+2:o+2+l])
	}
	var out bytes.Buffer
	if err := oggopus.Write(&out, packets, uint32(e.key)); err != nil {
		return nil, err
	}
	return io.NopCloser(&out), nil
}

// openBlock decodes a compressed entry: a header holding the key, a
// chunk count at 0x16 and 21-byte chunk descriptors
// { u8 codec, u32 packed size, u32 raw size, u32 data offset, u32 raw offset, u32 checksum }.
func openBlock(f *os.File, p string, e entry) (io.ReadCloser, error) {
	blk := make([]byte, e.size)
	if _, err := f.ReadAt(blk, e.off); err != nil {
		return nil, err
	}
	if len(blk) < 0x17 || blk[0] != 0x1D || blk[1] != 0xF0 {
		return nil, errors.New("unrecognised XSUB block header")
	}
	n := int(blk[0x16])
	out := make([]byte, e.raw)
	for i := 0; i < n; i++ {
		d := 0x17 + 21*i
		if d+21 > len(blk) {
			return nil, errors.New("truncated chunk table")
		}
		codec := blk[d]
		comp := int(binary.LittleEndian.Uint32(blk[d+1:]))
		raw := int(binary.LittleEndian.Uint32(blk[d+5:]))
		doff := int(binary.LittleEndian.Uint32(blk[d+9:]))
		roff := int(binary.LittleEndian.Uint32(blk[d+13:]))
		if doff+comp > len(blk) || roff+raw > len(out) {
			return nil, errors.New("chunk outside its block")
		}
		src := blk[doff : doff+comp]
		if codec == 0 || comp == raw {
			copy(out[roff:], src)
			continue
		}
		dec, err := oodle.Decompress(p, src, raw)
		if err != nil {
			return nil, err
		}
		copy(out[roff:], dec)
	}
	return io.NopCloser(bytes.NewReader(out)), nil
}
