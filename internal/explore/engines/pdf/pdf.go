// Package pdf extracts what a user would call the contents of a PDF:
// attached files, images, embedded fonts, scripts and XMP metadata.
package pdf

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/mehran9675/anyfile/internal/settings"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/explore"
)

func init() {
	explore.Register(explore.Engine{Name: "PDF", Priority: 40, Match: func(d *detect.Result, h []byte) bool {
		return bytes.HasPrefix(h, []byte("%PDF-"))
	}, List: listPDF})
}

// The reader is deliberately small: it scans for objects instead of
// trusting the xref table, which also makes it tolerant of damaged
// files. It extracts what a user would call the "contents" of a PDF:
// attached files, images, embedded fonts, scripts and XMP metadata.

type pdfObj struct {
	num    int
	dict   []byte // the object's dictionary (or whole body for non-dicts)
	stream []byte // raw stream bytes, nil if none
}

var (
	reObj      = regexp.MustCompile(`(\d+)\s+(\d+)\s+obj\b`)
	reLength   = regexp.MustCompile(`/Length\s+(\d+)(\s+(\d+)\s+R)?`)
	reRef      = func(key string) *regexp.Regexp { return regexp.MustCompile(`/` + key + `\s+(\d+)\s+\d+\s+R`) }
	reFontFile = regexp.MustCompile(`/(FontFile[23]?)\s+(\d+)\s+\d+\s+R`)
	reFontName = regexp.MustCompile(`/FontName\s*/([^\s/<>\[\]()]+)`)
	reJS       = regexp.MustCompile(`/JS\s+(\d+)\s+\d+\s+R`)
	reEF       = regexp.MustCompile(`/EF\s*<<[^>]*?/(?:UF|F)\s+(\d+)\s+\d+\s+R`)
	reInt      = func(key string) *regexp.Regexp { return regexp.MustCompile(`/` + key + `\s+(\d+)`) }
	reWidth    = reInt("Width")
	reHeight   = reInt("Height")
	reBPC      = reInt("BitsPerComponent")
	rePred     = reInt("Predictor")
	reColumns  = reInt("Columns")
	reColors   = reInt("Colors")
	reN        = reInt("N")
	reFirst    = reInt("First")
)

