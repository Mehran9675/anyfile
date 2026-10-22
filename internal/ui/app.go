// Package ui is AnyFile's interface, built with giode: a frameless
// window with a content tree on the left and an inspector on the
// right. All state lives in the session, which belongs to the UI
// goroutine; the UI renders it and turns clicks into session calls.
package ui

import (
	"fmt"
	"image"
	"log"
	"time"

	"github.com/mehran9675/anyfile/internal/explore"
	"github.com/mehran9675/anyfile/internal/oodle"
	"github.com/mehran9675/anyfile/internal/platform"
	"github.com/mehran9675/anyfile/internal/session"
	"github.com/mehran9675/anyfile/internal/tools"
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/components/titlebar"
)

// UI is the application window.
type UI struct {
	app    *giode.App
	sess   *session.Session
	bar    *titlebar.TitleBar
	toasts *giode.ToasterElement

	tree      *treeView
	inspector *inspector

	openBtn      *button.Button
	emptyOpenBtn *button.Button
	dropZone     *giode.PressableElement
	version      string
}

// Run opens the window and blocks until it closes. A path argument is
// opened right away; version is shown in the status bar.
func Run(args []string, version string) error {
	loadFonts()
	applyTheme()
	u := &UI{version: version}
	cfg := giode.Config{
		Title:      "AnyFile",
		Size:       image.Pt(1280, 820),
		MinSize:    image.Pt(900, 560),
		Resizable:  true,
		Frameless:  true,
		Center:     true,
		Icon:       iconPNG,
		Background: colBg,
		LogLevel:   giode.LogLevelError,
		Logger:     log.Default(),
		OnShutdown: func() { u.sess.Close() },
	}
	if platform.FileDropSupported() {
		cfg.OnFileDrop = func(paths []string) { u.sess.Open(paths[0]) }
	}
	u.app = giode.New(cfg)
	u.toasts = giode.Toaster(giode.Styles{Background: colRaised, BorderColor: colBorderHi, Color: colText, BorderRadius: 11}).
		Placement(giode.ToastBottomRight).Duration(5 * time.Second).Max(4)
	u.sess = session.New(u.app.Post, u.toast)
	u.tree = newTreeView(u)
	u.inspector = newInspector(u)
	u.sess.OnExpand(u.tree.expand)
	u.openBtn = newBtn("open", "Open file", btnSecondary, u.pickFile)
	u.emptyOpenBtn = newBtn("open", "Choose a file", btnPrimary, u.pickFile)
	u.dropZone = giode.Pressable(u.dropZoneView).Label("Choose a file").
		OnClick(func(giode.ClickEvent) { u.pickFile() })
	u.bar = giode.TitleBar(u.app, "AnyFile", giode.Styles{
		Background: colSidebar, Color: colTextSoft, Height: 44, FontSize: 13,
		BorderWidth: 1, BorderColor: colBorder,
	}).Icon(logo(22)).Trailing(u.openBtn)
	if len(args) > 0 {
		u.app.Post(func() { u.sess.Open(args[0]) })
	}
	return u.app.Run(u.view)
}

// toast reports a session outcome; safe from any goroutine.
func (u *UI) toast(kind, msg string) {
	k := giode.ToastSuccess
	if kind == "error" {
		k = giode.ToastError
	}
	u.toasts.Show(k, msg)
}

// pickFile shows the native dialog, which blocks, off the UI goroutine.
func (u *UI) pickFile() {
	go func() {
		if p, ok := platform.PickFile("Open any file"); ok {
			u.app.Post(func() { u.sess.Open(p) })
		}
	}()
}

// pickFolder asks for a folder, then runs fn with it on the UI goroutine.
func (u *UI) pickFolder(title string, fn func(dir string)) {
	go func() {
		if dir, ok := platform.PickFolder(title); ok {
			u.app.Post(func() { fn(dir) })
		}
	}()
}

func (u *UI) view() giode.Element {
	root, sel := u.sess.Root(), u.sess.Selected()
	var body giode.Element
	if root == nil {
		body = u.emptyState()
	} else {
		body = giode.Row(giode.Styles{Width: -1, Height: -1, Wrap: giode.WrapNoWrap},
			giode.Box(giode.Styles{Width: 400, Height: -1, Background: colSidebar}, u.tree.view(root)),
			giode.Box(giode.Styles{Width: 1, Height: -1, Background: colBorder}),
			giode.Box(giode.Styles{FlexGrow: 1, Width: -1, Height: -1}, u.inspector.view(sel)),
		)
	}
	return giode.Stack(giode.Styles{Width: -1, Height: -1, Background: colBg},
		u.bar,
		giode.Box(giode.Styles{Width: -1, FlexGrow: 1}, body),
		u.statusBar(root),
		u.toasts,
	)
}

