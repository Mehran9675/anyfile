package session

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/explore"
	"github.com/mehran9675/anyfile/internal/oodle"
	"github.com/mehran9675/anyfile/internal/platform"
)

// Session is one opened file and everything discovered inside it.
//
// Threading: the session and its nodes belong to the UI goroutine. Call
// its methods and read its nodes there only. Slow work (extracting,
// identifying, exploring, hashing) runs on other goroutines and hands
// its result back through post, so nothing needs a lock.
type Session struct {
	post   func(func())           // runs a function on the UI goroutine
	toast  func(kind, msg string) // safe from any goroutine
	expand func(*Node)            // asks the UI to open a node

	root     *Node
	selected *Node
	nextID   int
	tmp      string
	gen      int // bumps on Open, so results for a previous file are dropped
}

// New creates an empty session. post must run functions on the UI
// goroutine (giode's App.Post); toast reports outcomes.
func New(post func(func()), toast func(kind, msg string)) *Session {
	return &Session{post: post, toast: toast}
}

// OnExpand sets the function asked to open a node in the view: the
// opened file once explored, and lone folders inside containers.
func (s *Session) OnExpand(fn func(*Node)) { s.expand = fn }

// Root returns the opened file's node, nil before Open.
func (s *Session) Root() *Node { return s.root }

// Selected returns the inspected node.
func (s *Session) Selected() *Node { return s.selected }

func (s *Session) say(kind, format string, a ...any) {
	if s.toast != nil {
		s.toast(kind, fmt.Sprintf(format, a...))
	}
}

func (s *Session) expandNode(n *Node) {
	if s.expand != nil {
		s.expand(n)
	}
}

// background runs work on another goroutine; the function it returns
// then runs on the UI goroutine, told whether the session still shows
// the same file.
func (s *Session) background(work func() func(current bool)) {
	gen := s.gen
	go func() {
		apply := work()
		s.post(func() {
			if apply != nil {
				apply(gen == s.gen)
			}
		})
	}()
}

func (s *Session) newNode(name string, parent *Node) *Node {
	s.nextID++
	n := &Node{ID: s.nextID, Name: name, Parent: parent, Size: -1}
	if parent != nil {
		n.Depth = parent.Depth + 1
	}
	return n
}

// Close releases listings and deletes extracted temporaries.
func (s *Session) Close() {
	root, tmp := s.root, s.tmp
	s.root, s.selected, s.tmp = nil, nil, ""
	s.gen++
	closeTree(root)
	if tmp != "" {
		os.RemoveAll(tmp)
	}
}

func closeTree(n *Node) {
	if n == nil {
		return
	}
	for _, k := range n.Kids {
		closeTree(k)
	}
	n.Listing.Close()
}

// Open starts inspecting the file at p, replacing the current one.
func (s *Session) Open(p string) {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	st, err := os.Stat(p)
	if err != nil {
		s.say("error", "Cannot open %s: %v", filepath.Base(p), err)
		return
	}
	if st.IsDir() {
		s.say("error", "%s is a folder; open a file inside it.", filepath.Base(p))
		return
	}
	s.Close()
	oodle.Hint(p)
	tmp, _ := os.MkdirTemp("", "anyfile-")
	root := s.newNode(filepath.Base(p), nil)
	root.Local, root.Size = p, st.Size()
	s.root, s.selected, s.tmp = root, root, tmp
	s.prepare(root, func() {
		s.hash(root)
		s.explore(root, false)
	})
}

// Select makes n the inspected node and starts its analysis.
func (s *Session) Select(n *Node) {
	s.selected = n
	if n.IsFile() {
		s.prepare(n, func() { s.hash(n) })
	}
}

// Expand explores n the first time it is opened in the view.
func (s *Session) Expand(n *Node) {
	if !n.Dir && !n.Explored {
		s.prepare(n, func() { s.explore(n, false) })
	}
}

// DeepScan replaces n's children with what the signature carver finds.
func (s *Session) DeepScan(n *Node) {
	s.prepare(n, func() { s.explore(n, true) })
}

// AskTrID re-identifies n with TrID's opinion included.
func (s *Session) AskTrID(n *Node) {
	s.prepare(n, func() {
		local, name := n.Local, n.Name
		n.Busy = "Asking TrID…"
		s.background(func() func(bool) {
			r := detect.Detect(local, detect.Options{Name: name, ForceTrID: true})
			return func(current bool) {
				n.Busy = ""
				if !current {
					return
				}
				n.Result = r
				if !r.TrIDRan {
					s.say("error", "TrID is not available. Put trid.exe and triddefs.trd next to AnyFile or set ANYFILE_TRID.")
				}
			}
		})
	})
}

// prepare makes sure n has a file on disk (extracting it from its
// container if needed) and has been identified, then runs then on the
// UI goroutine. Concurrent requests for the same node share one job.
func (s *Session) prepare(n *Node, then func()) {
	s.prepareThen(n, func(ok bool) {
		if ok && then != nil {
			then()
		}
	})
}

