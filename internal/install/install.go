// Package install downloads the optional engines AnyFile delegates to
// (TrID, Magika, 7-Zip and FFmpeg) from their publishers and installs
// them into tools.ManagedDir, for the current user and without
// administrator rights. Downloads are verified against the publisher's
// checksums where it publishes them, and every program is run once
// before it replaces an earlier install.
package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mehran9675/anyfile/internal/tools"
	"github.com/mehran9675/giode/i18n"
)

// Engine is a tool AnyFile can install.
type Engine struct {
	ID   string // folder under tools.ManagedDir, and the tools lookup key
	Name string
	// Source names where the download comes from, shown before
	// installing.
	Source string
	// SizeMB is the rough download size, shown before installing.
	SizeMB int

	// commands lists the programs the install provides, with the
	// arguments that make each print its version.
	commands []command
	// unsupported says why the engine can't be installed here, or "".
	unsupported func(goos, goarch string) string
	// fetch downloads and unpacks the engine into j.dir.
	fetch func(ctx context.Context, j *job) error
}

type command struct {
	name string
	args []string
}

// Progress reports what an install is doing. Total is 0 when the size
// isn't known yet.
type Progress struct {
	Step        string
	Done, Total int64
}

// Engines lists what can be installed, in display order.
var Engines = []*Engine{tridEngine, magikaEngine, sevenZipEngine, ffmpegEngine}

// Find returns the engine with id, or nil.
func Find(id string) *Engine {
	for _, e := range Engines {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// Unsupported says why the engine has no build for this computer, or
// returns "" when it can be installed.
func (e *Engine) Unsupported() string {
	return e.unsupported(runtime.GOOS, runtime.GOARCH)
}

// Installed returns the manifest of AnyFile's install of the engine.
func (e *Engine) Installed() (tools.Manifest, bool) {
	return tools.ReadManifest(e.ID)
}

// Install downloads the engine's current release and installs it,
// replacing an earlier install by AnyFile. progress, when set, is called
// from the installing goroutine. Call tools.Reload afterwards.
func (e *Engine) Install(ctx context.Context, progress func(Progress)) error {
	if why := e.Unsupported(); why != "" {
		return errors.New(why)
	}
	if progress == nil {
		progress = func(Progress) {}
	}
	root := tools.ManagedDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	cleanLeftovers(root, e.ID)
	stage, err := os.MkdirTemp(root, "."+e.ID+"-new-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	j := &job{ctx: ctx, dir: stage, progress: progress, goos: runtime.GOOS, goarch: runtime.GOARCH}
	if err := e.fetch(ctx, j); err != nil {
		return err
	}
	progress(Progress{Step: i18n.Intl("Checking that {name} runs", "name", e.Name)})
	m := tools.Manifest{Source: j.source, Installed: time.Now(), Files: map[string]string{}}
	for _, c := range e.commands {
		rel := exe(c.name)
		p := filepath.Join(stage, rel)
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("the download has no %s", rel)
		}
		if runtime.GOOS != "windows" {
			os.Chmod(p, 0o755)
		}
		version, err := smokeTest(ctx, p, c.args)
		if err != nil {
			return fmt.Errorf("%s %w", i18n.Intl("{name} doesn't run on this computer:", "name", rel), err)
		}
		if m.Version == "" {
			m.Version = version
		}
		m.Files[c.name] = rel
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(stage, tools.ManifestName), b, 0o644); err != nil {
		return err
	}
	return swap(root, e.ID, stage)
}

// Remove deletes AnyFile's install of the engine. Call tools.Reload
// afterwards.
func (e *Engine) Remove() error {
	root := tools.ManagedDir()
	cleanLeftovers(root, e.ID)
	return os.RemoveAll(filepath.Join(root, e.ID))
}

// swap moves the staged install into place. The old one is moved aside
// first, so a failure leaves it working.
func swap(root, id, stage string) error {
	final := filepath.Join(root, id)
	old := ""
	if _, err := os.Stat(final); err == nil {
		old = filepath.Join(root, fmt.Sprintf(".%s-old-%d", id, time.Now().UnixNano()))
		if err := os.Rename(final, old); err != nil {
			return fmt.Errorf("%s (%w)", i18n.Intl("The installed copy is in use; close programs using it and try again."), err)
		}
	}
	if err := os.Rename(stage, final); err != nil {
		if old != "" {
			os.Rename(old, final)
		}
		return err
	}
	if old != "" {
		// A running copy can't be deleted on Windows; the next install
		// cleans it up.
		os.RemoveAll(old)
	}
	return nil
}

// cleanLeftovers removes staging and old folders an interrupted or
// in-use install left behind.
func cleanLeftovers(root, id string) {
	matches, _ := filepath.Glob(filepath.Join(root, "."+id+"-*"))
	for _, m := range matches {
		os.RemoveAll(m)
	}
}

// smokeTest runs the program once and returns the first line it
// prints. Many tools exit with an error when run without a file, so
// only failing to start counts.
func smokeTest(ctx context.Context, path string, args []string) (string, error) {
	if runtime.GOOS == "darwin" {
		adhocSign(path)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := tools.Command(path, args...)
	out, err := runContext(ctx, cmd)
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return versionLine(line), nil
		}
	}
	return "", nil
}

// versionLine shortens a program's banner to its name and version:
// "7-Zip 26.05 (x64) : Copyright (c) 1999-2026 Igor Pavlov" becomes
// "7-Zip 26.05 (x64)".
func versionLine(s string) string {
	lower := strings.ToLower(s)
	for _, cut := range []string{" copyright", " (c)"} {
		if i := strings.Index(lower, cut); i > 0 {
			s, lower = s[:i], lower[:i]
		}
	}
	return strings.TrimRight(s, " :-")
}

// runContext runs cmd, killing it when ctx ends.
func runContext(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	var buf strings.Builder
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return []byte(buf.String()), err
	case <-ctx.Done():
		cmd.Process.Kill()
		<-done
		return []byte(buf.String()), ctx.Err()
	}
}

// adhocSign signs an unsigned macOS program for this computer: Apple
// silicon refuses to run unsigned code.
func adhocSign(path string) {
	if exec.Command("codesign", "-v", path).Run() == nil {
		return
	}
	exec.Command("codesign", "--force", "--sign", "-", path).Run()
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
