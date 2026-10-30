package ui

import (
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/i18n"
	"github.com/xmhrnx/anyfile/internal/locales"
	"github.com/xmhrnx/anyfile/internal/settings"
)

// tr translates English text (the key in internal/locales) and fills
// its {placeholders} from name, value pairs or a map; a "count" value
// picks the plural form.
func tr(key string, data ...any) string { return i18n.Intl(key, data...) }

// relabeler re-applies translated text to widgets that keep the text
// they were given (button labels, placeholders, options) when the
// language changes.
type relabeler struct {
	version uint64
	fns     []func()
}

// add runs fn now and again after every language change.
func (r *relabeler) add(fn func()) {
	fn()
	r.fns = append(r.fns, fn)
}

// update runs the functions if the language changed; call it every
// frame.
func (r *relabeler) update() {
	if v := i18n.Version(); v != r.version {
		r.version = v
		for _, fn := range r.fns {
			fn()
		}
	}
}

// appText relabels the widgets that live as long as the window.
var appText relabeler

// relabelOptions translates a dropdown's or segmented control's
// options in place: giode has no way to replace them, but Options
// returns the slice it draws from.
func relabelOptions(opts []string, keys []string) {
	for i := range min(len(opts), len(keys)) {
		opts[i] = tr(keys[i])
	}
}

// translated copies keys for a widget's options, so relabelOptions
// never writes into a shared slice.
func translated(keys []string) []string {
	out := make([]string, len(keys))
	relabelOptions(out, keys)
	return out
}

// intlOptions loads the translations built into the executable.
func intlOptions() *giode.IntlOptions {
	return &giode.IntlOptions{FS: locales.FS, Dir: ".", Fallback: "en"}
}

// locale is the language the settings choose, or the system's.
func locale(s settings.Settings) string {
	if s.Language != "" {
		return s.Language
	}
	return i18n.SystemLocale()
}
