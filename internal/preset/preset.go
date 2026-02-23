package preset

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/afero"
)

// PromptFileSelection asks user to select files from a list. Returns selected filenames.
// This is injectable so tests can provide a mock instead of huh.
type PromptFileSelection func(files []string) ([]string, error)

// PromptOverwrite asks user whether to overwrite an existing file. Returns true to overwrite.
type PromptOverwrite func(filename string) (bool, error)

// RunPresetFlow orchestrates the full preset download flow:
// parse URL, ensure repo cache, list files, prompt for selection, download with source comments.
func RunPresetFlow(ctx context.Context, env *PresetEnv, cacheDir string, rawURL string, destDir string, selectFiles PromptFileSelection, confirmOverwrite PromptOverwrite, w io.Writer) error {
	url, err := ParsePresetURL(rawURL)
	if err != nil {
		return err
	}

	if url.HasCredentials() {
		_, _ = fmt.Fprintln(w, "Warning: The URL contains credentials that will be stored in downloaded files.")
		_, _ = fmt.Fprintln(w, "These files may be committed to version control. Consider using SSH keys or")
		_, _ = fmt.Fprintln(w, "git credential helpers instead.")
	}

	cm := NewCacheManager(env, cacheDir)

	repoDir, resolvedCommit, err := cm.EnsureRepo(ctx, url.CloneURL, url.CachePath(""), url.CommitHash)
	if err != nil {
		return err
	}

	files, err := cm.ListFiles(ctx, repoDir, resolvedCommit, url.DirPath)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		dirDesc := "repository root"
		if url.DirPath != "" {
			dirDesc = fmt.Sprintf("directory %q", url.DirPath)
		}
		return fmt.Errorf("no preset files (.alca.*.toml) found in %s: %w", dirDesc, ErrNoPresetFiles)
	}

	selected, err := selectFiles(files)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return nil
	}

	for _, filename := range selected {
		repoFilePath := filename
		if url.DirPath != "" {
			repoFilePath = url.DirPath + "/" + filename
		}

		localPath := filepath.Join(destDir, filename)

		exists, err := afero.Exists(env.Fs, localPath)
		if err != nil {
			return fmt.Errorf("checking file %s: %w", filename, err)
		}
		if exists {
			overwrite, err := confirmOverwrite(filename)
			if err != nil {
				return err
			}
			if !overwrite {
				continue
			}
		}

		content, err := cm.CheckoutFile(ctx, repoDir, resolvedCommit, repoFilePath)
		if err != nil {
			return err
		}

		if err := WriteSourceComment(env.Fs, localPath, content, url.SourceBase(), resolvedCommit, repoFilePath); err != nil {
			return fmt.Errorf("writing %s: %w", filename, err)
		}

		_, _ = fmt.Fprintf(w, "Downloaded %s\n", filename)
	}

	return nil
}
