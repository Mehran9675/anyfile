//go:build !windows

package install

import "errors"

func (j *job) unpackMSI(string, keepFunc) error {
	return errors.New("MSI packages can only be unpacked on Windows")
}
