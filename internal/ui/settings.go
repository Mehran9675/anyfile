package ui

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/mehran9675/anyfile/internal/platform"
	"github.com/mehran9675/anyfile/internal/settings"
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/components/input"
	"github.com/mehran9675/giode/components/scroll"
	"github.com/mehran9675/giode/components/segmented"
	"github.com/mehran9675/giode/components/toggle"
)

// settingsPage edits the limits and preferences in package settings.
// Every valid change applies immediately and is saved.
type settingsPage struct {
	u      *UI
	scroll *scroll.Scroll

	numbers     map[string]*numberField
	checks      map[string]*toggle.Switch
	checkLabels map[string]string
	tridMode    *segmented.Control
	magikaMode  *segmented.Control
	paths       map[string]*pathField

	reset, done, showFile, installAll *button.Button
	menuAdd, menuRemove               *button.Button
}

// numberField is a text input that only accepts whole numbers within
// the setting's limits. giode has no number input yet.
type numberField struct {
	in   *input.Input
	unit string
	err  string
	get  func(settings.Settings) int
	set  func(*settings.Settings, int)
	lim  settings.Limit
}

// pathField is a text input with a Browse button for a tool or folder.
type pathField struct {
	in     *input.Input
	browse *button.Button
	err    string
	get    func(settings.Settings) string
	set    func(*settings.Settings, string)
}

var tridModes = []string{settings.TrIDWhenNeeded, settings.TrIDAlways, settings.TrIDNever}
var tridModeLabels = []string{"If unsure", "Always", "Never"}

