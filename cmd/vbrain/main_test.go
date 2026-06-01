package main

import (
	"os"
	"path/filepath"
	"testing"
)

// When the running binary is already on PATH (the Homebrew / package-manager
// case), installSelf must NOT copy a duplicate into binDir — a second copy is
// what `vbrain update` would then diverge from the managed one.
func TestInstallSelfSkipsWhenAlreadyOnPath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(exe))

	binDir := t.TempDir()
	path, onPath, err := installSelf(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if !onPath || path != exe {
		t.Fatalf("installSelf = (%q, %v), want (%q, true)", path, onPath, exe)
	}
	if entries, _ := os.ReadDir(binDir); len(entries) != 0 {
		t.Fatalf("should not have copied a duplicate into binDir, found: %v", entries)
	}
}

// The default (curl) flow: the binary runs from a dir that is not on PATH, so
// installSelf copies it into binDir.
func TestInstallSelfCopiesWhenNotOnPath(t *testing.T) {
	t.Setenv("PATH", filepath.Join(t.TempDir(), "nowhere"))

	binDir := t.TempDir()
	path, _, err := installSelf(binDir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(binDir, "vbrain")
	if path != want {
		t.Fatalf("installSelf path = %q, want %q", path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("binary not copied into binDir: %v", err)
	}
}
