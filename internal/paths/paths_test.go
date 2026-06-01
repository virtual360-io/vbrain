package paths

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDataHomeUsesEnvWhenSet(t *testing.T) {
	t.Setenv("VBRAIN_HOME", "/tmp/custom-vbrain")
	if got := DataHome(); got != "/tmp/custom-vbrain" {
		t.Fatalf("DataHome() = %q, want /tmp/custom-vbrain", got)
	}
}

func TestDataHomeDefaultsToHomeVbrainWhenEnvBlankAndNotInBase(t *testing.T) {
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "vbrain")

	// cwd in a dir without wiki/ → not a base → falls back to ~/vbrain.
	t.Chdir(t.TempDir())
	t.Setenv("VBRAIN_HOME", "")
	if got := DataHome(); got != want {
		t.Fatalf("blank env, fora de base: DataHome() = %q, want %q", got, want)
	}
}

func TestDataHomeUsesLocalBaseWhenEnvBlankAndRunningInBase(t *testing.T) {
	// cwd is a base (has wiki/) → use it, even without VBRAIN_HOME (fixes the
	// cloud where the checkout is the base).
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("VBRAIN_HOME", "")
	if got := DataHome(); got != dir {
		t.Fatalf("dentro de base: DataHome() = %q, want %q", got, dir)
	}
}

func TestSourceCheckoutIsNotABase(t *testing.T) {
	// A dir with wiki/ but also a go.mod is the vbrain source checkout, not a
	// base — otherwise `vbrain install` would bootstrap base assets into the
	// source tree and push them to the code remote.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsBase(dir) {
		t.Fatal("source checkout (wiki/ + go.mod) must not be treated as a base")
	}
	t.Chdir(dir)
	t.Setenv("VBRAIN_HOME", "")
	home, _ := os.UserHomeDir()
	if got, want := DataHome(), filepath.Join(home, "vbrain"); got != want {
		t.Fatalf("inside source checkout: DataHome() = %q, want %q", got, want)
	}
}

func TestDerivedPathsAreUnderDataHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VBRAIN_HOME", dir)

	if got, want := RawDir(), filepath.Join(dir, "raw"); got != want {
		t.Errorf("RawDir() = %q, want %q", got, want)
	}
	if got, want := WikiDir(), filepath.Join(dir, "wiki"); got != want {
		t.Errorf("WikiDir() = %q, want %q", got, want)
	}
	if got, want := DBPath(), filepath.Join(dir, "db", "vbrain.sqlite3"); got != want {
		t.Errorf("DBPath() = %q, want %q", got, want)
	}
	if got, want := TmpDir(), filepath.Join(dir, "raw", ".tmp"); got != want {
		t.Errorf("TmpDir() = %q, want %q", got, want)
	}
}

func TestEnsureDirsCreatesFlatStructure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VBRAIN_HOME", dir)

	if err := EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"raw", "wiki", "db", filepath.Join("raw", ".tmp"), filepath.Join("wiki", RealtimeDir)} {
		if fi, err := os.Stat(filepath.Join(dir, sub)); err != nil || !fi.IsDir() {
			t.Errorf("expected directory %q created", sub)
		}
	}
}

func TestEnsureDirsDoesNotCreateTypeFolders(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VBRAIN_HOME", dir)

	if err := EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{"concepts", "decisions", "gotchas", "notes", "_rules"} {
		if _, err := os.Stat(filepath.Join(dir, "wiki", old)); !os.IsNotExist(err) {
			t.Errorf("type folder %q should no longer be created (flat layout)", old)
		}
	}
}

func TestKindsIncludeAllSupportedMetadata(t *testing.T) {
	for _, k := range []string{"concept", "decision", "gotcha", "note", "rule", "realtime"} {
		if !slices.Contains(Kinds, k) {
			t.Errorf("Kinds should contain %q", k)
		}
	}
}