func newSettingsPage(u *UI) *settingsPage {
	sp := &settingsPage{u: u, numbers: map[string]*numberField{}, checks: map[string]*toggle.Switch{}, checkLabels: map[string]string{}, paths: map[string]*pathField{}}
	sp.scroll = giode.Scroll(sp.content, giode.Styles{ScrollBar: giode.ScrollBar{ThumbColor: "#ffffff22", Width: 6, Radius: 3}})
	field := giode.Styles{Background: colSurface2, Color: colText, BorderColor: colBorderHi, BorderRadius: 8, Padding: giode.SymmetricInset(10, 7), FontSize: 13}

	num := func(key, unit string, get func(settings.Settings) int, set func(*settings.Settings, int)) {
		f := &numberField{unit: unit, get: get, set: set, lim: settings.Limits[key]}
		f.in = giode.Input.Text("", field.With(giode.Styles{Width: 110})).Label(key).
			OnChange(func(text string) { sp.changeNumber(f, text) })
		sp.numbers[key] = f
	}
	num("head_kb", "KB", func(s settings.Settings) int { return s.HeadKB }, func(s *settings.Settings, v int) { s.HeadKB = v })
	num("entropy_sample_mb", "MB", func(s settings.Settings) int { return s.EntropySampleMB }, func(s *settings.Settings, v int) { s.EntropySampleMB = v })
	num("trid_timeout_sec", "seconds", func(s settings.Settings) int { return s.TrIDTimeoutSec }, func(s *settings.Settings, v int) { s.TrIDTimeoutSec = v })
	num("trid_trust_pct", "%", func(s settings.Settings) int { return int(s.TrIDTrust*100 + 0.5) }, func(s *settings.Settings, v int) { s.TrIDTrust = float64(v) / 100 })
	num("prefetch_count", "files", func(s settings.Settings) int { return s.PrefetchCount }, func(s *settings.Settings, v int) { s.PrefetchCount = v })
	num("prefetch_max_mb", "MB", func(s settings.Settings) int { return s.PrefetchMaxMB }, func(s *settings.Settings, v int) { s.PrefetchMaxMB = v })
	num("carve_limit", "objects", func(s settings.Settings) int { return s.CarveLimit }, func(s *settings.Settings, v int) { s.CarveLimit = v })
	num("max_text_mb", "MB", func(s settings.Settings) int { return s.MaxTextMB }, func(s *settings.Settings, v int) { s.MaxTextMB = v })
	num("max_mail_mb", "MB", func(s settings.Settings) int { return s.MaxMailMB }, func(s *settings.Settings, v int) { s.MaxMailMB = v })
	num("max_pdf_mb", "MB", func(s settings.Settings) int { return s.MaxPDFMB }, func(s *settings.Settings, v int) { s.MaxPDFMB = v })
	num("max_decompress_mb", "MB", func(s settings.Settings) int { return s.MaxDecompressMB }, func(s *settings.Settings, v int) { s.MaxDecompressMB = v })
	num("checksum_max_mb", "MB", func(s settings.Settings) int { return s.ChecksumMaxMB }, func(s *settings.Settings, v int) { s.ChecksumMaxMB = v })

	check := func(key, label string, get func(settings.Settings) bool, set func(*settings.Settings, bool)) {
		// Unlabelled, so it lines up on the right like the other controls;
		// checkRow draws the label.
		c := giode.Input.Switch("", giode.Styles{Background: colAccent, BorderColor: colBorderHi})
		sp.checkLabels[key] = label
		c.SetChecked(get(settings.Get()))
		c.OnChange(func(v bool) { sp.apply(func(s *settings.Settings) { set(s, v) }) })
		sp.checks[key] = c
	}
	check("auto_carve", "Scan unknown files for embedded objects automatically",
		func(s settings.Settings) bool { return s.AutoCarve }, func(s *settings.Settings, v bool) { s.AutoCarve = v })
	check("checksums", "Compute MD5, SHA-1 and SHA-256 when a file is selected",
		func(s settings.Settings) bool { return s.Checksums }, func(s *settings.Settings, v bool) { s.Checksums = v })

	// Width -1 inside a fixed-width box (see segmentedBox): giode's
	// Segmented measures each segment as wide as the space it's given.
	modes := giode.Styles{Width: -1, Background: colSurface2, BorderColor: colRaised, Color: colText, FontSize: 13}
	sp.tridMode = giode.Input.Segmented(tridModeLabels...).Styles(modes).
		OnChange(func(i int) { sp.apply(func(s *settings.Settings) { s.TrIDMode = tridModes[i] }) })
	sp.magikaMode = giode.Input.Segmented(tridModeLabels...).Styles(modes).
		OnChange(func(i int) { sp.apply(func(s *settings.Settings) { s.MagikaMode = tridModes[i] }) })

	path := func(key string, folder bool, get func(settings.Settings) string, set func(*settings.Settings, string)) {
		f := &pathField{get: get, set: set}
		placeholder := "Find automatically"
		if folder {
			placeholder = "System temporary folder"
		}
		f.in = giode.Input.Text(placeholder, field.With(giode.Styles{Width: -1})).Label(key).
			OnChange(func(text string) { sp.changePath(f, text, folder) })
		f.browse = newBtn("open", "Browse…", btnSecondary, func() {
			go func() {
				var p string
				var ok bool
				if folder {
					p, ok = platform.PickFolder("Choose a folder")
				} else {
					p, ok = platform.PickFile("Choose the program")
				}
				if ok {
					u.app.Post(func() { f.in.SetText(p); sp.changePath(f, p, folder) })
				}
			}()
		})
		sp.paths[key] = f
	}
	path("trid_path", false, func(s settings.Settings) string { return s.TrIDPath }, func(s *settings.Settings, v string) { s.TrIDPath = v })
	path("magika_path", false, func(s settings.Settings) string { return s.MagikaPath }, func(s *settings.Settings, v string) { s.MagikaPath = v })
	path("sevenzip_path", false, func(s settings.Settings) string { return s.SevenZipPath }, func(s *settings.Settings, v string) { s.SevenZipPath = v })
	path("ffprobe_path", false, func(s settings.Settings) string { return s.FFprobePath }, func(s *settings.Settings, v string) { s.FFprobePath = v })
	path("ffmpeg_path", false, func(s settings.Settings) string { return s.FFmpegPath }, func(s *settings.Settings, v string) { s.FFmpegPath = v })
	path("oodle_path", false, func(s settings.Settings) string { return s.OodlePath }, func(s *settings.Settings, v string) { s.OodlePath = v })
	path("temp_dir", true, func(s settings.Settings) string { return s.TempDir }, func(s *settings.Settings, v string) { s.TempDir = v })

	sp.reset = newBtn("reset", "Reset to defaults", btnGhost, func() {
		if err := settings.Set(settings.Defaults()); err != nil {
			u.toast("error", "Could not save settings: "+err.Error())
		}
		sp.load()
		u.toast("ok", "Settings reset to defaults")
	})
	sp.showFile = newBtn("reveal", "Show settings file", btnGhost, func() {
		go platform.Reveal(settings.Path())
	})
	sp.done = newBtn("check", "Done", btnPrimary, func() { u.showSettings = false })
	sp.menuAdd = newBtn("check", "Add", btnSecondary, func() {
		go func() {
			err := u.app.InstallContextActions()
			u.app.Post(func() {
				if err != nil {
					u.toast("error", "Could not add the menu entry: "+err.Error())
					return
				}
				u.toast("ok", "\"Inspect with AnyFile\" added to the right-click menu")
			})
		}()
	})
	sp.menuRemove = newBtn("x", "Remove", btnGhost, func() {
		go func() {
			err := u.app.UninstallContextActions()
			u.app.Post(func() {
				if err != nil {
					u.toast("error", "Could not remove the menu entry: "+err.Error())
					return
				}
				u.toast("ok", "\"Inspect with AnyFile\" removed from the right-click menu")
			})
		}()
	})
	sp.installAll = newBtn("download", "Install missing engines", btnPrimary, u.installMissing)
	sp.load()
	return sp
}