// --- empty state ----------------------------------------------------------------

func (u *UI) emptyState() giode.Element {
	features := []struct{ ic, title, text string }{
		{"scan", "Identifies anything", "Signatures, structure probes and TrID, plus its own heuristics when those fail."},
		{"layers", "Explores inside", "Archives, Office files, PDFs, installers, mail, disc images, media streams, blobs."},
		{"external", "Opens in your apps", "Every item opens in the viewer your system uses for its real type."},
	}
	var cards []giode.Element
	for _, f := range features {
		cards = append(cards, giode.Box(giode.Styles{
			FlexDirection: giode.FlexDirectionColumn, Gap: 8, Width: 230,
			Padding: giode.UniformInset(16), Background: colSurface, BorderRadius: 12,
			BorderWidth: 1, BorderColor: colBorder,
		},
			giode.Box(giode.Styles{Padding: giode.UniformInset(8), Background: colAccentDim, BorderRadius: 9}, icon(f.ic, colAccent, 18)),
			giode.Text(f.title, txtw(14, colText, giode.Styles{FontWeight: giode.FontWeightBold})),
			giode.Text(f.text, txtw(12.5, colMuted, giode.Styles{LineHeight: 18})),
		))
	}
	return giode.Stack(giode.Styles{Width: -1, Height: -1, Align: giode.AlignCenter, Justify: giode.JustifyCenter, Gap: 28},
		u.dropZone,
		giode.Row(giode.Styles{Gap: 16, Justify: giode.JustifyCenter, Wrap: giode.WrapNoWrap}, cards...),
	)
}

func (u *UI) dropZoneView(s giode.PressState) giode.Element {
	border, bg := colBorderHi, colSurface
	if s.Hovered || s.Focused {
		border, bg = colAccent, "#151c2c"
	}
	hint := "or choose one, and AnyFile will tell you what it really is and what's inside"
	title := "Drop any file here"
	if !platform.FileDropSupported() {
		title, hint = "Open any file", "AnyFile will tell you what it really is and what's inside"
	}
	return giode.Box(giode.Styles{
		FlexDirection: giode.FlexDirectionColumn, Align: giode.AlignCenter, Justify: giode.JustifyCenter,
		Gap: 14, Width: 720, Height: 260, Background: bg, BorderRadius: 20,
		BorderWidth: 2, BorderColor: border,
	},
		giode.Box(giode.Styles{Padding: giode.UniformInset(16), BorderRadius: 18,
			BackgroundGradient: giode.Linear(135, "#7c83ff33", "#22d3ee22")}, icon("upload", colAccent2, 34)),
		giode.Text(title, txtw(22, colText, giode.Styles{FontWeight: giode.FontWeightBold})),
		giode.Text(hint, txt(13.5, colMuted)),
		u.emptyOpenBtn,
	)
}

// --- status bar -------------------------------------------------------------------

func (u *UI) statusBar(root *session.Node) giode.Element {
	status := func(name string, ok bool) giode.Element {
		c, ic := colSuccess, "check"
		if !ok {
			c, ic = colFaint, "x"
		}
		return giode.Row(giode.Styles{Gap: 4, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			icon(ic, c, 11), giode.Text(name, txt(11.5, c)))
	}
	engines := []giode.Element{giode.Text("Engines", txt(11.5, colFaint))}
	for _, t := range tools.All() {
		engines = append(engines, status(t.Name, t.Found()))
	}
	// Oodle comes with games, so whether it's available depends on
	// where the opened file lives.
	near := ""
	if root != nil {
		near = root.Local
	}
	engines = append(engines,
		status("Oodle", oodle.Find(near) != ""),
		status(fmt.Sprintf("Built-in (signatures, carver, %d engines)", explore.EngineCount()), true))
	right := giode.Text("AnyFile "+u.version+" · Drop a file anywhere in the window", txt(11.5, colFaint))
	if !platform.FileDropSupported() {
		right = giode.Text("AnyFile "+u.version+" · Use Open file to inspect a file", txt(11.5, colFaint))
	}
	if root != nil {
		right = giode.Text(root.Local, txtw(11.5, colFaint, giode.Styles{MaxLines: 1, TextOverflow: giode.TextOverflowEllipsis}))
	}
	return giode.Row(giode.Styles{
		Width: -1, Height: 28, Align: giode.AlignCenter, Justify: giode.JustifySpaceBetween,
		Padding: giode.SymmetricInset(14, 0), Background: colSidebar, Wrap: giode.WrapNoWrap,
	},
		giode.Row(giode.Styles{Gap: 14, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap}, engines...),
		giode.Box(giode.Styles{MaxWidth: 520}, right),
	)
}
