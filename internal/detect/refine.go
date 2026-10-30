package detect

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/xmhrnx/anyfile/internal/codec"

	"github.com/mehran9675/giode/i18n"
	"github.com/xmhrnx/anyfile/internal/cfb"
)

// refineZip tells the many ZIP-based formats apart by their members.
func refineZip(p string, r *Result) {
	zr, err := zip.OpenReader(p)
	if err != nil {
		r.note(i18n.Intl("ZIP signature present but the central directory could not be read: {err}", "err", err))
		r.Confidence = 0.7
		return
	}
	defer zr.Close()
	names := map[string]bool{}
	var encrypted int
	for _, f := range zr.File {
		names[f.Name] = true
		if f.Flags&1 != 0 {
			encrypted++
		}
	}
	r.detail("Entries", fmt.Sprint(len(zr.File)))
	if encrypted > 0 {
		r.detail("Encrypted entries", fmt.Sprint(encrypted))
	}
	has := func(n string) bool { return names[n] }
	prefix := func(pfx string) bool {
		for n := range names {
			if strings.HasPrefix(n, pfx) {
				return true
			}
		}
		return false
	}
	mimetype := ""
	if f := find(zr, "mimetype"); f != nil {
		if rc, err := f.Open(); err == nil {
			b, _ := io.ReadAll(io.LimitReader(rc, 200))
			rc.Close()
			mimetype = strings.TrimSpace(string(b))
		}
	}
	set := func(label, ext, mime string, cat Category) { r.set(label, ext, mime, cat, 0.98, SrcStructure) }
	switch {
	case has("[Content_Types].xml") && prefix("word/"):
		set("Word document (OOXML)", "docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", CatDocument)
		if has("word/vbaProject.bin") {
			set("Word macro-enabled document", "docm", "application/vnd.ms-word.document.macroEnabled.12", CatDocument)
		}
	case has("[Content_Types].xml") && prefix("xl/"):
		set("Excel workbook (OOXML)", "xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", CatSpreadsheet)
		if has("xl/vbaProject.bin") {
			set("Excel macro-enabled workbook", "xlsm", "application/vnd.ms-excel.sheet.macroEnabled.12", CatSpreadsheet)
		}
		if has("xl/workbook.bin") {
			set("Excel binary workbook", "xlsb", "application/vnd.ms-excel.sheet.binary.macroEnabled.12", CatSpreadsheet)
		}
	case has("[Content_Types].xml") && prefix("ppt/"):
		set("PowerPoint presentation (OOXML)", "pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", CatPresentation)
	case has("[Content_Types].xml") && prefix("visio/"):
		set("Visio drawing (OOXML)", "vsdx", "application/vnd.ms-visio.drawing", CatDocument)
	case has("[Content_Types].xml") && has("AppxManifest.xml"):
		set("Windows app package (MSIX/AppX)", "msix", "application/msix", CatArchive)
	case has("[Content_Types].xml") && has("extension.vsixmanifest"):
		set("Visual Studio extension", "vsix", "application/vsix", CatArchive)
	case has("[Content_Types].xml") && prefix("3D/"):
		set("3D Manufacturing Format", "3mf", "model/3mf", CatData)
	case has("[Content_Types].xml") && hasSuffix(names, ".nuspec"):
		set("NuGet package", "nupkg", "application/octet-stream", CatArchive)
	case has("[Content_Types].xml") && prefix("Documents/") || has("FixedDocSeq.fdseq"):
		set("XPS document", "xps", "application/vnd.ms-xpsdocument", CatDocument)
	case mimetype == "application/epub+zip":
		set("EPUB e-book", "epub", mimetype, CatDocument)
	case strings.HasPrefix(mimetype, "application/vnd.oasis.opendocument."):
		kind := strings.TrimPrefix(mimetype, "application/vnd.oasis.opendocument.")
		switch {
		case strings.HasPrefix(kind, "text"):
			set("OpenDocument text", "odt", mimetype, CatDocument)
		case strings.HasPrefix(kind, "spreadsheet"):
			set("OpenDocument spreadsheet", "ods", mimetype, CatSpreadsheet)
		case strings.HasPrefix(kind, "presentation"):
			set("OpenDocument presentation", "odp", mimetype, CatPresentation)
		case strings.HasPrefix(kind, "graphics"):
			set("OpenDocument drawing", "odg", mimetype, CatImage)
		default:
			set("OpenDocument file", "od"+string(kind[0]), mimetype, CatDocument)
		}
	case strings.HasPrefix(mimetype, "application/vnd.sun.xml."):
		set("OpenOffice.org 1.x document", "sxw", mimetype, CatDocument)
	case mimetype != "" && strings.Contains(mimetype, "krita"):
		set("Krita image", "kra", mimetype, CatImage)
	case mimetype == "image/openraster":
		set("OpenRaster image", "ora", mimetype, CatImage)
	case has("AndroidManifest.xml") && has("classes.dex"):
		set("Android application (APK)", "apk", "application/vnd.android.package-archive", CatExecutable)
	case has("AndroidManifest.xml") || prefix("base/manifest/"):
		set("Android app bundle", "aab", "application/octet-stream", CatExecutable)
	case prefix("Payload/") && hasSuffix(names, ".app/Info.plist"):
		set("iOS application (IPA)", "ipa", "application/octet-stream", CatExecutable)
	case has("META-INF/MANIFEST.MF") && hasSuffix(names, ".class"):
		set("Java archive (JAR)", "jar", "application/java-archive", CatExecutable)
		if prefix("WEB-INF/") {
			set("Java web archive (WAR)", "war", "application/java-archive", CatArchive)
		}
	case has("META-INF/MANIFEST.MF"):
		set("Java archive (JAR)", "jar", "application/java-archive", CatArchive)
	case has("manifest.json") && (has("background.js") || prefix("_locales/")) || has("install.rdf"):
		set("Browser extension", "xpi", "application/x-xpinstall", CatArchive)
	case prefix("") && hasSuffix(names, ".dist-info/WHEEL"):
		set("Python wheel", "whl", "application/zip", CatArchive)
	case has("doc.kml"):
		set("Google Earth KMZ", "kmz", "application/vnd.google-earth.kmz", CatData)
	case has("Contents/Info.plist") || prefix("Sketch/"):
		set("Application bundle archive", "zip", "application/zip", CatArchive)
	case has("document.json") && has("meta.json") && prefix("pages/"):
		set("Sketch document", "sketch", "application/zip", CatImage)
	case has("Thumbnails/thumbnail.png") && has("content.xml"):
		set("OpenDocument file", "odt", "application/vnd.oasis.opendocument.text", CatDocument)
	case has("_rels/.rels") && has("[Content_Types].xml"):
		set("Open Packaging Conventions package", "zip", "application/zip", CatArchive)
	default:
		r.Confidence = 0.95
		r.Source = SrcStructure
	}
}