// load fills the controls from the current settings.
func (sp *settingsPage) load() {
	s := settings.Get()
	for _, f := range sp.numbers {
		f.in.SetText(strconv.Itoa(f.get(s)))
		f.err = ""
	}
	for _, f := range sp.paths {
		f.in.SetText(f.get(s))
		f.err = ""
	}
	sp.checks["auto_carve"].SetChecked(s.AutoCarve)
	sp.checks["checksums"].SetChecked(s.Checksums)
	for i, m := range tridModes {
		if m == s.MagikaMode {
			sp.magikaMode.SetSelected(i)
		}
		if m == s.TrIDMode {
			sp.tridMode.SetSelected(i)
		}
	}
}

// apply changes the settings, saves them and finds the tools again.
func (sp *settingsPage) apply(change func(s *settings.Settings)) {
	s := settings.Get()
	change(&s)
	if err := settings.Set(s); err != nil {
		sp.u.toast("error", "Could not save settings: "+err.Error())
	}
}

func (sp *settingsPage) changeNumber(f *numberField, text string) {
	v, err := strconv.Atoi(strings.TrimSpace(text))
	switch {
	case err != nil:
		f.err = "Enter a whole number."
	case v < f.lim.Min || v > f.lim.Max:
		f.err = fmt.Sprintf("Enter a number from %d to %d.", f.lim.Min, f.lim.Max)
	default:
		f.err = ""
		sp.apply(func(s *settings.Settings) { f.set(s, v) })
	}
}

func (sp *settingsPage) changePath(f *pathField, text string, folder bool) {
	text = strings.Trim(strings.TrimSpace(text), `"`)
	if text != "" {
		ok := false
		if folder {
			ok = platform.IsDir(text)
		} else {
			ok = platform.IsFile(text)
		}
		if !ok {
			f.err = "Not found."
			if folder {
				f.err = "Folder not found."
			}
			return
		}
	}
	f.err = ""
	sp.apply(func(s *settings.Settings) { f.set(s, text) })
}

func (sp *settingsPage) view() giode.Element {
	return giode.Box(giode.Styles{Width: -1, Height: -1, Padding: giode.Inset{Right: 4}}, sp.scroll)
}

