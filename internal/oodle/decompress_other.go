//go:build !windows

package oodle

import "errors"

// Decompress is only implemented on Windows, where games ship the
// oo2core DLL.
func Decompress(near string, src []byte, rawSize int) ([]byte, error) {
	return nil, errors.New("Oodle decompression is only supported on Windows")
}
