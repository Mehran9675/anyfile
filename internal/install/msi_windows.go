package install

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mehran9675/giode/i18n"
	"github.com/xmhrnx/anyfile/internal/tools"
)

// unpackMSI extracts a Windows Installer package with an administrative
// install (msiexec /a), which copies its files to a folder without
// installing anything or asking for administrator rights, then keeps
// the files keep wants, flat in the job's folder.
func (j *job) unpackMSI(msi string, keep keepFunc) error {
	defer os.Remove(msi)
	j.progress(Progress{Step: i18n.Intl("Unpacking")})
	out := j.tempPath("msi")
	defer os.RemoveAll(out)
	// msiexec parses TARGETDIR=... itself, so the path is quoted for it.
	cmd := tools.Command("msiexec")
	cmd.SysProcAttr.CmdLine = fmt.Sprintf(`msiexec /a "%s" /qn TARGETDIR="%s"`, msi, out)
	if b, err := runContext(j.ctx, cmd); err != nil {
		return fmt.Errorf("could not unpack %s: %v %s", filepath.Base(msi), err, b)
	}
	return j.keepFrom(out, func(name string) bool {
		return keep(name) && filepath.Ext(name) != ".msi"
	})
}