func (sp *settingsPage) content() giode.Element {
	header := giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Justify: giode.JustifySpaceBetween, Wrap: giode.WrapNoWrap},
		giode.Stack(giode.Styles{Gap: 4},
			giode.Text("Settings", txtw(22, colText, giode.Styles{FontWeight: giode.FontWeightBold})),
			giode.Text("Changes apply right away and are saved for next time.", txt(13, colMuted)),
		),
		giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap}, sp.showFile, sp.reset, sp.done),
	)
	ident := card(giode.Styles{},
		sectionTitle("scan", "Identification"),
		sp.numberRow("head_kb", "Bytes read from each file", "The start of a file holds its signature. The disc-image check reads up to 36 KB in, so 40 KB is the minimum. Larger samples help tell text formats apart."),
		sp.numberRow("entropy_sample_mb", "Entropy sample", "Data sampled across the file to estimate how random it is."),
		sp.row("Ask Magika", "Google's deep-learning model: fast, and strong on text, code and files without a header. Also asked about generic plain text.", segmentedBox(sp.magikaMode), ""),
		sp.row("Ask TrID", "TrID has 18,000+ definitions but is slower than the built-in engines.", segmentedBox(sp.tridMode), ""),
		sp.numberRow("trid_trust_pct", "Skip Magika and TrID above", "Built-in confidence at which neither is asked."),
		sp.numberRow("trid_timeout_sec", "TrID time limit", "Give up on TrID after this long."),
	)
	explore := card(giode.Styles{},
		sectionTitle("layers", "Exploring"),
		sp.numberRow("prefetch_count", "Identify in the background", "Members of a container identified right after it opens, so the tree shows their real types."),
		sp.numberRow("prefetch_max_mb", "Background size limit", "Larger members are identified when you select them."),
		sp.checkRow("auto_carve", "Used when no format engine understands a file. \"Deep scan\" always works."),
		sp.numberRow("carve_limit", "Embedded objects per scan", "Most objects a scan reports."),
		sp.numberRow("max_decompress_mb", "Decompression memory", "Memory a zstd stream may use. Raise it if a file reports the decompression limit."),
		sp.numberRow("max_text_mb", "Text scanned for encoded files", "Largest text file searched for base64 and data: URIs."),
		sp.numberRow("max_mail_mb", "Largest email", "Largest .eml, .mht or mailbox explored."),
		sp.numberRow("max_pdf_mb", "Largest PDF", "Largest PDF explored for attachments, images and fonts."),
	)
	fileRows := []giode.Element{
		sectionTitle("file", "Files"),
		sp.checkRow("checksums", "Hashes are read from the whole file."),
		sp.numberRow("checksum_max_mb", "Checksum size limit", "Skip checksums for larger files. 0 means no limit."),
		sp.pathRow("temp_dir", "Extraction folder", "Where members are extracted while you inspect them. Cleaned up when the tab closes."),
	}
	if menu := sp.contextMenuRow(); menu != nil {
		fileRows = append(fileRows, menu)
	}
	files := card(giode.Styles{}, fileRows...)
	engineHead := giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Justify: giode.JustifySpaceBetween, Wrap: giode.WrapNoWrap},
		sectionTitle("cpu", "Engines"), giode.Box(giode.Styles{}))
	if sp.u.anyMissing() {
		engineHead = giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Justify: giode.JustifySpaceBetween, Wrap: giode.WrapNoWrap},
			sectionTitle("cpu", "Engines"), sp.installAll)
	}
	tools := card(giode.Styles{},
		engineHead,
		giode.Text("AnyFile can download each engine from its publisher and install it for your account only, without administrator rights. Or leave AnyFile to find a copy you already have (environment variable, next to AnyFile, PATH, common install folders), or choose one below.", txtw(12.5, colMuted, giode.Styles{LineHeight: 18})),
		sp.engineRow("trid", "TrID", "18,000+ format definitions, asked when the built-in engines aren't sure.", "trid_path"),
		sp.engineRow("magika", "Magika", "Google's deep-learning content-type model, strong on text, code and files without a header.", "magika_path"),
		sp.engineRow("7zip", "7-Zip", "Opens 7z, rar, cab, wim, vhd(x), dmg, installers and other archives.", "sevenzip_path"),
		sp.engineRow("ffmpeg", "FFmpeg", "Lists media streams (ffprobe) and copies them out without re-encoding (ffmpeg).", "ffmpeg_path", "ffprobe_path"),
		sp.pathRow("oodle_path", "Oodle", "oo2core_*_win64.dll, shipped with many games. It is proprietary, so AnyFile can't install it; it also looks in the opened file's folder."),
	)
	return giode.Box(giode.Styles{Width: -1, Justify: giode.JustifyCenter, Padding: giode.Inset{Top: 22, Right: 26, Bottom: 30, Left: 26}},
		giode.Stack(giode.Styles{Width: -1, MaxWidth: 860, Gap: 16}, header, ident, explore, files, tools))
}

// row lays out a label and help text on the left and a control on the
// right, with an error message under them when there is one.
func (sp *settingsPage) row(label, help string, control giode.Element, err string) giode.Element {
	left := giode.Stack(giode.Styles{FlexGrow: 1, Width: -1, Gap: 3},
		giode.Text(label, txtw(13.5, colText, giode.Styles{FontWeight: giode.FontWeightMedium})),
		giode.Text(help, txtw(12, colMuted, giode.Styles{LineHeight: 17})),
	)
	kids := []giode.Element{giode.Row(giode.Styles{Width: -1, Gap: 18, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap}, left, control)}
	if err != "" {
		kids = append(kids, giode.Row(giode.Styles{Gap: 6, Align: giode.AlignCenter},
			icon("alert", colDanger, 13), giode.Text(err, txt(12, colDanger))))
	}
	return giode.Stack(giode.Styles{Width: -1, Gap: 6, Padding: giode.SymmetricInset(0, 4)}, kids...)
}

