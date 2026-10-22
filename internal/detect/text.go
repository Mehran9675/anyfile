package detect

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// textInfo is the outcome of the encoding analysis.
type textInfo struct {
	encoding string
	text     string // decoded sample
}

// analyzeText decides whether h looks like text and decodes it.
func analyzeText(h []byte) (textInfo, bool) {
	switch {
	case bytes.HasPrefix(h, []byte("\xEF\xBB\xBF")):
		return textInfo{"UTF-8 (BOM)", string(h[3:])}, true
	case bytes.HasPrefix(h, []byte("\xFF\xFE\x00\x00")):
		return textInfo{"UTF-32 LE", ""}, true
	case bytes.HasPrefix(h, []byte("\xFF\xFE")):
		return textInfo{"UTF-16 LE", decode16(h[2:], false)}, true
	case bytes.HasPrefix(h, []byte("\xFE\xFF")):
		return textInfo{"UTF-16 BE", decode16(h[2:], true)}, true
	}
	if len(h) == 0 {
		return textInfo{}, false
	}
	// UTF-16 without BOM: every other byte is zero for Latin text.
	if len(h) >= 16 {
		zOdd, zEven := 0, 0
		for i := 0; i+1 < len(h) && i < 4096; i += 2 {
			if h[i] == 0 {
				zEven++
			}
			if h[i+1] == 0 {
				zOdd++
			}
		}
		pairs := min(len(h), 4096) / 2
		if zOdd > pairs*9/10 && zEven < pairs/20 {
			s := decode16(h, false)
			if printableRatio(s) > 0.95 {
				return textInfo{"UTF-16 LE (no BOM)", s}, true
			}
		}
		if zEven > pairs*9/10 && zOdd < pairs/20 {
			s := decode16(h, true)
			if printableRatio(s) > 0.95 {
				return textInfo{"UTF-16 BE (no BOM)", s}, true
			}
		}
	}
	// Trim a possibly cut multibyte sequence at the end of the sample.
	sample := h
	for i := 0; i < 3 && len(sample) > 0 && !utf8.Valid(sample); i++ {
		sample = sample[:len(sample)-1]
	}
	ctrl := 0
	high := 0
	for _, c := range h {
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' && c != '\f' && c != 0x1B {
			ctrl++
		}
		if c >= 0x80 {
			high++
		}
	}
	if ctrl > len(h)/200 {
		return textInfo{}, false
	}
	if utf8.Valid(sample) {
		if high == 0 {
			return textInfo{"ASCII", string(h)}, true
		}
		return textInfo{"UTF-8", string(sample)}, true
	}
	// 8-bit legacy encodings: accept when high bytes are rare and the
	// rest is clean.
	if high < len(h)/5 {
		return textInfo{"8-bit (Windows-1252 / Latin-1)", latin1(h)}, true
	}
	return textInfo{}, false
}

