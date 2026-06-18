package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/git"
	"github.com/virtual360-io/vbrain/internal/routines"
	"github.com/virtual360-io/vbrain/internal/skills"
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

// `vbrain version` / `--version` / `-v` print the injected version. Built as a
// subprocess because main() calls os.Exit; ldflags proves the injection seam.
func TestVersionFlag(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "vbrain")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=v9.9.9", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	for _, flag := range []string{"version", "--version", "-v"} {
		out, err := exec.Command(bin, flag).Output()
		if err != nil {
			t.Fatalf("vbrain %s: %v", flag, err)
		}
		if got := strings.TrimSpace(string(out)); got != "v9.9.9" {
			t.Errorf("vbrain %s = %q, want v9.9.9", flag, got)
		}
	}
}

// `vbrain update` must do more than swap the binary: it has to sync the base
// from the NEW binary's embed and seed that version's default routines —
// otherwise a freshly-added default (e.g. the `soul` routine) never lands on an
// updated base, which is the bug that motivated unifying install/update.
// cmdInstall closes that gap by re-exec'ing the freshly-installed binary's
// hidden __bootstrap; this exercises that hop against a real build and asserts
// the seed actually reaches the base.
func TestReexecBootstrapSeedsDefaultRoutines(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "vbrain")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}

	// Build with the real env (warm cache), then sandbox HOME (global skills land
	// in ~/.claude/skills) and VBRAIN_HOME (the base) so neither touches the
	// developer's real files. The child re-exec inherits both.
	home, base := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("VBRAIN_HOME", base)

	out, err := reexecBootstrap(bin, "none", "vbrain", "")
	if err != nil {
		t.Fatalf("reexecBootstrap: %v", err)
	}
	if seeded, _ := out["seeded_routines"].([]any); len(seeded) == 0 {
		t.Fatalf("__bootstrap seeded no routines; out=%v", out)
	}
	data, err := os.ReadFile(filepath.Join(base, "config", "routines", "routines.yml"))
	if err != nil {
		t.Fatalf("routines.yml not written by __bootstrap: %v", err)
	}
	for _, slug := range []string{"soul", "dream"} {
		if !strings.Contains(string(data), "slug: "+slug) {
			t.Errorf("default routine %q not seeded into the updated base", slug)
		}
	}
}

// The terminal install path: a valid HH:MM creates the suggested routine in the
// base; empty/invalid creates nothing. (The TTY prompt itself is the existing
// isTerminal/prompt helpers; here we test the creation seam directly.)
func TestCreateRoutineFromTime(t *testing.T) {
	t.Setenv("VBRAIN_HOME", t.TempDir())
	sug := &skills.RoutineSuggestion{Slug: "sync-transcriptions", Description: "d", Prompt: "do it"}

	if createRoutineFromTime(sug, "") != nil {
		t.Fatal("empty time must not create a routine")
	}
	if createRoutineFromTime(sug, "nonsense") != nil {
		t.Fatal("invalid time must not create a routine")
	}

	res := createRoutineFromTime(sug, "07:00")
	if res == nil || res["schedule"] != "0 7 * * *" {
		t.Fatalf("res = %v, want schedule 0 7 * * *", res)
	}
	data, err := os.ReadFile(routines.ConfigPath())
	if err != nil {
		t.Fatalf("routines.yml not written: %v", err)
	}
	if !strings.Contains(string(data), "sync-transcriptions") || !strings.Contains(string(data), "0 7 * * *") {
		t.Fatalf("routine not persisted correctly:\n%s", data)
	}
}

func TestHHMMToCron(t *testing.T) {
	ok := map[string]string{"07:00": "0 7 * * *", "7:30": "30 7 * * *", "23:59": "59 23 * * *", "00:00": "0 0 * * *"}
	for in, want := range ok {
		got, valid := hhmmToCron(in)
		if !valid || got != want {
			t.Errorf("hhmmToCron(%q) = (%q,%v), want (%q,true)", in, got, valid, want)
		}
	}
	for _, bad := range []string{"", "25:00", "07:60", "7", "noon", "7:5:5"} {
		if _, valid := hhmmToCron(bad); valid {
			t.Errorf("hhmmToCron(%q) should be invalid", bad)
		}
	}
}

// Core and optional skills install into the same ~/.claude/skills/<name>, so a
// shared name would clobber the core one. The embeds must stay disjoint.
func TestOptionalSkillsDontCollideWithCore(t *testing.T) {
	coreFS, err := embeddedSkills()
	if err != nil {
		t.Fatal(err)
	}
	optFS, err := embeddedOptionalSkills()
	if err != nil {
		t.Fatal(err)
	}
	coreEntries, err := fs.ReadDir(coreFS, ".")
	if err != nil {
		t.Fatal(err)
	}
	core := map[string]bool{}
	for _, e := range coreEntries {
		if e.IsDir() {
			core[e.Name()] = true
		}
	}
	optEntries, err := fs.ReadDir(optFS, ".")
	if err != nil {
		t.Fatal(err)
	}
	sawOptional := false
	for _, e := range optEntries {
		if !e.IsDir() {
			continue
		}
		sawOptional = true
		if core[e.Name()] {
			t.Errorf("optional skill %q collides with a core skill", e.Name())
		}
	}
	if !sawOptional {
		t.Fatal("expected at least one optional skill in the embed")
	}
}

// The optional-skills lifecycle against a real build: install lands the skill in
// base + global; the update hop (__bootstrap) re-syncs the installed optional
// from the new binary's embed (the "kept current when installed" guarantee);
// remove drops it from both.
func TestSkillInstallSyncedByBootstrapThenRemove(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "vbrain")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	home, base := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("VBRAIN_HOME", base)
	if err := git.Init(base); err != nil {
		t.Fatal(err)
	}

	const name = "vbrain-sync-transcriptions"
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
			t.Fatalf("vbrain %v: %v: %s", args, err, out)
		}
	}
	baseSkill := filepath.Join(base, ".claude", "skills", name, "SKILL.md")
	globalSkill := filepath.Join(home, ".claude", "skills", name, "SKILL.md")

	// Non-interactive install (no TTY): the skill's routine.yml surfaces as
	// suggested_routine so an agent can offer it, rather than blocking on a prompt.
	installOut, err := exec.Command(bin, "skill", "install", name).Output()
	if err != nil {
		t.Fatalf("skill install: %v", err)
	}
	if !strings.Contains(string(installOut), "suggested_routine") {
		t.Errorf("non-TTY install should surface suggested_routine; got %s", installOut)
	}
	for _, p := range []string{baseSkill, globalSkill} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("after install, missing %s: %v", p, err)
		}
	}

	// Drop the global copy, then run the update hop; Sync must re-materialize it.
	os.RemoveAll(filepath.Join(home, ".claude", "skills", name))
	out, err := reexecBootstrap(bin, "none", "vbrain", "")
	if err != nil {
		t.Fatalf("reexecBootstrap: %v", err)
	}
	synced, _ := out["optional_skills_synced"].([]any)
	found := false
	for _, s := range synced {
		if s == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("update did not re-sync the installed optional; optional_skills_synced=%v", synced)
	}
	if _, err := os.Stat(globalSkill); err != nil {
		t.Fatalf("update did not re-materialize the optional into global: %v", err)
	}

	run("skill", "remove", name)
	for _, p := range []string{baseSkill, globalSkill} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("after remove, %s should be gone", p)
		}
	}
}