func (sp *settingsPage) numberRow(key, label, help string) giode.Element {
	f := sp.numbers[key]
	control := giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
		f.in, giode.Box(giode.Styles{Width: 56}, giode.Text(f.unit, txt(12.5, colMuted))))
	return sp.row(label, help, control, f.err)
}

func (sp *settingsPage) checkRow(key, help string) giode.Element {
	return sp.row(sp.checkLabels[key], help, sp.checks[key], "")
}

func (sp *settingsPage) pathRow(key, label, help string) giode.Element {
	f := sp.paths[key]
	head := giode.Stack(giode.Styles{Width: -1, Gap: 3},
		giode.Text(label, txtw(13.5, colText, giode.Styles{FontWeight: giode.FontWeightMedium})),
		giode.Text(help, txtw(12, colMuted, giode.Styles{LineHeight: 17})),
	)
	kids := []giode.Element{head,
		giode.Row(giode.Styles{Width: -1, Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			giode.Box(giode.Styles{FlexGrow: 1, Width: -1}, f.in), f.browse)}
	if f.err != "" {
		kids = append(kids, giode.Row(giode.Styles{Gap: 6, Align: giode.AlignCenter},
			icon("alert", colDanger, 13), giode.Text(f.err, txt(12, colDanger))))
	}
	return giode.Stack(giode.Styles{Width: -1, Gap: 8, Padding: giode.SymmetricInset(0, 4)}, kids...)
}

// contextMenuRow adds or removes "Inspect with AnyFile" in the file
// manager's right-click menu, for copies the setup program didn't
// install. macOS takes the entry from the app bundle, so it has none.
func (sp *settingsPage) contextMenuRow() giode.Element {
	help := "Adds it to Explorer's menu for your account (under \"Show more options\" on Windows 11). The setup program adds it for you."
	switch runtime.GOOS {
	case "darwin":
		return nil
	case "linux":
		help = "Adds it to GNOME Files and Caja (under Scripts), Dolphin and Nemo for your account."
	}
	return sp.row("\"Inspect with AnyFile\" in the right-click menu", help,
		giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap}, sp.menuRemove, sp.menuAdd), "")
}

// engineRow is an installable engine: what it does, where it stands with
// its Install, Update and Remove buttons, and the paths that override
// the automatic lookup.
func (sp *settingsPage) engineRow(id, label, help string, pathKeys ...string) giode.Element {
	kids := []giode.Element{
		giode.Stack(giode.Styles{Width: -1, Gap: 3},
			giode.Text(label, txtw(13.5, colText, giode.Styles{FontWeight: giode.FontWeightMedium})),
			giode.Text(help, txtw(12, colMuted, giode.Styles{LineHeight: 17})),
		),
		sp.u.engines[id].view(),
	}
	for _, key := range pathKeys {
		f := sp.paths[key]
		row := []giode.Element{giode.Box(giode.Styles{FlexGrow: 1, Width: -1}, f.in), f.browse}
		if len(pathKeys) > 1 {
			// Several programs: name each field.
			name := strings.TrimSuffix(key, "_path")
			row = append([]giode.Element{giode.Box(giode.Styles{Width: 64}, giode.Text(name, txt(12.5, colMuted)))}, row...)
		}
		kids = append(kids, giode.Row(giode.Styles{Width: -1, Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap}, row...))
		if f.err != "" {
			kids = append(kids, giode.Row(giode.Styles{Gap: 6, Align: giode.AlignCenter},
				icon("alert", colDanger, 13), giode.Text(f.err, txt(12, colDanger))))
		}
	}
	return giode.Stack(giode.Styles{Width: -1, Gap: 8, Padding: giode.Inset{Top: 6, Bottom: 6}}, kids...)
}

// segmentedBox gives a segmented control a fixed width to divide among
// its segments.
func segmentedBox(c giode.Element) giode.Element {
	return giode.Box(giode.Styles{Width: 330}, c)
}
