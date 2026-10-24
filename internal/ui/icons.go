package ui

import (
	"github.com/mehran9675/anyfile/internal/detect"
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/styles/properties"
)

// Outline icons on a 24×24 grid, in the style of Lucide (ISC license).
var icons = map[string]string{
	"file":     "M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z M14 2v4a2 2 0 0 0 2 2h4",
	"filetext": "M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z M14 2v4a2 2 0 0 0 2 2h4 M10 9H8 M16 13H8 M16 17H8",
	"folder":   "M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z",
	"open":     "M6 14l1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.54 6a2 2 0 0 1-1.95 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H18a2 2 0 0 1 2 2v2",
	"archive":  "M3 3h18v5H3z M5 8v11a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8 M10 12h4",
	"image":    "M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z M21 15l-5-5L5 21 M9 9m-2 0a2 2 0 1 0 4 0a2 2 0 1 0-4 0",
	"video":    "M2 6a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2z M7 4v16 M17 4v16 M2 9h5 M2 15h5 M17 9h5 M17 15h5",
	"audio":    "M9 18V5l12-2v13 M9 18a3 3 0 1 1-6 0a3 3 0 1 1 6 0 M21 16a3 3 0 1 1-6 0a3 3 0 1 1 6 0",
	"sheet":    "M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z M3 9h18 M3 15h18 M9 3v18",
	"slides":   "M2 3h20 M21 3v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V3 M7 21l5-5 5 5",
	"code":     "M16 18l6-6-6-6 M8 6l-6 6 6 6",
	"cpu":      "M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z M9 9h6v6H9z M15 2v2 M15 20v2 M2 15h2 M2 9h2 M20 15h2 M20 9h2 M9 2v2 M9 20v2",
	"database": "M12 2c4.97 0 9 1.34 9 3s-4.03 3-9 3-9-1.34-9-3 4.03-3 9-3z M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5 M3 12c0 1.66 4 3 9 3s9-1.34 9-3",
	"lock":     "M5 11h14a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2z M7 11V7a5 5 0 0 1 10 0v4",
	"font":     "M4 7V4h16v3 M9 20h6 M12 4v16",
	"disc":     "M12 12m-10 0a10 10 0 1 0 20 0a10 10 0 1 0-20 0 M12 12m-2 0a2 2 0 1 0 4 0a2 2 0 1 0-4 0",
	"mail":     "M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z M22 6l-10 7L2 6",
	"key":      "M7.5 15.5m-5.5 0a5.5 5.5 0 1 0 11 0a5.5 5.5 0 1 0-11 0 M21 2l-9.6 9.6 M15.5 7.5l3 3L22 7l-3-3",
	"help":     "M12 12m-10 0a10 10 0 1 0 20 0a10 10 0 1 0-20 0 M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3 M12 17h.01",
	"blank":    "M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z M14 2v4a2 2 0 0 0 2 2h4",
	"layers":   "M12 2L2 7l10 5 10-5-10-5z M2 17l10 5 10-5 M2 12l10 5 10-5",
	"chevR":    "M9 18l6-6-6-6",
	"chevD":    "M6 9l6 6 6-6",
	"download": "M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4 M7 10l5 5 5-5 M12 15V3",
	"extract":  "M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4 M17 8l-5-5-5 5 M12 3v12",
	"external": "M15 3h6v6 M10 14L21 3 M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6",
	"grid":     "M3 3h7v7H3z M14 3h7v7h-7z M14 14h7v7h-7z M3 14h7v7H3z",
	"scan":     "M3 7V5a2 2 0 0 1 2-2h2 M17 3h2a2 2 0 0 1 2 2v2 M21 17v2a2 2 0 0 1-2 2h-2 M7 21H5a2 2 0 0 1-2-2v-2 M12 12m-3 0a3 3 0 1 0 6 0a3 3 0 1 0-6 0 M16 16l-1.9-1.9",
	"sparkles": "M9.94 15.5A2 2 0 0 0 8.5 14.06l-6.13-1.58a.5.5 0 0 1 0-.96L8.5 9.94A2 2 0 0 0 9.94 8.5l1.58-6.13a.5.5 0 0 1 .96 0L14.06 8.5A2 2 0 0 0 15.5 9.94l6.13 1.58a.5.5 0 0 1 0 .96L15.5 14.06a2 2 0 0 0-1.44 1.44l-1.58 6.13a.5.5 0 0 1-.96 0z",
	"reveal":   "M2 5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H20a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2Z M12 10v6 M9 13l3 3 3-3",
	"copy":     "M10 8h10a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H10a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2z M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2",
	"check":    "M20 6L9 17l-5-5",
	"alert":    "M12 12m-10 0a10 10 0 1 0 20 0a10 10 0 1 0-20 0 M12 8v4 M12 16h.01",
	"info":     "M12 12m-10 0a10 10 0 1 0 20 0a10 10 0 1 0-20 0 M12 16v-4 M12 8h.01",
	"x":        "M18 6L6 18 M6 6l12 12",
	"settings": "M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z M12 12m-3 0a3 3 0 1 0 6 0a3 3 0 1 0-6 0",
	"reset":    "M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8 M3 3v5h5",
	"brain":    "M12 5a3 3 0 1 0-5.997.125 4 4 0 0 0-2.526 5.77 4 4 0 0 0 .556 6.588A4 4 0 1 0 12 18Z M12 5a3 3 0 1 1 5.997.125 4 4 0 0 1 2.526 5.77 4 4 0 0 1-.556 6.588A4 4 0 1 1 12 18Z M15 13a4.5 4.5 0 0 1-3-4 4.5 4.5 0 0 1-3 4 M12 5v13",
	"sortAsc":  "M3 16l4 4 4-4 M7 20V4 M11 4h4 M11 8h7 M11 12h10",
	"sortDesc": "M3 16l4 4 4-4 M7 20V4 M11 4h10 M11 8h7 M11 12h4",
	"hash":     "M4 9h16 M4 15h16 M10 3L8 21 M16 3l-2 18",
	"tag":      "M12.59 2.59A2 2 0 0 0 11.17 2H4a2 2 0 0 0-2 2v7.17a2 2 0 0 0 .59 1.41l8.7 8.7a2.43 2.43 0 0 0 3.42 0l6.58-6.58a2.43 2.43 0 0 0 0-3.42z M7.5 7.5m-.5 0a.5.5 0 1 0 1 0a.5.5 0 1 0-1 0",
	"eye":      "M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z M12 12m-3 0a3 3 0 1 0 6 0a3 3 0 1 0-6 0",
	"upload":   "M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4 M17 8l-5-5-5 5 M12 3v12",
}

// icon renders a named outline icon.
func icon(name, color string, size int) giode.Element {
	return giode.IconSVGStroke(icons[name], 2, giode.Styles{Color: properties.Color(color), Width: properties.Width(size), Height: properties.Height(size)})
}

func catIcon(c detect.Category) string {
	switch c {
	case detect.CatArchive, detect.CatCompressed:
		return "archive"
	case detect.CatDocument:
		return "filetext"
	case detect.CatSpreadsheet:
		return "sheet"
	case detect.CatPresentation:
		return "slides"
	case detect.CatImage:
		return "image"
	case detect.CatAudio:
		return "audio"
	case detect.CatVideo:
		return "video"
	case detect.CatExecutable:
		return "cpu"
	case detect.CatFont:
		return "font"
	case detect.CatDatabase:
		return "database"
	case detect.CatText:
		return "filetext"
	case detect.CatCode:
		return "code"
	case detect.CatDiskImage:
		return "disc"
	case detect.CatEmail:
		return "mail"
	case detect.CatCrypto:
		return "key"
	case detect.CatEncrypted:
		return "lock"
	case detect.CatEmpty:
		return "blank"
	case "Folder":
		return "folder"
	case detect.CatUnknown:
		return "help"
	}
	return "file"
}
