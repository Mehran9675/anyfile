package session

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mehran9675/anyfile/internal/detect"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/all"
)

// uiLoop stands in for giode's UI goroutine: post queues functions and
// one goroutine runs them in order.
type uiLoop struct {
	mu     sync.Mutex
	closed bool
	calls  chan func()
}

func newUILoop(t *testing.T) *uiLoop {
	l := &uiLoop{calls: make(chan func(), 4096)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for fn := range l.calls {
			fn()
		}
	}()
	t.Cleanup(func() {
		l.mu.Lock()
		l.closed = true
		close(l.calls)
		l.mu.Unlock()
		<-done
	})
	return l
}

// post drops calls once the loop is closed, as a closed window does.
func (l *uiLoop) post(fn func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closed {
		l.calls <- fn
	}
}

// do runs fn on the UI goroutine and waits for it.
func (l *uiLoop) do(fn func()) {
	done := make(chan struct{})
	l.post(func() { fn(); close(done) })
	<-done
}

// wait polls cond on the UI goroutine until it holds.
func (l *uiLoop) wait(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		ok := false
		l.do(func() { ok = cond() })
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out")
}

// A spreadsheet without an extension must reach the OS as .xlsx.
func TestViewPathRetypes(t *testing.T) {
	ui := newUILoop(t)
	s := New(ui.post, nil)
	ui.do(func() { s.Open("testdata/mystery") })
	ui.wait(t, func() bool { return s.Root().Result != nil && s.Root().Explored })
	var local, name, dir string
	var res *detect.Result
	ui.do(func() {
		r := s.Root()
		local, name, res, dir = r.Local, r.Name, r.Result, filepath.Join(s.tmp, "x")
	})
	p, ext, err := viewPath(local, name, res, dir)
	if err != nil {
		t.Fatal(err)
	}
	if ext != "xlsx" || !strings.HasSuffix(p, "mystery.xlsx") {
		t.Fatalf("got %q (%s), want a .xlsx copy", p, ext)
	}
	ui.do(s.Close)
}

// Nested containers expand and extract with their structure intact.
func TestNestedExtractAll(t *testing.T) {
	ui := newUILoop(t)
	var mu sync.Mutex
	var msgs []string
	s := New(ui.post, func(kind, msg string) { mu.Lock(); msgs = append(msgs, kind+": "+msg); mu.Unlock() })
	var expanded []string
	s.OnExpand(func(n *Node) { expanded = append(expanded, n.Name) })
	ui.do(func() { s.Open("testdata/nested.zip") })
	ui.wait(t, func() bool { return s.Root().Explored && len(s.Root().Kids) > 0 })

	var book *Node
	ui.do(func() {
		for _, k := range s.Root().Kids {
			if k.Name == "docs" {
				book = k.Kids[0]
			}
		}
		s.Expand(book)
	})
	ui.wait(t, func() bool { return book.Explored })
	ui.do(func() {
		if book.Result == nil || book.Result.Ext != "xlsx" || len(book.Kids) == 0 {
			t.Errorf("book.xlsx not explored: %+v", book.Result)
		}
		if len(expanded) == 0 || expanded[0] != "nested.zip" {
			t.Errorf("the opened file was not expanded: %v", expanded)
		}
	})

	out := t.TempDir()
	ui.do(func() { s.ExtractAll(s.Root(), out) })
	ui.wait(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(msgs) > 0 })
	for _, f := range []string{"docs/book.xlsx", "inner/bundle.tar.gz", "readme.txt"} {
		if _, err := os.Stat(filepath.Join(out, "nested", f)); err != nil {
			t.Errorf("missing %s: %v (%v)", f, err, msgs)
		}
	}
	ui.do(s.Close)
}