func decode16(b []byte, be bool) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		if be {
			u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			u[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
	}
	return string(utf16.Decode(u))
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func printableRatio(s string) float64 {
	if s == "" {
		return 0
	}
	ok, n := 0, 0
	for _, c := range s {
		n++
		if c >= 0x20 || c == '\n' || c == '\r' || c == '\t' {
			ok++
		}
	}
	return float64(ok) / float64(n)
}

var (
	reEmailHeader = regexp.MustCompile(`(?mi)^(from|to|subject|date|received|message-id|mime-version|return-path|delivered-to|x-[a-z-]+):\s`)
	reSRT         = regexp.MustCompile(`(?m)^\d+\r?\n\d\d:\d\d:\d\d[,.]\d{3} --> \d\d:\d\d:\d\d`)
	reBase64      = regexp.MustCompile(`^[A-Za-z0-9+/\r\n]+=*\s*$`)
	reINISection  = regexp.MustCompile(`(?m)^\s*\[[^\]\r\n]+\]\s*$`)
	reINIKey      = regexp.MustCompile(`(?m)^\s*[\w.\- ]+\s*=`)
	reYAMLKey     = regexp.MustCompile(`(?m)^[\w\-]+:(\s|$)`)
	reMarkdown    = regexp.MustCompile(`(?m)^(#{1,6} |\* |- |\d+\. |> |` + "```" + `)`)
)

// classifyText fills r for a text file. full reports whether s is the
// whole file (so validators such as JSON can run on all of it).
func classifyText(ti textInfo, name string, full bool, r *Result) {
	s := ti.text
	trim := strings.TrimSpace(s)
	low := strings.ToLower(trim[:min(len(trim), 4096)])
	r.detail("Encoding", ti.encoding)
	if lf := strings.Count(s, "\n"); lf > 0 {
		crlf := strings.Count(s, "\r\n")
		switch {
		case crlf == lf:
			r.detail("Line endings", "CRLF (Windows)")
		case crlf == 0:
			r.detail("Line endings", "LF (Unix)")
		default:
			r.detail("Line endings", "Mixed")
		}
	}
	set := func(label, ext, mime string, cat Category, conf float64) {
		r.set(label, ext, mime, cat, conf, SrcHeuristic)
	}

	switch {
	case strings.HasPrefix(low, "<?xml") || strings.HasPrefix(low, "<svg") || strings.HasPrefix(low, "<!doctype") || strings.HasPrefix(low, "<html") || strings.HasPrefix(low, "<"):
		if classifyMarkup(low, set) {
			return
		}
	case strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "["):
		if classifyJSON(trim, low, full, set) {
			return
		}
	}

	switch {
	case strings.HasPrefix(trim, "#!"):
		line := strings.ToLower(strings.SplitN(trim, "\n", 2)[0])
		lang := map[string][2]string{
			"python": {"Python script", "py"}, "node": {"JavaScript (Node.js) script", "js"},
			"perl": {"Perl script", "pl"}, "ruby": {"Ruby script", "rb"}, "php": {"PHP script", "php"},
			"pwsh": {"PowerShell script", "ps1"}, "lua": {"Lua script", "lua"}, "deno": {"TypeScript (Deno) script", "ts"},
		}
		for k, v := range lang {
			if strings.Contains(line, k) {
				set(v[0], v[1], "text/plain", CatCode, 0.9)
				return
			}
		}
		set("Shell script", "sh", "application/x-sh", CatCode, 0.88)
	case strings.HasPrefix(low, "<?php"):
		set("PHP script", "php", "application/x-php", CatCode, 0.9)
	case strings.HasPrefix(trim, "From ") && strings.Contains(s, "\nFrom:"):
		set("Mailbox (mbox)", "mbox", "application/mbox", CatEmail, 0.85)
	case len(reEmailHeader.FindAllStringIndex(s[:min(len(s), 8192)], -1)) >= 3 && reEmailHeader.MatchString(s[:min(len(s), 200)]):
		if strings.Contains(low, "multipart/related") && strings.Contains(low, "content-location") {
			set("MHTML web archive", "mht", "multipart/related", CatDocument, 0.9)
		} else {
			set("Email message (MIME)", "eml", "message/rfc822", CatEmail, 0.88)
		}
	case strings.HasPrefix(trim, "WEBVTT"):
		set("WebVTT subtitles", "vtt", "text/vtt", CatText, 0.95)
	case reSRT.MatchString(s[:min(len(s), 2048)]):
		set("SubRip subtitles", "srt", "application/x-subrip", CatText, 0.92)
	case strings.HasPrefix(trim, "[Script Info]"):
		set("Advanced SubStation subtitles", "ass", "text/x-ssa", CatText, 0.95)
	case strings.HasPrefix(trim, "BEGIN:VCARD"):
		set("vCard contact", "vcf", "text/vcard", CatData, 0.95)
	case strings.HasPrefix(trim, "BEGIN:VCALENDAR"):
		set("iCalendar", "ics", "text/calendar", CatData, 0.95)
	case strings.HasPrefix(trim, "Windows Registry Editor") || strings.HasPrefix(trim, "REGEDIT4"):
		set("Registry script", "reg", "text/plain", CatData, 0.95)
	case strings.HasPrefix(low, "@echo off"):
		set("Windows batch script", "bat", "application/x-bat", CatCode, 0.85)
	case strings.Contains(low, "\\documentclass") || strings.Contains(low, "\\begin{document}"):
		set("LaTeX document", "tex", "application/x-tex", CatDocument, 0.9)
	case isBase64Blob(trim):
		set("Base64-encoded data", "b64", "text/plain", CatData, 0.75)
		if raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(trim[:min(len(trim), 4096)/4*4]), "")); err == nil {
			if inner := matchSig(raw); inner != nil {
				r.Label = "Base64-encoded " + inner.label
				r.note("The text decodes to " + inner.label + "; explore the file to extract it.")
			}
		}
	default:
		if classifyTabular(s, set) || classifyCode(trim, name, set) {
			return
		}
		switch {
		case reINISection.MatchString(s) && len(reINIKey.FindAllString(s, 5)) >= 2:
			set("INI configuration", "ini", "text/plain", CatText, 0.75)
		case strings.HasPrefix(trim, "---") || len(reYAMLKey.FindAllString(s, 6)) >= 5 && !strings.Contains(s, "{"):
			set("YAML document", "yaml", "application/yaml", CatText, 0.6)
		case len(reMarkdown.FindAllString(s, 8)) >= 4:
			set("Markdown document", "md", "text/markdown", CatText, 0.6)
		default:
			set("Plain text", "txt", "text/plain", CatText, 0.85)
		}
	}
}

