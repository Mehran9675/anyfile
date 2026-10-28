package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/mehran9675/anyfile/internal/install"
	"github.com/mehran9675/anyfile/internal/tools"
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/button"
)

// engineInstall is the in-app installer of one engine: its buttons and,
// while it runs, its progress. It belongs to the UI goroutine.
type engineInstall struct {
	e *install.Engine

	install, remove, cancelBtn *button.Button
	// inspectorBtn offers the install where the inspector says the engine
	// is missing.
	inspectorBtn *button.Button

	busy   bool
	cancel context.CancelFunc
	prog   install.Progress
	err    string
}

func newEngineInstalls(u *UI) map[string]*engineInstall {
	out := map[string]*engineInstall{}
	for _, e := range install.Engines {
		ei := &engineInstall{e: e}
		ei.install = newBtn("download", "Install", btnSecondary, func() { u.startInstall(ei) })
		ei.inspectorBtn = newBtn("download", "Install "+e.Name, btnSecondary, func() { u.startInstall(ei) })
		ei.remove = newBtn("x", "Remove", btnGhost, func() { u.removeEngine(ei) })
		ei.cancelBtn = newBtn("x", "Cancel", btnGhost, func() {
			if ei.cancel != nil {
				ei.cancel()
			}
		})
		out[e.ID] = ei
	}
	return out
}

// engineTools are the tools an engine's install provides.
func engineTools(id string) []tools.Tool {
	switch id {
	case "trid":
		return []tools.Tool{tools.TrID()}
	case "magika":
		return []tools.Tool{tools.Magika()}
	case "7zip":
		return []tools.Tool{tools.SevenZip()}
	case "ffmpeg":
		return []tools.Tool{tools.FFmpeg(), tools.FFprobe()}
	}
	return nil
}

// found reports whether every tool the engine provides is available,
// from AnyFile's install or elsewhere.
func (ei *engineInstall) found() bool {
	for _, t := range engineTools(ei.e.ID) {
		if !t.Found() {
			return false
		}
	}
	return true
}

// canInstall reports whether an Install button makes sense here.
func (ei *engineInstall) canInstall() bool {
	return !ei.busy && ei.e.Unsupported() == ""
}

// startInstall downloads and installs an engine in the background,
// reporting progress on the engine's row.
func (u *UI) startInstall(ei *engineInstall) {
	if !ei.canInstall() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ei.busy, ei.cancel, ei.err = true, cancel, ""
	ei.prog = install.Progress{Step: "Starting"}
	go func() {
		err := ei.e.Install(ctx, func(p install.Progress) {
			u.app.Post(func() { ei.prog = p })
		})
		cancel()
		tools.Reload()
		u.app.Post(func() {
			ei.busy, ei.cancel = false, nil
			switch {
			case errors.Is(err, context.Canceled):
				u.toast("ok", ei.e.Name+" install canceled")
			case err != nil:
				ei.err = err.Error()
				u.toast("error", "Could not install "+ei.e.Name+": "+err.Error())
			case !ei.found():
				// A path in Settings or an environment variable points
				// elsewhere and takes precedence.
				u.toast("ok", ei.e.Name+" installed, but the path set in Settings is used instead")
			default:
				u.toast("ok", ei.e.Name+" installed")
			}
		})
	}()
}

// removeEngine deletes AnyFile's install of an engine.
func (u *UI) removeEngine(ei *engineInstall) {
	if ei.busy {
		return
	}
	ei.err = ""
	go func() {
		err := ei.e.Remove()
		tools.Reload()
		u.app.Post(func() {
			if err != nil {
				ei.err = err.Error()
				u.toast("error", "Could not remove "+ei.e.Name+": "+err.Error())
				return
			}
			u.toast("ok", ei.e.Name+" removed")
		})
	}()
}

// installMissing installs every engine that isn't found and has a build
// for this computer.
func (u *UI) installMissing() {
	for _, e := range install.Engines {
		if ei := u.engines[e.ID]; !ei.found() && ei.canInstall() {
			u.startInstall(ei)
		}
	}
}

