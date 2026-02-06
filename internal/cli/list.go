package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all Alcatraz containers",
	Long:  `List all containers managed by Alcatraz across all projects.`,
	RunE:  runList,
}

// runList displays all alca-managed containers.
func runList(cmd *cobra.Command, args []string) error {
	cwd, err := getCwd()
	if err != nil {
		return err
	}

	// Create shared dependencies once
	cmdRunner := util.NewCommandRunner()
	env := &util.Env{Fs: afero.NewReadOnlyFs(afero.NewOsFs()), Cmd: cmdRunner}
	runtimeEnv := runtime.NewRuntimeEnv(cmdRunner)

	// Load config (optional) and select runtime
	// Log warning if config has issues but continue
	cfg, rt, err := loadConfigAndRuntimeOptional(env, runtimeEnv, cwd)
	if err != nil {
		return err
	}
	_ = cfg // Config loaded for runtime selection only

	containers, err := rt.ListContainers(runtimeEnv)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Println("No Alcatraz containers found.")
		return nil
	}

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSTATUS\tPROJECT ID\tPROJECT PATH\tCREATED")

	for _, c := range containers {
		status := string(c.State)
		projectPath := c.ProjectPath
		if projectPath == "" {
			projectPath = "(unknown)"
		}
		projectID := c.ProjectID
		if len(projectID) > 12 {
			projectID = projectID[:12]
		}
		createdAt := c.CreatedAt
		if len(createdAt) > 19 {
			createdAt = createdAt[:19]
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			c.Name, status, projectID, projectPath, createdAt)
	}

	_ = w.Flush()
	return nil
}
