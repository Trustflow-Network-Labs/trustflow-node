package repo

import (
	"os"
	"os/exec"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

type GitManager struct {
}

func NewGitManager() *GitManager {
	return &GitManager{}
}

// ValidateRepo checks if a Git repo exists by running `git ls-remote`.
// Supports HTTPS and SSH. Logs errors with reason.
func (gm *GitManager) ValidateRepo(repoURL string) ([]byte, error) {
	cmd := exec.Command("git", "ls-remote", repoURL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, err
	}

	return output, nil
}

// Clone or pull git repo
func (gm *GitManager) CloneOrPull(remote string, local string, user string, password string) error {
	repo, err := git.PlainClone(local, false, &git.CloneOptions{
		URL:      remote,
		Progress: os.Stdout,
		Auth: &http.BasicAuth{
			Username: user,
			Password: password,
		},
	})
	if err != nil {
		if err == git.ErrRepositoryAlreadyExists {
			repo, err = git.PlainOpen(local)
			if err != nil {
				return err
			}
		}
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}
	err = worktree.Pull(&git.PullOptions{
		RemoteName: "origin",
		RemoteURL:  remote,
		Auth: &http.BasicAuth{
			Username: user,
			Password: password,
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}

	return nil
}

// List git commits
func (gm *GitManager) ListCommits(repo *git.Repository) ([]*object.Commit, error) {
	var commits []*object.Commit

	ref, err := repo.Head()
	if err != nil {
		return nil, err
	}
	log, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, err
	}
	err = log.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return commits, nil
}