func find(zr *zip.ReadCloser, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func hasSuffix(names map[string]bool, sfx string) bool {
	for n := range names {
		if strings.HasSuffix(n, sfx) {
			return true
		}
	}
	return false
}

// refineOLE classifies compound files by their streams and CLSID.
func refineOLE(p string, r *Result) {
	f, err := os.Open(p)
	if err != nil {
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	c, err := cfb.Open(f, st.Size())
	if err != nil {
		r.note(i18n.Intl("Compound file header found but unreadable: {err}", "err", err))
		return
	}
	r.detail("Streams", fmt.Sprint(len(c.Entries())))
	clsid := fmt.Sprintf("%X", c.Root().CLSID)
	has := func(n string) bool { return c.Find(n) != nil }
	set := func(label, ext, mime string, cat Category) { r.set(label, ext, mime, cat, 0.97, SrcStructure) }
	encrypted := has("EncryptedPackage")
	switch {
	case encrypted:
		set("Encrypted Office document", "docx", "application/octet-stream", CatEncrypted)
		r.note(i18n.Intl("This is a password-protected Office file (OOXML inside an encrypted OLE container)."))
	case has("WordDocument"):
		set("Word 97-2003 document", "doc", "application/msword", CatDocument)
	case has("Workbook") || has("Book"):
		set("Excel 97-2003 workbook", "xls", "application/vnd.ms-excel", CatSpreadsheet)
	case has("PowerPoint Document"):
		set("PowerPoint 97-2003 presentation", "ppt", "application/vnd.ms-powerpoint", CatPresentation)
	case has("VisioDocument"):
		set("Visio drawing", "vsd", "application/vnd.visio", CatDocument)
	case has("__properties_version1.0") || hasPrefixEntry(c, "__substg1.0_"):
		set("Outlook message", "msg", "application/vnd.ms-outlook", CatEmail)
	case strings.HasPrefix(clsid, "84100C00") || strings.HasPrefix(clsid, "86100C00") || has("!_StringPool"):
		set("Windows Installer package", "msi", "application/x-msi", CatExecutable)
		if strings.HasPrefix(clsid, "86100C00") {
			set("Windows Installer patch", "msp", "application/octet-stream", CatExecutable)
		}
	case has("Catalog") && hasPrefixEntry(c, "256_"):
		set("Windows thumbnail cache", "db", "application/octet-stream", CatDatabase)
	case has("Ole10Native") || has("\x01Ole10Native"):
		set("OLE packaged object", "bin", "application/octet-stream", CatArchive)
	case has("Contents") && has("\x01CompObj"):
		set("OLE embedded object", "bin", "application/octet-stream", CatArchive)
	case has("PerfectOffice_MAIN"):
		set("WordPerfect document", "wpd", "application/vnd.wordperfect", CatDocument)
	case has("Quill"):
		set("Publisher document", "pub", "application/x-mspublisher", CatDocument)
	default:
		r.Confidence = 0.8
	}
}

func hasPrefixEntry(c *cfb.File, pfx string) bool {
	for _, e := range c.Entries() {
		if strings.HasPrefix(e.Name, pfx) {
			return true
		}
	}
	return false
}

// refineRIFF identifies the RIFF form type.
func refineRIFF(h []byte, r *Result) bool {
	if len(h) < 12 {
		return false
	}
	set := func(label, ext, mime string, cat Category) { r.set(label, ext, mime, cat, 0.97, SrcStructure) }
	switch string(h[8:12]) {
	case "WAVE":
		set("WAVE audio", "wav", "audio/wav", CatAudio)
	case "AVI ":
		set("AVI video", "avi", "video/x-msvideo", CatVideo)
	case "WEBP":
		set("WebP image", "webp", "image/webp", CatImage)
	case "RMID":
		set("RIFF MIDI", "rmi", "audio/midi", CatAudio)
	case "ACON":
		set("Animated cursor", "ani", "application/x-navi-animation", CatImage)
	case "CDR9", "CDRA", "cdr9":
		set("CorelDRAW drawing", "cdr", "application/vnd.corel-draw", CatImage)
	case "AMV ":
		set("AMV video", "amv", "video/x-amv", CatVideo)
	case "CDXA":
		set("Video CD track", "dat", "video/mpeg", CatVideo)
	default:
		set("RIFF container ("+strings.TrimSpace(string(h[8:12]))+")", "riff", "application/octet-stream", CatData)
		r.Confidence = 0.8
	}
	return true
}

// refineFtyp maps ISO base media file brands.
func refineFtyp(h []byte, r *Result) bool {
	if len(h) < 16 || string(h[4:8]) != "ftyp" {
		return false
	}
	brand := string(h[8:12])
	n := int(binary.BigEndian.Uint32(h[:4]))
	compat := ""
	if n <= len(h) && n > 16 {
		compat = string(h[16:n])
	}
	r.detail("Brand", strings.TrimSpace(brand))
	set := func(label, ext, mime string, cat Category) { r.set(label, ext, mime, cat, 0.96, SrcStructure) }
	switch {
	case brand == "heic" || brand == "heix" || brand == "heim" || brand == "heis" || brand == "mif1" && strings.Contains(compat, "heic"):
		set("HEIC image", "heic", "image/heic", CatImage)
	case brand == "avif" || brand == "avis":
		set("AVIF image", "avif", "image/avif", CatImage)
	case brand == "mif1" || brand == "msf1":
		set("HEIF image", "heif", "image/heif", CatImage)
	case brand == "crx ":
		set("Canon RAW 3 image", "cr3", "image/x-canon-cr3", CatImage)
	case brand == "qt  ":
		set("QuickTime movie", "mov", "video/quicktime", CatVideo)
	case brand == "M4A " || brand == "M4B " || brand == "M4P ":
		set("MPEG-4 audio", "m4a", "audio/mp4", CatAudio)
	case brand == "M4V " || brand == "M4VH" || brand == "M4VP":
		set("MPEG-4 video (iTunes)", "m4v", "video/x-m4v", CatVideo)
	case strings.HasPrefix(brand, "3gp") || strings.HasPrefix(brand, "3ge") || strings.HasPrefix(brand, "3gg"):
		set("3GPP media", "3gp", "video/3gpp", CatVideo)
	case strings.HasPrefix(brand, "3g2"):
		set("3GPP2 media", "3g2", "video/3gpp2", CatVideo)
	case brand == "f4v ":
		set("Flash MP4 video", "f4v", "video/mp4", CatVideo)
	case brand == "jp2 ":
		set("JPEG 2000 image", "jp2", "image/jp2", CatImage)
	default:
		set("MPEG-4 media", "mp4", "video/mp4", CatVideo)
	}
	return true
}

func refineEBML(h []byte, r *Result) {
	if bytes.Contains(h[:min(len(h), 64)], []byte("webm")) {
		r.set("WebM video", "webm", "video/webm", CatVideo, 0.97, SrcStructure)
		return
	}
	r.set("Matroska media", "mkv", "video/x-matroska", CatVideo, 0.95, SrcStructure)
	if bytes.Contains(h[:min(len(h), 64)], []byte("matroska")) {
		r.Confidence = 0.97
	}
}

func refineOgg(h []byte, r *Result) {
	head := h[:min(len(h), 256)]
	switch {
	case bytes.Contains(head, []byte("OpusHead")):
		r.set("Ogg Opus audio", "opus", "audio/ogg", CatAudio, 0.97, SrcStructure)
	case bytes.Contains(head, []byte("\x01vorbis")):
		r.set("Ogg Vorbis audio", "ogg", "audio/ogg", CatAudio, 0.97, SrcStructure)
	case bytes.Contains(head, []byte("\x7FFLAC")):
		r.set("Ogg FLAC audio", "oga", "audio/ogg", CatAudio, 0.97, SrcStructure)
	case bytes.Contains(head, []byte("\x80theora")):
		r.set("Ogg Theora video", "ogv", "video/ogg", CatVideo, 0.97, SrcStructure)
	case bytes.Contains(head, []byte("Speex")):
		r.set("Ogg Speex audio", "spx", "audio/ogg", CatAudio, 0.95, SrcStructure)
	default:
		r.set("Ogg media", "ogg", "application/ogg", CatAudio, 0.9, SrcStructure)
	}
}

// refinePE describes Windows executables and spots installer stubs.
func refinePE(p string, h []byte, r *Result) bool {
	f, err := pe.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	var chars uint16 = f.Characteristics
	var subsystem uint16
	var clr bool
	var dirs []pe.DataDirectory
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		subsystem = oh.Subsystem
		dirs = oh.DataDirectory[:oh.NumberOfRvaAndSizes]
	case *pe.OptionalHeader64:
		subsystem = oh.Subsystem
		dirs = oh.DataDirectory[:oh.NumberOfRvaAndSizes]
	}
	if len(dirs) > 14 && dirs[14].VirtualAddress != 0 {
		clr = true
	}
	arch := map[uint16]string{
		pe.IMAGE_FILE_MACHINE_I386: "x86", pe.IMAGE_FILE_MACHINE_AMD64: "x64",
		pe.IMAGE_FILE_MACHINE_ARM64: "ARM64", pe.IMAGE_FILE_MACHINE_ARMNT: "ARM",
		pe.IMAGE_FILE_MACHINE_IA64: "Itanium",
	}[f.Machine]
	if arch == "" {
		arch = fmt.Sprintf("0x%04X", f.Machine)
	}
	sub := map[uint16]string{1: "Native (driver)", 2: "GUI", 3: "Console", 10: "EFI application", 11: "EFI boot driver", 12: "EFI runtime driver"}[subsystem]
	r.detail("Architecture", arch)
	r.detail("Subsystem", sub)
	r.detail("Sections", fmt.Sprint(len(f.Sections)))
	if clr {
		r.detail("Runtime", ".NET (CLR)")
	}

	kind, ext := "Windows executable", "exe"
	switch {
	case chars&pe.IMAGE_FILE_DLL != 0:
		kind, ext = "Windows library (DLL)", "dll"
	case subsystem == 1:
		kind, ext = "Windows driver", "sys"
	case subsystem >= 10 && subsystem <= 12:
		kind, ext = "EFI executable", "efi"
	}
	if clr {
		kind = ".NET " + strings.TrimPrefix(kind, "Windows ")
	}
	r.set(kind+" ("+arch+")", ext, "application/vnd.microsoft.portable-executable", CatExecutable, 0.97, SrcStructure)

	// Overlay: data appended after the last section is where installers
	// and self-extracting archives keep their payload.
	var end int64
	for _, s := range f.Sections {
		if e := int64(s.Offset) + int64(s.Size); e > end {
			end = e
		}
	}
	st, _ := os.Stat(p)
	var certOff int64
	if len(dirs) > 4 && dirs[4].VirtualAddress != 0 {
		certOff = int64(dirs[4].VirtualAddress) // file offset for the security directory
	}
	overlay := st.Size() - end
	if certOff > end {
		overlay = certOff - end
	}
	if certOff > 0 {
		r.detail("Signed", "Authenticode signature present")
	}
	if overlay > 0 {
		r.detail("Overlay", i18n.Intl("{size} appended after the image", "size", humanSize(overlay)))
	}
	installer := installerKind(p, end, overlay, h)
	if installer != "" {
		r.Label = installer + " (" + arch + ")"
		r.detail("Installer", installer)
		r.Category = CatExecutable
	}
	return true
}

// installerKind looks for the markers of common installer and SFX
// stubs in the overlay and the image.
func installerKind(p string, end, overlay int64, h []byte) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	var ov []byte
	if overlay > 0 {
		ov = make([]byte, min(overlay, 1<<16))
		f.ReadAt(ov, end)
	}
	img := make([]byte, min(end, 4<<20))
	f.ReadAt(img, 0)
	switch {
	case bytes.Contains(ov[:min(len(ov), 64)], []byte("NullsoftInst")) || bytes.Contains(ov, []byte("\xEF\xBE\xAD\xDENullsoftInst")):
		return "NSIS installer"
	case bytes.Contains(img, []byte("Inno Setup")) || bytes.Contains(ov[:min(len(ov), 64)], []byte("zlb\x1a")) || bytes.Contains(ov, []byte("Inno Setup Setup Data")):
		return "Inno Setup installer"
	case bytes.HasPrefix(ov, []byte("7z\xBC\xAF\x27\x1C")):
		return "7-Zip self-extracting archive"
	case bytes.HasPrefix(ov, []byte("Rar!")) || bytes.Contains(img, []byte("WinRAR SFX")):
		return "WinRAR self-extracting archive"
	case bytes.HasPrefix(ov, []byte("PK\x03\x04")):
		return "ZIP self-extracting archive"
	case bytes.HasPrefix(ov, []byte("MSCF")):
		return "Cabinet self-extracting archive"
	case bytes.Contains(img, []byte("InstallShield")):
		return "InstallShield installer"
	case bytes.Contains(img, []byte("WiX Toolset")) || bytes.Contains(img, []byte("WixBundle")):
		return "WiX bundle installer"
	case bytes.Contains(ov, []byte("pyi-windows-manifest")) || bytes.Contains(ov, []byte("MEI\x0c\x0b\x0a\x0b\x0e")):
		return "PyInstaller bundle"
	}
	return ""
}

