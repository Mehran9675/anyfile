package ui

import (
	"fmt"
	"strings"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/anyfile/internal/platform"
	"github.com/mehran9675/anyfile/internal/session"
	"github.com/mehran9675/anyfile/internal/tools"
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/components/scroll"
)

// inspector is the right panel: what the selected item is, what can be
// done with it, and the evidence behind the verdict.
type inspector struct {
	u      *UI
	scroll *scroll.Scroll
	node   *session.Node
	root   *session.Node

	view_, with, save, extract, reveal, deep, trid, magika *button.Button
	copyBtns                                               map[string]giode.Element
}

func newInspector(u *UI) *inspector {
	in := &inspector{u: u, copyBtns: map[string]giode.Element{}}
	in.scroll = giode.Scroll(in.content, giode.Styles{ScrollBar: giode.ScrollBar{ThumbColor: "#ffffff22", Width: 6, Radius: 3}})
	// Handlers run on the UI goroutine, where the sessions live, and act
	// on the active tab's session at click time.
	act := func(fn func(s *session.Session, n *session.Node)) func() {
		return func() {
			if t := u.active; t != nil && in.node != nil {
				fn(t.sess, in.node)
			}
		}
	}
	in.view_ = newBtn("eye", "Open in viewer", btnPrimary, act((*session.Session).View))
	in.with = newBtn("external", "Open with…", btnSecondary, act((*session.Session).ViewWith))
	in.save = newBtn("download", "Save file…", btnSecondary, act(func(s *session.Session, n *session.Node) {
		u.pickFolder("Save "+n.Name+" to…", func(dir string) { s.SaveAs(n, dir) })
	}))
	in.extract = newBtn("extract", "Extract all…", btnSecondary, act(func(s *session.Session, n *session.Node) {
		u.pickFolder("Extract the contents of "+n.Name+" to…", func(dir string) { s.ExtractAll(n, dir) })
	}))
	in.reveal = newBtn("reveal", "Show in folder", btnGhost, act((*session.Session).Reveal))
	in.deep = newBtn("scan", "Deep scan", btnGhost, act((*session.Session).DeepScan))
	in.trid = newBtn("sparkles", "Ask TrID", btnGhost, act((*session.Session).AskTrID))
	in.magika = newBtn("brain", "Ask Magika", btnGhost, act((*session.Session).AskMagika))
	for _, k := range []string{"MD5", "SHA-1", "SHA-256"} {
		k := k
		in.copyBtns[k] = giode.CopyButton(func() string {
			if n := in.node; n != nil && n.Hashes != nil {
				return n.Hashes[k]
			}
			return ""
		}, giode.Styles{Color: colMuted})
	}
	return in
}

func (in *inspector) view(sel *session.Node) giode.Element {
	in.node = sel
	return giode.Box(giode.Styles{Width: -1, Height: -1, Padding: giode.Inset{Right: 4}}, in.scroll)
}

func (in *inspector) content() giode.Element {
	n := in.node
	if n == nil {
		return giode.Box(giode.Styles{})
	}
	sections := []giode.Element{in.header(n)}
	if b := in.banners(n); b != nil {
		sections = append(sections, b...)
	}
	sections = append(sections, in.actions(n))
	if !n.Dir {
		sections = append(sections, in.identity(n))
		if d := in.details(n); d != nil {
			sections = append(sections, d)
		}
		sections = append(sections, in.magikaCard(n), in.tridCard(n))
		if h := in.hashes(n); h != nil {
			sections = append(sections, h)
		}
	} else {
		sections = append(sections, in.folderCard(n))
	}
	return giode.Box(giode.Styles{Width: -1, Justify: giode.JustifyCenter, Padding: giode.Inset{Top: 22, Right: 26, Bottom: 30, Left: 26}},
		giode.Stack(giode.Styles{Width: -1, MaxWidth: 940, Gap: 16}, sections...))
}

