package ui

import (
	"os"
	"path/filepath"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"

	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/styles/properties"
)

// Palette: a deep slate canvas with an indigo→cyan accent.
const (
	colBg        = "#0a0e16"
	colSidebar   = "#0e131d"
	colSurface   = "#121926"
	colSurface2  = "#172031"
	colRaised    = "#1c2639"
	colBorder    = "#222d42"
	colBorderHi  = "#2f3d58"
	colText      = "#e7edf7"
	colTextSoft  = "#b9c3d4"
	colMuted     = "#7d8aa3"
	colFaint     = "#56627a"
	colAccent    = "#7c83ff"
	colAccent2   = "#22d3ee"
	colAccentDim = "#7c83ff26"
	colSuccess   = "#34d399"
	colWarning   = "#fbbf24"
	colDanger    = "#f87171"
)

const (
	fontUI   = "Segoe UI"
	fontMono = "Go Mono"
)

// catColor gives every category its own hue.
func catColor(c detect.Category) string {
	switch c {
	case detect.CatArchive, detect.CatCompressed:
		return "#f5b041"
	case detect.CatDocument:
		return "#60a5fa"
	case detect.CatSpreadsheet:
		return "#34d399"
	case detect.CatPresentation:
		return "#fb923c"
	case detect.CatImage:
		return "#f472b6"
	case detect.CatAudio:
		return "#a78bfa"
	case detect.CatVideo:
		return "#f87171"
	case detect.CatExecutable:
		return "#22d3ee"
	case detect.CatFont:
		return "#2dd4bf"
	case detect.CatDatabase:
		return "#facc15"
	case detect.CatText:
		return "#cbd5e1"
	case detect.CatCode:
		return "#38bdf8"
	case detect.CatDiskImage:
		return "#c084fc"
	case detect.CatEmail:
		return "#4ade80"
	case detect.CatCrypto, detect.CatEncrypted:
		return "#fda4af"
	case "Folder":
		return "#94a3b8"
	}
	return "#8b98ad"
}

// loadFonts prefers the platform UI font for a native feel and keeps
// the Go fonts for monospace text and as a fallback.
func loadFonts() {
	faces := []font.FontFace{}
	dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
	for _, f := range []struct {
		file   string
		weight font.Weight
		style  font.Style
	}{
		{"segoeui.ttf", font.Normal, font.Regular},
		{"seguisb.ttf", font.SemiBold, font.Regular},
		{"segoeuib.ttf", font.Bold, font.Regular},
		{"segoeuil.ttf", font.Light, font.Regular},
		{"segoeuii.ttf", font.Normal, font.Italic},
	} {
		b, err := os.ReadFile(filepath.Join(dir, f.file))
		if err != nil {
			continue
		}
		parsed, err := opentype.ParseCollection(b)
		if err != nil {
			continue
		}
		for _, p := range parsed {
			p.Font.Typeface = fontUI
			p.Font.Weight = f.weight
			p.Font.Style = f.style
			faces = append(faces, p)
		}
	}
	faces = append(faces, gofont.Collection()...)
	giode.RegisterFonts(faces)
}

// Text style helpers.
func txt(size float32, color string) giode.Styles {
	return giode.Styles{FontSize: properties.FontSize(size), Color: properties.Color(color), FontFamily: fontUI}
}

func txtw(size float32, color string, w giode.Styles) giode.Styles {
	s := txt(size, color)
	return s.With(w)
}

// applyTheme switches giode to dark mode with AnyFile's palette, so
// components styled with theme tokens (the tree, toasts, copy buttons)
// match the hand-styled ones.
func applyTheme() {
	giode.SetPalette(giode.ModeDark, giode.Palette{
		Primary:    colAccent,
		Secondary:  colTextSoft,
		Background: colBg,
		Surface:    colRaised,
		Text:       colText,
		Border:     colBorderHi,
		OnPrimary:  "#ffffff",
		Muted:      colMuted,
		Accent:     colAccent2,
		Success:    colSuccess,
		Warning:    colWarning,
		Danger:     colDanger,
		Info:       colAccent2,
	})
	giode.SetMode(giode.ModeDark)
}

// color converts a plain string for a style field.
func color(s string) properties.Color { return properties.Color(s) }
