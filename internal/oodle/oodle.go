// Package oodle decompresses Oodle (Kraken, Mermaid, Selkie, Leviathan)
// data using the oo2core library that games ship with. Oodle is
// proprietary, so AnyFile never bundles it: it uses the copy already
// on the machine, found next to AnyFile, through ANYFILE_OODLE, or in
// the folders around the file being inspected.
package oodle

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/xmhrnx/anyfile/internal/settings"
)

// ErrMissing means no Oodle library could be found.
var ErrMissing = errors.New("Oodle is needed to decompress this data: place oo2core_*_win64.dll next to AnyFile or set ANYFILE_OODLE")

var (
	mu    sync.Mutex
	found = map[string]string{} // search dir -> library ("" when none)
	hints []string
)

// Hint adds a file whose surroundings may hold the library. AnyFile
// hints the opened file, so members extracted to a temp folder still
// find the DLL shipped next to the original.
func Hint(path string) {
	mu.Lock()
	defer mu.Unlock()
	for _, h := range hints {
		if h == path {
			return
		}
	}
	hints = append(hints, path)
}

// Find returns the Oodle library to use for a file at near, or "".
func Find(near string) string {
	if p := settings.Get().OodlePath; p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p := os.Getenv("ANYFILE_OODLE"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe), filepath.Join(filepath.Dir(exe), "tools"))
	}
	mu.Lock()
	near2 := append([]string{near}, hints...)
	mu.Unlock()
	for _, near := range near2 {
		if near == "" {
			continue
		}
		// The file's folder and two levels up: games keep the DLL in
		// their root or binary folder.
		d := filepath.Dir(near)
		for i := 0; i < 3; i++ {
			dirs = append(dirs, d)
			up := filepath.Dir(d)
			if up == d {
				break
			}
			d = up
		}
	}
	for _, d := range dirs {
		if p := inDir(d); p != "" {
			return p
		}
	}
	return ""
}

// inDir returns the newest oo2core library in dir.
func inDir(dir string) string {
	mu.Lock()
	defer mu.Unlock()
	if p, ok := found[dir]; ok {
		return p
	}
	ents, _ := os.ReadDir(dir)
	var libs []string
	for _, e := range ents {
		n := strings.ToLower(e.Name())
		if strings.HasPrefix(n, "oo2core_") && (strings.HasSuffix(n, "_win64.dll") || strings.HasSuffix(n, ".so") || strings.HasSuffix(n, ".dylib")) {
			libs = append(libs, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(libs) // oo2core_9 sorts after oo2core_8
	p := ""
	if len(libs) > 0 {
		p = libs[len(libs)-1]
	}
	found[dir] = p
	return p
}
