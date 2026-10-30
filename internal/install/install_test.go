package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xmhrnx/anyfile/internal/tools"
)

func TestChecksumFor(t *testing.T) {
	list := "aaaa  other.zip\n" +
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  ffmpeg-n8.1-win64-gpl-8.1.zip\n"
	if got := checksumFor(list, "ffmpeg-n8.1-win64-gpl-8.1.zip"); got != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("list: got %q", got)
	}
	single := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210\n"
	if got := checksumFor(single, "magika.zip"); got != "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210" {
		t.Fatalf("single: got %q", got)
	}
	if got := checksumFor(list, "missing.zip"); got != "" {
		t.Fatalf("missing: got %q", got)
	}
}

func TestVersionLine(t *testing.T) {
	cases := map[string]string{
		"7-Zip 26.05 (x64) : Copyright (c) 1999-2026 Igor Pavlov : 2026-10-21":                        "7-Zip 26.05 (x64)",
		"TrID/32 - File Identifier v2.24 - (C) 2003-16 By M.Pontello":                                 "TrID/32 - File Identifier v2.24",
		"ffmpeg version n9.0.2-37-gae19868753-20261028 Copyright (c) 2000-2026 the FFmpeg developers": "ffmpeg version n9.0.2-37-gae19868753-20261028",
		"magika 1.1.0 standard_v3_3": "magika 1.1.0 standard_v3_3",
	}
	for in, want := range cases {
		if got := versionLine(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestNewestFFmpeg(t *testing.T) {
	assets := []asset{
		{Name: "ffmpeg-N-128142-g28fda48e0b-win64-gpl.zip"},
		{Name: "ffmpeg-n7.1.2-20-gabc-win64-gpl-7.1.zip"},
		{Name: "ffmpeg-n8.1.3-25-g611cec792a-win64-gpl-shared-8.1.zip"},
		{Name: "ffmpeg-n8.1.3-25-g611cec792a-win64-gpl-8.1.zip"},
		{Name: "ffmpeg-n8.1.3-25-g611cec792a-winarm64-gpl-8.1.zip"},
		{Name: "ffmpeg-n8.1.3-25-g611cec792a-win64-lgpl-8.1.zip"},
	}
	if a := newestFFmpeg(assets, "win64"); a == nil || a.Name != "ffmpeg-n8.1.3-25-g611cec792a-win64-gpl-8.1.zip" {
		t.Fatalf("win64: got %+v", a)
	}
	if a := newestFFmpeg(assets, "linux64"); a != nil {
		t.Fatalf("linux64: got %+v", a)
	}
}

func TestSevenZipAsset(t *testing.T) {
	cases := map[string]string{
		"windows/amd64": "7z2605-x64.msi",
		"windows/386":   "7z2605.msi",
		"darwin/arm64":  "7z2605-mac.tar.xz",
		"linux/arm64":   "7z2605-linux-arm64.tar.xz",
	}
	all := []string{"7z2605-x64.msi", "7z2605.msi", "7z2605-x64.exe", "7z2605-mac.tar.xz", "7z2605-linux-arm64.tar.xz", "7z2605-linux-arm.tar.xz", "7z2605-extra.7z"}
	for platform, want := range cases {
		var goos, goarch string
		for i := range platform {
			if platform[i] == '/' {
				goos, goarch = platform[:i], platform[i+1:]
			}
		}
		re := sevenZipAsset(goos, goarch)
		var got []string
		for _, n := range all {
			if re.MatchString(n) {
				got = append(got, n)
			}
		}
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s: got %v, want %s", platform, got, want)
		}
	}
	if sevenZipAsset("plan9", "amd64") != nil {
		t.Error("plan9 should have no build")
	}
}

// TestInstallLive downloads and installs every engine this computer
// supports into a temporary folder. It needs the network, so it runs
// only with ANYFILE_NET_TEST=1.
func TestInstallLive(t *testing.T) {
	if os.Getenv("ANYFILE_NET_TEST") == "" {
		t.Skip("set ANYFILE_NET_TEST=1 to download the engines")
	}
	dir := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", dir)
	case "darwin":
		t.Setenv("HOME", dir)
	default:
		t.Setenv("XDG_DATA_HOME", dir)
	}
	for _, e := range Engines {
		t.Run(e.ID, func(t *testing.T) {
			if why := e.Unsupported(); why != "" {
				t.Skip(why)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := e.Install(ctx, func(p Progress) {}); err != nil {
				t.Fatal(err)
			}
			m, ok := e.Installed()
			if !ok {
				t.Fatal("no manifest after installing")
			}
			for name, rel := range m.Files {
				if _, err := os.Stat(filepath.Join(tools.ManagedDir(), e.ID, rel)); err != nil {
					t.Errorf("%s: %v", name, err)
				}
			}
			t.Logf("%s: %s from %s", e.Name, m.Version, m.Source)
			tools.Reload()
			for _, tl := range tools.All() {
				if tl.Found() && strings.HasPrefix(tl.Path, tools.ManagedDir()) {
					t.Logf("  found %s at %s", tl.Name, tl.Path)
				}
			}
			// Installing again replaces the first install.
			if err := e.Install(ctx, nil); err != nil {
				t.Fatalf("reinstall: %v", err)
			}
			if err := e.Remove(); err != nil {
				t.Fatal(err)
			}
			if _, ok := e.Installed(); ok {
				t.Fatal("still installed after Remove")
			}
		})
	}
}

// TestFetchOtherPlatforms downloads and unpacks the builds for other
// platforms, without running them, to check that their sources and
// archive layouts still match. It needs the network, so it runs only
// with ANYFILE_NET_TEST=1.
func TestFetchOtherPlatforms(t *testing.T) {
	if os.Getenv("ANYFILE_NET_TEST") == "" {
		t.Skip("set ANYFILE_NET_TEST=1 to download the engines")
	}
	platforms := []struct {
		goos, goarch string
		files        map[string][]string
	}{
		{"linux", "amd64", map[string][]string{"trid": {"trid", "triddefs.trd"}, "magika": {"magika"}, "7zip": {"7zz"}, "ffmpeg": {"ffmpeg", "ffprobe"}}},
		{"linux", "arm64", map[string][]string{"magika": {"magika"}, "7zip": {"7zz"}, "ffmpeg": {"ffmpeg", "ffprobe"}}},
		{"darwin", "arm64", map[string][]string{"magika": {"magika"}, "7zip": {"7zz"}, "ffmpeg": {"ffmpeg", "ffprobe"}}},
		{"darwin", "amd64", map[string][]string{"magika": {"magika"}, "ffmpeg": {"ffmpeg", "ffprobe"}}},
		{"windows", "arm64", map[string][]string{"ffmpeg": {"ffmpeg.exe", "ffprobe.exe"}}},
	}
	for _, pl := range platforms {
		for _, e := range Engines {
			want, ok := pl.files[e.ID]
			if !ok {
				continue
			}
			t.Run(pl.goos+"-"+pl.goarch+"/"+e.ID, func(t *testing.T) {
				if why := e.unsupported(pl.goos, pl.goarch); why != "" {
					t.Fatal(why)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()
				dir := t.TempDir()
				j := &job{ctx: ctx, dir: dir, progress: func(Progress) {}, goos: pl.goos, goarch: pl.goarch}
				if err := e.fetch(ctx, j); err != nil {
					t.Fatal(err)
				}
				for _, f := range want {
					if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
						t.Error(err)
					}
				}
			})
		}
	}
}
