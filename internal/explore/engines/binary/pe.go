// Package binary explores executables: PE resources (icons rebuilt as
// .ico, bitmaps, manifests, string tables), sections, overlays and
// signatures, plus ELF and Mach-O sections and slices.
package binary

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/xmhrnx/anyfile/internal/detect"
	"github.com/xmhrnx/anyfile/internal/explore"
	"github.com/xmhrnx/anyfile/internal/explore/engines/sevenzip"
)

func init() {
	explore.Register(explore.Engine{Name: "PE executable", Priority: 30, Match: matchPE, List: listPE})
	explore.Register(explore.Engine{Name: "ELF", Priority: 31, Match: func(d *detect.Result, h []byte) bool {
		return bytes.HasPrefix(h, []byte("\x7FELF"))
	}, List: listELF})
	explore.Register(explore.Engine{Name: "Mach-O", Priority: 32, Match: func(d *detect.Result, h []byte) bool {
		return d.Ext == "macho"
	}, List: listMachO})
}

func matchPE(d *detect.Result, h []byte) bool {
	return bytes.HasPrefix(h, []byte("MZ")) && d.Category == detect.CatExecutable
}

// --- PE: resources, overlay, sections ----------------------------------------

var resTypes = map[uint32]string{
	1: "CURSOR", 2: "BITMAP", 3: "ICON", 4: "MENU", 5: "DIALOG", 6: "STRING", 7: "FONTDIR", 8: "FONT",
	9: "ACCELERATOR", 10: "RCDATA", 11: "MESSAGETABLE", 12: "GROUP_CURSOR", 14: "GROUP_ICON",
	16: "VERSION", 17: "DLGINCLUDE", 19: "PLUGPLAY", 20: "VXD", 21: "ANICURSOR", 22: "ANIICON",
	23: "HTML", 24: "MANIFEST",
}

var resExt = map[string]string{
	"BITMAP": "bmp", "HTML": "html", "MANIFEST": "xml", "ANICURSOR": "ani", "ANIICON": "ani", "FONT": "fnt",
}

type peRes struct {
	typ, name string
	typID     uint32
	nameID    uint32
	lang      uint32
	off, size int64
}

func listPE(p string, d *detect.Result) (*explore.Listing, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	st, _ := f.Stat()
	pf, err := pe.NewFile(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	l := &explore.Listing{Engine: "PE (built-in)"}
	l.OnClose(func() { f.Close() })

	var dirs []pe.DataDirectory
	switch oh := pf.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		dirs = oh.DataDirectory[:oh.NumberOfRvaAndSizes]
	case *pe.OptionalHeader64:
		dirs = oh.DataDirectory[:oh.NumberOfRvaAndSizes]
	}
	rvaToOff := func(rva uint32) (int64, bool) {
		for _, s := range pf.Sections {
			if rva >= s.VirtualAddress && rva < s.VirtualAddress+max(s.VirtualSize, s.Size) {
				return int64(rva-s.VirtualAddress) + int64(s.Offset), true
			}
		}
		return 0, false
	}

	// Resources.
	if len(dirs) > 2 && dirs[2].VirtualAddress != 0 {
		if base, ok := rvaToOff(dirs[2].VirtualAddress); ok {
			res := walkResources(f, base, rvaToOff)
			addResources(l, f, res)
		}
	}
	// Sections.
	var end int64
	for _, s := range pf.Sections {
		if e := int64(s.Offset) + int64(s.Size); e > end {
			end = e
		}
		if s.Size == 0 {
			continue
		}
		name := strings.TrimRight(s.Name, "\x00")
		if name == "" {
			name = "(unnamed)"
		}
		l.Entries = append(l.Entries, &explore.Entry{
			Path: "Sections/" + explore.SafeName(name) + ".bin",
			Size: int64(s.Size),
			Note: fmt.Sprintf("raw section @ 0x%X", s.Offset),
			Open: explore.SectionOpener(f, int64(s.Offset), int64(s.Size)),
		})
	}
	// Overlay (installer payloads) and signature.
	var certOff, certLen int64
	if len(dirs) > 4 && dirs[4].VirtualAddress != 0 {
		certOff, certLen = int64(dirs[4].VirtualAddress), int64(dirs[4].Size)
	}
	ovEnd := st.Size()
	if certOff >= end && certOff < ovEnd {
		ovEnd = certOff
	}
	if ovEnd > end {
		l.Entries = append(l.Entries, &explore.Entry{
			Path: "Overlay.bin",
			Size: ovEnd - end,
			Note: "data appended after the image (installer payload?)",
			Open: explore.SectionOpener(f, end, ovEnd-end),
		})
	}
	if certLen > 8 && certOff+certLen <= st.Size() {
		l.Entries = append(l.Entries, &explore.Entry{
			Path: "Signature/authenticode.p7b",
			Size: certLen - 8,
			Note: "PKCS#7 signature",
			Open: explore.SectionOpener(f, certOff+8, certLen-8),
		})
	}
	// Installer payloads that 7-Zip understands (NSIS, Inno-wrapped CABs...).
	if d.Detail("Installer") != "" || d.Detail("Overlay") != "" {
		if sl, err := sevenzip.List(p, d); err == nil && sl.Format != "" && sl.Format != "PE" {
			for _, e := range sl.Entries {
				e.Path = "Payload (" + sl.Format + ")/" + e.Path
				l.Entries = append(l.Entries, e)
			}
			l.Engine += " + 7-Zip"
			l.OnClose(sl.Close)
		}
	}
	return l, nil
}

