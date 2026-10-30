// Package locales holds AnyFile's translations: one <locale>.json per
// language, in giode's message format. The keys are the English text,
// so anything a language lacks shows in English; en.json only holds
// English plural forms.
package locales

import "embed"

// FS holds the message files.
//
//go:embed *.json
var FS embed.FS

// Language is a translation the settings page offers.
type Language struct {
	Locale string
	Name   string // in the language itself
}

// Languages are the interface languages, sorted by their own names.
var Languages = []Language{
	{"cs", "Čeština"},
	{"da", "Dansk"},
	{"de", "Deutsch"},
	{"et", "Eesti"},
	{"en", "English"},
	{"es", "Español"},
	{"fr", "Français"},
	{"hr", "Hrvatski"},
	{"it", "Italiano"},
	{"lv", "Latviešu"},
	{"lt", "Lietuvių"},
	{"hu", "Magyar"},
	{"nl", "Nederlands"},
	{"nb", "Norsk bokmål"},
	{"pl", "Polski"},
	{"pt", "Português"},
	{"ro", "Română"},
	{"sk", "Slovenčina"},
	{"sl", "Slovenščina"},
	{"fi", "Suomi"},
	{"sv", "Svenska"},
	{"el", "Ελληνικά"},
	{"bg", "Български"},
	{"ru", "Русский"},
	{"uk", "Українська"},
}
