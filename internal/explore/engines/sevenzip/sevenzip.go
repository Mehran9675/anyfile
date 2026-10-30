// Package sevenzip delegates to 7-Zip, when installed, for the many
// archive and disk-image formats it understands (7z, RAR, CAB, WIM,
// VHD, DMG, NSIS installers, ...).
package sevenzip

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/xmhrnx/anyfile/internal/detect"
	"github.com/xmhrnx/anyfile/internal/explore"
	"github.com/xmhrnx/anyfile/internal/tools"
)

func init() {
	explore.Register(explore.Engine{Name: "7-Zip", Priority: 100, Match: match7z, List: List})
}

// --- 7-Zip ----------------------------------------------------------------------

var sevenZipExts = map[string]bool{
	"7z": true, "rar": true, "cab": true, "iso": true, "wim": true, "chm": true, "msi": true, "msp": true,
	"vhd": true, "vhdx": true, "vmdk": true, "qcow2": true, "squashfs": true, "xar": true, "rpm": true,
	"cpio": true, "dmg": true, "img": true, "lzma": true, "lz": true, "Z": true, "arj": true, "lzh": true,
	"lha": true, "udf": true, "hfs": true, "apfs": true, "ext": true, "fat": true, "ntfs": true, "deb": true,
	"lz4": true, "swf": true, "flv": true, "zip": true, "jar": true, "apk": true, "epub": true,
}

func match7z(d *detect.Result, h []byte) bool {
	if !tools.SevenZip().Found() {
		return false
	}
	if d.Category == detect.CatExecutable {
		return false // the PE explorer merges 7-Zip's view of installers
	}
	switch d.Category {
	case detect.CatArchive, detect.CatCompressed, detect.CatDiskImage:
		return true
	}
	return sevenZipExts[d.Ext]
}

// List runs `7z l -slt` and parses the technical listing.
func List(p string, d *detect.Result) (*explore.Listing, error) {
	t := tools.SevenZip()
	if !t.Found() {
		return nil, errors.New("7-Zip not installed")
	}
	cmd := tools.Command(t.Path, "l", "-slt", "-sccUTF-8", "-p", p)
	cmd.Stdin = strings.NewReader("") // never wait for a password prompt
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("7-Zip could not open the file: %v", err)
	}
	l := &explore.Listing{Engine: "7-Zip"}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	inFiles := false
	var cur map[string]string
	flush := func() {
		if cur == nil || cur["Path"] == "" {
			cur = nil
			return
		}
		e := &explore.Entry{Path: cur["Path"], Dir: cur["Folder"] == "+", Size: -1}
		if n, err := strconv.ParseInt(cur["Size"], 10, 64); err == nil {
			e.Size = n
		}
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.SplitN(cur["Modified"], ".", 2)[0], time.Local); err == nil {
			e.ModTime = t
		}
		if cur["Encrypted"] == "+" {
			e.Note = "encrypted"
		}
		if !e.Dir {
			name := e.Path
			e.Open = func() (io.ReadCloser, error) { return sevenZipStream(t.Path, p, name) }
		}
		l.Entries = append(l.Entries, e)
		cur = nil
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "----------") {
			inFiles = true
			continue
		}
		if !inFiles {
			if k, v, ok := strings.Cut(line, " = "); ok && k == "Type" {
				l.Format = v
			}
			continue
		}
		if line == "" {
			flush()
			continue
		}
		if k, v, ok := strings.Cut(line, " = "); ok {
			if cur == nil {
				cur = map[string]string{}
			}
			cur[k] = v
		}
	}
	flush()
	if l.Format != "" {
		l.Engine = "7-Zip (" + l.Format + ")"
	}
	if len(l.Entries) == 0 {
		return l, errors.New("7-Zip found no entries")
	}
	return l, nil
}

func sevenZipStream(exe, archive, name string) (io.ReadCloser, error) {
	cmd := tools.Command(exe, "x", "-so", "-p", "-sccUTF-8", archive, "--", name)
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return explore.NewReadCloser(out, func() error { out.Close(); return cmd.Wait() }), nil
}
