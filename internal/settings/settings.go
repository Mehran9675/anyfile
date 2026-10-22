// Package settings holds the limits and preferences users can change
// on the settings page, persisted as JSON in the user's config folder.
// Every package reads the current values with Get, which is safe from
// any goroutine; Set applies new values immediately and saves them.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// TrID modes.
const (
	TrIDWhenNeeded = "auto"   // only when the built-in engines aren't sure
	TrIDAlways     = "always" // for every file
	TrIDNever      = "never"
)

// Settings are the user's choices. Sizes are in the unit their name
// says, so the JSON file stays readable.
type Settings struct {
	// Identification.
	HeadKB          int     `json:"head_kb"`           // bytes read from the start of a file for signatures and text analysis
	EntropySampleMB int     `json:"entropy_sample_mb"` // bytes sampled to estimate entropy
	TrIDMode        string  `json:"trid_mode"`
	TrIDTrust       float64 `json:"trid_trust"` // built-in confidence (0..1) at which TrID is skipped
	TrIDTimeoutSec  int     `json:"trid_timeout_sec"`

	// Exploring.
	PrefetchCount   int  `json:"prefetch_count"`    // members identified in the background
	PrefetchMaxMB   int  `json:"prefetch_max_mb"`   // larger members wait until selected
	AutoCarve       bool `json:"auto_carve"`        // scan unknown files for embedded objects without asking
	CarveLimit      int  `json:"carve_limit"`       // most embedded objects reported per scan
	MaxTextMB       int  `json:"max_text_mb"`       // largest text file scanned for encoded blobs
	MaxMailMB       int  `json:"max_mail_mb"`       // largest email or mailbox explored
	MaxPDFMB        int  `json:"max_pdf_mb"`        // largest PDF explored
	MaxDecompressMB int  `json:"max_decompress_mb"` // memory a decompressor may use (zstd window, xz dictionary)

	// Files.
	Checksums     bool   `json:"checksums"`       // compute MD5/SHA-1/SHA-256 when a file is selected
	ChecksumMaxMB int    `json:"checksum_max_mb"` // skip checksums above this size (0: no limit)
	TempDir       string `json:"temp_dir"`        // where members are extracted ("" = system temp)

	// External tools ("" = find automatically).
	TrIDPath     string `json:"trid_path"`
	SevenZipPath string `json:"sevenzip_path"`
	FFprobePath  string `json:"ffprobe_path"`
	FFmpegPath   string `json:"ffmpeg_path"`
	OodlePath    string `json:"oodle_path"`
}

// Defaults are the values a fresh install starts with.
func Defaults() Settings {
	return Settings{
		HeadKB:          64,
		EntropySampleMB: 4,
		TrIDMode:        TrIDWhenNeeded,
		TrIDTrust:       0.85,
		TrIDTimeoutSec:  45,
		PrefetchCount:   300,
		PrefetchMaxMB:   256,
		AutoCarve:       true,
		CarveLimit:      1000,
		MaxTextMB:       128,
		MaxMailMB:       512,
		MaxPDFMB:        1024,
		MaxDecompressMB: 2048,
		Checksums:       true,
		ChecksumMaxMB:   0,
	}
}

var (
	current atomic.Pointer[Settings]
	saveMu  sync.Mutex
	watchMu sync.Mutex
	watch   []func(Settings)
)

func init() {
	s := Defaults()
	current.Store(&s)
}

// Get returns the current settings.
func Get() Settings { return *current.Load() }

// Path is where settings are saved.
func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "AnyFile", "settings.json")
}

// Load reads saved settings, keeping defaults for anything missing or
// invalid.
func Load() error {
	b, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	s := Defaults()
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	s = s.Clamped()
	current.Store(&s)
	return nil
}

// Set applies s right away, notifies watchers and saves it.
func Set(s Settings) error {
	s = s.Clamped()
	current.Store(&s)
	watchMu.Lock()
	fns := append([]func(Settings){}, watch...)
	watchMu.Unlock()
	for _, fn := range fns {
		fn(s)
	}
	return save(s)
}

// Watch registers fn to run after every Set.
func Watch(fn func(Settings)) {
	watchMu.Lock()
	watch = append(watch, fn)
	watchMu.Unlock()
}

func save(s Settings) error {
	saveMu.Lock()
	defer saveMu.Unlock()
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Limits for each numeric setting: the settings page validates against
// them and Clamped enforces them on load.
type Limit struct{ Min, Max int }

var Limits = map[string]Limit{
	"head_kb":           {40, 4096}, // the ISO 9660 probe reads up to 36 KB in
	"entropy_sample_mb": {1, 1024},
	"trid_timeout_sec":  {5, 600},
	"prefetch_count":    {0, 100000},
	"prefetch_max_mb":   {1, 65536},
	"carve_limit":       {10, 100000},
	"max_text_mb":       {1, 16384},
	"max_mail_mb":       {1, 16384},
	"max_pdf_mb":        {1, 65536},
	"max_decompress_mb": {64, 65536},
	"checksum_max_mb":   {0, 1048576},
	"trid_trust_pct":    {10, 100}, // TrIDTrust shown as a percentage
}

func clamp(v int, l Limit) int { return max(l.Min, min(l.Max, v)) }

// Clamped returns s with every value inside its limits.
func (s Settings) Clamped() Settings {
	s.HeadKB = clamp(s.HeadKB, Limits["head_kb"])
	s.EntropySampleMB = clamp(s.EntropySampleMB, Limits["entropy_sample_mb"])
	s.TrIDTimeoutSec = clamp(s.TrIDTimeoutSec, Limits["trid_timeout_sec"])
	s.PrefetchCount = clamp(s.PrefetchCount, Limits["prefetch_count"])
	s.PrefetchMaxMB = clamp(s.PrefetchMaxMB, Limits["prefetch_max_mb"])
	s.CarveLimit = clamp(s.CarveLimit, Limits["carve_limit"])
	s.MaxTextMB = clamp(s.MaxTextMB, Limits["max_text_mb"])
	s.MaxMailMB = clamp(s.MaxMailMB, Limits["max_mail_mb"])
	s.MaxPDFMB = clamp(s.MaxPDFMB, Limits["max_pdf_mb"])
	s.MaxDecompressMB = clamp(s.MaxDecompressMB, Limits["max_decompress_mb"])
	s.ChecksumMaxMB = clamp(s.ChecksumMaxMB, Limits["checksum_max_mb"])
	if s.TrIDTrust < 0.1 || s.TrIDTrust > 1 {
		s.TrIDTrust = 0.85
	}
	switch s.TrIDMode {
	case TrIDWhenNeeded, TrIDAlways, TrIDNever:
	default:
		s.TrIDMode = TrIDWhenNeeded
	}
	return s
}

// MB converts a megabyte setting to bytes.
func MB(v int) int64 { return int64(v) << 20 }