func isBase64Blob(s string) bool {
	if len(s) < 64 {
		return false
	}
	sample := s[:min(len(s), 8192)]
	if !reBase64.MatchString(sample) && !reBase64.MatchString(sample+"=") {
		// The sample may end mid-line; check just the characters.
		for _, c := range sample {
			if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/' || c == '=' || c == '\r' || c == '\n') {
				return false
			}
		}
	}
	// Prose never lacks spaces; base64 always does.
	return !strings.Contains(sample, " ")
}

func classifyMarkup(low string, set func(string, string, string, Category, float64)) bool {
	body := low
	if strings.HasPrefix(body, "<?xml") {
		if i := strings.Index(body, "?>"); i > 0 {
			body = strings.TrimSpace(body[i+2:])
		}
	}
	for strings.HasPrefix(body, "<!--") {
		i := strings.Index(body, "-->")
		if i < 0 {
			break
		}
		body = strings.TrimSpace(body[i+3:])
	}
	switch {
	case strings.Contains(low, "<svg"):
		set("SVG vector image", "svg", "image/svg+xml", CatImage, 0.92)
	case strings.HasPrefix(body, "<!doctype html") || strings.HasPrefix(body, "<html") || strings.Contains(low[:min(len(low), 1024)], "<head") || strings.Contains(low[:min(len(low), 1024)], "<body"):
		set("HTML document", "html", "text/html", CatDocument, 0.9)
	case strings.Contains(low, "progid=\"excel.sheet\"") || strings.Contains(low, "urn:schemas-microsoft-com:office:spreadsheet"):
		set("Excel 2003 XML spreadsheet", "xml", "application/vnd.ms-excel", CatSpreadsheet, 0.9)
	case strings.Contains(low, "progid=\"word.document\"") || strings.HasPrefix(body, "<w:worddocument") || strings.HasPrefix(body, "<pkg:package"):
		set("Word XML document", "xml", "application/msword", CatDocument, 0.9)
	case strings.HasPrefix(body, "<rss"):
		set("RSS feed", "rss", "application/rss+xml", CatText, 0.92)
	case strings.HasPrefix(body, "<feed"):
		set("Atom feed", "atom", "application/atom+xml", CatText, 0.9)
	case strings.HasPrefix(body, "<plist") || strings.HasPrefix(body, "<!doctype plist"):
		set("Property list (XML)", "plist", "application/x-plist", CatData, 0.92)
	case strings.HasPrefix(body, "<gpx"):
		set("GPS track (GPX)", "gpx", "application/gpx+xml", CatData, 0.92)
	case strings.HasPrefix(body, "<kml"):
		set("KML geodata", "kml", "application/vnd.google-earth.kml+xml", CatData, 0.92)
	case strings.HasPrefix(body, "<project") && strings.Contains(low, "maven"):
		set("Maven project", "xml", "application/xml", CatCode, 0.9)
	case strings.HasPrefix(body, "<project") || strings.Contains(low, "<itemgroup"):
		set("MSBuild project", "xml", "application/xml", CatCode, 0.8)
	case strings.HasPrefix(body, "<assembly"):
		set("Windows manifest", "manifest", "application/xml", CatData, 0.85)
	case strings.HasPrefix(body, "<x:xmpmeta") || strings.HasPrefix(body, "<?xpacket"):
		set("XMP metadata", "xmp", "application/rdf+xml", CatData, 0.9)
	case strings.HasPrefix(low, "<?xml"):
		set("XML document", "xml", "application/xml", CatText, 0.88)
	default:
		return false
	}
	return true
}