func walkResources(f io.ReaderAt, base int64, rvaToOff func(uint32) (int64, bool)) []peRes {
	var out []peRes
	readDir := func(off int64) [][2]uint32 {
		h := make([]byte, 16)
		if _, err := f.ReadAt(h, off); err != nil {
			return nil
		}
		n := int(binary.LittleEndian.Uint16(h[12:])) + int(binary.LittleEndian.Uint16(h[14:]))
		if n > 4096 {
			return nil
		}
		b := make([]byte, 8*n)
		f.ReadAt(b, off+16)
		ents := make([][2]uint32, n)
		for i := range ents {
			ents[i] = [2]uint32{binary.LittleEndian.Uint32(b[8*i:]), binary.LittleEndian.Uint32(b[8*i+4:])}
		}
		return ents
	}
	resName := func(v uint32) (string, uint32) {
		if v&0x80000000 == 0 {
			return fmt.Sprint(v), v
		}
		off := base + int64(v&0x7FFFFFFF)
		h := make([]byte, 2)
		f.ReadAt(h, off)
		n := int(binary.LittleEndian.Uint16(h))
		b := make([]byte, 2*n)
		f.ReadAt(b, off+2)
		u := make([]rune, n)
		for i := range u {
			u[i] = rune(binary.LittleEndian.Uint16(b[2*i:]))
		}
		return string(u), 0
	}
	for _, t := range readDir(base) {
		if t[1]&0x80000000 == 0 {
			continue
		}
		typ, typID := resName(t[0])
		if s, ok := resTypes[typID]; ok && t[0]&0x80000000 == 0 {
			typ = s
		}
		for _, nm := range readDir(base + int64(t[1]&0x7FFFFFFF)) {
			name, nameID := resName(nm[0])
			if nm[1]&0x80000000 == 0 {
				continue
			}
			for _, lg := range readDir(base + int64(nm[1]&0x7FFFFFFF)) {
				if lg[1]&0x80000000 != 0 {
					continue
				}
				de := make([]byte, 16)
				if _, err := f.ReadAt(de, base+int64(lg[1])); err != nil {
					continue
				}
				off, ok := rvaToOff(binary.LittleEndian.Uint32(de))
				if !ok {
					continue
				}
				out = append(out, peRes{typ: typ, typID: typID, name: name, nameID: nameID, lang: lg[0],
					off: off, size: int64(binary.LittleEndian.Uint32(de[4:]))})
			}
		}
	}
	return out
}

