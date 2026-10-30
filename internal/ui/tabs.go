package ui

import (
	"path/filepath"
	"strings"

	"github.com/mehran9675/giode"
	"github.com/xmhrnx/anyfile/internal/session"
)

// tab is one opened file: its own session (with its temp folder) and
// its own tree, so expansion, selection and filters stay per file.
type tab struct {
	sess  *session.Session
	tree  *treeView
	btn   *giode.PressableElement
	close *giode.PressableElement
}

const maxTabWidth = 190 // dp; a fixed width so names grow and ellipsize inside it

// openPaths opens each path in a tab of its own, or switches to the tab
// already showing it.
func (u *UI) openPaths(paths []string) {
	for _, p := range paths {
		u.openPath(p)
	}
}

func (u *UI) openPath(p string) {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	for _, t := range u.tabs {
		if r := t.sess.Root(); r != nil && strings.EqualFold(r.Local, p) {
			u.activate(t)
			return
		}
	}
	t := &tab{sess: session.New(u.app.Post, u.toast)}
	t.sess.KeepAwakeWith(u.app.KeepAlive)
	t.tree = newTreeView(u, t.sess)
	t.sess.OnExpand(t.tree.expand)
	t.sess.Open(p)
	if t.sess.Root() == nil {
		return // Open reported why
	}
	t.btn = giode.Pressable(func(s giode.PressState) giode.Element { return u.tabView(t, s) }).
		Label("tab: " + t.sess.Root().Name).
		OnClick(func(e giode.ClickEvent) {
			if e.Button == giode.MouseMiddle {
				u.closeTab(t)
				return
			}
			u.activate(t)
		})
	t.close = giode.Pressable(func(s giode.PressState) giode.Element {
		c := colMuted
		var bg string
		if s.Hovered {
			c, bg = colText, "#ffffff1a"
		}
		return giode.Box(giode.Styles{Width: 20, Height: 20, Align: giode.AlignCenter, Justify: giode.JustifyCenter,
			BorderRadius: 5, Background: color(bg)}, icon("x", c, 12))
	}).Label("close: " + t.sess.Root().Name).
		OnClick(func(giode.ClickEvent) { u.closeTab(t) })
	u.tabs = append(u.tabs, t)
	u.activate(t)
}

func (u *UI) activate(t *tab) {
	u.showSettings = false
	if u.active != t {
		u.active = t
		u.inspector.scroll.ScrollTo(0)
	}
}

func (u *UI) closeTab(t *tab) {
	i := -1
	for k, x := range u.tabs {
		if x == t {
			i = k
		}
	}
	if i < 0 {
		return
	}
	t.sess.Close()
	u.tabs = append(u.tabs[:i], u.tabs[i+1:]...)
	if u.active == t {
		u.active = nil
		if len(u.tabs) > 0 {
			u.activate(u.tabs[min(i, len(u.tabs)-1)])
		}
	}
}

// tabStrip lays the tabs out in the title bar.
func (u *UI) tabStrip() giode.Element {
	if len(u.tabs) == 0 {
		return giode.Box(giode.Styles{})
	}
	items := make([]giode.Element, 0, len(u.tabs))
	for _, t := range u.tabs {
		items = append(items, t.btn)
	}
	return giode.Row(giode.Styles{Gap: 4, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap, Padding: giode.Inset{Left: 4}}, items...)
}

func (u *UI) tabView(t *tab, s giode.PressState) giode.Element {
	root := t.sess.Root()
	if root == nil {
		return giode.Box(giode.Styles{})
	}
	active := t == u.active
	bg, fg, border := "", colMuted, "#00000000"
	switch {
	case active:
		bg, fg, border = colRaised, colText, colBorderHi
	case s.Hovered:
		bg, fg = "#ffffff0d", colTextSoft
	}
	cat := root.Category()
	lead := icon(catIcon(cat), catColor(cat), 14)
	if root.Busy != "" {
		lead = giode.Spinner(giode.Styles{Width: 13, Height: 13, Color: colAccent})
	}
	return giode.Row(giode.Styles{
		Gap: 7, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap, Width: maxTabWidth, Height: 30,
		Padding: giode.Inset{Left: 10, Right: 5}, Background: color(bg), BorderRadius: 8,
		BorderWidth: 1, BorderColor: color(border),
	},
		lead,
		giode.Box(giode.Styles{FlexGrow: 1, Width: -1},
			giode.Text(root.Name, txtw(12.5, fg, giode.Styles{MaxLines: 1, TextOverflow: giode.TextOverflowEllipsis}))),
		t.close,
	)
}