// anyMissing reports whether an engine could be installed but isn't.
func (u *UI) anyMissing() bool {
	for _, e := range install.Engines {
		if ei := u.engines[e.ID]; !ei.found() && ei.canInstall() {
			return true
		}
	}
	return false
}

// status describes where the engine stands: a chip and a detail line.
func (ei *engineInstall) status() (giode.Element, string) {
	ts := engineTools(ei.e.ID)
	m, managed := ei.e.Installed()
	switch {
	case ei.busy:
		return chip("Installing", colAccent, "download"), ""
	case ei.found() && managed && ts[0].Managed():
		return chip("Installed by AnyFile", colSuccess, "check"), m.Version
	case ei.found():
		return chip("Found", colSuccess, "check"), ts[0].Path
	case ei.e.Unsupported() != "":
		return chip("Not available", colFaint, "x"), ei.e.Unsupported()
	default:
		return chip("Not installed", colWarning, "alert"), "Downloads " + ei.e.Size + " from " + ei.e.Source + "."
	}
}

// view is the engine's status, progress and buttons, for the settings
// page.
func (ei *engineInstall) view() giode.Element {
	c, detail := ei.status()
	_, managed := ei.e.Installed()
	var btns []giode.Element
	switch {
	case ei.busy:
		btns = append(btns, ei.cancelBtn)
	case ei.e.Unsupported() != "":
	case managed:
		ei.install.Label("Update")
		btns = append(btns, ei.remove, ei.install)
	default:
		ei.install.Label("Install")
		btns = append(btns, ei.install)
	}
	line := giode.Row(giode.Styles{Width: -1, Gap: 10, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
		c,
		giode.Box(giode.Styles{FlexGrow: 1, Width: -1},
			giode.Text(detail, txtw(12, colMuted, giode.Styles{MaxLines: 1, TextOverflow: giode.TextOverflowEllipsis}))),
		giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap}, btns...),
	)
	kids := []giode.Element{line}
	if ei.busy {
		kids = append(kids, ei.progressView())
	}
	if ei.err != "" {
		kids = append(kids, giode.Row(giode.Styles{Gap: 6, Align: giode.AlignCenter},
			icon("alert", colDanger, 13), giode.Text(ei.err, txtw(12, colDanger, giode.Styles{LineHeight: 17}))))
	}
	return giode.Stack(giode.Styles{Width: -1, Gap: 8}, kids...)
}

// progressView is a spinner, the current step and, when the size is
// known, a meter.
func (ei *engineInstall) progressView() giode.Element {
	p := ei.prog
	step := p.Step
	kids := []giode.Element{giode.Spinner(giode.Styles{Width: 13, Height: 13, Color: colAccent})}
	if p.Total > 0 {
		step += fmt.Sprintf("  %.1f of %.1f MB", float64(p.Done)/(1<<20), float64(p.Total)/(1<<20))
		kids = append(kids, bar(float32(p.Done)/float32(p.Total), colAccent, 160))
	}
	kids = append(kids, giode.Text(step, txtw(12, colTextSoft, giode.Styles{MaxLines: 1, TextOverflow: giode.TextOverflowEllipsis})))
	return giode.Row(giode.Styles{Width: -1, Gap: 10, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap}, kids...)
}

// inspectorOffer is the install button, or the progress, the inspector
// shows on a card for a missing engine; nil when there's nothing to
// offer.
func (u *UI) inspectorOffer(id string) giode.Element {
	ei := u.engines[id]
	switch {
	case ei == nil, ei.e.Unsupported() != "":
		return nil
	case ei.busy:
		return ei.progressView()
	default:
		return giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter}, ei.inspectorBtn)
	}
}

// installHint ends a "not installed" note, depending on whether AnyFile
// offers to install the engine here.
func installHint(offer giode.Element) string {
	if offer != nil {
		return "AnyFile can install it, or you can set its path in Settings."
	}
	return "Set its path in Settings if you have a copy."
}
