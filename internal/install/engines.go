package install

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// TrID comes from its author's site as a zip per platform plus the
// definitions database. Neither is versioned or checksummed there.
var tridEngine = &Engine{
	ID:       "trid",
	Name:     "TrID",
	Source:   "mark0.net (Marco Pontello)",
	Size:     "about 2 MB",
	commands: []command{{name: "trid"}},
	unsupported: func(goos, goarch string) string {
		switch {
		case goos == "windows":
			return ""
		case goos == "linux" && goarch == "amd64":
			return ""
		case goos == "darwin":
			return "TrID has no macOS version."
		default:
			return "TrID has no build for " + goos + " " + goarch + "."
		}
	},
	fetch: func(ctx context.Context, j *job) error {
		pkg := "trid_w32.zip"
		if j.goos == "linux" {
			pkg = "trid_linux_64.zip"
		}
		j.source = "https://mark0.net/soft-trid-e.html"
		for _, name := range []string{pkg, "triddefs.zip"} {
			p, err := j.download("https://mark0.net/download/"+name, name, "", 0)
			if err != nil {
				return err
			}
			if err := j.unzip(p, keepAll); err != nil {
				return err
			}
		}
		return nil
	},
}

// magikaTriples are the Rust target names of Magika's CLI builds. Its
// Windows build is x86-64 only, which Windows on ARM runs emulated.
var magikaTriples = map[string]string{
	"windows/amd64": "x86_64-pc-windows-msvc",
	"windows/arm64": "x86_64-pc-windows-msvc",
	"darwin/arm64":  "aarch64-apple-darwin",
	"darwin/amd64":  "x86_64-apple-darwin",
	"linux/amd64":   "x86_64-unknown-linux-gnu",
	"linux/arm64":   "aarch64-unknown-linux-gnu",
}

// Magika's command-line tool is published on GitHub with a .sha256 file
// beside each archive, under tags like cli/v1.1.0.
var magikaEngine = &Engine{
	ID:       "magika",
	Name:     "Magika",
	Source:   "github.com/google/magika",
	Size:     "about 10 MB",
	commands: []command{{name: "magika", args: []string{"--version"}}},
	unsupported: func(goos, goarch string) string {
		if magikaTriples[goos+"/"+goarch] == "" {
			return "Magika has no build for " + goos + " " + goarch + "."
		}
		return ""
	},
	fetch: func(ctx context.Context, j *job) error {
		triple := magikaTriples[j.goos+"/"+j.goarch]
		rs, err := j.githubReleases("google/magika")
		if err != nil {
			return err
		}
		// Not every release has every platform (Intel Macs were dropped),
		// so take the newest that has this one.
		for _, r := range rs {
			if !strings.HasPrefix(r.Tag, "cli/v") {
				continue
			}
			for _, prefix := range []string{"magika-cli-", "magika-"} {
				for _, ext := range []string{".zip", ".tar.xz"} {
					a := r.find(prefix + triple + ext)
					if a == nil {
						continue
					}
					sum := r.find(a.Name + ".sha256")
					if sum == nil {
						return fmt.Errorf("Magika %s has no checksum for %s", r.Tag, a.Name)
					}
					list, err := j.fetchText(sum.URL)
					if err != nil {
						return err
					}
					sha := checksumFor(list, a.Name)
					if sha == "" {
						return fmt.Errorf("could not read the checksum of %s", a.Name)
					}
					p, err := j.download(a.URL, a.Name, sha, a.Size)
					if err != nil {
						return err
					}
					j.source = r.URL
					if ext == ".zip" {
						return j.unzip(p, keepNames("magika"))
					}
					return j.untarXZ(p, keepNames("magika"))
				}
			}
		}
		return fmt.Errorf("no Magika release has a build for %s", triple)
	},
}

// 7-Zip's releases on GitHub: an MSI for Windows (unpacked without
// installing, see unpackMSI) and a tar.xz with the standalone 7zz
// elsewhere. Igor Pavlov publishes no checksums.
var sevenZipEngine = &Engine{
	ID:       "7zip",
	Name:     "7-Zip",
	Source:   "github.com/ip7z/7zip (Igor Pavlov)",
	Size:     "about 2 MB",
	commands: sevenZipCommand(),
	unsupported: func(goos, goarch string) string {
		if sevenZipAsset(goos, goarch) == nil {
			return "7-Zip has no build for " + goos + " " + goarch + "."
		}
		return ""
	},
	fetch: func(ctx context.Context, j *job) error {
		rs, err := j.githubReleases("ip7z/7zip")
		if err != nil {
			return err
		}
		if len(rs) == 0 {
			return fmt.Errorf("7-Zip has no releases on GitHub")
		}
		r := rs[0]
		pattern := sevenZipAsset(j.goos, j.goarch)
		for _, a := range r.Assets {
			if !pattern.MatchString(a.Name) {
				continue
			}
			p, err := j.download(a.URL, a.Name, "", a.Size)
			if err != nil {
				return err
			}
			j.source = r.URL
			if strings.HasSuffix(a.Name, ".msi") {
				return j.unpackMSI(p, keepNames("7z", "7z.dll", "License.txt"))
			}
			return j.untarXZ(p, keepNames("7zz", "License.txt"))
		}
		return fmt.Errorf("7-Zip %s has no build for %s %s", r.Tag, j.goos, j.goarch)
	},
}

