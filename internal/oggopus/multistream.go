package oggopus

// Opus multistream packets (surround audio) are several packets back
// to back: all but the last use the self-delimiting framing of
// RFC 6716 Appendix B. Nothing outside the packets records how many
// streams there are, so Layout infers it from the framing itself.

// selfDelimitedLen returns the size of a self-delimited Opus packet at
// the start of p, or 0 when p does not start with one.
func selfDelimitedLen(p []byte) int {
	if len(p) < 2 {
		return 0
	}
	pos := 1
	size := func() (int, bool) {
		if pos >= len(p) {
			return 0, false
		}
		b0 := int(p[pos])
		if b0 < 252 {
			pos++
			return b0, true
		}
		if pos+1 >= len(p) {
			return 0, false
		}
		v := b0 + 4*int(p[pos+1])
		pos += 2
		return v, true
	}
	var body int
	switch p[0] & 3 {
	case 0:
		n, ok := size()
		if !ok {
			return 0
		}
		body = n
	case 1:
		n, ok := size()
		if !ok {
			return 0
		}
		body = 2 * n
	case 2:
		n1, ok1 := size()
		n2, ok2 := size()
		if !ok1 || !ok2 {
			return 0
		}
		body = n1 + n2
	case 3:
		if pos >= len(p) {
			return 0
		}
		m := p[pos]
		pos++
		count := int(m & 0x3F)
		if count == 0 {
			return 0
		}
		pad := 0
		if m&0x40 != 0 {
			for {
				if pos >= len(p) {
					return 0
				}
				b := int(p[pos])
				pos++
				if b == 255 {
					pad += 254
					continue
				}
				pad += b
				break
			}
		}
		if m&0x80 != 0 { // VBR: every frame's size is coded
			for i := 0; i < count; i++ {
				n, ok := size()
				if !ok {
					return 0
				}
				body += n
			}
		} else {
			n, ok := size()
			if !ok {
				return 0
			}
			body = count * n
		}
		body += pad
	}
	total := pos + body
	if total > len(p) {
		return 0
	}
	return total
}

// split cuts p into n streams, or returns nil if it doesn't fit: every
// sub-packet must be well formed and last the same time.
func split(p []byte, n int) [][]byte {
	out := make([][]byte, 0, n)
	rest := p
	for i := 0; i < n-1; i++ {
		l := selfDelimitedLen(rest)
		if l == 0 {
			return nil
		}
		out = append(out, rest[:l])
		rest = rest[l:]
	}
	if len(rest) == 0 || !ValidTOC(rest) {
		return nil
	}
	out = append(out, rest)
	want := Samples(out[0])
	for _, s := range out[1:] {
		if Samples(s) != want {
			return nil
		}
	}
	return out
}

// Layout infers the stream and coupled-stream counts of a packet
// sequence: the largest stream count every packet splits into cleanly.
func Layout(packets [][]byte) (streams, coupled int) {
	sample := packets
	if len(sample) > 400 {
		sample = sample[:400]
	}
	streams = 1
	// A handful of packets can split into "streams" by coincidence; only
	// trust the framing when enough packets agree.
	minPackets := 8
	if len(sample) < minPackets {
		return 1, Channels(packets[0]) - 1
	}
	for n := 2; n <= 64; n++ {
		ok := true
		for _, p := range sample {
			if split(p, n) == nil {
				ok = false
				break
			}
		}
		if ok {
			streams = n
		}
	}
	if streams == 1 {
		return 1, Channels(packets[0]) - 1
	}
	for _, s := range split(packets[0], streams) {
		if Channels(s) == 2 {
			coupled++
		}
	}
	return streams, coupled
}
