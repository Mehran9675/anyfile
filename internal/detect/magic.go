package detect

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// sig is one fixed signature. Structural refiners (refine.go) take over
// for families that share a signature (ZIP, OLE, RIFF, ISO-BMFF...).
type sig struct {
	off   int
	magic string
	ext   string
	label string
	mime  string
	cat   Category
	conf  float64
	check func(h []byte) bool
}

// Ordered: longer, more specific signatures come first.
var sigs = []sig{
	// Archives and compression
	{0, "PK\x03\x04", "zip", "ZIP archive", "application/zip", CatArchive, 0.9, nil},
	{0, "PK\x05\x06", "zip", "ZIP archive (empty)", "application/zip", CatArchive, 0.9, nil},
	{0, "PK\x07\x08", "zip", "ZIP archive (spanned)", "application/zip", CatArchive, 0.9, nil},
	{0, "7z\xBC\xAF\x27\x1C", "7z", "7-Zip archive", "application/x-7z-compressed", CatArchive, 0.97, nil},
	{0, "Rar!\x1A\x07\x01\x00", "rar", "RAR 5 archive", "application/vnd.rar", CatArchive, 0.97, nil},
	{0, "Rar!\x1A\x07\x00", "rar", "RAR archive", "application/vnd.rar", CatArchive, 0.97, nil},
	{0, "MSCF\x00\x00\x00\x00", "cab", "Microsoft Cabinet", "application/vnd.ms-cab-compressed", CatArchive, 0.97, nil},
	{0, "ISc(", "cab", "InstallShield Cabinet", "application/octet-stream", CatArchive, 0.9, nil},
	{0, "\x1F\x8B\x08", "gz", "GZIP compressed data", "application/gzip", CatCompressed, 0.95, nil},
	{0, "BZh", "bz2", "BZIP2 compressed data", "application/x-bzip2", CatCompressed, 0.93, func(h []byte) bool {
		return len(h) > 9 && h[3] >= '1' && h[3] <= '9' && string(h[4:10]) == "1AY&SY"
	}},
	{0, "\xFD7zXZ\x00", "xz", "XZ compressed data", "application/x-xz", CatCompressed, 0.97, nil},
	{0, "\x28\xB5\x2F\xFD", "zst", "Zstandard compressed data", "application/zstd", CatCompressed, 0.95, nil},
	{0, "\x04\x22\x4D\x18", "lz4", "LZ4 frame", "application/x-lz4", CatCompressed, 0.95, nil},
	{0, "LZIP", "lz", "LZIP compressed data", "application/x-lzip", CatCompressed, 0.95, nil},
	{0, "\x5D\x00\x00", "lzma", "LZMA compressed data", "application/x-lzma", CatCompressed, 0.6, nil},
	{0, "\x1F\x9D", "Z", "UNIX compress data", "application/x-compress", CatCompressed, 0.7, nil},
	{0, "!<arch>\ndebian", "deb", "Debian package", "application/vnd.debian.binary-package", CatArchive, 0.97, nil},
	{0, "!<arch>\n", "a", "ar archive / static library", "application/x-archive", CatArchive, 0.95, nil},
	{0, "\xED\xAB\xEE\xDB", "rpm", "RPM package", "application/x-rpm", CatArchive, 0.97, nil},
	{0, "070701", "cpio", "cpio archive", "application/x-cpio", CatArchive, 0.9, nil},
	{0, "070707", "cpio", "cpio archive", "application/x-cpio", CatArchive, 0.9, nil},
	{0, "xar!", "xar", "XAR archive", "application/x-xar", CatArchive, 0.95, nil},
	{0, "hsqs", "squashfs", "SquashFS filesystem", "application/octet-stream", CatDiskImage, 0.9, nil},
	{0, "MSWIM\x00\x00\x00", "wim", "Windows Imaging (WIM)", "application/x-ms-wim", CatDiskImage, 0.97, nil},
	{0, "conectix", "vhd", "Virtual PC disk (VHD)", "application/x-vhd", CatDiskImage, 0.95, nil},
	{0, "vhdxfile", "vhdx", "Hyper-V disk (VHDX)", "application/x-vhdx", CatDiskImage, 0.97, nil},
	{0, "KDMV", "vmdk", "VMware disk (VMDK)", "application/x-vmdk", CatDiskImage, 0.95, nil},
	{0, "QFI\xFB", "qcow2", "QEMU disk (QCOW2)", "application/x-qemu-disk", CatDiskImage, 0.95, nil},
	{0, "KAPI", "xsub", "Call of Duty XSUB stream package", "application/octet-stream", CatArchive, 0.95, nil},
	{0, "ITSF", "chm", "Compiled HTML Help", "application/vnd.ms-htmlhelp", CatDocument, 0.95, nil},

	// Documents
	{0, "%PDF-", "pdf", "PDF document", "application/pdf", CatDocument, 0.98, nil},
	{0, "{\\rtf", "rtf", "Rich Text Format", "application/rtf", CatDocument, 0.95, nil},
	{0, "AT&TFORM", "djvu", "DjVu document", "image/vnd.djvu", CatDocument, 0.95, nil},
	{0, "%!PS", "ps", "PostScript document", "application/postscript", CatDocument, 0.9, nil},
	{0, "\xC5\xD0\xD3\xC6", "eps", "Encapsulated PostScript (binary)", "application/postscript", CatDocument, 0.9, nil},
	{0, "BOOKMOBI", "mobi", "Mobipocket e-book", "application/x-mobipocket-ebook", CatDocument, 0.9, nil},
	{60, "BOOKMOBI", "mobi", "Mobipocket e-book", "application/x-mobipocket-ebook", CatDocument, 0.95, nil},
	{60, "TPZ3", "azw", "Kindle e-book", "application/vnd.amazon.ebook", CatDocument, 0.9, nil},

	// Images
	{0, "\x89PNG\r\n\x1A\n", "png", "PNG image", "image/png", CatImage, 0.98, nil},
	{0, "\xFF\xD8\xFF", "jpg", "JPEG image", "image/jpeg", CatImage, 0.95, nil},
	{0, "GIF87a", "gif", "GIF image", "image/gif", CatImage, 0.98, nil},
	{0, "GIF89a", "gif", "GIF image", "image/gif", CatImage, 0.98, nil},
	{0, "II*\x00", "tif", "TIFF image", "image/tiff", CatImage, 0.85, nil},
	{0, "MM\x00*", "tif", "TIFF image", "image/tiff", CatImage, 0.85, nil},
	{0, "II\x2A\x00\x10\x00\x00\x00CR", "cr2", "Canon RAW image", "image/x-canon-cr2", CatImage, 0.95, nil},
	{0, "8BPS", "psd", "Photoshop image", "image/vnd.adobe.photoshop", CatImage, 0.95, nil},
	{0, "\x00\x00\x01\x00", "ico", "Windows icon", "image/x-icon", CatImage, 0.7, func(h []byte) bool {
		return len(h) > 6 && h[4] != 0 && h[5] == 0
	}},
	{0, "\x00\x00\x02\x00", "cur", "Windows cursor", "image/x-icon", CatImage, 0.7, func(h []byte) bool {
		return len(h) > 6 && h[4] != 0 && h[5] == 0
	}},
	{0, "BM", "bmp", "BMP image", "image/bmp", CatImage, 0.85, validBMP},
	{0, "\x76\x2F\x31\x01", "exr", "OpenEXR image", "image/x-exr", CatImage, 0.95, nil},
	{0, "#?RADIANCE", "hdr", "Radiance HDR image", "image/vnd.radiance", CatImage, 0.95, nil},
	{0, "\x00\x00\x00\x0CjP  \r\n\x87\n", "jp2", "JPEG 2000 image", "image/jp2", CatImage, 0.98, nil},
	{0, "\xFF\x0A", "jxl", "JPEG XL image", "image/jxl", CatImage, 0.8, nil},
	{0, "\x00\x00\x00\x0CJXL \r\n\x87\n", "jxl", "JPEG XL image", "image/jxl", CatImage, 0.98, nil},
	{0, "gimp xcf", "xcf", "GIMP image", "image/x-xcf", CatImage, 0.95, nil},
	{0, "qoif", "qoi", "QOI image", "image/qoi", CatImage, 0.95, nil},
	{0, "DDS ", "dds", "DirectDraw Surface", "image/vnd.ms-dds", CatImage, 0.95, nil},
	{0, "\x01\xDA", "sgi", "SGI image", "image/sgi", CatImage, 0.5, nil},
	{0, "P1\n", "pbm", "Netpbm bitmap", "image/x-portable-bitmap", CatImage, 0.6, nil},
	{0, "P4\n", "pbm", "Netpbm bitmap", "image/x-portable-bitmap", CatImage, 0.6, nil},
	{0, "P5\n", "pgm", "Netpbm graymap", "image/x-portable-graymap", CatImage, 0.6, nil},
	{0, "P6\n", "ppm", "Netpbm pixmap", "image/x-portable-pixmap", CatImage, 0.6, nil},
	{0, "\x0A\x05\x01\x08", "pcx", "PCX image", "image/x-pcx", CatImage, 0.6, nil},

	// Audio / video
	{0, "ID3", "mp3", "MP3 audio (ID3 tagged)", "audio/mpeg", CatAudio, 0.9, nil},
	{0, "fLaC", "flac", "FLAC audio", "audio/flac", CatAudio, 0.97, nil},
	{0, "MThd", "mid", "MIDI audio", "audio/midi", CatAudio, 0.95, nil},
	{0, "#!AMR", "amr", "AMR audio", "audio/amr", CatAudio, 0.95, nil},
	{0, "FORM", "aiff", "AIFF audio", "audio/aiff", CatAudio, 0.8, func(h []byte) bool {
		return len(h) > 12 && (string(h[8:12]) == "AIFF" || string(h[8:12]) == "AIFC")
	}},
	{0, "MAC ", "ape", "Monkey's Audio", "audio/ape", CatAudio, 0.9, nil},
	{0, "wvpk", "wv", "WavPack audio", "audio/wavpack", CatAudio, 0.9, nil},
	{0, ".snd", "au", "Sun audio", "audio/basic", CatAudio, 0.9, nil},
	{0, "FLV\x01", "flv", "Flash video", "video/x-flv", CatVideo, 0.95, nil},
	{0, "\x30\x26\xB2\x75\x8E\x66\xCF\x11", "wmv", "Windows Media (ASF)", "video/x-ms-asf", CatVideo, 0.95, nil},
	{0, "\x00\x00\x01\xBA", "mpg", "MPEG program stream", "video/mpeg", CatVideo, 0.9, nil},
	{0, "\x00\x00\x01\xB3", "mpg", "MPEG video", "video/mpeg", CatVideo, 0.9, nil},
	{0, "G", "ts", "MPEG transport stream", "video/mp2t", CatVideo, 0.8, func(h []byte) bool {
		return len(h) > 188*3 && h[188] == 'G' && h[376] == 'G'
	}},
	{0, "\x2E\x52\x4D\x46", "rm", "RealMedia", "application/vnd.rn-realmedia", CatVideo, 0.95, nil},

	// Executables and code
	{0, "\x7FELF", "elf", "ELF executable", "application/x-executable", CatExecutable, 0.95, nil},
	{0, "\xCA\xFE\xBA\xBE", "class", "Java class / Mach-O universal", "application/java-vm", CatExecutable, 0.7, nil},
	{0, "\xFE\xED\xFA\xCE", "macho", "Mach-O executable (32-bit)", "application/x-mach-binary", CatExecutable, 0.95, nil},
	{0, "\xFE\xED\xFA\xCF", "macho", "Mach-O executable (64-bit)", "application/x-mach-binary", CatExecutable, 0.95, nil},
	{0, "\xCE\xFA\xED\xFE", "macho", "Mach-O executable (32-bit)", "application/x-mach-binary", CatExecutable, 0.95, nil},
	{0, "\xCF\xFA\xED\xFE", "macho", "Mach-O executable (64-bit)", "application/x-mach-binary", CatExecutable, 0.95, nil},
	{0, "dex\n", "dex", "Android Dalvik executable", "application/vnd.android.dex", CatExecutable, 0.95, nil},
	{0, "\x00asm", "wasm", "WebAssembly module", "application/wasm", CatExecutable, 0.97, nil},
	{0, "\x4C\x00\x00\x00\x01\x14\x02\x00", "lnk", "Windows shortcut", "application/x-ms-shortcut", CatData, 0.97, nil},
	{0, "\x1BLua", "luac", "Lua bytecode", "application/x-lua-bytecode", CatExecutable, 0.9, nil},
	{0, "FWS", "swf", "Flash movie", "application/x-shockwave-flash", CatVideo, 0.85, nil},
	{0, "CWS", "swf", "Flash movie (compressed)", "application/x-shockwave-flash", CatVideo, 0.85, nil},
	{0, "ZWS", "swf", "Flash movie (LZMA)", "application/x-shockwave-flash", CatVideo, 0.85, nil},

	// Fonts
	{0, "wOFF", "woff", "WOFF font", "font/woff", CatFont, 0.97, nil},
	{0, "wOF2", "woff2", "WOFF2 font", "font/woff2", CatFont, 0.97, nil},
	{0, "OTTO", "otf", "OpenType font (CFF)", "font/otf", CatFont, 0.95, nil},
	{0, "\x00\x01\x00\x00", "ttf", "TrueType font", "font/ttf", CatFont, 0.75, validSFNT},
	{0, "true", "ttf", "TrueType font (Mac)", "font/ttf", CatFont, 0.7, validSFNT},
	{0, "ttcf", "ttc", "TrueType collection", "font/collection", CatFont, 0.95, nil},
	{0, "\x80\x01", "pfb", "PostScript Type 1 font", "application/x-font-type1", CatFont, 0.5, nil},

	// Databases and data
	{0, "SQLite format 3\x00", "sqlite", "SQLite database", "application/vnd.sqlite3", CatDatabase, 0.99, nil},
	{0, "\x00\x01\x00\x00Standard Jet DB", "mdb", "Access database", "application/x-msaccess", CatDatabase, 0.97, nil},
	{0, "\x00\x01\x00\x00Standard ACE DB", "accdb", "Access database", "application/x-msaccess", CatDatabase, 0.97, nil},
	{0, "regf", "dat", "Windows registry hive", "application/octet-stream", CatDatabase, 0.95, nil},
	{0, "ElfFile\x00", "evtx", "Windows event log", "application/octet-stream", CatDatabase, 0.95, nil},
	{0, "PAR1", "parquet", "Apache Parquet", "application/vnd.apache.parquet", CatDatabase, 0.9, nil},
	{0, "Obj\x01", "avro", "Apache Avro", "application/avro", CatDatabase, 0.9, nil},
	{0, "\x89HDF\r\n\x1A\n", "h5", "HDF5 data", "application/x-hdf5", CatData, 0.97, nil},
	{0, "\x93NUMPY", "npy", "NumPy array", "application/octet-stream", CatData, 0.97, nil},
	{0, "\xD4\xC3\xB2\xA1", "pcap", "Packet capture", "application/vnd.tcpdump.pcap", CatData, 0.95, nil},
	{0, "\xA1\xB2\xC3\xD4", "pcap", "Packet capture", "application/vnd.tcpdump.pcap", CatData, 0.95, nil},
	{0, "\x0A\x0D\x0D\x0A", "pcapng", "Packet capture (pcapng)", "application/octet-stream", CatData, 0.95, nil},
	{0, "bplist00", "plist", "Binary property list", "application/x-bplist", CatData, 0.97, nil},
	{0, "glTF", "glb", "glTF binary 3D model", "model/gltf-binary", CatData, 0.95, nil},
	{0, "Kaydara FBX Binary", "fbx", "FBX 3D model", "application/octet-stream", CatData, 0.97, nil},
	{0, "solid ", "stl", "STL 3D model (ASCII)", "model/stl", CatData, 0.6, nil},
	{0, "BLENDER", "blend", "Blender scene", "application/x-blender", CatData, 0.97, nil},
	{0, "torrent", "torrent", "BitTorrent file", "application/x-bittorrent", CatData, 0.4, nil},
	{0, "d8:announce", "torrent", "BitTorrent file", "application/x-bittorrent", CatData, 0.95, nil},

	// Security
	{0, "-----BEGIN CERTIFICATE-----", "crt", "X.509 certificate (PEM)", "application/x-x509-ca-cert", CatCrypto, 0.98, nil},
	{0, "-----BEGIN PGP", "asc", "OpenPGP armored data", "application/pgp-encrypted", CatCrypto, 0.95, nil},
	{0, "-----BEGIN ", "pem", "PEM encoded data", "application/x-pem-file", CatCrypto, 0.9, nil},
	{0, "ssh-rsa ", "pub", "SSH public key", "text/plain", CatCrypto, 0.9, nil},
	{0, "ssh-ed25519 ", "pub", "SSH public key", "text/plain", CatCrypto, 0.9, nil},
	{0, "openssh-key-v1", "key", "OpenSSH private key", "text/plain", CatCrypto, 0.9, nil},
	{0, "KeePass", "kdbx", "KeePass database", "application/x-keepass2", CatCrypto, 0.6, nil},
	{0, "\x03\xD9\xA2\x9A\x67\xFB\x4B\xB5", "kdbx", "KeePass database", "application/x-keepass2", CatCrypto, 0.97, nil},
	{0, "-FVE-FS-", "", "BitLocker volume", "application/octet-stream", CatEncrypted, 0.95, nil},
	{0, "LUKS\xBA\xBE", "luks", "LUKS encrypted volume", "application/octet-stream", CatEncrypted, 0.97, nil},
}

