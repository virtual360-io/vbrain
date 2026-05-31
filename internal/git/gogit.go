package git

import (
	"errors"
	"os"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// gogitBackend implements the write operations with pure-Go go-git (fallback
// when there's no system git). Push uses GITHUB_TOKEN (go-git doesn't read the
// system's credential helpers).
type gogitBackend struct{}

func (gogitBackend) Init(dir string) error {
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

func (gogitBackend) Commit(message, dir string) (CommitResult, error) {
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

func (gogitBackend) Push(dir, name, branch string) (PushResult, error) {
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

func (gogitBackend) AddRemote(url, dir, name string) error {
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		return err
	}
	_, err = repo.CreateRemote(&config.RemoteConfig{Name: name, URLs: []string{url}})
	return err
}

// authorSignature uses the user's global git identity, falling back to a vbrain
// identity when not configured (go-git requires an author on commit).
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
