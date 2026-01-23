package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all Alcatraz containers",
	Long:  `List all containers managed by Alcatraz across all projects.`,
	RunE:  runList,
}

// runList displays all alca-managed containers.
func runList(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Try to load config for runtime selection, use default if not found
	configPath := filepath.Join(cwd, ConfigFilename)
	cfg, _ := config.LoadConfig(configPath)

	// Select runtime
	rt, err := runtime.SelectRuntime(&cfg)
	if err != nil {
		return fmt.Errorf("failed to select runtime: %w", err)
	}

	ctx := context.Background()
	containers, err := rt.ListContainers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Println("No Alcatraz containers found.")
		return nil
	}

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tPROJECT ID\tPROJECT PATH\tCREATED")

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

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			c.Name, status, projectID, projectPath, createdAt)
	}

	w.Flush()
	return nil
}
