// Package git embrulha as operações git do vbrain (init, commit, push) com dois
// backends selecionados em runtime: o git do sistema quando presente (usa as
// credenciais já configuradas do usuário no push), senão go-git puro-Go (sem
// dependência externa; push usa um PAT coletado no install).
//
// Operações de leitura (branch, remote, changes) usam um único caminho go-git —
// elas leem um repositório git padrão independentemente de quem o criou. Porta
// de lib/vbrain/git.rb. O índice SQLite é versionado de propósito: /db/ NÃO
// entra no .gitignore.
package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
)

// Gitignore: só staging volátil e lixo de SO. /db/ fica versionado.
const Gitignore = "/raw/.tmp/\n.DS_Store\n"

// CommitResult espelha o hash retornado por Git.commit! no Ruby.
type CommitResult struct {
	Committed bool   `json:"committed"`
	Reason    string `json:"reason,omitempty"`
	SHA       string `json:"sha,omitempty"`
	Message   string `json:"message,omitempty"`
}

// PushResult espelha o hash retornado por Git.push!.
type PushResult struct {
	Pushed bool   `json:"pushed"`
	Reason string `json:"reason,omitempty"`
	Remote string `json:"remote,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// backend abstrai as operações que mutam o repo e dependem de credenciais.
type backend interface {
	Init(dir string) error
	Commit(message, dir string) (CommitResult, error)
	Push(dir, name, branch string) (PushResult, error)
	AddRemote(url, dir, name string) error
}

// systemGitAvailable detecta o git no PATH; é uma var para os testes poderem
// forçar o backend go-git.
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

// BackendName devolve "system" ou "gogit" — útil pro install reportar e decidir
// se precisa coletar PAT.
func BackendName() string {
	if systemGitAvailable() {
		return "system"
	}
	return "gogit"
}

// Init cria o repo (branch main), escreve o .gitignore e faz o commit inicial.
func Init(dir string) error { return selected().Init(dir) }

// Commit faz o stage de tudo e commita; no-op se nada mudou.
func Commit(message, dir string) (CommitResult, error) { return selected().Commit(message, dir) }

// Push faz push da branch; no-op se não houver remote.
func Push(dir, name, branch string) (PushResult, error) { return selected().Push(dir, name, branch) }

// AddRemote adiciona um remote nomeado.
func AddRemote(url, dir, name string) error { return selected().AddRemote(url, dir, name) }

// --- operações de leitura (backend-agnósticas, via go-git) ---

// RepoInitialized indica se dir já tem um repositório git.
func RepoInitialized(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && fi.IsDir()
}

// WriteGitignore escreve o .gitignore se ainda não contiver a marca; idempotente.
func WriteGitignore(dir string) (string, error) {
	path := filepath.Join(dir, ".gitignore")
	if b, err := os.ReadFile(path); err == nil && strings.Contains(string(b), "/raw/.tmp/") {
		return path, nil
	}
	return path, os.WriteFile(path, []byte(Gitignore), 0o644)
}

// CurrentBranch devolve a branch atual, ou "" se indeterminada.
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

// HasRemote indica se o remote nomeado existe com URL.
func HasRemote(dir, name string) bool {
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		return false
	}
	r, err := repo.Remote(name)
	return err == nil && len(r.Config().URLs) > 0
}

// Changes indica se há mudanças no working tree.
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
