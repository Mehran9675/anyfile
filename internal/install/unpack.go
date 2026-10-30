package install

import (
	"archive/tar"
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"

	"github.com/mehran9675/anyfile/internal/tools"
	"github.com/mehran9675/giode/i18n"
)

// keepFunc decides which archive members to install: it gets a
// member's base name and returns whether to keep it. Kept members are
// written flat into the job's folder under their base names.
type keepFunc func(name string) bool

// keepAll keeps every member.
func keepAll(string) bool { return true }

// keepNames keeps members with these base names, compared without case
// and without an .exe suffix.
func keepNames(names ...string) keepFunc {
	return func(n string) bool {
		n = strings.TrimSuffix(strings.ToLower(n), ".exe")
		for _, k := range names {
			if n == strings.ToLower(k) {
				return true
			}
		}
		return false
	}
}

// unzip unpacks a zip archive, then deletes it.
func (j *job) unzip(archive string, keep keepFunc) error {
	defer os.Remove(archive)
	j.progress(Progress{Step: i18n.Intl("Unpacking")})
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("the download isn't a valid zip archive: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
		if !keep(name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = j.write(name, rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return j.ctx.Err()
}

// untarXZ unpacks a .tar.xz archive, then deletes it. Archives
// compressed with filters the Go decoder lacks (7-Zip's use a BCJ filter
// for executables) go to the system's tar, which macOS and Linux have.
func (j *job) untarXZ(archive string, keep keepFunc) error {
	defer os.Remove(archive)
	j.progress(Progress{Step: i18n.Intl("Unpacking")})
	err := j.untarXZGo(archive, keep)
	if err != nil && strings.Contains(err.Error(), "unsupported filter") {
		return j.untarXZSystem(archive, keep)
	}
	return err
}

func (j *job) untarXZGo(archive string, keep keepFunc) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	xr, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("the download isn't a valid .tar.xz archive: %w", err)
	}
	tr := tar.NewReader(xr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := j.ctx.Err(); err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Base(h.Name)
		if !keep(name) {
			continue
		}
		if err := j.write(name, tr); err != nil {
			return err
		}
	}
}

// untarXZSystem unpacks with the system's tar into a temporary folder
// and keeps the wanted files from it.
func (j *job) untarXZSystem(archive string, keep keepFunc) error {
	out := j.tempPath("tar")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(out)
	// Relative paths: GNU tar takes "C:" in a path for a remote host.
	cmd := tools.Command("tar", "-xJf", filepath.Base(archive), "-C", filepath.Base(out))
	cmd.Dir = filepath.Dir(archive)
	if b, err := runContext(j.ctx, cmd); err != nil {
		return fmt.Errorf("could not unpack %s with tar: %v %s", filepath.Base(archive), err, strings.TrimSpace(string(b)))
	}
	return j.keepFrom(out, keep)
}

// keepFrom copies the files keep wants from an unpacked folder tree
// into the job's folder.
func (j *job) keepFrom(dir string, keep keepFunc) error {
	return filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() || !keep(d.Name()) {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		return j.write(d.Name(), f)
	})
}

// write saves one unpacked file into the job's folder.
func (j *job) write(name string, r io.Reader) error {
	if name == "" || name == "." || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\:`) {
		return nil
	}
	out, err := os.OpenFile(filepath.Join(j.dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(r, maxDownload)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
