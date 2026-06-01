package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/git"
)

func gitLog(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = dir
	out, _ := cmd.Output()
	return string(out)
}

func TestRepoInitializedFalseInEmptyDir(t *testing.T) {
	if git.RepoInitialized(t.TempDir()) {
		t.Fatal("empty dir should not be a repo")
	}
}

func TestInitCreatesRepoWithGitignoreAndInitialCommit(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatal(err)
	}
	if !git.RepoInitialized(dir) {
		t.Fatal("should be initialized")
	}
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gi), "/db/") {
		t.Error("/db/ must NOT be ignored (versioned index)")
	}
	if !strings.Contains(string(gi), "/raw/.tmp/") {
		t.Error(".gitignore should contain /raw/.tmp/")
	}
	if git.CurrentBranch(dir) != "main" {
		t.Errorf("branch = %q, want main", git.CurrentBranch(dir))
	}
	if !strings.Contains(gitLog(t, dir), "initialize vbrain") {
		t.Error("initial commit missing")
	}
}

func TestInitRaisesIfAlreadyInitialized(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := git.Init(dir); err == nil {
		t.Fatal("second init should fail")
	}
}

func TestCommitReturnsNoChangesWhenNothingChanged(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatal(err)
	}
	res, err := git.Commit("noop", dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Committed || res.Reason != "no changes" {
		t.Fatalf("res = %+v", res)
	}
}

func TestCommitStagesAndCommitsNewFiles(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "wiki-page.md"), []byte("# Hi\n"), 0o644)
	res, err := git.Commit("add: hi", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Committed || res.SHA == "" || res.Message != "add: hi" {
		t.Fatalf("res = %+v", res)
	}
	if !strings.Contains(gitLog(t, dir), "add: hi") {
		t.Error("commit didn't appear in the log")
	}
}

func TestCommitVersionsTheSQLiteIndex(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "db"), 0o755)
	os.WriteFile(filepath.Join(dir, "db", "vbrain.sqlite3"), []byte("SQLite format 3\x00"), 0o644)
	res, err := git.Commit("index", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Committed {
		t.Fatal("should commit")
	}
	cmd := exec.Command("git", "ls-files", "db/")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if !strings.Contains(string(out), "db/vbrain.sqlite3") {
		t.Error("SQLite index should be versioned")
	}
}

func TestHasRemoteFalseWhenNoOrigin(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatal(err)
	}
	if git.HasRemote(dir, "origin") {
		t.Fatal("should not have a remote")
	}
}

func TestPushNoOpWhenNoRemote(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o644)
	git.Commit("x", dir)
	res, err := git.Push(dir, "origin", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed || res.Reason != "no remote" {
		t.Fatalf("res = %+v", res)
	}
}

func TestGitignoreIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := git.WriteGitignore(dir); err != nil {
		t.Fatal(err)
	}
	fi1, _ := os.Stat(filepath.Join(dir, ".gitignore"))
	if _, err := git.WriteGitignore(dir); err != nil {
		t.Fatal(err)
	}
	fi2, _ := os.Stat(filepath.Join(dir, ".gitignore"))
	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Error("should not rewrite an existing .gitignore")
	}
}

// run is a tiny helper for arranging git state in tests.
func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

// A push that's rejected non-fast-forward must be recoverable: PullRebase
// integrates the remote's commit so the retried push fast-forwards. This is the
// case `vbrain install` hits when the base moved on another machine / the cloud.
func TestPullRebaseLetsAStaleClonePush(t *testing.T) {
	// Isolate git identity: useConfigOnly + no configured name/email reproduces
	// CI / the cloud, where the rebase fails with "Committer identity unknown"
	// unless PullRebase injects the vbrain fallback. Without this, the test would
	// pass on any dev machine that happens to have a global identity.
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tuseConfigOnly = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	remote := t.TempDir()
	run(t, remote, "init", "--bare", "-b", "main", remote)

	// clone A seeds the remote
	a := t.TempDir()
	run(t, a, "clone", remote, a)
	os.WriteFile(filepath.Join(a, "x"), []byte("1"), 0o644)
	run(t, a, "add", "x")
	run(t, a, "commit", "-m", "x")
	run(t, a, "push", "origin", "main")

	// clone B starts level with the remote
	b := t.TempDir()
	run(t, b, "clone", remote, b)

	// A advances the remote
	os.WriteFile(filepath.Join(a, "y"), []byte("2"), 0o644)
	run(t, a, "add", "y")
	run(t, a, "commit", "-m", "y")
	run(t, a, "push", "origin", "main")

	// B commits on the old tip → its push is rejected (non-fast-forward)
	os.WriteFile(filepath.Join(b, "z"), []byte("3"), 0o644)
	run(t, b, "add", "z")
	run(t, b, "commit", "-m", "z")
	if _, err := git.Push(b, "origin", "main"); err == nil {
		t.Fatal("expected the stale push to be rejected")
	}

	// PullRebase + retry must succeed
	if err := git.PullRebase(b, "origin", "main"); err != nil {
		t.Fatalf("pull --rebase: %v", err)
	}
	if _, err := git.Push(b, "origin", "main"); err != nil {
		t.Fatalf("push after rebase: %v", err)
	}
	// the remote now carries z on top of y
	if log := gitLog(t, b); !strings.Contains(log, "z") || !strings.Contains(log, "y") {
		t.Fatalf("expected y and z in history, got:\n%s", log)
	}
}
