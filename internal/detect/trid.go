package detect

import (
	"bufio"
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xmhrnx/anyfile/internal/settings"

	"github.com/xmhrnx/anyfile/internal/tools"
)

// Matches lines such as:
//
//	44.4% (.EXE) Win64 Executable (generic) (10523/12/4)
var reTrID = regexp.MustCompile(`^\s*([\d.]+)%\s+\(([^)]*)\)\s+(.*?)\s+\((\d+)(?:/(\d+))?(?:/\d+)*\)\s*$`)

// runTrID asks TrID for its ranked guesses. ok is false when TrID is
// not installed or could not run; an empty slice means "Unknown!".
func runTrID(p string) (cands []Candidate, ok bool) {
	t := tools.TrID()
	if !t.Found() {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(settings.Get().TrIDTimeoutSec)*time.Second)
	defer cancel()
	cmd := tools.Command(t.Path, p, "-n:8")
	cmd.Dir = filepath.Dir(t.Path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return nil, false
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-ctx.Done():
		cmd.Process.Kill()
		return nil, false
	case <-done:
	}
	sc := bufio.NewScanner(&out)
	for sc.Scan() {
		m := reTrID.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		pct, _ := strconv.ParseFloat(m[1], 64)
		ext := strings.ToLower(strings.TrimPrefix(strings.SplitN(m[2], "/", 2)[0], "."))
		pts, _ := strconv.Atoi(m[4])
		pats, _ := strconv.Atoi(m[5])
		cands = append(cands, Candidate{Percent: pct, Ext: ext, Label: m[3], Points: pts, Patterns: pats})
	}
	return cands, true
}

// tridCategory guesses a category from TrID's free-form label.
func tridCategory(label, ext string) Category {
	l := strings.ToLower(label + " " + ext)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(l, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("archive", "compressed", "package", "installer", "zip", "cabinet"):
		return CatArchive
	case has("spreadsheet", "excel", "workbook"):
		return CatSpreadsheet
	case has("presentation", "powerpoint", "slides"):
		return CatPresentation
	case has("image", "bitmap", "icon", "picture", "photo", "graphic", "drawing"):
		return CatImage
	case has("audio", "sound", "music", "midi", "wave", "song"):
		return CatAudio
	case has("video", "movie", "animation", "film"):
		return CatVideo
	case has("executable", "library", "dll", "driver", "bytecode", "program", "binary"):
		return CatExecutable
	case has("font"):
		return CatFont
	case has("database", "db "):
		return CatDatabase
	case has("document", "text", "e-book", "ebook", "word", "pdf"):
		return CatDocument
	case has("disk", "iso", "volume", "filesystem"):
		return CatDiskImage
	case has("mail", "message"):
		return CatEmail
	case has("certificate", "key", "encrypted"):
		return CatCrypto
	}
	return CatData
}