func classifyJSON(trim, low string, full bool, set func(string, string, string, Category, float64)) bool {
	valid := false
	if full {
		valid = json.Valid([]byte(trim))
		if !valid {
			// JSON Lines: every line is a document.
			lines := strings.Split(trim, "\n")
			ok := len(lines) > 1
			for _, l := range lines {
				if l = strings.TrimSpace(l); l != "" && !json.Valid([]byte(l)) {
					ok = false
					break
				}
			}
			if ok {
				set("JSON Lines", "jsonl", "application/jsonl", CatData, 0.9)
				return true
			}
			return false
		}
	}
	conf := 0.75
	if valid {
		conf = 0.95
	}
	switch {
	case strings.Contains(low, `"nbformat"`):
		set("Jupyter notebook", "ipynb", "application/x-ipynb+json", CatCode, conf)
	case strings.Contains(low, `"featurecollection"`) || strings.Contains(low, `"type": "feature"`):
		set("GeoJSON", "geojson", "application/geo+json", CatData, conf)
	case strings.Contains(low, `"asset"`) && strings.Contains(low, `"gltf`) || strings.Contains(low, `"asset":{"version"`):
		set("glTF 3D model", "gltf", "model/gltf+json", CatData, conf)
	case strings.Contains(low, `"log"`) && strings.Contains(low, `"entries"`) && strings.Contains(low, `"creator"`):
		set("HTTP archive (HAR)", "har", "application/json", CatData, conf)
	case strings.Contains(low, `"$schema"`):
		set("JSON document (with schema)", "json", "application/json", CatData, conf)
	default:
		set("JSON document", "json", "application/json", CatData, conf)
	}
	return true
}

// classifyTabular spots CSV/TSV by a consistent delimiter count.
func classifyTabular(s string, set func(string, string, string, Category, float64)) bool {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(lines) > 50 {
		lines = lines[:50]
	}
	var use []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			use = append(use, l)
		}
	}
	if len(use) > 2 {
		use = use[:len(use)-1] // the sample may cut the last line
	}
	if len(use) < 2 {
		return false
	}
	for _, d := range []struct {
		sep        string
		label, ext string
		mime       string
	}{{"\t", "Tab-separated values", "tsv", "text/tab-separated-values"}, {",", "Comma-separated values", "csv", "text/csv"}, {";", "Semicolon-separated values", "csv", "text/csv"}, {"|", "Pipe-separated values", "csv", "text/csv"}} {
		n := strings.Count(use[0], d.sep)
		if n == 0 {
			continue
		}
		ok := true
		for _, l := range use[1:] {
			if strings.Count(l, d.sep) != n {
				ok = false
				break
			}
		}
		if ok {
			set(d.label, d.ext, d.mime, CatSpreadsheet, 0.8)
			return true
		}
	}
	return false
}

