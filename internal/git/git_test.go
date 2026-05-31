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
		t.Fatal("dir vazio não deveria ser repo")
	}
}

func TestInitCreatesRepoWithGitignoreAndInitialCommit(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatal(err)
	}
	if !git.RepoInitialized(dir) {
		t.Fatal("deveria estar inicializado")
	}
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gi), "/db/") {
		t.Error("/db/ NÃO deve ser ignorado (índice versionado)")
	}
	if !strings.Contains(string(gi), "/raw/.tmp/") {
		t.Error(".gitignore deveria conter /raw/.tmp/")
	}
	if git.CurrentBranch(dir) != "main" {
		t.Errorf("branch = %q, want main", git.CurrentBranch(dir))
	}
	if !strings.Contains(gitLog(t, dir), "initialize vbrain") {
		t.Error("commit inicial ausente")
	}
}

func TestInitRaisesIfAlreadyInitialized(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := git.Init(dir); err == nil {
		t.Fatal("segundo init deveria falhar")
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
		t.Error("commit não apareceu no log")
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
		t.Fatal("deveria commitar")
	}
	cmd := exec.Command("git", "ls-files", "db/")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if !strings.Contains(string(out), "db/vbrain.sqlite3") {
		t.Error("índice SQLite deveria ser versionado")
	}
}

func TestHasRemoteFalseWhenNoOrigin(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatal(err)
	}
	if git.HasRemote(dir, "origin") {
		t.Fatal("não deveria ter remote")
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
		t.Error("não deveria reescrever .gitignore existente")
	}
}