func addResources(l *explore.Listing, f io.ReaderAt, res []peRes) {
	read := func(r peRes) []byte {
		b := make([]byte, r.size)
		f.ReadAt(b, r.off)
		return b
	}
	icons := map[uint32]peRes{}
	cursors := map[uint32]peRes{}
	for _, r := range res {
		switch r.typID {
		case 3:
			icons[r.nameID] = r
		case 1:
			cursors[r.nameID] = r
		}
	}
	for _, r := range res {
		dir := "Resources/" + explore.SafeName(r.typ)
		name := explore.SafeName(r.name)
		switch r.typID {
		case 3, 1:
			continue // folded into their groups
		case 14, 12:
			// Rebuild a standalone .ico / .cur from the group directory.
			members := icons
			ext, kind := "ico", uint16(1)
			if r.typID == 12 {
				members, ext, kind = cursors, "cur", 2
			}
			grp := read(r)
			ico := buildIcon(grp, members, read, kind)
			if ico == nil {
				continue
			}
			l.Entries = append(l.Entries, &explore.Entry{
				Path: path.Join("Resources", map[uint32]string{14: "ICON", 12: "CURSOR"}[r.typID], name+"."+ext),
				Size: int64(len(ico)), Note: fmt.Sprintf("%d image(s)", binary.LittleEndian.Uint16(grp[4:])),
				Open: explore.BytesOpener(ico),
			})
		case 2:
			// Bitmaps are stored without their file header.
			dib := read(r)
			if len(dib) < 40 {
				continue
			}
			bmp := make([]byte, 14, 14+len(dib))
			bmp[0], bmp[1] = 'B', 'M'
			binary.LittleEndian.PutUint32(bmp[2:], uint32(14+len(dib)))
			hdr := binary.LittleEndian.Uint32(dib)
			colors := binary.LittleEndian.Uint32(dib[32:])
			bpp := binary.LittleEndian.Uint16(dib[14:])
			if colors == 0 && bpp <= 8 {
				colors = 1 << bpp
			}
			binary.LittleEndian.PutUint32(bmp[10:], 14+hdr+4*colors)
			bmp = append(bmp, dib...)
			l.Entries = append(l.Entries, &explore.Entry{Path: dir + "/" + name + ".bmp", Size: int64(len(bmp)), Open: explore.BytesOpener(bmp)})
		case 6:
			// String tables become readable text.
			txt := stringTable(read(r), r.nameID)
			l.Entries = append(l.Entries, &explore.Entry{Path: dir + "/" + name + ".txt", Size: int64(len(txt)), Open: explore.BytesOpener([]byte(txt))})
		default:
			ext := resExt[r.typ]
			if ext == "" {
				ext = "bin"
			}
			l.Entries = append(l.Entries, &explore.Entry{Path: dir + "/" + name + "." + ext, Size: r.size, Open: explore.SectionOpener(f, r.off, r.size)})
		}
	}
}

// buildIcon turns a GROUP_ICON directory plus its ICON members into an
// .ico file.
func buildIcon(grp []byte, members map[uint32]peRes, read func(peRes) []byte, kind uint16) []byte {
	if len(grp) < 6 {
		return nil
	}
	n := int(binary.LittleEndian.Uint16(grp[4:]))
	if len(grp) < 6+14*n {
		return nil
	}
	type img struct {
		ent  []byte
		data []byte
	}
	var imgs []img
	for i := 0; i < n; i++ {
		e := grp[6+14*i : 20+14*i]
		id := uint32(binary.LittleEndian.Uint16(e[12:]))
		m, ok := members[id]
		if !ok {
			continue
		}
		data := read(m)
		if kind == 2 && len(data) > 4 { // cursors carry their hotspot first
			data = data[4:]
		}
		imgs = append(imgs, img{e[:12], data})
	}
	if len(imgs) == 0 {
		return nil
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, [3]uint16{0, kind, uint16(len(imgs))})
	off := uint32(6 + 16*len(imgs))
	for _, im := range imgs {
		out.Write(im.ent[:8])
		binary.Write(&out, binary.LittleEndian, uint32(len(im.data)))
		binary.Write(&out, binary.LittleEndian, off)
		off += uint32(len(im.data))
	}
	for _, im := range imgs {
		out.Write(im.data)
	}
	return out.Bytes()
}

func stringTable(b []byte, block uint32) string {
	var sb strings.Builder
	p := 0
	for i := 0; i < 16 && p+2 <= len(b); i++ {
		n := int(binary.LittleEndian.Uint16(b[p:]))
		p += 2
		if n == 0 {
			continue
		}
		if p+2*n > len(b) {
			break
		}
		u := make([]rune, n)
		for k := range u {
			u[k] = rune(binary.LittleEndian.Uint16(b[p+2*k:]))
		}
		p += 2 * n
		fmt.Fprintf(&sb, "%d\t%s\r\n", (block-1)*16+uint32(i), string(u))
	}
	return sb.String()
}
