package git

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// systemBackend implementa as operações de escrita via o git do sistema,
// aproveitando as credenciais já configuradas do usuário no push. vbrain não
// impõe assinatura nos próprios commits (commit.gpgsign=false) para funcionar em
// qualquer máquina sem signing configurado.
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
	_, err := sysRun(dir, "git", "-c", "commit.gpgsign=false", "commit", "-m", "chore: initialize vbrain")
	return err
}

func (systemBackend) Commit(message, dir string) (CommitResult, error) {
	if _, err := sysRun(dir, "git", "add", "-A"); err != nil {
		return CommitResult{}, err
	}
	if !changesStaged(dir) {
		return CommitResult{Committed: false, Reason: "no changes"}, nil
	}
	if _, err := sysRun(dir, "git", "-c", "commit.gpgsign=false", "commit", "-m", message); err != nil {
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

// changesStaged: git diff --cached --quiet sai != 0 quando há algo staged.
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
	ok := cmd.Run() == nil // rodar antes de ler o buffer
	return out.String(), ok
}