func refineELF(p string, r *Result) {
	f, err := elf.Open(p)
	if err != nil {
		return
	}
	defer f.Close()
	kind := map[elf.Type]string{elf.ET_EXEC: "ELF executable", elf.ET_DYN: "ELF shared object / PIE", elf.ET_REL: "ELF object file", elf.ET_CORE: "ELF core dump"}[f.Type]
	if kind == "" {
		kind = "ELF file"
	}
	ext := map[elf.Type]string{elf.ET_DYN: "so", elf.ET_REL: "o", elf.ET_CORE: "core"}[f.Type]
	if ext == "" {
		ext = "elf"
	}
	arch := strings.TrimPrefix(f.Machine.String(), "EM_")
	r.set(kind+" ("+arch+")", ext, "application/x-executable", CatExecutable, 0.97, SrcStructure)
	r.detail("Architecture", arch)
	r.detail("OS ABI", strings.TrimPrefix(f.OSABI.String(), "ELFOSABI_"))
	r.detail("Sections", fmt.Sprint(len(f.Sections)))
}

func refineMachO(p string, h []byte, r *Result) {
	if bytes.HasPrefix(h, []byte("\xCA\xFE\xBA\xBE")) {
		if ff, err := macho.OpenFat(p); err == nil {
			var archs []string
			for _, a := range ff.Arches {
				archs = append(archs, strings.TrimPrefix(a.Cpu.String(), "Cpu"))
			}
			ff.Close()
			r.set("Mach-O universal binary", "macho", "application/x-mach-binary", CatExecutable, 0.97, SrcStructure)
			r.detail("Architectures", strings.Join(archs, ", "))
			return
		}
		// Java class: major version follows the magic.
		if len(h) > 8 {
			r.set("Java class file", "class", "application/java-vm", CatExecutable, 0.95, SrcStructure)
			r.detail("Class version", fmt.Sprint(binary.BigEndian.Uint16(h[6:])))
		}
		return
	}
	if f, err := macho.Open(p); err == nil {
		r.detail("Architecture", strings.TrimPrefix(f.Cpu.String(), "Cpu"))
		r.detail("Type", f.Type.String())
		f.Close()
	}
}

