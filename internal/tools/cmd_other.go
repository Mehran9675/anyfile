//go:build !windows

package tools

import "os/exec"

// Command builds an exec.Cmd for path.
func Command(path string, args ...string) *exec.Cmd {
	return exec.Command(path, args...)
}
