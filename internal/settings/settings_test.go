package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// useTempConfig points the user config folder at a test directory.
func useTempConfig(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{"APPDATA", "XDG_CONFIG_HOME", "HOME"} {
		t.Setenv(v, dir)
	}
	current.Store(ptr(Defaults()))
}

func ptr(s Settings) *Settings { return &s }

func TestSaveAndLoad(t *testing.T) {
	useTempConfig(t)
	s := Defaults()
	s.HeadKB, s.TrIDMode, s.AutoCarve = 128, TrIDNever, false
	if err := Set(s); err != nil {
		t.Fatal(err)
	}
	current.Store(ptr(Defaults()))
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if got := Get(); got.HeadKB != 128 || got.TrIDMode != TrIDNever || got.AutoCarve {
		t.Fatalf("loaded %+v", got)
	}
}

func TestLoadClampsAndKeepsDefaults(t *testing.T) {
	useTempConfig(t)
	os.MkdirAll(filepath.Dir(Path()), 0o755)
	os.WriteFile(Path(), []byte(`{"head_kb": 1, "trid_mode": "sometimes"}`), 0o644)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	got := Get()
	if got.HeadKB != Limits["head_kb"].Min || got.TrIDMode != TrIDWhenNeeded || got.PrefetchCount != Defaults().PrefetchCount {
		t.Fatalf("loaded %+v", got)
	}
}
