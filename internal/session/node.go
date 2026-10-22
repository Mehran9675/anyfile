// Package session holds what the user is inspecting: a tree of nodes,
// one per file or container member, and the background jobs that
// detect, explore, extract and hand them to viewers. It knows nothing
// about the UI; the UI renders it and calls its methods.
package session

import (
	"path"
	"strings"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/explore"
)

// Node is one file (the opened one or a member of a container) or a
// virtual directory inside a container.
type Node struct {
	ID     int
	Name   string
	Path   string // path inside the parent container ("" for the root)
	Parent *Node
	Kids   []*Node
	Depth  int

	Dir   bool  // virtual directory inside a container
	Size  int64 // -1 unknown
	Entry *explore.Entry
	Local string // the file on disk: the original, or an extracted copy

	Result   *detect.Result
	Listing  *explore.Listing
	Explored bool   // exploration ran (Kids is final)
	Leaf     bool   // exploration found nothing inside
	Carved   bool   // children come from a deep scan
	Busy     string // current job, "" when idle
	Err      string

	Hashes map[string]string

	// Job bookkeeping, so concurrent requests share one job.
	preparing bool
	waiters   []func(ok bool)
	exploring bool
	hashing   bool
}

// IsRoot reports whether n is the opened file.
func (n *Node) IsRoot() bool { return n.Parent == nil }

// IsFile reports whether n has data of its own.
func (n *Node) IsFile() bool { return !n.Dir }

// Container is the nearest ancestor file that n lives inside.
func (n *Node) Container() *Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if !p.Dir {
			return p
		}
	}
	return nil
}

// Trail is the chain of names from the root to n.
func (n *Node) Trail() []string {
	var out []string
	for c := n; c != nil; c = c.Parent {
		out = append([]string{c.Name}, out...)
	}
	return out
}

// Ext is the lower-case extension of the name, without dot.
func (n *Node) Ext() string {
	return strings.ToLower(strings.TrimPrefix(path.Ext(n.Name), "."))
}

// MayContain reports whether expanding n could show children.
func (n *Node) MayContain() bool {
	if n.Dir {
		return len(n.Kids) > 0
	}
	if n.Explored {
		return len(n.Kids) > 0
	}
	return !n.Leaf
}

// Category is the detected category, or a guess from the name.
func (n *Node) Category() detect.Category {
	if n.Dir {
		return "Folder"
	}
	if n.Result != nil {
		return n.Result.Category
	}
	return guessCategory(n.Ext())
}

func guessCategory(ext string) detect.Category {
	switch ext {
	case "zip", "7z", "rar", "tar", "gz", "tgz", "bz2", "xz", "zst", "cab", "jar", "apk", "deb", "rpm":
		return detect.CatArchive
	case "doc", "docx", "pdf", "odt", "rtf", "epub", "html", "htm":
		return detect.CatDocument
	case "xls", "xlsx", "ods", "csv", "tsv":
		return detect.CatSpreadsheet
	case "ppt", "pptx", "odp":
		return detect.CatPresentation
	case "png", "jpg", "jpeg", "gif", "bmp", "webp", "svg", "ico", "tif", "tiff", "heic", "jp2", "cur":
		return detect.CatImage
	case "mp3", "wav", "flac", "aac", "ogg", "opus", "m4a", "wma", "mka":
		return detect.CatAudio
	case "mp4", "mkv", "webm", "avi", "mov", "wmv", "flv":
		return detect.CatVideo
	case "exe", "dll", "sys", "elf", "so", "macho", "msi":
		return detect.CatExecutable
	case "ttf", "otf", "woff", "woff2", "cff", "pfb":
		return detect.CatFont
	case "txt", "md", "log", "ini", "json", "xml", "yaml", "yml":
		return detect.CatText
	case "js", "go", "py", "c", "cpp", "cs", "java", "ts", "css":
		return detect.CatCode
	case "eml", "msg", "mbox":
		return detect.CatEmail
	case "iso", "img", "vhd", "vhdx", "wim":
		return detect.CatDiskImage
	case "pem", "crt", "cer", "p7b", "key":
		return detect.CatCrypto
	}
	return detect.CatUnknown
}

// buildTree turns a flat listing into child nodes of parent.
func (s *Session) buildTree(parent *Node, l *explore.Listing) {
	dirs := map[string]*Node{}
	var attach func(p string) *Node
	attach = func(p string) *Node {
		if p == "." || p == "" {
			return parent
		}
		if d, ok := dirs[p]; ok {
			return d
		}
		up := attach(path.Dir(p))
		d := s.newNode(path.Base(p), up)
		d.Dir, d.Size, d.Path = true, -1, p
		d.Explored = true
		dirs[p] = d
		up.Kids = append(up.Kids, d)
		return d
	}
	for _, e := range l.Entries {
		if e.Dir {
			attach(e.Path)
			continue
		}
		up := attach(path.Dir(e.Path))
		n := s.newNode(e.Name(), up)
		n.Path, n.Size, n.Entry = e.Path, e.Size, e
		up.Kids = append(up.Kids, n)
	}
	// Virtual folders report the total of what they hold.
	var total func(n *Node) int64
	total = func(n *Node) int64 {
		if !n.Dir {
			return max(n.Size, 0)
		}
		var t int64
		for _, k := range n.Kids {
			t += total(k)
		}
		n.Size = t
		return t
	}
	for _, k := range parent.Kids {
		total(k)
	}
	// Open single-folder chains so the files show without extra clicks.
	for n := parent; len(n.Kids) == 1 && n.Kids[0].Dir; n = n.Kids[0] {
		s.expandNode(n.Kids[0])
	}
}
