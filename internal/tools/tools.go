// Package tools locates the optional external programs AnyFile can
// delegate to: TrID for identification, 7-Zip for exotic archives and
// FFmpeg for media streams. Package install can download each of them
// into ManagedDir. Every tool is optional; the app degrades to
// its built-in engines when one is missing.
package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/xmhrnx/anyfile/internal/settings"
)

// Tool is one external program.
type Tool struct {
	Name string // display name
	Path string // resolved executable, empty when not found
}

// Found reports whether the tool is available.
func (t Tool) Found() bool { return t.Path != "" }

// Managed reports whether the tool is AnyFile's own install of it.
func (t Tool) Managed() bool {
	rel, err := filepath.Rel(ManagedDir(), t.Path)
	return t.Path != "" && err == nil && !strings.HasPrefix(rel, "..")
}

var (
	mu                                      sync.Mutex
	loaded                                  bool
	trid, magika, sevenZip, ffprobe, ffmpeg Tool
)

// Reload finds the tools again, after the paths in the settings change.
func Reload() {
	mu.Lock()
	loaded = false
	mu.Unlock()
	load()
}

// TrID returns the TrID command-line identifier.
func TrID() Tool { load(); mu.Lock(); defer mu.Unlock(); return trid }

// Magika returns Google's Magika content-type classifier.
func Magika() Tool { load(); mu.Lock(); defer mu.Unlock(); return magika }

// SevenZip returns the 7-Zip console program.
func SevenZip() Tool { load(); mu.Lock(); defer mu.Unlock(); return sevenZip }

// FFprobe returns ffprobe.
func FFprobe() Tool { load(); mu.Lock(); defer mu.Unlock(); return ffprobe }

// FFmpeg returns ffmpeg.
func FFmpeg() Tool { load(); mu.Lock(); defer mu.Unlock(); return ffmpeg }

// All lists every tool in display order.
func All() []Tool {
	load()
	mu.Lock()
	defer mu.Unlock()
	return []Tool{trid, magika, sevenZip, ffprobe, ffmpeg}
}

func load() {
	mu.Lock()
	defer mu.Unlock()
	if loaded {
		return
	}
	loaded = true
	cfg := settings.Get()
	func() {
		trid = Tool{Name: "TrID", Path: find(cfg.TrIDPath, "ANYFILE_TRID", "trid", []string{"trid"}, tridDirs())}
		// TrID is useless without its definitions database.
		if trid.Path != "" {
			if _, err := os.Stat(filepath.Join(filepath.Dir(trid.Path), "triddefs.trd")); err != nil {
				trid.Path = ""
			}
		}
		magika = Tool{Name: "Magika", Path: find(cfg.MagikaPath, "ANYFILE_MAGIKA", "magika", []string{"magika"}, magikaDirs())}
		sevenZip = Tool{Name: "7-Zip", Path: find(cfg.SevenZipPath, "ANYFILE_7Z", "7zip", []string{"7z", "7zz", "7za"}, sevenZipDirs())}
		ffprobe = Tool{Name: "ffprobe", Path: find(cfg.FFprobePath, "ANYFILE_FFPROBE", "ffmpeg", []string{"ffprobe"}, nil)}
		ffmpeg = Tool{Name: "FFmpeg", Path: find(cfg.FFmpegPath, "ANYFILE_FFMPEG", "ffmpeg", []string{"ffmpeg"}, nil)}
	}()
}

// find resolves a tool from the path set in the settings, an
// environment override, AnyFile's own install of it (managed, see
// ManagedDir), the directory of the running executable (so tools can
// ship alongside AnyFile), a "tools" subdirectory, PATH and finally
// well-known install locations.
func find(configured, env, managed string, names []string, dirs []string) string {
	if isFile(configured) {
		return configured
	}
	if p := os.Getenv(env); p != "" {
		if isFile(p) {
			return p
		}
	}
	if m, ok := ReadManifest(managed); ok {
		for _, n := range names {
			if rel, ok := m.Files[n]; ok {
				if p := filepath.Join(ManagedDir(), managed, rel); isFile(p) {
					return p
				}
			}
		}
	}
	var local []string
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		local = append(local, base, filepath.Join(base, "tools"))
		for _, n := range names {
			local = append(local, filepath.Join(base, "tools", n))
		}
	}
	for _, d := range append(local, dirs...) {
		for _, n := range names {
			if p := filepath.Join(d, exeName(n)); isFile(p) {
				return p
			}
		}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil && isFile(p) {
			return p
		}
	}
	return ""
}

func exeName(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func tridDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, "Downloads", "trid_w32"),
		filepath.Join(home, "Downloads", "trid"),
		filepath.Join(home, "trid"),
		`C:\trid`,
		`C:\Program Files\TrID`,
		`C:\Program Files (x86)\TrID`,
	}
}

func sevenZipDirs() []string {
	return []string{
		`C:\Program Files\7-Zip`,
		`C:\Program Files (x86)\7-Zip`,
		`C:\ProgramData\chocolatey\bin`,
		"/opt/homebrew/bin",
		"/usr/local/bin",
	}
}

// magikaDirs are where pip and cargo put the magika command when their
// script folders aren't on PATH.
func magikaDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(home, ".cargo", "bin"), filepath.Join(home, ".local", "bin"), "/opt/homebrew/bin", "/usr/local/bin"}
	for _, base := range []string{os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA")} {
		if base == "" {
			continue
		}
		// Per-user pip installs: %APPDATA%\Python\Python3xx\Scripts and
		// the Microsoft Store layout under %LOCALAPPDATA%.
		matches, _ := filepath.Glob(filepath.Join(base, "Python", "Python3*", "Scripts"))
		dirs = append(dirs, matches...)
		matches, _ = filepath.Glob(filepath.Join(base, "Programs", "Python", "Python3*", "Scripts"))
		dirs = append(dirs, matches...)
	}
	return dirs
}
