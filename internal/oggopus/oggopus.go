// Package oggopus wraps raw Opus packets in an Ogg container, producing
// a standard .opus file any player can open. Games often store Opus
// without a container; this is how AnyFile makes those streams viewable.
package oggopus

import (
	"encoding/binary"
	"errors"
	"io"
)

// Samples returns the number of 48 kHz samples in one Opus packet,
// from its TOC byte (RFC 6716, section 3.1).
func Samples(p []byte) int {
	if len(p) == 0 {
		return 0
	}
	cfg := p[0] >> 3
	var frame int // in 48 kHz samples
	switch {
	case cfg < 12: // SILK: 10, 20, 40, 60 ms
		frame = []int{480, 960, 1920, 2880}[cfg&3]
	case cfg < 16: // Hybrid: 10, 20 ms
		frame = []int{480, 960}[cfg&1]
	default: // CELT: 2.5, 5, 10, 20 ms
		frame = []int{120, 240, 480, 960}[cfg&3]
	}
	frames := 1
	switch p[0] & 3 {
	case 1, 2:
		frames = 2
	case 3:
		if len(p) < 2 {
			return 0
		}
		frames = int(p[1] & 0x3F)
	}
	return frame * frames
}

// Channels reports 1 or 2 from the TOC stereo flag.
func Channels(p []byte) int {
	if len(p) > 0 && p[0]&0x04 != 0 {
		return 2
	}
	return 1
}

// ValidTOC reports whether p looks like a well-formed Opus packet.
func ValidTOC(p []byte) bool {
	if len(p) == 0 {
		return false
	}
	if p[0]&3 == 3 {
		return len(p) >= 2 && p[1]&0x3F != 0
	}
	return true
}

const preSkip = 312 // the encoder delay libopus reports for 48 kHz

// Write muxes packets into an Ogg Opus stream on w.
func Write(w io.Writer, packets [][]byte, serial uint32) error {
	if len(packets) == 0 {
		return errors.New("oggopus: no packets")
	}
	streams, coupled := Layout(packets)
	ch := streams + coupled
	head := make([]byte, 19)
	copy(head, "OpusHead")
	head[8] = 1 // version
	head[9] = byte(ch)
	binary.LittleEndian.PutUint16(head[10:], preSkip)
	binary.LittleEndian.PutUint32(head[12:], 48000)
	if streams > 1 {
		// Family 1 covers up to 8 channels, 255 any number of discrete
		// ones. The mapping is the identity: coupled streams' left/right
		// first, then the mono streams.
		head[18] = 1
		if ch > 8 {
			head[18] = 255
		}
		head = append(head, byte(streams), byte(coupled))
		for i := 0; i < ch; i++ {
			head = append(head, byte(i))
		}
	}
	tags := []byte("OpusTags")
	vendor := "AnyFile"
	tags = binary.LittleEndian.AppendUint32(tags, uint32(len(vendor)))
	tags = append(tags, vendor...)
	tags = binary.LittleEndian.AppendUint32(tags, 0)

	pw := &pageWriter{w: w, serial: serial}
	if err := pw.single(head, 0x02); err != nil {
		return err
	}
	if err := pw.single(tags, 0); err != nil {
		return err
	}
	var granule uint64
	for i, p := range packets {
		granule += uint64(Samples(p))
		if err := pw.add(p, granule, i == len(packets)-1); err != nil {
			return err
		}
	}
	return nil
}

// pageWriter packs packets into pages of at most 255 lacing segments,
// continuing a packet on the next page when it doesn't fit.
type pageWriter struct {
	w        io.Writer
	serial   uint32
	seq      uint32
	lacing   []byte
	body     []byte
	granule  uint64 // granule of the last packet completed on this page
	complete bool   // a packet ends on the current page
	cont     bool   // the current page starts with a continued packet
}

const noGranule = ^uint64(0)

func (pw *pageWriter) single(p []byte, flags byte) error {
	pw.lacing = append(pw.lacing[:0], lacingFor(len(p))...)
	pw.body = append(pw.body[:0], p...)
	return pw.flush(0, flags)
}

func lacingFor(n int) []byte {
	l := make([]byte, 0, n/255+1)
	for ; n >= 255; n -= 255 {
		l = append(l, 255)
	}
	return append(l, byte(n))
}

func (pw *pageWriter) add(p []byte, granule uint64, last bool) error {
	segs := lacingFor(len(p))
	started := false
	for len(segs) > 0 {
		room := 255 - len(pw.lacing)
		if room == 0 {
			g := noGranule
			if pw.complete {
				g = pw.granule
			}
			if err := pw.flush(g, 0); err != nil {
				return err
			}
			pw.cont = started // only a packet split across pages continues
			continue
		}
		take := min(room, len(segs))
		n := 0
		for _, s := range segs[:take] {
			n += int(s)
		}
		pw.lacing = append(pw.lacing, segs[:take]...)
		pw.body = append(pw.body, p[:n]...)
		p, segs = p[n:], segs[take:]
		started = true
	}
	pw.granule, pw.complete = granule, true
	if last {
		return pw.flush(granule, 0x04)
	}
	return nil
}

func (pw *pageWriter) flush(granule uint64, flags byte) error {
	if pw.cont {
		flags |= 0x01
	}
	h := make([]byte, 27, 27+len(pw.lacing)+len(pw.body))
	copy(h, "OggS")
	h[5] = flags
	binary.LittleEndian.PutUint64(h[6:], granule)
	binary.LittleEndian.PutUint32(h[14:], pw.serial)
	binary.LittleEndian.PutUint32(h[18:], pw.seq)
	h[26] = byte(len(pw.lacing))
	h = append(h, pw.lacing...)
	h = append(h, pw.body...)
	binary.LittleEndian.PutUint32(h[22:], crc(h))
	pw.seq++
	pw.lacing, pw.body = pw.lacing[:0], pw.body[:0]
	pw.complete, pw.cont = false, false
	_, err := pw.w.Write(h)
	return err
}

var crcTable = func() (t [256]uint32) {
	for i := range t {
		r := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if r&0x80000000 != 0 {
				r = r<<1 ^ 0x04C11DB7
			} else {
				r <<= 1
			}
		}
		t[i] = r
	}
	return
}()

// crc is Ogg's CRC-32 (polynomial 0x04C11DB7, no reflection).
func crc(b []byte) uint32 {
	var c uint32
	for _, x := range b {
		c = c<<8 ^ crcTable[byte(c>>24)^x]
	}
	return c
}
