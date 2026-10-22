//go:build windows

package platform

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32           = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
)

// Open launches the user's default application for the file.
func Open(path string) error {
	verb, _ := syscall.UTF16PtrFromString("open")
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	const swShowNormal = 1
	r, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, swShowNormal)
	switch {
	case r > 32:
		return nil
	case r == 31: // SE_ERR_NOASSOC
		return OpenWith(path)
	}
	return fmt.Errorf("Windows could not open the file (code %d)", r)
}

// OpenWith shows the system "Open with" chooser.
func OpenWith(path string) error {
	cmd := exec.Command("rundll32.exe", "shell32.dll,OpenAs_RunDLL", path)
	return cmd.Start()
}

// Reveal opens the file manager with the file selected.
func Reveal(path string) error {
	cmd := exec.Command("explorer.exe")
	// explorer parses its own command line; /select needs one argument.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	return cmd.Start()
}

// CanOpenWith reports whether OpenWith shows a chooser here.
func CanOpenWith() bool { return true }