var codeExts = map[string]string{
	"go": "Go source", "c": "C source", "h": "C/C++ header", "cpp": "C++ source", "cc": "C++ source", "hpp": "C++ header",
	"cs": "C# source", "java": "Java source", "kt": "Kotlin source", "py": "Python source", "js": "JavaScript source",
	"mjs": "JavaScript module", "ts": "TypeScript source", "tsx": "TypeScript JSX", "jsx": "JavaScript JSX", "rs": "Rust source",
	"rb": "Ruby source", "php": "PHP source", "swift": "Swift source", "m": "Objective-C source", "lua": "Lua source",
	"sh": "Shell script", "bash": "Bash script", "ps1": "PowerShell script", "psm1": "PowerShell module", "bat": "Batch script",
	"cmd": "Batch script", "sql": "SQL script", "css": "CSS stylesheet", "scss": "Sass stylesheet", "less": "Less stylesheet",
	"vue": "Vue component", "dart": "Dart source", "scala": "Scala source", "r": "R script", "pl": "Perl script",
	"toml": "TOML configuration", "yaml": "YAML document", "yml": "YAML document", "ini": "INI configuration", "cfg": "Configuration",
	"conf": "Configuration", "md": "Markdown document", "rst": "reStructuredText", "log": "Log file", "txt": "Plain text",
	"gradle": "Gradle script", "cmake": "CMake script", "mk": "Makefile", "asm": "Assembly source", "s": "Assembly source",
	"vb": "Visual Basic source", "vbs": "VBScript", "fs": "F# source", "hs": "Haskell source", "ex": "Elixir source",
	"erl": "Erlang source", "clj": "Clojure source", "zig": "Zig source", "nim": "Nim source", "jl": "Julia source",
	"proto": "Protocol Buffers schema", "graphql": "GraphQL schema", "tf": "Terraform configuration", "dockerfile": "Dockerfile",
	"nfo": "NFO text", "html": "HTML document", "htm": "HTML document", "xhtml": "XHTML document", "diz": "Description text", "properties": "Java properties", "env": "Environment file",
}

var codeHints = []struct {
	re    *regexp.Regexp
	label string
	ext   string
}{
	{regexp.MustCompile(`(?m)^package \w+\s*$[\s\S]*\bfunc\b`), "Go source", "go"},
	{regexp.MustCompile(`(?m)^#include\s*[<"]`), "C/C++ source", "c"},
	{regexp.MustCompile(`(?m)^using System[\w.]*;`), "C# source", "cs"},
	{regexp.MustCompile(`(?m)^(import java\.|public (final )?class \w+)`), "Java source", "java"},
	{regexp.MustCompile(`(?m)^(fn main\(\)|use std::|pub fn |impl\b)`), "Rust source", "rs"},
	{regexp.MustCompile(`(?m)^(def \w+\(.*\):|from [\w.]+ import |import \w+$|class \w+(\(.*\))?:)`), "Python source", "py"},
	{regexp.MustCompile(`(?m)^(import .* from ['"]|export (default |const |function )|const \w+ = require\()`), "JavaScript / TypeScript source", "js"},
	{regexp.MustCompile(`(?m)^\s*(function \w+\(|var \w+ = |document\.|window\.)`), "JavaScript source", "js"},
	{regexp.MustCompile(`(?mi)^(select|insert into|create table|update \w+ set|alter table)\b`), "SQL script", "sql"},
	{regexp.MustCompile(`(?m)^[.#]?[\w-]+\s*\{\s*$[\s\S]*:\s*[^;]+;`), "CSS stylesheet", "css"},
	{regexp.MustCompile(`(?m)^(param\(|\$\w+ = |Write-Host |Get-\w+)`), "PowerShell script", "ps1"},
	{regexp.MustCompile(`(?m)^\w+:.*\n\t`), "Makefile", "mk"},
	{regexp.MustCompile(`(?m)^FROM \S+(\s+AS \w+)?\s*$`), "Dockerfile", "dockerfile"},
}

func classifyCode(trim, name string, set func(string, string, string, Category, float64)) bool {
	ext := extOf(name)
	if l, ok := codeExts[ext]; ok {
		cat, conf := CatCode, 0.65
		switch ext {
		case "txt", "log", "md", "nfo", "rst":
			cat, conf = CatText, 0.85
		case "html", "htm", "xhtml":
			cat, conf = CatDocument, 0.8
		}
		set(l, ext, "text/plain", cat, conf)
		return true
	}
	sample := trim[:min(len(trim), 16384)]
	for _, h := range codeHints {
		if h.re.MatchString(sample) {
			set(h.label, h.ext, "text/plain", CatCode, 0.65)
			return true
		}
	}
	return false
}
