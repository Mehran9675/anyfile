package ui

import (
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/styles/properties"
)

type btnKind int

const (
	btnPrimary btnKind = iota
	btnSecondary
	btnGhost
)

// newBtn builds an action button with an icon in one of AnyFile's three
// looks. The icon follows the label color.
func newBtn(iconName, label string, kind btnKind, onClick func()) *button.Button {
	st := giode.Styles{
		Padding:      giode.SymmetricInset(14, 9),
		BorderRadius: 9,
		FontSize:     13.5,
	}
	b := giode.Button(label)
	switch kind {
	case btnPrimary:
		st.BackgroundGradient = giode.Linear(120, "#6d74ff", "#4f9dff")
		st.BoxShadow = giode.BoxShadow{Y: 4, Blur: 14, Color: "#5b63ff55"}
		st.Color = "#ffffff"
	case btnSecondary:
		st.Background, st.Color = colRaised, colText
		st.BorderWidth, st.BorderColor = 1, colBorderHi
	default:
		st.Background, st.Color = colTextSoft, colTextSoft
		b.Variant(giode.VariantGhost)
	}
	return b.Styles(st).Icon(icon(iconName, "", 16)).OnClick(onClick)
}

// chip is a small rounded label tinted with color.
func chip(label, color string, iconName string) giode.Element {
	c := giode.Chip(label, giode.Styles{
		Background:  properties.Color(color + "1f"),
		BorderColor: properties.Color(color + "40"),
		Color:       properties.Color(color),
		FontSize:    11.5,
		Padding:     giode.SymmetricInset(9, 3),
		Gap:         5,
	})
	if iconName != "" {
		c.Icon(icon(iconName, color, 12))
	}
	return c
}

// card is a rounded panel.
func card(st giode.Styles, kids ...giode.Element) giode.Element {
	base := giode.Styles{
		FlexDirection: giode.FlexDirectionColumn,
		Width:         -1,
		Gap:           12,
		Padding:       giode.UniformInset(18),
		Background:    colSurface,
		BorderRadius:  14,
		BorderWidth:   1,
		BorderColor:   colBorder,
	}
	return giode.Box(base.With(st), kids...)
}

func sectionTitle(iconName, s string) giode.Element {
	return giode.Row(giode.Styles{Align: giode.AlignCenter, Gap: 8, Wrap: giode.WrapNoWrap},
		icon(iconName, colMuted, 15),
		giode.Text(s, txtw(12, colMuted, giode.Styles{FontWeight: giode.FontWeightSemiBold, TextTransform: giode.TextTransformUppercase})),
	)
}

// kv is one "label  value" line.
func kv(k, v string, mono bool) giode.Element {
	vs := txt(13, colText)
	if mono {
		vs.FontFamily = fontMono
		vs.FontSize = 12
	}
	return giode.Row(giode.Styles{Width: -1, Gap: 12, Align: giode.AlignStart, Wrap: giode.WrapNoWrap},
		giode.Box(giode.Styles{Width: 132}, giode.Text(k, txt(13, colMuted))),
		giode.Box(giode.Styles{FlexGrow: 1, Width: -1}, giode.Text(v, vs)),
	)
}

// bar is a slim horizontal meter.
func bar(frac float32, color string, width int) giode.Element {
	frac = max(0, min(1, frac))
	return giode.Box(giode.Styles{Width: properties.Width(width), Height: 6, Background: "#ffffff12", BorderRadius: 3, Overflow: giode.OverflowHidden},
		giode.Box(giode.Styles{Width: properties.Width(max(1, int(float32(width)*frac))), Height: 6, Background: properties.Color(color), BorderRadius: 3}),
	)
}
