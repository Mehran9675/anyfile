package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// ManifestName is the file in an installed tool's folder that says what
// AnyFile put there.
const ManifestName = "anyfile-install.json"

// Manifest describes a tool AnyFile downloaded and installed itself.
type Manifest struct {
	// Version is the first line the tool prints about itself.
	Version string `json:"version"`
	// Source is the page or release it came from.
	Source    string    `json:"source"`
	Installed time.Time `json:"installed"`
	// Files maps command names ("ffprobe") to paths relative to the
	// tool's folder.
	Files map[string]string `json:"files"`
}

// ManagedDir is where AnyFile installs tools, one folder per tool ID
// ("trid", "magika", "7zip", "ffmpeg"): per user, without
// administrator rights, and outside roaming profiles on Windows.
func ManagedDir() string {
	var base string
	switch runtime.GOOS {
	case "windows":
		base = os.Getenv("LOCALAPPDATA")
	case "darwin":
		base, _ = os.UserConfigDir() // ~/Library/Application Support
	default:
		base = os.Getenv("XDG_DATA_HOME")
		if base == "" {
			if home, err := os.UserHomeDir(); err == nil {
				base = filepath.Join(home, ".local", "share")
			}
		}
	}
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "AnyFile", "tools")
}

// ReadManifest returns the manifest of the tool AnyFile installed under
// id, if there is one.
func ReadManifest(id string) (Manifest, bool) {
	var m Manifest
	if id == "" {
		return m, false
	}
	b, err := os.ReadFile(filepath.Join(ManagedDir(), id, ManifestName))
	if err != nil || json.Unmarshal(b, &m) != nil {
		return m, false
	}
	return m, true
}
