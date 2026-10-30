package binary

import (
	"debug/elf"
	"debug/macho"
	"os"
	"strings"

	"github.com/xmhrnx/anyfile/internal/detect"
	"github.com/xmhrnx/anyfile/internal/explore"
)

// --- ELF / Mach-O sections ---------------------------------------------------------

func listELF(p string, d *detect.Result) (*explore.Listing, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	ef, err := elf.NewFile(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	l := &explore.Listing{Engine: "ELF (built-in)"}
	l.OnClose(func() { f.Close() })
	for _, s := range ef.Sections {
		if s.Type == elf.SHT_NOBITS || s.FileSize == 0 || s.Name == "" {
			continue
		}
		l.Entries = append(l.Entries, &explore.Entry{
			Path: "Sections/" + explore.SafeName(s.Name) + ".bin", Size: int64(s.FileSize),
			Note: strings.TrimPrefix(s.Type.String(), "SHT_"),
			Open: explore.SectionOpener(f, int64(s.Offset), int64(s.FileSize)),
		})
	}
	return l, nil
}

func listMachO(p string, d *detect.Result) (*explore.Listing, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	l := &explore.Listing{Engine: "Mach-O (built-in)"}
	l.OnClose(func() { f.Close() })
	add := func(prefix string, base int64, mf *macho.File) {
		for _, s := range mf.Sections {
			if s.Size == 0 || s.Offset == 0 {
				continue
			}
			l.Entries = append(l.Entries, &explore.Entry{
				Path: prefix + explore.SafeName(s.Seg+"."+s.Name) + ".bin", Size: int64(s.Size),
				Open: explore.SectionOpener(f, base+int64(s.Offset), int64(s.Size)),
			})
		}
	}
	if ff, err := macho.NewFatFile(f); err == nil {
		for _, a := range ff.Arches {
			arch := strings.TrimPrefix(a.Cpu.String(), "Cpu")
			l.Entries = append(l.Entries, &explore.Entry{
				Path: arch + ".macho", Size: int64(a.Size), Note: "single-architecture slice",
				Open: explore.SectionOpener(f, int64(a.Offset), int64(a.Size)),
			})
		}
		return l, nil
	}
	mf, err := macho.NewFile(f)
	if err != nil {
		l.Close()
		return nil, err
	}
	add("Sections/", 0, mf)
	return l, nil
}