func (in *inspector) header(n *session.Node) giode.Element {
	cat := n.Category()
	c := catColor(cat)
	label := "Identifying…"
	if n.Dir {
		label = fmt.Sprintf("Folder inside %s", n.Container().Name)
	} else if r := n.Result; r != nil {
		label = r.Label
	}
	trail := n.Trail()
	crumbs := []giode.Element{}
	for i, s := range trail {
		if i > 0 {
			crumbs = append(crumbs, icon("chevR", colFaint, 12))
		}
		crumbs = append(crumbs, giode.Text(s, txtw(12, colMuted, giode.Styles{MaxLines: 1})))
	}
	chips := []giode.Element{chip(string(cat), c, "tag")}
	if r := n.Result; r != nil && !n.Dir {
		if r.Ext != "" {
			chips = append(chips, chip("."+r.Ext, colTextSoft, ""))
		}
		chips = append(chips, chip(fmt.Sprintf("%s · %.0f%%", r.Source, r.Confidence*100), confColor(r.Confidence), "check"))
	}
	if n.Size >= 0 {
		chips = append(chips, chip(detect.HumanSize(n.Size), colTextSoft, ""))
	}
	return giode.Stack(giode.Styles{Width: -1, Gap: 14},
		giode.Row(giode.Styles{Gap: 6, Align: giode.AlignCenter}, crumbs...),
		giode.Row(giode.Styles{Width: -1, Gap: 18, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			giode.Box(giode.Styles{Width: 64, Height: 64, Align: giode.AlignCenter, Justify: giode.JustifyCenter, BorderRadius: 16,
				BackgroundGradient: giode.Linear(135, c+"40", c+"14"), BorderWidth: 1, BorderColor: c + "50"},
				icon(catIcon(cat), c, 30)),
			giode.Stack(giode.Styles{FlexGrow: 1, Width: -1, Gap: 4},
				giode.Text(n.Name, txtw(22, colText, giode.Styles{FontWeight: giode.FontWeightBold, MaxLines: 2})),
				giode.Text(label, txt(14, colTextSoft)),
			),
		),
		giode.Row(giode.Styles{Gap: 8}, chips...),
	)
}

func confColor(c float64) string {
	switch {
	case c >= 0.85:
		return colSuccess
	case c >= 0.55:
		return colWarning
	}
	return colDanger
}

func banner(color, iconName, text string, extra ...giode.Element) giode.Element {
	kids := []giode.Element{icon(iconName, color, 18),
		giode.Box(giode.Styles{FlexGrow: 1, Width: -1}, giode.Text(text, txtw(13, colText, giode.Styles{LineHeight: 19})))}
	kids = append(kids, extra...)
	return giode.Row(giode.Styles{Width: -1, Gap: 12, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap,
		Padding: giode.SymmetricInset(16, 12), Background: color + "14", BorderRadius: 12, BorderWidth: 1, BorderColor: color + "40"},
		kids...)
}

func (in *inspector) banners(n *session.Node) []giode.Element {
	var out []giode.Element
	if n.Busy != "" {
		out = append(out, banner(colAccent, "sparkles", n.Busy, giode.Spinner(giode.Styles{Width: 16, Height: 16, Color: colAccent})))
	}
	if n.Err != "" {
		out = append(out, banner(colWarning, "alert", n.Err))
	}
	if b := badgeFor(n); b != "" && n.Result != nil {
		ext := n.Ext()
		what := "has no extension"
		if ext != "" {
			what = "is named ." + ext
		}
		out = append(out, banner(colAccent2, "info", fmt.Sprintf("This file %s but is really %s (.%s). Viewing hands it to the app for .%s.", what, article(n.Result.Label), n.Result.Ext, n.Result.Ext)))
	}
	return out
}

func article(s string) string {
	if s == "" {
		return s
	}
	switch strings.ToLower(s[:1]) {
	case "a", "e", "i", "o", "u":
		return "an " + s
	}
	return "a " + s
}

func (in *inspector) actions(n *session.Node) giode.Element {
	var b []giode.Element
	container := n.Explored && len(n.Kids) > 0
	if !n.Dir {
		b = append(b, in.view_)
		if platform.CanOpenWith() {
			b = append(b, in.with)
		}
		// Only members of a container can be extracted; the opened file
		// itself is already a file.
		if !n.IsRoot() {
			b = append(b, in.save)
		}
	}
	if container || n.Dir {
		in.extract.Label("Extract all…")
		if !n.Dir {
			in.extract.Label(fmt.Sprintf("Extract %d items…", countFiles(n)))
		}
		b = append(b, in.extract)
	}
	if !n.Dir {
		b = append(b, giode.Box(giode.Styles{Width: 6}), in.reveal, in.deep)
		if tools.Magika().Found() {
			b = append(b, in.magika)
		}
		if tools.TrID().Found() {
			b = append(b, in.trid)
		}
	}
	for _, x := range []*button.Button{in.view_, in.with, in.save, in.extract, in.deep, in.trid, in.magika} {
		x.Disabled(n.Busy != "")
	}
	return giode.Row(giode.Styles{Width: -1, Gap: 8, Align: giode.AlignCenter}, b...)
}

func countFiles(n *session.Node) int {
	c := 0
	for _, k := range n.Kids {
		if k.Dir {
			c += countFiles(k)
		} else {
			c++
		}
	}
	return c
}

func (in *inspector) identity(n *session.Node) giode.Element {
	r := n.Result
	rows := []giode.Element{sectionTitle("scan", "Identification")}
	if r == nil {
		rows = append(rows, giode.Text("Analysing…", txt(13, colMuted)))
		return card(giode.Styles{}, rows...)
	}
	mime := r.MIME
	if mime == "" {
		mime = "Unknown"
	}
	rows = append(rows,
		kv("Type", r.Label, false),
		kv("Category", string(r.Category), false),
		kv("Extension", dash("."+r.Ext, r.Ext == ""), false),
		kv("Media type", mime, true),
		kv("Identified by", engineText(r), false),
		giode.Row(giode.Styles{Width: -1, Gap: 12, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			giode.Box(giode.Styles{Width: 132}, giode.Text("Confidence", txt(13, colMuted))),
			bar(float32(r.Confidence), confColor(r.Confidence), 180),
			giode.Text(fmt.Sprintf("%.0f%%", r.Confidence*100), txt(12.5, colTextSoft)),
		),
		giode.Row(giode.Styles{Width: -1, Gap: 12, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			giode.Box(giode.Styles{Width: 132}, giode.Text("Entropy", txt(13, colMuted))),
			bar(float32(r.Entropy/8), entropyColor(r.Entropy), 180),
			giode.Text(fmt.Sprintf("%.2f bits/byte · %s", r.Entropy, entropyWord(r.Entropy)), txt(12.5, colTextSoft)),
		),
	)
	if n.Listing != nil {
		rows = append(rows, kv("Contents", fmt.Sprintf("%d items · read by %s", countFiles(n), n.Listing.Engine), false))
		if n.Listing.Format != "" {
			rows = append(rows, kv("Container format", n.Listing.Format, false))
		}
	} else if n.Leaf {
		rows = append(rows, kv("Contents", "Nothing extractable inside. Open it in its viewer.", false))
	}
	if n.Local != "" && !n.IsRoot() {
		rows = append(rows, kv("Extracted copy", n.Local, true))
	}
	return card(giode.Styles{}, rows...)
}

func dash(s string, empty bool) string {
	if empty {
		return "None"
	}
	return s
}

func engineText(r *detect.Result) string {
	switch r.Source {
	case detect.SrcSignature:
		return "Built-in signature database"
	case detect.SrcStructure:
		return "Built-in structural analysis"
	case detect.SrcTrID:
		return "TrID"
	case detect.SrcMagika:
		return "Magika (deep-learning model)"
	}
	var asked []string
	if r.MagikaRan {
		asked = append(asked, "Magika")
	}
	if r.TrIDRan {
		asked = append(asked, "TrID")
	}
	if len(asked) > 0 {
		return "AnyFile heuristics (" + strings.Join(asked, " and ") + " consulted)"
	}
	return "AnyFile heuristics"
}

func entropyColor(e float64) string {
	switch {
	case e > 7.5:
		return "#f472b6"
	case e > 5:
		return colAccent
	}
	return colAccent2
}

func entropyWord(e float64) string {
	switch {
	case e > 7.9:
		return "compressed or encrypted"
	case e > 7:
		return "dense binary"
	case e > 5:
		return "mixed"
	case e > 3:
		return "text-like"
	}
	return "sparse"
}

func (in *inspector) details(n *session.Node) giode.Element {
	r := n.Result
	if r == nil {
		return nil
	}
	var rows []giode.Element
	for _, d := range r.Details {
		if d.Key == "Entropy" {
			continue
		}
		rows = append(rows, kv(d.Key, d.Value, false))
	}
	notes := append([]string(nil), r.Notes...)
	if n.Listing != nil {
		notes = append(notes, n.Listing.Notes...)
	}
	if len(rows) == 0 && len(notes) == 0 {
		return nil
	}
	out := []giode.Element{sectionTitle("info", "Details")}
	out = append(out, rows...)
	for _, s := range notes {
		out = append(out, giode.Row(giode.Styles{Width: -1, Gap: 10, Align: giode.AlignStart, Wrap: giode.WrapNoWrap,
			Padding: giode.SymmetricInset(12, 9), Background: colSurface2, BorderRadius: 9},
			giode.Box(giode.Styles{Padding: giode.Inset{Top: 2}}, icon("info", colAccent2, 14)),
			giode.Box(giode.Styles{FlexGrow: 1, Width: -1}, giode.Text(s, txtw(12.5, colTextSoft, giode.Styles{LineHeight: 18})))))
	}
	return card(giode.Styles{}, out...)
}

func (in *inspector) magikaCard(n *session.Node) giode.Element {
	r := n.Result
	out := []giode.Element{sectionTitle("brain", "Magika")}
	muted := func(s string) giode.Element {
		return giode.Text(s, txtw(12.5, colMuted, giode.Styles{LineHeight: 18}))
	}
	switch {
	case r == nil:
		out = append(out, muted("Waiting for the built-in analysis…"))
	case !tools.Magika().Found():
		out = append(out, muted("Magika is not installed. It is Google's deep-learning content-type model (200+ types, strong on text, code and files without a header). Install it with \"pip install magika\" or set its path in Settings."))
	case !r.MagikaRan:
		out = append(out, muted("Not needed: the built-in engines were confident. Use \"Ask Magika\" for a second opinion."))
	default:
		v := r.Magika
		label := v.Description
		if !v.Known() {
			label = "No confident answer"
		}
		out = append(out,
			kv("Verdict", label, false),
			giode.Row(giode.Styles{Width: -1, Gap: 12, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
				giode.Box(giode.Styles{Width: 132}, giode.Text("Model score", txt(13, colMuted))),
				bar(float32(v.Score), confColor(v.Score), 180),
				giode.Text(fmt.Sprintf("%.0f%%", v.Score*100), txt(12.5, colTextSoft)),
			),
		)
		if v.Known() {
			out = append(out, kv("Group", v.Group, false), kv("Media type", v.MIME, true))
			if len(v.Extensions) > 0 {
				out = append(out, kv("Extensions", "."+strings.Join(v.Extensions, ", ."), false))
			}
		}
		if v.RawLabel != "" && v.RawLabel != v.Label {
			out = append(out, kv("Model's raw guess", v.RawDescription, false))
		}
		if r.Source != detect.SrcMagika && v.Known() {
			out = append(out, muted("The verdict above comes from a more confident engine; Magika's answer is shown for comparison."))
		}
	}
	return card(giode.Styles{}, out...)
}

func (in *inspector) tridCard(n *session.Node) giode.Element {
	r := n.Result
	out := []giode.Element{sectionTitle("sparkles", "TrID")}
	switch {
	case r == nil:
		out = append(out, giode.Text("Waiting for the built-in analysis…", txt(13, colMuted)))
	case !tools.TrID().Found():
		out = append(out, giode.Text("TrID is not installed. Built-in engines handle detection; place trid.exe and triddefs.trd next to AnyFile (or set ANYFILE_TRID) to add its 18,000+ definitions.", txtw(12.5, colMuted, giode.Styles{LineHeight: 18})))
	case !r.TrIDRan:
		out = append(out, giode.Text("Not needed: the built-in engines were confident. Use \"Ask TrID\" for a second opinion.", txt(12.5, colMuted)))
	case len(r.Candidates) == 0:
		out = append(out, giode.Text("TrID could not identify this file; AnyFile's own analysis was used.", txt(12.5, colMuted)))
	default:
		for _, c := range r.Candidates {
			ext := c.Ext
			if ext == "" {
				ext = "?"
			}
			out = append(out, giode.Row(giode.Styles{Width: -1, Gap: 12, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
				giode.Box(giode.Styles{Width: 52}, giode.Text(fmt.Sprintf("%.1f%%", c.Percent), txtw(12.5, colText, giode.Styles{FontWeight: giode.FontWeightBold}))),
				bar(float32(c.Percent/100), colAccent, 120),
				giode.Box(giode.Styles{Width: 64}, giode.Text("."+strings.ToLower(ext), txtw(12, colAccent2, giode.Styles{FontFamily: fontMono}))),
				giode.Box(giode.Styles{FlexGrow: 1, Width: -1}, giode.Text(c.Label, txtw(12.5, colTextSoft, giode.Styles{MaxLines: 1, TextOverflow: giode.TextOverflowEllipsis}))),
			))
		}
	}
	return card(giode.Styles{}, out...)
}

func (in *inspector) hashes(n *session.Node) giode.Element {
	out := []giode.Element{sectionTitle("hash", "Checksums")}
	if n.Hashes == nil {
		out = append(out, giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter},
			giode.Spinner(giode.Styles{Width: 14, Height: 14, Color: colAccent}), giode.Text("Computing…", txt(12.5, colMuted))))
		return card(giode.Styles{}, out...)
	}
	for _, k := range []string{"MD5", "SHA-1", "SHA-256"} {
		out = append(out, giode.Row(giode.Styles{Width: -1, Gap: 12, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			giode.Box(giode.Styles{Width: 132}, giode.Text(k, txt(13, colMuted))),
			giode.Box(giode.Styles{FlexGrow: 1, Width: -1}, giode.Text(n.Hashes[k], txtw(12, colText, giode.Styles{FontFamily: fontMono, MaxLines: 1, TextOverflow: giode.TextOverflowEllipsis}))),
			in.copyBtns[k],
		))
	}
	return card(giode.Styles{}, out...)
}

func (in *inspector) folderCard(n *session.Node) giode.Element {
	return card(giode.Styles{},
		sectionTitle("folder", "Folder"),
		kv("Items", fmt.Sprint(countFiles(n)), false),
		kv("Total size", detect.HumanSize(max(n.Size, 0)), false),
		kv("Path inside", n.Path, true),
		kv("Container", n.Container().Name, false),
	)
}
