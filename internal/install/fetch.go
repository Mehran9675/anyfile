package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// maxDownload caps a single download.
const maxDownload = 1 << 30

var client = &http.Client{Timeout: 30 * time.Minute}

// job is one install in progress: downloads land in dir, and the
// engine's files are unpacked flat into it.
type job struct {
	ctx          context.Context
	dir          string
	progress     func(Progress)
	goos, goarch string
	// source is recorded in the manifest.
	source string
}

// download fetches url into a temporary file in the job's folder and
// returns its path. A non-empty sha256 must match the content.
func (j *job) download(url, name, sha string, size int64) (string, error) {
	req, err := http.NewRequestWithContext(j.ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AnyFile")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download of %s failed: %s", name, resp.Status)
	}
	if resp.ContentLength > 0 {
		size = resp.ContentLength
	}
	if size > maxDownload {
		return "", fmt.Errorf("%s is larger than expected (%d MB)", name, size>>20)
	}
	f, err := os.CreateTemp(j.dir, ".download-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	step := "Downloading " + name
	var done int64
	buf := make([]byte, 256<<10)
	last := time.Time{}
	body := io.LimitReader(resp.Body, maxDownload+1)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return "", err
			}
			h.Write(buf[:n])
			done += int64(n)
			if time.Since(last) > 100*time.Millisecond {
				j.progress(Progress{Step: step, Done: done, Total: size})
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	if done > maxDownload {
		return "", fmt.Errorf("%s is larger than expected", name)
	}
	j.progress(Progress{Step: step, Done: done, Total: done})
	if sha != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sha) {
		return "", fmt.Errorf("%s doesn't match its published checksum; nothing was installed", name)
	}
	return f.Name(), nil
}

// fetchText downloads a small text file, such as a checksum list.
func (j *job) fetchText(url string) (string, error) {
	req, err := http.NewRequestWithContext(j.ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AnyFile")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}

// checksumFor finds name's SHA-256 in a sha256sum-style list ("<hash>
// <name>" lines), or the single hash of a .sha256 file.
func checksumFor(list, name string) string {
	for _, line := range strings.Split(list, "\n") {
		f := strings.Fields(line)
		switch {
		case len(f) == 1 && len(f[0]) == 64:
			return f[0]
		case len(f) >= 2 && strings.TrimPrefix(f[len(f)-1], "*") == name:
			return f[0]
		}
	}
	return ""
}

// release is a GitHub release with the fields the installers use.
type release struct {
	Tag    string  `json:"tag_name"`
	URL    string  `json:"html_url"`
	Draft  bool    `json:"draft"`
	Pre    bool    `json:"prerelease"`
	Assets []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// find returns the release's asset named name.
func (r *release) find(name string) *asset {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

// githubReleases lists a repository's recent releases, newest first.
func (j *job) githubReleases(repo string) ([]release, error) {
	j.progress(Progress{Step: "Looking up the latest release"})
	req, err := http.NewRequestWithContext(j.ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases?per_page=30", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AnyFile")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("GitHub is limiting requests from this network; try again in an hour")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub: %s", resp.Status)
	}
	var rs []release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&rs); err != nil {
		return nil, err
	}
	out := rs[:0]
	for _, r := range rs {
		if !r.Draft && !r.Pre {
			out = append(out, r)
		}
	}
	return out, nil
}

// urlName is the file name at the end of a URL.
func urlName(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	return path.Base(u)
}

// tempPath is a path for an intermediate file in the job's folder.
func (j *job) tempPath(name string) string {
	return filepath.Join(j.dir, ".tmp-"+name)
}
