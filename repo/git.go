package repo

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

func CloneOrPull(remote string, local string, user string, password string) {
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
				panic(err)
			}
		}
	}

	worktree, err := repo.Worktree()
	if err != nil {
		panic(fmt.Sprintf("Worktree: %v", err))
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
		panic(fmt.Sprintf("Pull: %v\n", err))
	}

	ref, err := repo.Head()
	if err != nil {
		panic(fmt.Sprintf("Head: %v\n", err))
	}
	log, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		panic(fmt.Sprintf("Log: %v\n", err))
	}
	err = log.ForEach(func(c *object.Commit) error {
		fmt.Println(c)
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("%v", err))
	}
}
