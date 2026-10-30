package ui

import (
	"strings"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/session"
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/dropdown"
	"github.com/mehran9675/giode/components/iconbutton"
	"github.com/mehran9675/giode/components/input"
)

// treeView is one tab's left panel: the opened file and everything
// found inside it, on giode's TreeView, with filtering and sorting.
type treeView struct {
	u     *UI
	sess  *session.Session
	tree  *giode.TreeViewElement
	nodes map[*session.Node]*treeNode

	// Toolbar.
	search  *input.Input
	group   *dropdown.Select
	sortBy  *dropdown.Select
	sortDir *iconbutton.Button

	text relabeler // the toolbar's translated text

	opts viewOptions
	// The filter result, recomputed when the options or the tree change.
	keep      map[*session.Node]bool
	hits      int
	keepFor   viewOptions
	keepVer   int
	optsEpoch int // bumps with every option change, to refresh child lists
}

// treeNode adapts a session node to giode.TreeNode. Children are
// wrapped, filtered and sorted once, and redone only when the node's
// children, the session or the view options change.
type treeNode struct {
	t     *treeView
	n     *session.Node
	kids  []giode.TreeNode
	built struct {
		count, version, epoch int
		first                 *session.Node
	}
}

func (w *treeNode) Key() any          { return w.n.ID }
func (w *treeNode) Label() string     { return w.n.Name }
func (w *treeNode) HasChildren() bool { return w.n.MayContain() }

func (w *treeNode) Children() []giode.TreeNode {
	t := w.t
	var first *session.Node
	if len(w.n.Kids) > 0 {
		first = w.n.Kids[0]
	}
	b := &w.built
	if w.kids != nil && b.count == len(w.n.Kids) && b.first == first && b.version == t.sess.Version() && b.epoch == t.optsEpoch {
		return w.kids
	}
	kids := make([]*session.Node, 0, len(w.n.Kids))
	for _, k := range w.n.Kids {
		if t.keep == nil || t.keep[k] {
			kids = append(kids, k)
		}
	}
	t.opts.sortNodes(kids)
	w.kids = make([]giode.TreeNode, len(kids))
	for i, k := range kids {
		w.kids[i] = t.wrap(k)
	}
	b.count, b.first, b.version, b.epoch = len(w.n.Kids), first, t.sess.Version(), t.optsEpoch
	return w.kids
}

func newTreeView(u *UI, sess *session.Session) *treeView {
	t := &treeView{u: u, sess: sess, nodes: map[*session.Node]*treeNode{}}
	t.tree = giode.TreeView(t.roots, t.row, giode.Styles{
		Height:      30,
		Color:       colMuted,
		BorderColor: colAccent,
		ScrollBar:   giode.ScrollBar{ThumbColor: "#ffffff22", Width: 6, Radius: 3},
	}).
		OnSelect(func(n giode.TreeNode) { sess.Select(n.(*treeNode).n) }).
		OnExpand(func(n giode.TreeNode) { sess.Expand(n.(*treeNode).n) }).
		OnActivate(func(n giode.TreeNode) {
			node := n.(*treeNode).n
			if node.MayContain() {
				t.tree.Toggle(n.Key())
			} else {
				sess.View(node)
			}
		})

	field := giode.Styles{Background: colSurface, Color: colText, BorderColor: colBorderHi, BorderRadius: 8}
	t.search = giode.Input.Text("", field.With(giode.Styles{Padding: giode.SymmetricInset(10, 7), FontSize: 13})).
		OnChange(func(s string) { t.setOptions(func(o *viewOptions) { o.query = strings.ToLower(strings.TrimSpace(s)) }) })
	t.group = giode.Input.Select(field.With(giode.Styles{Width: -1}), translated(typeGroupLabels())...).
		OnChange(func(i int) { t.setOptions(func(o *viewOptions) { o.group = i }) })
	t.sortBy = giode.Input.Select(field.With(giode.Styles{Width: -1}), translated(sortLabels)...).
		OnChange(func(i int) { t.setOptions(func(o *viewOptions) { o.sort = i }) })
	t.sortDir = giode.IconButton(icon("sortAsc", colTextSoft, 16), giode.Styles{Background: colTextSoft}).
		Variant(giode.IconButtonGhost).
		OnClick(func() {
			t.setOptions(func(o *viewOptions) { o.desc = !o.desc })
			name := "sortAsc"
			if t.opts.desc {
				name = "sortDesc"
			}
			t.sortDir.Icon(icon(name, colTextSoft, 16))
		})
	t.text.add(func() {
		t.search.Placeholder(tr("Filter by name")).Label(tr("Filter by name"))
		relabelOptions(t.group.Options(), typeGroupLabels())
		relabelOptions(t.sortBy.Options(), sortLabels)
		t.sortDir.Label(tr("Sort order"))
	})
	return t
}