func validBMP(h []byte) bool {
	if len(h) < 30 {
		return false
	}
	off := binary.LittleEndian.Uint32(h[10:])
	dib := binary.LittleEndian.Uint32(h[14:])
	switch dib {
	case 12, 40, 52, 56, 64, 108, 124:
	default:
		return false
	}
	return binary.LittleEndian.Uint32(h[6:]) == 0 && off >= 14+dib
}

func validSFNT(h []byte) bool {
	if len(h) < 12 {
		return false
	}
	n := int(binary.BigEndian.Uint16(h[4:]))
	if n < 4 || n > 64 || len(h) < 12+16*n {
		return n >= 4 && n <= 64 // too short to verify the table list; accept the count
	}
	for i := 0; i < n; i++ {
		tag := h[12+16*i : 16+16*i]
		for _, c := range tag {
			if c < 0x20 || c > 0x7E {
				return false
			}
		}
	}
	return true
}

func matchSig(h []byte) *sig {
	for i := range sigs {
		s := &sigs[i]
		if s.label == "" {
			continue
		}
		end := s.off + len(s.magic)
		if len(h) < end || !bytes.Equal(h[s.off:end], []byte(s.magic)) {
			continue
		}
		if s.check != nil && !s.check(h) {
			continue
		}
		return s
	}
	return nil
}

