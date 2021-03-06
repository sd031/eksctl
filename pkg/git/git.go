package git

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/kris-nova/logger"
	"github.com/weaveworks/eksctl/pkg/git/executor"
)

// Cloner can clone git repositories
type Cloner interface {
	CloneRepo(cloneDirPrefix string, branch string, gitURL string) (string, error)
}

// Client can perform git operations on the given directory
type Client struct {
	executor executor.Executor
	dir      string
}

// NewGitClient returns a client that can perform git operations
func NewGitClient(ctx context.Context, timeout time.Duration) *Client {
	return &Client{
		executor: executor.NewShellExecutor(ctx, timeout),
	}
}

// NewGitClientFromExecutor returns a client that can have an executor injected. Useful for testing
func NewGitClientFromExecutor(executor executor.Executor) *Client {
	return &Client{
		executor: executor,
	}
}

// CloneRepo clones a repo specified in the gitURL and checks out the specified branch
func (git *Client) CloneRepo(cloneDirPrefix string, branch string, gitURL string) (string, error) {
	cloneDir, err := ioutil.TempDir(os.TempDir(), cloneDirPrefix)
	if err != nil {
		return "", fmt.Errorf("cannot create temporary directory: %s", err)
	}

	return cloneDir, git.CloneRepoInPath(cloneDir, branch, gitURL)
}

// CloneRepoInPath clones a repo to the specified directory
func (git *Client) CloneRepoInPath(clonePath string, branch string, gitURL string) error {
	git.dir = clonePath
	args := []string{"clone", "-b", branch, gitURL, git.dir}
	return git.runGitCmd(args...)
}

// Add performs can perform a `git add` operation on the given file paths
func (git Client) Add(files ...string) error {
	args := append([]string{"add", "--"}, files...)
	if err := git.runGitCmd(args...); err != nil {
		return err
	}
	return nil
}

// Commit  makes a commit if there are staged changes
func (git Client) Commit(message, user, email string) error {
	// Note, this useed to do runGitCmd(diffCtx, git.dir, "diff", "--cached", "--quiet", "--", fi.opts.gitFluxPath); err == nil {
	if err := git.runGitCmd("diff", "--cached", "--quiet"); err == nil {
		logger.Info("Nothing to commit (the repository contained identical files), moving on")
		return nil
	} else if _, ok := err.(*exec.ExitError); !ok {
		return err
	}

	// Commit
	args := []string{"commit",
		"-m", message,
		fmt.Sprintf("--author=%s <%s>", user, email),
	}
	if err := git.runGitCmd(args...); err != nil {
		return err
	}
	return nil
}

// Push pushes the changes to the origin remote
func (git Client) Push() error {
	return git.runGitCmd("push")
}

// DeleteLocalRepo deletes the local copy of a repository, including the directory
func (git Client) DeleteLocalRepo() error {
	if git.dir != "" {
		return os.RemoveAll(git.dir)
	}
	return fmt.Errorf("no cloned directory to delete")
}

func (git Client) runGitCmd(args ...string) error {
	return git.executor.Exec("git", git.dir, args...)
}
