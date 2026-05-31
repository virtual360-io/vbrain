package git

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// systemBackend implements the write operations via the system git, leveraging
// the user's already-configured credentials on push. vbrain does not impose
// signing on its own commits (commit.gpgsign=false) so it works on any machine
// without signing configured.
type systemBackend struct{}

func (systemBackend) Init(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if RepoInitialized(dir) {
		return errors.New("repo already initialized at " + dir)
	}
	if _, err := sysRun(dir, "git", "init", "-b", "main"); err != nil {
		return err
	}
	if _, err := WriteGitignore(dir); err != nil {
		return err
	}
	if _, err := sysRun(dir, "git", "add", ".gitignore"); err != nil {
		return err
	}
	_, err := sysRun(dir, commitArgs(dir, "chore: initialize vbrain")...)
	return err
}

func (systemBackend) Commit(message, dir string) (CommitResult, error) {
	if _, err := sysRun(dir, "git", "add", "-A"); err != nil {
		return CommitResult{}, err
	}
	if !changesStaged(dir) {
		return CommitResult{Committed: false, Reason: "no changes"}, nil
	}
	if _, err := sysRun(dir, commitArgs(dir, message)...); err != nil {
		return CommitResult{}, err
	}
	sha, _ := sysStatus(dir, "git", "rev-parse", "HEAD")
	return CommitResult{Committed: true, SHA: strings.TrimSpace(sha), Message: message}, nil
}

func (systemBackend) Push(dir, name, branch string) (PushResult, error) {
	if !HasRemote(dir, name) {
		return PushResult{Pushed: false, Reason: "no remote"}, nil
	}
	if branch == "" {
		branch = CurrentBranch(dir)
	}
	if _, err := sysRun(dir, "git", "push", "-u", name, branch); err != nil {
		return PushResult{}, err
	}
	return PushResult{Pushed: true, Remote: name, Branch: branch}, nil
}

func (systemBackend) AddRemote(url, dir, name string) error {
	_, err := sysRun(dir, "git", "remote", "add", name, url)
	return err
}

// commitArgs builds `git [-c …] commit -m msg`: it never imposes signing
// (commit.gpgsign=false) and, if the user has no identity configured, uses a
// vbrain fallback author (mirrors the go-git backend) — without overwriting the
// real one.
func commitArgs(dir, message string) []string {
	args := []string{"git", "-c", "commit.gpgsign=false"}
	if !configSet(dir, "user.name") || !configSet(dir, "user.email") {
		args = append(args, "-c", "user.name=vbrain", "-c", "user.email=vbrain@localhost")
	}
	return append(args, "commit", "-m", message)
}

func configSet(dir, key string) bool {
	out, ok := sysStatus(dir, "git", "config", key)
	return ok && strings.TrimSpace(out) != ""
}

// changesStaged: git diff --cached --quiet exits != 0 when something is staged.
func changesStaged(dir string) bool {
	_, ok := sysStatus(dir, "git", "diff", "--cached", "--quiet")
	return !ok
}

func sysRun(dir string, args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", errors.New(strings.Join(args, " ") + " failed: " +
			strings.TrimSpace(errBuf.String()) + "\n" + strings.TrimSpace(out.String()))
	}
	return out.String(), nil
}

func sysStatus(dir string, args ...string) (string, bool) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	ok := cmd.Run() == nil // run before reading the buffer
	return out.String(), ok
}