// SniffBytes identifies an in-memory buffer by signature alone, for
// callers that decode data (base64, PDF streams) before saving it.
func SniffBytes(b []byte) (ext, label string, ok bool) {
	if s := matchSig(b); s != nil {
		return s.ext, s.label, true
	}
	switch {
	case len(b) >= 12 && string(b[:4]) == "RIFF":
		return map[string]string{"WAVE": "wav", "AVI ": "avi", "WEBP": "webp"}[string(b[8:12])], "RIFF " + string(b[8:12]), true
	case len(b) >= 12 && string(b[4:8]) == "ftyp":
		return "mp4", "MPEG-4 media", true
	case len(b) >= 8 && string(b[:8]) == "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1":
		return "ole", "OLE compound file", true
	}
	if ti, ok := analyzeText(b[:min(len(b), 4096)]); ok {
		low := strings.ToLower(strings.TrimSpace(ti.text))
		switch {
		case strings.Contains(low[:min(len(low), 512)], "<svg"):
			return "svg", "SVG image", true
		case strings.HasPrefix(low, "<"):
			return "xml", "XML/HTML text", true
		case strings.HasPrefix(low, "{") || strings.HasPrefix(low, "["):
			return "json", "JSON", true
		}
		return "txt", "Text", true
	}
	return "bin", "Binary data", false
}