// refineCompressed peeks into a compressed stream to spot tarballs and
// report the original name.
func refineCompressed(p string, r *Result) {
	f, err := os.Open(p)
	if err != nil {
		return
	}
	defer f.Close()
	var rd io.Reader
	switch r.Ext {
	case "gz":
		gr, err := gzip.NewReader(f)
		if err != nil {
			return
		}
		if gr.Name != "" {
			r.detail("Original name", gr.Name)
		}
		rd = gr
	case "bz2", "xz", "zst":
		cr, done, err := codec.Open(r.Ext, f)
		if err != nil {
			return
		}
		defer done()
		rd = cr
	default:
		return
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(rd, head)
	head = head[:n]
	if n >= 262 && string(head[257:262]) == "ustar" {
		r.Label = "TAR archive (" + strings.ToUpper(r.Ext) + " compressed)"
		r.Ext = "tar." + r.Ext
		r.Category = CatArchive
		return
	}
	if inner := matchSig(head); inner != nil {
		r.detail("Contains", inner.label)
	}
}

func refineISO(h []byte, r *Result) bool {
	for _, off := range []int{0x8001, 0x8801, 0x9001} {
		if len(h) >= off+5 && string(h[off:off+5]) == "CD001" {
			r.set("ISO 9660 disc image", "iso", "application/x-iso9660-image", CatDiskImage, 0.97, SrcStructure)
			vol := strings.TrimSpace(string(h[0x8028:0x8048]))
			r.detail("Volume", vol)
			return true
		}
		if len(h) >= off+5 && (string(h[off:off+5]) == "BEA01" || string(h[off:off+5]) == "NSR02" || string(h[off:off+5]) == "NSR03") {
			r.set("UDF disc image", "iso", "application/x-iso9660-image", CatDiskImage, 0.95, SrcStructure)
			return true
		}
	}
	return false
}

func refineMBR(h []byte, r *Result) bool {
	if len(h) < 512 || h[510] != 0x55 || h[511] != 0xAA {
		return false
	}
	if len(h) >= 520 && string(h[512:520]) == "EFI PART" {
		r.set("Disk image (GPT)", "img", "application/octet-stream", CatDiskImage, 0.9, SrcStructure)
		return true
	}
	if string(h[3:11]) == "NTFS    " {
		r.set("NTFS volume image", "img", "application/octet-stream", CatDiskImage, 0.9, SrcStructure)
		return true
	}
	if string(h[0x36:0x3B]) == "FAT12" || string(h[0x36:0x3B]) == "FAT16" || string(h[0x52:0x57]) == "FAT32" {
		r.set("FAT volume image", "img", "application/octet-stream", CatDiskImage, 0.9, SrcStructure)
		return true
	}
	// A plausible partition table: status bytes 0x00 or 0x80.
	for i := 0; i < 4; i++ {
		s := h[446+16*i]
		if s != 0 && s != 0x80 {
			return false
		}
	}
	if h[450] == 0 && h[466] == 0 {
		return false
	}
	r.set("Disk image (MBR)", "img", "application/octet-stream", CatDiskImage, 0.75, SrcStructure)
	return true
}

func extOf(name string) string {
	return strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
}

func humanSize(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(u), 0
	for m := n / u; m >= u; m /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// HumanSize formats a byte count.
func HumanSize(n int64) string { return humanSize(n) }
