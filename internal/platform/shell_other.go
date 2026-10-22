//go:build !windows

package platform

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// Open launches the user's default application for the file.
func Open(path string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", path).Start()
	}
	return exec.Command("xdg-open", path).Start()
}

// OpenWith lets the user pick the application where the desktop
// offers a chooser; elsewhere it falls back to the default handler.
func OpenWith(path string) error {
	if runtime.GOOS == "darwin" {
		// Finder's Get Info is the closest to a chooser.
		return exec.Command("open", "-R", path).Start()
	}
	if p, err := exec.LookPath("gio"); err == nil {
		return exec.Command(p, "open", path).Start()
	}
	return Open(path)
}

// Reveal opens the file manager at the file.
func Reveal(path string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", path).Start()
	}
	return exec.Command("xdg-open", filepath.Dir(path)).Start()
}

// CanOpenWith reports whether OpenWith shows a chooser here.
func CanOpenWith() bool { return runtime.GOOS != "darwin" }
