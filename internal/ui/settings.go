package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mehran9675/anyfile/internal/platform"
	"github.com/mehran9675/anyfile/internal/settings"
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/components/checkbox"
	"github.com/mehran9675/giode/components/dropdown"
	"github.com/mehran9675/giode/components/input"
	"github.com/mehran9675/giode/components/scroll"
)

// settingsPage edits the limits and preferences in package settings.
// Every valid change applies immediately and is saved.
type settingsPage struct {
	u      *UI
	scroll *scroll.Scroll

	numbers    map[string]*numberField
	checks     map[string]*checkbox.Checkbox
	tridMode   *dropdown.Select
	magikaMode *dropdown.Select
	paths      map[string]*pathField

	reset, done, showFile *button.Button
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
var tridModeLabels = []string{"When the built-in engines aren't sure", "For every file", "Never"}

func newSettingsPage(u *UI) *settingsPage {
	sp := &settingsPage{u: u, numbers: map[string]*numberField{}, checks: map[string]*checkbox.Checkbox{}, paths: map[string]*pathField{}}
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
		c := giode.Input.Checkbox(label, giode.Styles{Color: colText, Background: colAccent, BorderColor: colBorderHi})
		c.SetChecked(get(settings.Get()))
		c.OnChange(func(v bool) { sp.apply(func(s *settings.Settings) { set(s, v) }) })
		sp.checks[key] = c
	}
	check("auto_carve", "Scan unknown files for embedded objects automatically",
		func(s settings.Settings) bool { return s.AutoCarve }, func(s *settings.Settings, v bool) { s.AutoCarve = v })
	check("checksums", "Compute MD5, SHA-1 and SHA-256 when a file is selected",
		func(s settings.Settings) bool { return s.Checksums }, func(s *settings.Settings, v bool) { s.Checksums = v })

	sp.tridMode = giode.Input.Select(field.With(giode.Styles{Width: 290}), tridModeLabels...).
		OnChange(func(i int) { sp.apply(func(s *settings.Settings) { s.TrIDMode = tridModes[i] }) })
	sp.magikaMode = giode.Input.Select(field.With(giode.Styles{Width: 290}), tridModeLabels...).
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
		sp.row("Ask Magika", "Google's deep-learning model: fast, and strong on text, code and files without a header. Also asked about generic plain text.", sp.magikaMode, ""),
		sp.row("Ask TrID", "TrID has 18,000+ definitions but is slower than the built-in engines.", sp.tridMode, ""),
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
	files := card(giode.Styles{},
		sectionTitle("file", "Files"),
		sp.checkRow("checksums", "Hashes are read from the whole file."),
		sp.numberRow("checksum_max_mb", "Checksum size limit", "Skip checksums for larger files. 0 means no limit."),
		sp.pathRow("temp_dir", "Extraction folder", "Where members are extracted while you inspect them. Cleaned up when the tab closes."),
	)
	tools := card(giode.Styles{},
		sectionTitle("cpu", "External tools"),
		giode.Text("Leave empty to find each tool automatically (environment variable, next to AnyFile, PATH, common install folders).", txtw(12.5, colMuted, giode.Styles{LineHeight: 18})),
		sp.pathRow("trid_path", "TrID", "trid.exe, with triddefs.trd in the same folder."),
		sp.pathRow("magika_path", "Magika", "The magika command (pip install magika, or cargo install magika-cli)."),
		sp.pathRow("sevenzip_path", "7-Zip", "7z.exe"),
		sp.pathRow("ffprobe_path", "ffprobe", "Needed with FFmpeg for media streams."),
		sp.pathRow("ffmpeg_path", "FFmpeg", "Copies streams out without re-encoding."),
		sp.pathRow("oodle_path", "Oodle", "oo2core_*_win64.dll, shipped with many games."),
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
	return giode.Stack(giode.Styles{Width: -1, Gap: 3, Padding: giode.SymmetricInset(0, 4)},
		sp.checks[key],
		giode.Box(giode.Styles{Padding: giode.Inset{Left: 28}}, giode.Text(help, txtw(12, colMuted, giode.Styles{LineHeight: 17}))),
	)
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