// sevenZipCommand is the program the install provides: the Windows
// MSI has the full 7z.exe, the other builds the standalone 7zz.
func sevenZipCommand() []command {
	if runtime.GOOS == "windows" {
		return []command{{name: "7z"}}
	}
	return []command{{name: "7zz"}}
}

// sevenZipAsset matches the 7-Zip release file for a platform, or is
// nil when there's none. Windows on ARM runs the x86-64 build.
func sevenZipAsset(goos, goarch string) *regexp.Regexp {
	suffix := map[string]string{
		"windows/amd64": `-x64\.msi`,
		"windows/arm64": `-x64\.msi`,
		"windows/386":   `\.msi`,
		"darwin/amd64":  `-mac\.tar\.xz`,
		"darwin/arm64":  `-mac\.tar\.xz`,
		"linux/amd64":   `-linux-x64\.tar\.xz`,
		"linux/arm64":   `-linux-arm64\.tar\.xz`,
		"linux/386":     `-linux-x86\.tar\.xz`,
		"linux/arm":     `-linux-arm\.tar\.xz`,
	}[goos+"/"+goarch]
	if suffix == "" {
		return nil
	}
	return regexp.MustCompile(`^7z\d+` + suffix + `$`)
}

// ffmpegBuilds names BtbN's static builds per platform.
var ffmpegBuilds = map[string]string{
	"windows/amd64": "win64",
	"windows/arm64": "winarm64",
	"linux/amd64":   "linux64",
	"linux/arm64":   "linuxarm64",
}

// FFmpeg: BtbN's static GPL builds of the latest release branch on
// GitHub (checksummed) for Windows and Linux, and Martin Riedl's signed
// builds for macOS, which come as one zip per program.
var ffmpegEngine = &Engine{
	ID:       "ffmpeg",
	Name:     "FFmpeg",
	Source:   ffmpegSource(),
	Size:     "about 100 MB",
	commands: []command{{name: "ffmpeg", args: []string{"-version"}}, {name: "ffprobe", args: []string{"-version"}}},
	unsupported: func(goos, goarch string) string {
		if goos == "darwin" && (goarch == "arm64" || goarch == "amd64") {
			return ""
		}
		if ffmpegBuilds[goos+"/"+goarch] == "" {
			return "There is no FFmpeg build for " + goos + " " + goarch + " to install; install it with your package manager."
		}
		return ""
	},
	fetch: func(ctx context.Context, j *job) error {
		if j.goos == "darwin" {
			j.source = "https://ffmpeg.martin-riedl.de"
			for _, prog := range []string{"ffmpeg", "ffprobe"} {
				url := "https://ffmpeg.martin-riedl.de/redirect/latest/macos/" + j.goarch + "/release/" + prog + ".zip"
				p, err := j.download(url, prog+".zip", "", 0)
				if err != nil {
					return err
				}
				if err := j.unzip(p, keepNames(prog)); err != nil {
					return err
				}
			}
			return nil
		}
		build := ffmpegBuilds[j.goos+"/"+j.goarch]
		rs, err := j.githubReleases("BtbN/FFmpeg-Builds")
		if err != nil {
			return err
		}
		if len(rs) == 0 {
			return fmt.Errorf("no FFmpeg builds found")
		}
		r := rs[0]
		a := newestFFmpeg(r.Assets, build)
		if a == nil {
			return fmt.Errorf("the latest FFmpeg builds have none for %s", build)
		}
		sums := r.find("checksums.sha256")
		if sums == nil {
			return fmt.Errorf("the FFmpeg builds have no checksums")
		}
		list, err := j.fetchText(sums.URL)
		if err != nil {
			return err
		}
		sha := checksumFor(list, a.Name)
		if sha == "" {
			return fmt.Errorf("no checksum is published for %s", a.Name)
		}
		p, err := j.download(a.URL, a.Name, sha, a.Size)
		if err != nil {
			return err
		}
		j.source = r.URL
		keep := keepNames("ffmpeg", "ffprobe", "LICENSE.txt")
		if strings.HasSuffix(a.Name, ".zip") {
			return j.unzip(p, keep)
		}
		return j.untarXZ(p, keep)
	},
}

func ffmpegSource() string {
	if runtime.GOOS == "darwin" {
		return "ffmpeg.martin-riedl.de"
	}
	return "github.com/BtbN/FFmpeg-Builds"
}

// ffmpegRelease matches BtbN's builds of a release branch, as in
// ffmpeg-n8.1.3-25-g611cec792a-win64-gpl-8.1.zip; the master builds
// (ffmpeg-N-...) and shared-library builds are skipped.
var ffmpegRelease = regexp.MustCompile(`^ffmpeg-n[\d.]+-.*-([a-z0-9]+)-gpl-(\d+)\.(\d+)\.(zip|tar\.xz)$`)

// newestFFmpeg picks the build for platform from the newest release
// branch.
func newestFFmpeg(assets []asset, build string) *asset {
	var best *asset
	bestVer := -1
	for i, a := range assets {
		m := ffmpegRelease.FindStringSubmatch(a.Name)
		if m == nil || m[1] != build {
			continue
		}
		major, _ := strconv.Atoi(m[2])
		minor, _ := strconv.Atoi(m[3])
		if v := major*1000 + minor; v > bestVer {
			best, bestVer = &assets[i], v
		}
	}
	return best
}
