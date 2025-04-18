package repo

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/adgsm/trustflow-node/utils"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
)

type GitManager struct {
}

func NewGitManager() *GitManager {
	return &GitManager{}
}

// Get appropriate auth basing on provided parameters
func (gm *GitManager) getAuth(repoURL, username, password string) (transport.AuthMethod, error) {
	// No auth for public repo
	if username == "" && password == "" && strings.HasPrefix(repoURL, "http") {
		return nil, nil
	}

	// HTTPS basic auth (token or user/pass)
	if strings.HasPrefix(repoURL, "http") {
		return &githttp.BasicAuth{
			Username: username, // Can be anything for token-based auth
			Password: password,
		}, nil
	}

	// SSH auth (assumes key in ~/.ssh/id_rsa)
	if strings.HasPrefix(repoURL, "git@") || strings.HasPrefix(repoURL, "ssh://") {

		usr, err := user.Current()
		if err != nil {
			return nil, err
		}
		sshPath := filepath.Join(usr.HomeDir, ".ssh", "id_rsa")

		publicKeys, err := gitssh.NewPublicKeysFromFile("git", sshPath, "")
		if err != nil {
			return nil, err
		}
		publicKeys.HostKeyCallback = ssh.InsecureIgnoreHostKey()
		return publicKeys, nil
	}

	return nil, errors.New("unsupported repo auth method")
}

// ValidateRepo checks if the Git repo is reachable by attempting a shallow clone without checkout.
func (gm *GitManager) ValidateRepo(repoUrl, username, password string) error {
	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		return err
	}
	tmpRoot := configs["local_tmp"]

	// Make sure tmpRoot exists
	if err := os.MkdirAll(tmpRoot, 0755); err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp(tmpRoot, "gitcheck-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	auth, err := gm.getAuth(repoUrl, username, password)
	if err != nil {
		return err
	}

	_, err = git.PlainClone(tmpDir, false, &git.CloneOptions{
		URL:        repoUrl,
		Depth:      1,
		NoCheckout: true,
		Auth:       auth,
	})

	if err != nil {
		return err
	}

	return nil
}

// DockerFileCheckResult holds the results of scanning for docker-related files.
type DockerFileCheckResult struct {
	HasDockerfile bool
	HasCompose    bool
	Dockerfiles   []string
	Composes      []string
}

// Check Docker files in git repo
func (gm *GitManager) CheckDockerFiles(path string) (DockerFileCheckResult, error) {
	result := DockerFileCheckResult{}

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		switch strings.ToLower(info.Name()) {
		case "Dockerfile", "dockerfile":
			result.HasDockerfile = true
			result.Dockerfiles = append(result.Dockerfiles, filePath)
		case "docker-compose.yml", "docker-compose.yaml":
			result.HasCompose = true
			result.Composes = append(result.Composes, filePath)
		}
		return nil
	})

	return result, err
}

// getRepoCachePath generates a unique folder name based on repo URL and branch.
func (gm *GitManager) getRepoCachePath(basePath, repoUrl, branch string) string {
	key := repoUrl
	if branch != "" {
		key = key + ":" + branch
	}
	hash := sha1.Sum([]byte(key))
	hashStr := hex.EncodeToString(hash[:])[:12] // 12 chars is short but quite unique

	// Extract repo name from URL for readability
	name := strings.TrimSuffix(filepath.Base(repoUrl), ".git")
	if name == "" {
		name = "repo"
	}

	return filepath.Join(basePath, name+"-"+hashStr)
}

// Clone or pull git repo
func (gm *GitManager) CloneOrPull(basePath, repoUrl, branch, username, password string) (string, error) {
	auth, err := gm.getAuth(repoUrl, username, password)
	if err != nil {
		return "", err
	}

	cloneOptions := &git.CloneOptions{
		URL:      repoUrl,
		Progress: os.Stdout,
		Auth:     auth,
	}
	if branch != "" {
		cloneOptions.ReferenceName = plumbing.NewBranchReferenceName(branch)
		cloneOptions.SingleBranch = true
	}

	// Generate unique and redable folder name for git contents
	path := gm.getRepoCachePath(basePath, repoUrl, branch)

	// Make sure path exists
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}

	repo, err := git.PlainClone(path, false, cloneOptions)
	if err != nil {
		if err == git.ErrRepositoryAlreadyExists {
			repo, err = git.PlainOpen(path)
			if err != nil {
				return "", err
			}
		}
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	err = worktree.Pull(&git.PullOptions{
		RemoteName: "origin",
		RemoteURL:  repoUrl,
		Auth:       auth,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return "", err
	}

	return path, nil
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
