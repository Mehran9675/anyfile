// Package platform hands files to the operating system: opening them
// in the user's own viewers (AnyFile never renders content itself),
// revealing them in the file manager and native file dialogs.
package platform

import (
	"runtime"

	"github.com/ncruces/zenity"
)

// PickFile shows the native "open file" dialog. ok is false when the
// user cancelled.
func PickFile(title string) (string, bool) {
	p, err := zenity.SelectFile(zenity.Title(title))
	return p, err == nil && p != ""
}

// PickFolder shows the native folder picker.
func PickFolder(title string) (string, bool) {
	p, err := zenity.SelectFile(zenity.Title(title), zenity.Directory())
	return p, err == nil && p != ""
}

// FileDropSupported reports whether files can be dropped on the window
// here (giode implements drops on Windows only for now).
func FileDropSupported() bool { return runtime.GOOS == "windows" }