func (s *Session) prepareThen(n *Node, then func(ok bool)) {
	if n.Dir {
		then(false)
		return
	}
	if n.Local != "" && n.Result != nil {
		then(true)
		return
	}
	n.waiters = append(n.waiters, then)
	if n.preparing {
		return
	}
	n.preparing = true
	local, entry, name := n.Local, n.Entry, n.Name
	dst := filepath.Join(s.tmp, fmt.Sprint(n.ID), explore.SafeName(n.Name))
	n.Busy = "Identifying…"
	if local == "" {
		n.Busy = "Extracting…"
	}
	s.background(func() func(bool) {
		var err error
		if local == "" {
			if entry == nil {
				err = errors.New("nothing to extract")
			} else if err = explore.WriteTo(entry, dst); err == nil {
				local = dst
			}
		}
		var r *detect.Result
		var leaf bool
		size := int64(-1)
		if err == nil {
			r = detect.Detect(local, detect.Options{Name: name})
			leaf = !explore.Explorable(local, r)
			if st, e := os.Stat(local); e == nil {
				size = st.Size()
			}
		}
		return func(current bool) {
			n.preparing, n.Busy = false, ""
			waiters := n.waiters
			n.waiters = nil
			ok := current && err == nil
			if current && err != nil {
				n.Err = "Could not extract: " + err.Error()
			}
			if ok {
				n.Local, n.Result = local, r
				if size >= 0 {
					n.Size = size
				}
				if leaf && !n.Explored {
					n.Leaf = true
				}
			}
			for _, w := range waiters {
				w(ok)
			}
		}
	})
}

// Limits for identifying a container's members in the background.
const (
	prefetchCount = 300
	prefetchSize  = 256 << 20
)

// prefetch identifies the members of n one after another, so the tree
// shows their real types and which of them can be expanded without
// waiting for clicks.
func (s *Session) prefetch(n *Node) {
	var files []*Node
	var collect func(*Node)
	collect = func(c *Node) {
		for _, k := range c.Kids {
			if len(files) >= prefetchCount {
				return
			}
			switch {
			case k.Dir:
				collect(k)
			case k.Result == nil && k.Size <= prefetchSize:
				files = append(files, k)
			}
		}
	}
	collect(n)
	gen := s.gen
	var next func(i int)
	next = func(i int) {
		if i < len(files) && gen == s.gen {
			s.prepareThen(files[i], func(bool) { next(i + 1) })
		}
	}
	next(0)
}

func (s *Session) explore(n *Node, deep bool) {
	if n.exploring || n.Result == nil || n.Local == "" {
		return
	}
	n.exploring = true
	local, r := n.Local, n.Result
	n.Busy = "Exploring…"
	if deep {
		n.Busy = "Deep scanning…"
	}
	s.background(func() func(bool) {
		var l *explore.Listing
		var err error
		if deep {
			l, err = explore.DeepScan(local, r)
		} else {
			l, err = explore.Explore(local, r)
		}
		return func(current bool) {
			n.exploring, n.Busy = false, ""
			if !current {
				l.Close()
				return
			}
			switch {
			case err != nil && deep:
				n.Err = "Deep scan found no embedded files."
			case err != nil:
				n.Explored, n.Leaf = true, true
				if !errors.Is(err, explore.ErrNotContainer) || strings.Contains(err.Error(), "(") {
					n.Err = strings.TrimPrefix(err.Error(), explore.ErrNotContainer.Error()+" ")
				}
			default:
				for _, k := range n.Kids {
					closeTree(k)
				}
				n.Listing.Close()
				n.Kids = nil
				n.Listing, n.Explored, n.Leaf, n.Carved, n.Err = l, true, false, deep, ""
				s.buildTree(n, l)
				s.expandNode(n)
				s.prefetch(n)
				if deep {
					s.say("ok", "Deep scan found %d embedded object(s).", len(l.Entries))
				}
			}
		}
	})
}

func (s *Session) hash(n *Node) {
	if n.Hashes != nil || n.hashing || n.Local == "" {
		return
	}
	n.hashing = true
	local := n.Local
	s.background(func() func(bool) {
		out, err := hashFile(local)
		return func(current bool) {
			n.hashing = false
			if current && err == nil {
				n.Hashes = out
			}
		}
	})
}

func hashFile(p string) (map[string]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	hs := []hash.Hash{md5.New(), sha1.New(), sha256.New()}
	if _, err := io.Copy(io.MultiWriter(hs[0], hs[1], hs[2]), f); err != nil {
		return nil, err
	}
	return map[string]string{
		"MD5":     hex.EncodeToString(hs[0].Sum(nil)),
		"SHA-1":   hex.EncodeToString(hs[1].Sum(nil)),
		"SHA-256": hex.EncodeToString(hs[2].Sum(nil)),
	}, nil
}

