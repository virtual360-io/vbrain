// Package git embrulha as operações git do vbrain (init, commit, push) usando
// go-git puro-Go — sem dependência do binário git do sistema. Porta de
// lib/vbrain/git.rb. O índice SQLite é versionado de propósito: /db/ NÃO entra
// no .gitignore.
package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
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

// RepoInitialized indica se dir já tem um repositório git.
func RepoInitialized(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && fi.IsDir()
}

// Init cria o repo (branch main), escreve o .gitignore e faz o commit inicial.
func Init(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if RepoInitialized(dir) {
		return errors.New("repo already initialized at " + dir)
	}
	repo, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{
		InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		return err
	}
	if _, err := WriteGitignore(dir); err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if _, err := wt.Add(".gitignore"); err != nil {
		return err
	}
	_, err = wt.Commit("chore: initialize vbrain", &gogit.CommitOptions{Author: authorSignature()})
	return err
}

// WriteGitignore escreve o .gitignore se ainda não contiver a marca; é
// idempotente (não reescreve um já existente).
func WriteGitignore(dir string) (string, error) {
	path := filepath.Join(dir, ".gitignore")
	if b, err := os.ReadFile(path); err == nil && strings.Contains(string(b), "/raw/.tmp/") {
		return path, nil
	}
	return path, os.WriteFile(path, []byte(Gitignore), 0o644)
}

// AddRemote adiciona um remote nomeado.
func AddRemote(url, dir, name string) error {
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		return err
	}
	_, err = repo.CreateRemote(&config.RemoteConfig{Name: name, URLs: []string{url}})
	return err
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

// Commit faz o stage de tudo (git add -A) e commita; no-op se nada mudou.
func Commit(message, dir string) (CommitResult, error) {
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		return CommitResult{}, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return CommitResult{}, err
	}
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return CommitResult{}, err
	}
	st, err := wt.Status()
	if err != nil {
		return CommitResult{}, err
	}
	if st.IsClean() {
		return CommitResult{Committed: false, Reason: "no changes"}, nil
	}
	hash, err := wt.Commit(message, &gogit.CommitOptions{Author: authorSignature()})
	if err != nil {
		return CommitResult{}, err
	}
	return CommitResult{Committed: true, SHA: hash.String(), Message: message}, nil
}

// Push faz push da branch; no-op se não houver remote. Usa GITHUB_TOKEN se
// presente (go-git não lê credential helpers do sistema).
func Push(dir, name, branch string) (PushResult, error) {
	if !HasRemote(dir, name) {
		return PushResult{Pushed: false, Reason: "no remote"}, nil
	}
	if branch == "" {
		branch = CurrentBranch(dir)
	}
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		return PushResult{}, err
	}
	opts := &gogit.PushOptions{RemoteName: name}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		opts.Auth = &http.BasicAuth{Username: "x-access-token", Password: token}
	}
	if err := repo.Push(opts); err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return PushResult{}, err
	}
	return PushResult{Pushed: true, Remote: name, Branch: branch}, nil
}

// authorSignature usa a identidade git global do usuário, com fallback para uma
// identidade vbrain quando não configurada (go-git exige author no commit).
func authorSignature() *object.Signature {
	name, email := "vbrain", "vbrain@localhost"
	if cfg, err := config.LoadConfig(config.GlobalScope); err == nil {
		if cfg.User.Name != "" {
			name = cfg.User.Name
		}
		if cfg.User.Email != "" {
			email = cfg.User.Email
		}
	}
	return &object.Signature{Name: name, Email: email, When: time.Now()}
}
