package preset

import (
	"context"
	"strings"

	"github.com/bolasblack/alcatraz/internal/util"
)

// gitOps provides thin wrappers around git commands for readability.
type gitOps struct {
	cmd util.CommandRunner
}

func (g *gitOps) InitBare(ctx context.Context, dir string) error {
	_, err := g.cmd.RunQuiet(ctx, "git", "init", "--bare", dir)
	return err
}

func (g *gitOps) ShallowFetch(ctx context.Context, dir, url, ref string) error {
	_, err := g.cmd.RunQuiet(ctx, "git", "-C", dir, "fetch", "--depth", "1", url, ref)
	return err
}

func (g *gitOps) RevParse(ctx context.Context, dir, ref string) (string, error) {
	out, err := g.cmd.RunQuiet(ctx, "git", "-C", dir, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *gitOps) ObjectType(ctx context.Context, dir, hash string) (string, error) {
	out, err := g.cmd.RunQuiet(ctx, "git", "-C", dir, "cat-file", "-t", hash)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *gitOps) ShowFile(ctx context.Context, dir, commit, path string) ([]byte, error) {
	return g.cmd.RunQuiet(ctx, "git", "-C", dir, "show", commit+":"+path)
}

func (g *gitOps) ListTree(ctx context.Context, dir, ref string) ([]byte, error) {
	return g.cmd.RunQuiet(ctx, "git", "-C", dir, "ls-tree", "--name-only", ref)
}
