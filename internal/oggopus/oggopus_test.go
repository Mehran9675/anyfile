package oggopus

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"
)

// readPackets reassembles the packets of an Ogg stream, checking every
// page's CRC.
func readPackets(t *testing.T, d []byte) [][]byte {
	t.Helper()
	var out [][]byte
	var cur []byte
	for len(d) > 0 {
		if string(d[:4]) != "OggS" {
			t.Fatalf("lost page sync")
		}
		n := int(d[26])
		lacing := d[27 : 27+n]
		size := 27 + n
		for _, l := range lacing {
			size += int(l)
		}
		page := append([]byte(nil), d[:size]...)
		want := binary.LittleEndian.Uint32(page[22:])
		binary.LittleEndian.PutUint32(page[22:], 0)
		if crc(page) != want {
			t.Fatalf("bad page CRC")
		}
		body := d[27+n : size]
		for _, l := range lacing {
			cur = append(cur, body[:l]...)
			body = body[l:]
			if l < 255 {
				out = append(out, cur)
				cur = nil
			}
		}
		d = d[size:]
	}
	return out
}

func TestRoundTripIncludingHugePacket(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	var packets [][]byte
	for _, n := range []int{300, 70000, 1, 510, 255, 4000} {
		p := make([]byte, n)
		r.Read(p)
		p[0] = 0xF8 // CELT fullband 20 ms, mono, one frame
		packets = append(packets, p)
	}
	var buf bytes.Buffer
	if err := Write(&buf, packets, 7); err != nil {
		t.Fatal(err)
	}
	got := readPackets(t, buf.Bytes())
	if len(got) != len(packets)+2 {
		t.Fatalf("got %d packets, want %d", len(got), len(packets)+2)
	}
	for i, p := range packets {
		if !bytes.Equal(got[i+2], p) {
			t.Fatalf("packet %d differs", i)
		}
	}
	if got[0][9] != 1 {
		t.Fatalf("channels = %d, want 1", got[0][9])
	}
}

func TestLayoutMultistream(t *testing.T) {
	// Two coupled streams: a self-delimited packet then a normal one.
	var packets [][]byte
	for i := 0; i < 20; i++ {
		a := append([]byte{0xFC, 10}, make([]byte, 10)...) // stereo, code 0, 10-byte frame
		b := append([]byte{0xFC}, make([]byte, 7+i)...)
		packets = append(packets, append(a, b...))
	}
	s, c := Layout(packets)
	if s != 2 || c != 2 {
		t.Fatalf("layout = %d streams, %d coupled; want 2, 2", s, c)
	}
}