func listPDF(p string, d *detect.Result) (*explore.Listing, error) {
	if limit := settings.Get().MaxPDFMB; d.Size > settings.MB(limit) {
		return nil, fmt.Errorf("larger than the %d MB PDF limit; raise it in Settings", limit)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	objs := scanPDF(data)
	if len(objs) == 0 {
		return nil, errors.New("no PDF objects found")
	}
	l := &explore.Listing{Engine: "PDF (built-in)"}
	if bytes.Contains(data, []byte("/Encrypt")) {
		l.Notes = append(l.Notes, "This PDF is encrypted; embedded streams cannot be decoded without its password.")
		return l, errors.New("encrypted PDF")
	}
	byNum := map[int]*pdfObj{}
	for _, o := range objs {
		byNum[o.num] = o
	}
	// Objects packed inside object streams (dictionaries only).
	for _, o := range objs {
		if o.stream == nil || !hasName(o.dict, "Type", "ObjStm") {
			continue
		}
		raw, err := pdfDecode(o.dict, o.stream)
		if err != nil {
			continue
		}
		for _, in := range unpackObjStm(o.dict, raw) {
			if _, ok := byNum[in.num]; !ok {
				byNum[in.num] = in
				objs = append(objs, in)
			}
		}
	}

	// Classify referenced streams.
	fileName := map[int]string{}
	fontKind := map[int]string{}
	fontName := map[int]string{}
	jsObj := map[int]bool{}
	for _, o := range objs {
		if m := reEF.FindSubmatch(o.dict); m != nil {
			n, _ := strconv.Atoi(string(m[1]))
			fileName[n] = filespecName(o.dict)
		}
		for _, m := range reFontFile.FindAllSubmatch(o.dict, -1) {
			n, _ := strconv.Atoi(string(m[2]))
			fontKind[n] = string(m[1])
			if fm := reFontName.FindSubmatch(o.dict); fm != nil {
				fontName[n] = string(fm[1])
			}
		}
		for _, m := range reJS.FindAllSubmatch(o.dict, -1) {
			n, _ := strconv.Atoi(string(m[1]))
			jsObj[n] = true
		}
		// Inline JavaScript strings.
		if i := bytes.Index(o.dict, []byte("/JS")); i >= 0 && !reJS.Match(o.dict) {
			if s, ok := pdfStringAt(o.dict[i+3:]); ok && len(s) > 0 {
				js := []byte(s)
				l.Entries = append(l.Entries, &explore.Entry{Path: fmt.Sprintf("Scripts/inline_%d.js", o.num), Size: int64(len(js)), Note: "JavaScript action", Open: explore.BytesOpener(js)})
			}
		}
	}

	images := 0
	for _, o := range objs {
		if o.stream == nil {
			continue
		}
		o := o
		switch {
		case hasName(o.dict, "Type", "EmbeddedFile") || fileName[o.num] != "":
			name := fileName[o.num]
			if name == "" {
				name = fmt.Sprintf("attachment_%d.bin", o.num)
			}
			l.Entries = append(l.Entries, lazyPDF("Attachments/"+explore.SafeName(name), "attached file", o, nil))
		case hasName(o.dict, "Subtype", "Image"):
			images++
			l.Entries = append(l.Entries, pdfImage(o, byNum))
		case fontKind[o.num] != "":
			ext := map[string]string{"FontFile": "pfb", "FontFile2": "ttf", "FontFile3": "cff"}[fontKind[o.num]]
			if hasName(o.dict, "Subtype", "OpenType") {
				ext = "otf"
			}
			name := fontName[o.num]
			if name == "" {
				name = fmt.Sprintf("font_%d", o.num)
			}
			l.Entries = append(l.Entries, lazyPDF("Fonts/"+explore.SafeName(name)+"."+ext, "embedded font", o, nil))
		case jsObj[o.num]:
			l.Entries = append(l.Entries, lazyPDF(fmt.Sprintf("Scripts/script_%d.js", o.num), "JavaScript", o, nil))
		case hasName(o.dict, "Type", "Metadata"):
			l.Entries = append(l.Entries, lazyPDF(fmt.Sprintf("Metadata/xmp_%d.xml", o.num), "XMP metadata", o, nil))
		}
	}
	if images > 0 {
		l.Notes = append(l.Notes, fmt.Sprintf("%d image(s) extracted as stored; JPEG and JPEG 2000 images are bit-exact.", images))
	}
	if len(l.Entries) == 0 {
		return l, errors.New("the PDF has no attachments, images or embedded fonts")
	}
	return l, nil
}

func lazyPDF(path, note string, o *pdfObj, conv func([]byte) ([]byte, error)) *explore.Entry {
	e := &explore.Entry{Path: path, Size: -1, Note: note}
	e.Open = func() (io.ReadCloser, error) {
		b, err := pdfDecode(o.dict, o.stream)
		if err != nil {
			return nil, err
		}
		if conv != nil {
			if b, err = conv(b); err != nil {
				return nil, err
			}
		}
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	return e
}

func pdfImage(o *pdfObj, byNum map[int]*pdfObj) *explore.Entry {
	filters := pdfFilters(o.dict)
	last := ""
	if len(filters) > 0 {
		last = filters[len(filters)-1]
	}
	w, _ := intOf(reWidth, o.dict)
	h, _ := intOf(reHeight, o.dict)
	note := fmt.Sprintf("%dx%d", w, h)
	base := fmt.Sprintf("Images/image_%d", o.num)
	switch last {
	case "DCTDecode":
		return lazyPDF(base+".jpg", note+" JPEG", o, nil)
	case "JPXDecode":
		return lazyPDF(base+".jp2", note+" JPEG 2000", o, nil)
	case "JBIG2Decode":
		return lazyPDF(base+".jb2", note+" JBIG2", o, nil)
	case "CCITTFaxDecode":
		return lazyPDF(base+".g4", note+" CCITT fax data", o, nil)
	}
	comps := colorComponents(o.dict, byNum)
	bpc, _ := intOf(reBPC, o.dict)
	if comps == 0 || bpc != 8 || w <= 0 || h <= 0 {
		return lazyPDF(base+".raw", note+" raw pixel data", o, nil)
	}
	return lazyPDF(base+".png", note+" (converted to PNG)", o, func(b []byte) ([]byte, error) {
		return pixelsToPNG(b, w, h, comps)
	})
}

func colorComponents(dict []byte, byNum map[int]*pdfObj) int {
	s := string(dict)
	switch {
	case strings.Contains(s, "/DeviceRGB") || strings.Contains(s, "/CalRGB"):
		return 3
	case strings.Contains(s, "/DeviceGray") || strings.Contains(s, "/CalGray"):
		return 1
	case strings.Contains(s, "/DeviceCMYK"):
		return 4
	case strings.Contains(s, "/Indexed"):
		return 0
	}
	if m := regexp.MustCompile(`/ICCBased\s+(\d+)\s+\d+\s+R`).FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		if ref := byNum[n]; ref != nil {
			if c, ok := intOf(reN, ref.dict); ok {
				return c
			}
		}
	}
	if m := reRef("ColorSpace").FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		if ref := byNum[n]; ref != nil && ref.num != 0 {
			return colorComponents(ref.dict, map[int]*pdfObj{})
		}
	}
	return 0
}

