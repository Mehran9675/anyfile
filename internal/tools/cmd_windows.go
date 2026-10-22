//go:build windows

package tools

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// Command builds an exec.Cmd that never flashes a console window.
func Command(path string, args ...string) *exec.Cmd {
	c := exec.Command(path, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return c
}
