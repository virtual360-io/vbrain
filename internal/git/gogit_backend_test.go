package git

import (
	"os"
	"path/filepath"
	"testing"
)

// forceGoGit força o seletor a usar o backend go-git, restaurando ao fim.
func forceGoGit(t *testing.T) {
	t.Helper()
	orig := systemGitAvailable
	systemGitAvailable = func() bool { return false }
	t.Cleanup(func() { systemGitAvailable = orig })
}

func TestGoGitBackendInitCommitFlow(t *testing.T) {
	forceGoGit(t)
	if BackendName() != "gogit" {
		t.Fatalf("backend = %q, want gogit", BackendName())
	}

	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if !RepoInitialized(dir) || CurrentBranch(dir) != "main" {
		t.Fatalf("init falhou: initialized=%v branch=%q", RepoInitialized(dir), CurrentBranch(dir))
	}

	// no-op quando limpo
	res, err := Commit("noop", dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Committed || res.Reason != "no changes" {
		t.Fatalf("res = %+v", res)
	}

	// commita arquivo novo
	os.WriteFile(filepath.Join(dir, "p.md"), []byte("# Hi\n"), 0o644)
	res, err = Commit("add: p", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Committed || res.SHA == "" {
		t.Fatalf("res = %+v", res)
	}

	// push sem remote → no-op
	pr, err := Push(dir, "origin", "")
	if err != nil {
		t.Fatal(err)
	}
	if pr.Pushed || pr.Reason != "no remote" {
		t.Fatalf("push = %+v", pr)
	}
}
