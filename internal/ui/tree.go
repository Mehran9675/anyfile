package ui

import (
	"fmt"
	"strings"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/session"
	"github.com/mehran9675/giode"
)

// treeView is the left panel: the opened file and everything found
// inside it, nested containers included, on giode's TreeView.
type treeView struct {
	u     *UI
	tree  *giode.TreeViewElement
	root  *session.Node
	nodes map[*session.Node]*treeNode
}

// treeNode adapts a session node to giode.TreeNode. Children are
// wrapped once and rewrapped only when the node's children change.
type treeNode struct {
	t     *treeView
	n     *session.Node
	kids  []giode.TreeNode
	first *session.Node // Kids[0] when kids was built
}

func (w *treeNode) Key() any          { return w.n.ID }
func (w *treeNode) Label() string     { return w.n.Name }
func (w *treeNode) HasChildren() bool { return w.n.MayContain() }

func (w *treeNode) Children() []giode.TreeNode {
	var first *session.Node
	if len(w.n.Kids) > 0 {
		first = w.n.Kids[0]
	}
	if len(w.kids) != len(w.n.Kids) || w.first != first {
		w.kids = make([]giode.TreeNode, len(w.n.Kids))
		for i, k := range w.n.Kids {
			w.kids[i] = w.t.wrap(k)
		}
		w.first = first
	}
	return w.kids
}

func newTreeView(u *UI) *treeView {
	t := &treeView{u: u, nodes: map[*session.Node]*treeNode{}}
	t.tree = giode.TreeView(t.roots, t.row, giode.Styles{
		Height:      30,
		Color:       colMuted,
		BorderColor: colAccent,
		ScrollBar:   giode.ScrollBar{ThumbColor: "#ffffff22", Width: 6, Radius: 3},
	}).
		OnSelect(func(n giode.TreeNode) { u.sess.Select(n.(*treeNode).n) }).
		OnExpand(func(n giode.TreeNode) { u.sess.Expand(n.(*treeNode).n) }).
		OnActivate(func(n giode.TreeNode) {
			node := n.(*treeNode).n
			if node.MayContain() {
				t.tree.Toggle(n.Key())
			} else {
				u.sess.View(node)
			}
		})
	return t
}

func (t *treeView) wrap(n *session.Node) *treeNode {
	w := t.nodes[n]
	if w == nil {
		w = &treeNode{t: t, n: n}
		t.nodes[n] = w
	}
	return w
}

func (t *treeView) roots() []giode.TreeNode {
	if t.root == nil {
		return nil
	}
	return []giode.TreeNode{t.wrap(t.root)}
}

// expand opens a node the session asks for: the opened file once
// explored, and lone folders inside containers.
func (t *treeView) expand(n *session.Node) {
	t.tree.Expand(n.ID, true)
}

func (t *treeView) view(root *session.Node) giode.Element {
	if root != t.root {
		// A new file: forget the previous tree's wrappers.
		t.root = root
		t.nodes = map[*session.Node]*treeNode{}
		t.tree.Select(root.ID)
	}
	header := giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Justify: giode.JustifySpaceBetween,
		Padding: giode.Inset{Top: 16, Right: 16, Bottom: 10, Left: 18}, Wrap: giode.WrapNoWrap},
		sectionTitle("layers", "Contents"),
		giode.Text(fmt.Sprintf("%d items", countFiles(root)), txt(11.5, colFaint)),
	)
	return giode.Stack(giode.Styles{Width: -1, Height: -1},
		header,
		giode.Box(giode.Styles{Width: -1, FlexGrow: 1, Padding: giode.Inset{Left: 8, Right: 4, Bottom: 8}}, t.tree),
	)
}

// row draws one node's content; the tree adds indentation, the arrow
// and the selection background.
func (t *treeView) row(tn giode.TreeNode, s giode.TreeRowState) giode.Element {
	n := tn.(*treeNode).n
	cat := n.Category()
	c := catColor(cat)
	nameColor := colTextSoft
	if s.Selected {
		nameColor = colText
	}
	weight := giode.FontWeightNormal
	if n.IsRoot() {
		weight = giode.FontWeightBold
	}
	kids := []giode.Element{
		icon(catIcon(cat), c, 16),
		giode.Box(giode.Styles{FlexGrow: 1, Width: -1, Padding: giode.Inset{Left: 2}},
			giode.Text(n.Name, txtw(13, nameColor, giode.Styles{FontWeight: weight, MaxLines: 1, TextOverflow: giode.TextOverflowEllipsis}))),
	}
	switch {
	case n.Busy != "":
		kids = append(kids, giode.Spinner(giode.Styles{Width: 13, Height: 13, Color: colAccent}))
	case !n.Dir && badgeFor(n) != "":
		kids = append(kids, giode.Text(badgeFor(n), txtw(10.5, c, giode.Styles{FontWeight: giode.FontWeightBold})))
	}
	if n.Size >= 0 {
		sizeColor := colFaint
		if s.Selected {
			sizeColor = colTextSoft
		}
		kids = append(kids, giode.Box(giode.Styles{Width: 62},
			giode.Text(detect.HumanSize(n.Size), txtw(11.5, sizeColor, giode.Styles{TextAlign: giode.TextAlignEnd}))))
	}
	return giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Gap: 7, Wrap: giode.WrapNoWrap, Padding: giode.Inset{Left: 2}}, kids...)
}

// badgeFor flags items whose real type differs from their name.
func badgeFor(n *session.Node) string {
	r := n.Result
	if r == nil || r.Ext == "" || r.Ext == "bin" || r.Category == detect.CatText && r.Ext == "txt" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(n.Name), "."+r.Ext) {
		return ""
	}
	for _, c := range r.Candidates {
		if c.Ext == n.Ext() {
			return ""
		}
	}
	return strings.ToUpper(r.Ext)
}