func pixelsToPNG(b []byte, w, h, comps int) ([]byte, error) {
	if len(b) < w*h*comps {
		return nil, errors.New("image data is shorter than its dimensions")
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * comps
			var c color.NRGBA
			switch comps {
			case 1:
				c = color.NRGBA{b[i], b[i], b[i], 255}
			case 3:
				c = color.NRGBA{b[i], b[i+1], b[i+2], 255}
			case 4:
				k := 255 - int(b[i+3])
				c = color.NRGBA{uint8((255 - int(b[i])) * k / 255), uint8((255 - int(b[i+1])) * k / 255), uint8((255 - int(b[i+2])) * k / 255), 255}
			default:
				return nil, errors.New("unsupported colour space")
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// scanPDF finds every "N G obj" in the file.
func scanPDF(data []byte) []*pdfObj {
	var out []*pdfObj
	locs := reObj.FindAllSubmatchIndex(data, -1)
	for _, m := range locs {
		num, _ := strconv.Atoi(string(data[m[2]:m[3]]))
		p := skipWS(data, m[1])
		o := &pdfObj{num: num}
		if bytes.HasPrefix(data[p:], []byte("<<")) {
			end := dictEnd(data, p)
			if end < 0 {
				continue
			}
			o.dict = data[p:end]
			q := skipWS(data, end)
			if bytes.HasPrefix(data[q:], []byte("stream")) {
				q += 6
				if q < len(data) && data[q] == '\r' {
					q++
				}
				if q < len(data) && data[q] == '\n' {
					q++
				}
				o.stream = streamData(data, q, o.dict, locs)
			}
		} else {
			end := bytes.Index(data[p:], []byte("endobj"))
			if end < 0 {
				continue
			}
			o.dict = data[p : p+end]
		}
		out = append(out, o)
	}
	return out
}

func streamData(data []byte, start int, dict []byte, locs [][]int) []byte {
	if m := reLength.FindSubmatch(dict); m != nil && m[2] == nil {
		n, _ := strconv.Atoi(string(m[1]))
		if end := start + n; end <= len(data) {
			if bytes.HasPrefix(data[skipWS(data, end):], []byte("endstream")) {
				return data[start:end]
			}
		}
	}
	// Indirect or wrong /Length: trust the endstream keyword instead.
	i := bytes.Index(data[start:], []byte("endstream"))
	if i < 0 {
		return nil
	}
	end := start + i
	if end > start && data[end-1] == '\n' {
		end--
	}
	if end > start && data[end-1] == '\r' {
		end--
	}
	return data[start:end]
}

func skipWS(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\r' || b[i] == '\n' || b[i] == '\t' || b[i] == '\f' || b[i] == 0) {
		i++
	}
	return i
}

// dictEnd returns the index after the ">>" closing the dict at i.
func dictEnd(b []byte, i int) int {
	depth := 0
	for i < len(b)-1 {
		switch {
		case b[i] == '(':
			i = stringEnd(b, i)
			continue
		case b[i] == '<' && b[i+1] == '<':
			depth++
			i += 2
			continue
		case b[i] == '>' && b[i+1] == '>':
			depth--
			i += 2
			if depth == 0 {
				return i
			}
			continue
		}
		i++
	}
	return -1
}

func stringEnd(b []byte, i int) int {
	depth := 0
	for ; i < len(b); i++ {
		switch b[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return i
}

func hasName(dict []byte, key, val string) bool {
	return regexp.MustCompile(`/` + key + `\s*/` + val + `\b`).Match(dict)
}

func intOf(re *regexp.Regexp, b []byte) (int, bool) {
	m := re.FindSubmatch(b)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(string(m[1]))
	return n, err == nil
}

func pdfFilters(dict []byte) []string {
	i := bytes.Index(dict, []byte("/Filter"))
	if i < 0 {
		return nil
	}
	rest := dict[i+7:]
	rest = rest[skipWS(rest, 0):]
	var span []byte
	if len(rest) > 0 && rest[0] == '[' {
		j := bytes.IndexByte(rest, ']')
		if j < 0 {
			return nil
		}
		span = rest[1:j]
	} else {
		j := 1
		for j < len(rest) && !strings.ContainsRune(" /\r\n\t>[", rune(rest[j])) {
			j++
		}
		span = rest[:j]
	}
	var out []string
	for _, f := range bytes.Split(span, []byte("/")) {
		if f = bytes.TrimSpace(f); len(f) > 0 {
			out = append(out, string(f))
		}
	}
	return out
}

// pdfDecode applies the stream's decoding filters, stopping at image
// codecs that are meaningful file formats on their own.
func pdfDecode(dict, raw []byte) ([]byte, error) {
	b := raw
	for _, f := range pdfFilters(dict) {
		switch f {
		case "FlateDecode", "Fl":
			zr, err := zlib.NewReader(bytes.NewReader(b))
			if err != nil {
				return nil, fmt.Errorf("flate: %w", err)
			}
			out, err := io.ReadAll(zr)
			if err != nil && len(out) == 0 {
				return nil, fmt.Errorf("flate: %w", err)
			}
			b = out
			if pred, _ := intOf(rePred, dict); pred >= 10 {
				cols, _ := intOf(reColumns, dict)
				colors, ok := intOf(reColors, dict)
				if !ok {
					colors = 1
				}
				bpc, ok := intOf(reBPC, dict)
				if !ok {
					bpc = 8
				}
				b = unpredict(b, cols, colors*bpc)
			}
		case "ASCIIHexDecode", "AHx":
			b = hexDecode(b)
		case "ASCII85Decode", "A85":
			b = a85Decode(b)
		case "RunLengthDecode", "RL":
			b = rlDecode(b)
		case "DCTDecode", "JPXDecode", "JBIG2Decode", "CCITTFaxDecode", "DCT":
			return b, nil
		default:
			return nil, errors.New("unsupported PDF filter " + f)
		}
	}
	return b, nil
}

// unpredict reverses PNG row predictors.
func unpredict(b []byte, cols, bitsPerPixel int) []byte {
	if cols <= 0 {
		return b
	}
	bpp := max(1, bitsPerPixel/8)
	row := (cols*bitsPerPixel + 7) / 8
	var out []byte
	prev := make([]byte, row)
	for p := 0; p+1+row <= len(b); p += 1 + row {
		ft := b[p]
		cur := append([]byte(nil), b[p+1:p+1+row]...)
		for i := range cur {
			var a, c byte
			if i >= bpp {
				a, c = cur[i-bpp], prev[i-bpp]
			}
			up := prev[i]
			switch ft {
			case 1:
				cur[i] += a
			case 2:
				cur[i] += up
			case 3:
				cur[i] += byte((int(a) + int(up)) / 2)
			case 4:
				pa, pb, pc := abs(int(up)-int(c)), abs(int(a)-int(c)), abs(int(a)+int(up)-2*int(c))
				switch {
				case pa <= pb && pa <= pc:
					cur[i] += a
				case pb <= pc:
					cur[i] += up
				default:
					cur[i] += c
				}
			}
		}
		out = append(out, cur...)
		prev = cur
	}
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func hexDecode(b []byte) []byte {
	var out []byte
	hi := -1
	for _, c := range b {
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'f':
			v = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v = int(c-'A') + 10
		case c == '>':
			if hi >= 0 {
				out = append(out, byte(hi<<4))
			}
			return out
		default:
			continue
		}
		if hi < 0 {
			hi = v
		} else {
			out = append(out, byte(hi<<4|v))
			hi = -1
		}
	}
	return out
}

func unpackObjStm(dict, raw []byte) []*pdfObj {
	n, _ := intOf(reN, dict)
	first, ok := intOf(reFirst, dict)
	if !ok || first > len(raw) || n <= 0 || n > 100000 {
		return nil
	}
	nums := strings.Fields(string(raw[:first]))
	var out []*pdfObj
	for i := 0; i+1 < len(nums) && i/2 < n; i += 2 {
		num, err1 := strconv.Atoi(nums[i])
		off, err2 := strconv.Atoi(nums[i+1])
		if err1 != nil || err2 != nil || first+off > len(raw) {
			continue
		}
		end := len(raw)
		if i+3 < len(nums) {
			if next, err := strconv.Atoi(nums[i+3]); err == nil && first+next <= len(raw) {
				end = first + next
			}
		}
		if end < first+off {
			continue
		}
		out = append(out, &pdfObj{num: num, dict: raw[first+off : end]})
	}
	return out
}

// filespecName pulls /UF or /F out of a file specification.
func filespecName(dict []byte) string {
	for _, key := range []string{"/UF", "/F"} {
		for i := 0; ; {
			j := bytes.Index(dict[i:], []byte(key))
			if j < 0 {
				break
			}
			i += j + len(key)
			rest := dict[i:]
			rest = rest[skipWS(rest, 0):]
			if len(rest) > 0 && (rest[0] == '(' || rest[0] == '<' && (len(rest) < 2 || rest[1] != '<')) {
				if s, ok := pdfStringAt(rest); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// pdfStringAt decodes a literal or hex string at the start of b.
func pdfStringAt(b []byte) (string, bool) {
	b = b[skipWS(b, 0):]
	if len(b) == 0 {
		return "", false
	}
	var raw []byte
	switch b[0] {
	case '(':
		end := stringEnd(b, 0)
		s := b[1:max(1, end-1)]
		for i := 0; i < len(s); i++ {
			if s[i] != '\\' || i+1 >= len(s) {
				raw = append(raw, s[i])
				continue
			}
			i++
			switch c := s[i]; c {
			case 'n':
				raw = append(raw, '\n')
			case 'r':
				raw = append(raw, '\r')
			case 't':
				raw = append(raw, '\t')
			case 'b':
				raw = append(raw, '\b')
			case 'f':
				raw = append(raw, '\f')
			case '\r', '\n':
			default:
				if c >= '0' && c <= '7' {
					v := 0
					k := 0
					for ; k < 3 && i+k < len(s) && s[i+k] >= '0' && s[i+k] <= '7'; k++ {
						v = v*8 + int(s[i+k]-'0')
					}
					i += k - 1
					raw = append(raw, byte(v))
				} else {
					raw = append(raw, c)
				}
			}
		}
	case '<':
		raw = hexDecode(b[1:])
	default:
		return "", false
	}
	if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
		u := make([]uint16, (len(raw)-2)/2)
		for i := range u {
			u[i] = uint16(raw[2+2*i])<<8 | uint16(raw[3+2*i])
		}
		return string(utf16.Decode(u)), true
	}
	return string(raw), true
}

func a85Decode(b []byte) []byte {
	b = bytes.TrimSpace(b)
	b = bytes.TrimPrefix(b, []byte("<~"))
	if i := bytes.Index(b, []byte("~>")); i >= 0 {
		b = b[:i]
	}
	out := make([]byte, len(b))
	n, _, _ := ascii85.Decode(out, b, true)
	return out[:n]
}

func rlDecode(b []byte) []byte {
	var out []byte
	for i := 0; i < len(b); {
		n := int(b[i])
		i++
		switch {
		case n == 128:
			return out
		case n < 128:
			end := min(len(b), i+n+1)
			out = append(out, b[i:end]...)
			i = end
		default:
			if i < len(b) {
				out = append(out, bytes.Repeat(b[i:i+1], 257-n)...)
			}
			i++
		}
	}
	return out
}
