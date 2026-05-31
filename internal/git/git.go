// Package git wraps vbrain's git operations (init, commit, push) with two
// backends selected at runtime: the system git when present (uses the user's
// already-configured credentials on push), otherwise pure-Go go-git (no external
// dependency; push uses a PAT collected at install).
//
// Read operations (branch, remote, changes) use a single go-git path — they read
// a standard git repository regardless of who created it. Port of
// lib/vbrain/git.rb. The SQLite index is versioned on purpose: /db/ does NOT go
// into .gitignore.
package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
)

// Gitignore: only volatile staging and OS junk. /db/ stays versioned.
const Gitignore = "/raw/.tmp/\n.DS_Store\n"

// CommitResult mirrors the hash returned by Git.commit! in Ruby.
type CommitResult struct {
	Committed bool   `json:"committed"`
	Reason    string `json:"reason,omitempty"`
	SHA       string `json:"sha,omitempty"`
	Message   string `json:"message,omitempty"`
}

// PushResult mirrors the hash returned by Git.push!.
type PushResult struct {
	Pushed bool   `json:"pushed"`
	Reason string `json:"reason,omitempty"`
	Remote string `json:"remote,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// backend abstracts the operations that mutate the repo and depend on credentials.
type backend interface {
	Init(dir string) error
	Commit(message, dir string) (CommitResult, error)
	Push(dir, name, branch string) (PushResult, error)
	AddRemote(url, dir, name string) error
}

// systemGitAvailable detects git on the PATH; it's a var so tests can force the
// go-git backend.
var systemGitAvailable = func() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func selected() backend {
	if systemGitAvailable() {
		return systemBackend{}
	}
	return gogitBackend{}
}

// BackendName returns "system" or "gogit" — useful for install to report and
// decide whether it needs to collect a PAT.
func BackendName() string {
	if systemGitAvailable() {
		return "system"
	}
	return "gogit"
}

// Init creates the repo (branch main), writes the .gitignore, and makes the
// initial commit.
func Init(dir string) error { return selected().Init(dir) }

// Commit stages everything and commits; no-op if nothing changed.
func Commit(message, dir string) (CommitResult, error) { return selected().Commit(message, dir) }

// Push pushes the branch; no-op if there's no remote.
func Push(dir, name, branch string) (PushResult, error) { return selected().Push(dir, name, branch) }

// AddRemote adds a named remote.
func AddRemote(url, dir, name string) error { return selected().AddRemote(url, dir, name) }

// --- read operations (backend-agnostic, via go-git) ---

// RepoInitialized reports whether dir already has a git repository.
func RepoInitialized(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && fi.IsDir()
}

// WriteGitignore writes the .gitignore if it doesn't already contain the marker;
// idempotent.
func WriteGitignore(dir string) (string, error) {
	path := filepath.Join(dir, ".gitignore")
	if b, err := os.ReadFile(path); err == nil && strings.Contains(string(b), "/raw/.tmp/") {
		return path, nil
	}
	return path, os.WriteFile(path, []byte(Gitignore), 0o644)
}

// CurrentBranch returns the current branch, or "" if undetermined.
func CurrentBranch(dir string) string {
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		return ""
	}
	head, err := repo.Head()
	if err != nil {
		return ""
	}
	return head.Name().Short()
}

// HasRemote reports whether the named remote exists with a URL.
func HasRemote(dir, name string) bool {
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		return false
	}
	r, err := repo.Remote(name)
	return err == nil && len(r.Config().URLs) > 0
}

// Changes reports whether there are changes in the working tree.
func Changes(dir string) bool {
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		return false
	}
	wt, err := repo.Worktree()
	if err != nil {
		return false
	}
	st, err := wt.Status()
	if err != nil {
		return false
	}
	return !st.IsClean()
}
