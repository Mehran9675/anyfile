// Package explore lists and extracts what is inside a file: archive
// members, document parts, embedded objects, media streams, executable
// resources...
//
// This package is the core: the Entry/Listing model, the engine
// registry and safe extraction. The engines themselves live in
// explore/engines/*, each registering itself from init; import
// explore/engines/all to enable every one of them.
package explore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mehran9675/anyfile/internal/detect"
)

// Entry is one item inside a file.
type Entry struct {
	Path    string // slash-separated path inside the container
	Size    int64  // -1 when unknown
	ModTime time.Time
	Dir     bool
	Note    string // short engine-specific remark (codec, offset...)
	Open    func() (io.ReadCloser, error)
}

// Name is the last path element.
func (e *Entry) Name() string { return path.Base(e.Path) }

// Listing is the contents of one file.
type Listing struct {
	Engine  string
	Format  string // format name reported by an external engine
	Entries []*Entry
	Notes   []string

	mu      sync.Mutex
	closers []func()
}

// OnClose registers fn to run when the listing is closed; engines use
// it to release file handles their entries read from.
func (l *Listing) OnClose(fn func()) {
	l.mu.Lock()
	l.closers = append(l.closers, fn)
	l.mu.Unlock()
}

// Close releases everything held open by the listing.
func (l *Listing) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	fns := l.closers
	l.closers = nil
	l.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// ErrNotContainer means no engine could find anything inside.
var ErrNotContainer = errors.New("no embedded content found")

// Engine is one format explorer.
type Engine struct {
	Name string
	// Priority orders engines; lower runs first. Built-in engines use
	// 0-99, external tools 100+.
	Priority int
	// Fallback engines (the carver) run only when nothing else applies.
	Fallback bool
	Match    func(d *detect.Result, head []byte) bool
	List     func(p string, d *detect.Result) (*Listing, error)
}

var (
	regMu   sync.Mutex
	engines []Engine
)

// Register adds an engine. Engines call it from init.
func Register(e Engine) {
	regMu.Lock()
	defer regMu.Unlock()
	engines = append(engines, e)
	sort.SliceStable(engines, func(i, j int) bool { return engines[i].Priority < engines[j].Priority })
}

func registered() []Engine {
	regMu.Lock()
	defer regMu.Unlock()
	return append([]Engine(nil), engines...)
}

// Explore lists the contents of the file at p: the first matching
// engine that yields entries wins, the fallback engines run last.
func Explore(p string, d *detect.Result) (*Listing, error) {
	head := readHead(p, 64<<10)
	var errs []string
	all := registered()
	for _, e := range all {
		if e.Fallback || !e.Match(d, head) {
			continue
		}
		l, err := e.List(p, d)
		if err == nil && l != nil && len(l.Entries) > 0 {
			if l.Engine == "" {
				l.Engine = e.Name
			}
			finish(l)
			return l, nil
		}
		if l != nil {
			errs = append(errs, l.Notes...)
			l.Close()
		}
		if err != nil {
			errs = append(errs, e.Name+": "+err.Error())
		}
	}
	for _, e := range all {
		if !e.Fallback || !e.Match(d, head) {
			continue
		}
		if l, err := e.List(p, d); err == nil && l != nil && len(l.Entries) > 0 {
			l.Notes = append(l.Notes, errs...)
			finish(l)
			return l, nil
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%w (%s)", ErrNotContainer, strings.Join(errs, "; "))
	}
	return nil, ErrNotContainer
}

// DeepScan runs the fallback engines (the signature carver) regardless
// of the format.
func DeepScan(p string, d *detect.Result) (*Listing, error) {
	for _, e := range registered() {
		if e.Fallback {
			if l, err := e.List(p, d); err == nil && l != nil && len(l.Entries) > 0 {
				finish(l)
				return l, nil
			}
		}
	}
	return nil, ErrNotContainer
}

// Explorable reports whether some engine is likely to apply, so the UI
// can offer to expand a node before trying.
func Explorable(p string, d *detect.Result) bool {
	head := readHead(p, 64<<10)
	for _, e := range registered() {
		if e.Match(d, head) {
			return true
		}
	}
	return false
}

// finish cleans paths, adds the directories they imply, and sorts.
func finish(l *Listing) {
	have := map[string]bool{}
	for _, e := range l.Entries {
		e.Path = CleanPath(e.Path)
		have[e.Path] = true
	}
	var extra []*Entry
	for _, e := range l.Entries {
		for d := path.Dir(e.Path); d != "." && d != "/" && !have[d]; d = path.Dir(d) {
			have[d] = true
			extra = append(extra, &Entry{Path: d, Dir: true, Size: -1})
		}
	}
	l.Entries = append(l.Entries, extra...)
	sort.SliceStable(l.Entries, func(i, j int) bool {
		a, b := l.Entries[i], l.Entries[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Path) < strings.ToLower(b.Path)
	})
}

// CleanPath normalises an archive path to a relative slash path.
func CleanPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean("/" + p)
	p = strings.TrimPrefix(p, "/")
	if p == "" || p == "." {
		p = "(unnamed)"
	}
	return p
}

func readHead(p string, n int) []byte {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	b := make([]byte, n)
	k, _ := io.ReadFull(f, b)
	return b[:k]
}

// EngineCount reports how many engines are registered.
func EngineCount() int {
	regMu.Lock()
	defer regMu.Unlock()
	return len(engines)
}
