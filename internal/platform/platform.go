// Package platform hands files to the operating system: opening them
// in the user's own viewers (AnyFile never renders content itself),
// revealing them in the file manager and native file dialogs.
package platform

import (
	"os"
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

// PickFiles shows the native "open file" dialog allowing several files.
func PickFiles(title string) ([]string, bool) {
	ps, err := zenity.SelectFileMultiple(zenity.Title(title))
	return ps, err == nil && len(ps) > 0
}

// IsFile reports whether p is an existing regular file.
func IsFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// IsDir reports whether p is an existing folder.
func IsDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