// --- handing files to the system ---------------------------------------------

// viewPath returns a path the OS will open with the right application:
// when the name's extension disagrees with the detected type, a copy
// with the correct extension is made, so a spreadsheet saved as .dat
// still opens in the spreadsheet application. It may run on any
// goroutine: it only uses the values passed in.
func viewPath(local, name string, r *detect.Result, dir string) (string, string, error) {
	want := r.Ext
	if want == "" || want == "bin" || strings.HasSuffix(strings.ToLower(name), "."+want) || matchesCandidate(r, name) {
		return local, "", nil
	}
	if r.Category == detect.CatText && want == "txt" && strings.Contains(name, ".") {
		return local, "", nil // any text extension opens in a text editor
	}
	dst := filepath.Join(dir, "view", explore.SafeName(name)+"."+want)
	if _, err := os.Stat(dst); err != nil {
		if err := copyFile(local, dst); err != nil {
			return "", "", err
		}
	}
	return dst, want, nil
}

func matchesCandidate(r *detect.Result, name string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	for _, c := range r.Candidates {
		if c.Ext != "" && c.Ext == ext && c.Percent >= 10 {
			return true
		}
	}
	return false
}

// View opens n in the user's default application for its real type.
func (s *Session) View(n *Node) { s.handOff(n, false) }

// ViewWith lets the user choose the application.
func (s *Session) ViewWith(n *Node) { s.handOff(n, true) }

func (s *Session) handOff(n *Node, choose bool) {
	s.prepare(n, func() {
		local, name, r := n.Local, n.Name, n.Result
		dir := filepath.Join(s.tmp, fmt.Sprint(n.ID))
		go func() {
			p, retyped, err := viewPath(local, name, r, dir)
			if err != nil {
				s.say("error", "Could not prepare %s: %v", name, err)
				return
			}
			if choose {
				err = platform.OpenWith(p)
			} else {
				err = platform.Open(p)
			}
			switch {
			case err != nil:
				s.say("error", "%v", err)
			case retyped != "":
				s.say("ok", "Opened %s as .%s, its real type.", name, retyped)
			default:
				s.say("ok", "Opened %s in its viewer.", name)
			}
		}()
	})
}

// Reveal shows n in the file manager (extracting it first if needed).
func (s *Session) Reveal(n *Node) {
	s.prepare(n, func() {
		local := n.Local
		go func() {
			if err := platform.Reveal(local); err != nil {
				s.say("error", "%v", err)
			}
		}()
	})
}

// SaveAs extracts the single file n into dir.
func (s *Session) SaveAs(n *Node, dir string) {
	s.prepare(n, func() {
		src, name := n.Local, n.Name
		go func() {
			dst := uniquePath(filepath.Join(dir, explore.SafeName(name)))
			if err := copyFile(src, dst); err != nil {
				s.say("error", "Could not save %s: %v", name, err)
				return
			}
			s.say("ok", "Saved %s", dst)
		}()
	})
}

// ExtractAll writes everything inside n (a container or a virtual
// folder) under dir, preserving the internal structure.
func (s *Session) ExtractAll(n *Node, dir string) {
	type job struct {
		rel   string
		local string
		entry *explore.Entry
	}
	prefix := ""
	if n.Dir {
		prefix = n.Path
	}
	var jobs []job
	var collect func(*Node)
	collect = func(c *Node) {
		for _, k := range c.Kids {
			if k.Dir {
				collect(k)
				continue
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(k.Path, prefix), "/")
			jobs = append(jobs, job{rel, k.Local, k.Entry})
		}
	}
	collect(n)
	if len(jobs) == 0 {
		s.say("error", "%s has no files to extract.", n.Name)
		return
	}
	base := explore.SafeName(strings.TrimSuffix(n.Name, filepath.Ext(n.Name)))
	if n.Dir {
		base = explore.SafeName(n.Name)
	}
	out := uniquePath(filepath.Join(dir, base))
	n.Busy = fmt.Sprintf("Extracting %d files…", len(jobs))
	s.background(func() func(bool) {
		failed := 0
		for _, j := range jobs {
			dst, err := explore.SafeJoin(out, j.rel)
			if err == nil {
				if j.local != "" {
					err = copyFile(j.local, dst)
				} else {
					err = explore.WriteTo(j.entry, dst)
				}
			}
			if err != nil {
				failed++
			}
		}
		if failed > 0 {
			s.say("error", "Extracted %d of %d files to %s (%d failed).", len(jobs)-failed, len(jobs), out, failed)
		} else {
			s.say("ok", "Extracted %d files to %s", len(jobs), out)
		}
		return func(bool) { n.Busy = "" }
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

func uniquePath(p string) string {
	if _, err := os.Stat(p); err != nil {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(c); err != nil {
			return c
		}
	}
}