func (t *treeView) setOptions(change func(o *viewOptions)) {
	change(&t.opts)
	t.optsEpoch++
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
	if r := t.sess.Root(); r != nil {
		return []giode.TreeNode{t.wrap(r)}
	}
	return nil
}

// expand opens a node the session asks for: the opened file once
// explored, and lone folders inside containers.
func (t *treeView) expand(n *session.Node) {
	t.tree.Expand(n.ID, true)
}

// refilter recomputes which nodes the filter keeps, and opens the
// folders and containers that hold matches.
func (t *treeView) refilter(root *session.Node) {
	if t.keepFor == t.opts && t.keepVer == t.sess.Version() {
		return
	}
	t.keepFor, t.keepVer = t.opts, t.sess.Version()
	if !t.opts.filtering() {
		t.keep, t.hits = nil, 0
		return
	}
	keep, open, hits := t.opts.visibleSet(root)
	t.keep, t.hits = keep, hits
	t.optsEpoch++
	for _, n := range open {
		t.tree.Expand(n.ID, true)
	}
}

func (t *treeView) view() giode.Element {
	root := t.sess.Root()
	if root == nil {
		return giode.Box(giode.Styles{})
	}
	t.text.update()
	t.refilter(root)
	total := countFiles(root)
	count := tr("{count} items", "count", total)
	if t.opts.filtering() {
		count = tr("{hits} of {total}", "hits", t.hits, "total", total)
	}
	header := giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Justify: giode.JustifySpaceBetween,
		Padding: giode.Inset{Top: 14, Right: 16, Bottom: 8, Left: 18}, Wrap: giode.WrapNoWrap},
		sectionTitle("layers", "Contents"),
		giode.Text(count, txt(11.5, colFaint)),
	)
	toolbar := giode.Stack(giode.Styles{Width: -1, Gap: 8, Padding: giode.Inset{Right: 14, Bottom: 10, Left: 14}},
		giode.Box(giode.Styles{Width: -1}, t.search),
		giode.Row(giode.Styles{Width: -1, Gap: 6, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			// The two lists share the width, which translations need.
			giode.Box(giode.Styles{FlexGrow: 1, Width: -1}, t.group),
			giode.Box(giode.Styles{FlexGrow: 1, Width: -1}, t.sortBy),
			t.sortDir),
	)
	var body giode.Element = t.tree
	if t.opts.filtering() && t.hits == 0 {
		body = giode.Box(giode.Styles{Width: -1, Padding: giode.UniformInset(18)},
			giode.Text(tr("No files match the filter."), txt(12.5, colMuted)))
	}
	return giode.Stack(giode.Styles{Width: -1, Height: -1},
		header,
		toolbar,
		giode.Divider(giode.Styles{Color: colBorder}),
		giode.Box(giode.Styles{Width: -1, FlexGrow: 1, Padding: giode.Inset{Top: 6, Left: 8, Right: 4, Bottom: 8}}, body),
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
		kids = append(kids, giode.Badge(badgeFor(n), giode.Styles{Background: color(c + "26"), Color: color(c), FontSize: 10}))
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
