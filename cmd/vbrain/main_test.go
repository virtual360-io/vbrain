package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/virtual360-io/vbrain/internal/git"
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

// With the gh CLI available, repo creation must not need a PAT — createGitHubRepo
// returns the gh-provided URL without ever hitting the REST API (here: no token).
func TestCreateGitHubRepoPrefersGh(t *testing.T) {
	orig := ghRepoURL
	t.Cleanup(func() { ghRepoURL = orig })
	ghRepoURL = func(name string, private bool) (string, bool) {
		return "git@github.com:me/" + name + ".git", true
	}

	url, err := createGitHubRepo("vbrain", true, "") // empty token: must not reach the network
	if err != nil {
		t.Fatal(err)
	}
	if url != "git@github.com:me/vbrain.git" {
		t.Fatalf("url = %q, want the gh-provided SSH URL", url)
	}
}

// Re-running install over a base that already has a remote must push with the
// system git's own credentials — no GITHUB_TOKEN required. Uses a local bare
// repo as origin so the test is offline and deterministic.
func TestBootstrapPushesToExistingRemoteWithoutToken(t *testing.T) {
	base := t.TempDir()
	t.Setenv("VBRAIN_HOME", base)
	t.Setenv("GITHUB_TOKEN", "")

	remote := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}
	if err := git.Init(base); err != nil {
		t.Fatal(err)
	}
	if err := git.AddRemote(remote, base, "origin"); err != nil {
		t.Fatal(err)
	}

	out := map[string]any{}
	if err := bootstrapBase(out, "none", "", ""); err != nil {
		t.Fatal(err)
	}
	if out["pushed"] != true {
		t.Fatalf("expected push to the existing remote, got out=%v", out)
	}
	// the bare remote actually received main
	if o, err := exec.Command("git", "-C", remote, "rev-parse", "main").CombinedOutput(); err != nil {
		t.Fatalf("remote did not receive main: %v: %s", err, o)
	}
}
