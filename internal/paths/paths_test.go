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

func TestDataHomeDefaultsToHomeVbrainWhenEnvBlank(t *testing.T) {
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "vbrain")

	t.Setenv("VBRAIN_HOME", "")
	if got := DataHome(); got != want {
		t.Fatalf("blank env: DataHome() = %q, want %q", got, want)
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
			t.Errorf("esperava diretório %q criado", sub)
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
			t.Errorf("pasta de tipo %q não deve mais ser criada (layout plano)", old)
		}
	}
}

func TestKindsIncludeAllSupportedMetadata(t *testing.T) {
	for _, k := range []string{"concept", "decision", "gotcha", "note", "rule", "realtime"} {
		if !slices.Contains(Kinds, k) {
			t.Errorf("Kinds deveria conter %q", k)
		}
	}
}
