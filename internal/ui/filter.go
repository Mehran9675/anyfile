package ui

import (
	"sort"
	"strings"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/session"
)

// typeGroup is one entry of the "type" filter.
type typeGroup struct {
	label string
	cats  []detect.Category // nil: every category
}

var typeGroups = []typeGroup{
	{"All types", nil},
	{"Archives", []detect.Category{detect.CatArchive, detect.CatCompressed, detect.CatDiskImage}},
	{"Documents", []detect.Category{detect.CatDocument, detect.CatEmail}},
	{"Spreadsheets & slides", []detect.Category{detect.CatSpreadsheet, detect.CatPresentation}},
	{"Images", []detect.Category{detect.CatImage}},
	{"Audio", []detect.Category{detect.CatAudio}},
	{"Video", []detect.Category{detect.CatVideo}},
	{"Executables", []detect.Category{detect.CatExecutable}},
	{"Text & code", []detect.Category{detect.CatText, detect.CatCode}},
	{"Fonts", []detect.Category{detect.CatFont}},
	{"Data & other", []detect.Category{detect.CatData, detect.CatDatabase, detect.CatCrypto, detect.CatEncrypted, detect.CatEmpty, detect.CatUnknown}},
}

func typeGroupLabels() []string {
	out := make([]string, len(typeGroups))
	for i, g := range typeGroups {
		out[i] = g.label
	}
	return out
}

const (
	sortName = iota
	sortSize
	sortType
)

var sortLabels = []string{"Sort by name", "Sort by size", "Sort by type"}

// viewOptions is how one tab shows its tree.
type viewOptions struct {
	query string // lower case
	group int
	sort  int
	desc  bool
}

func (o viewOptions) filtering() bool { return o.query != "" || o.group != 0 }

// matches reports whether a file node passes the filter on its own.
func (o viewOptions) matches(n *session.Node) bool {
	if o.query != "" && !strings.Contains(strings.ToLower(n.Name), o.query) {
		return false
	}
	if g := typeGroups[o.group]; g.cats != nil && !n.Dir {
		cat := n.Category()
		for _, c := range g.cats {
			if c == cat {
				return true
			}
		}
		return false
	}
	return !n.Dir || o.query != ""
}

// visibleSet marks the nodes to show under a filter: the matches, and
// every folder or container holding one. It also returns the nodes to
// open so the matches are in view, and how many files matched.
func (o viewOptions) visibleSet(root *session.Node) (keep map[*session.Node]bool, open []*session.Node, hits int) {
	keep = map[*session.Node]bool{root: true}
	var walk func(n *session.Node) bool
	walk = func(n *session.Node) bool {
		inside := false
		for _, k := range n.Kids {
			if walk(k) {
				inside = true
			}
		}
		self := o.matches(n)
		if self && !n.Dir {
			hits++
		}
		if inside {
			open = append(open, n)
		}
		if self || inside {
			keep[n] = true
			return true
		}
		return false
	}
	walk(root)
	return keep, open, hits
}

// sortNodes orders siblings: folders first, then by the chosen key.
func (o viewOptions) sortNodes(ns []*session.Node) {
	less := func(a, b *session.Node) bool {
		switch o.sort {
		case sortSize:
			if a.Size != b.Size {
				return a.Size < b.Size
			}
		case sortType:
			ca, cb := string(a.Category()), string(b.Category())
			if ca != cb {
				return ca < cb
			}
			if ea, eb := a.Ext(), b.Ext(); ea != eb {
				return ea < eb
			}
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}
	sort.SliceStable(ns, func(i, j int) bool {
		a, b := ns[i], ns[j]
		if a.Dir != b.Dir {
			return a.Dir // folders stay on top in either direction
		}
		if o.desc {
			return less(b, a)
		}
		return less(a, b)
	})
}
