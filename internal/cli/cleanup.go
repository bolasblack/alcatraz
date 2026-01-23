package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/spf13/cobra"
)

var cleanupAll bool

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove orphaned Alcatraz containers",
	Long: `Find and remove orphaned Alcatraz containers.

An orphan container is one whose project directory no longer exists,
or whose state file (.alca/state.json) has been deleted.`,
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVar(&cleanupAll, "all", false, "Delete all orphan containers without prompting")
}

// runCleanup finds and removes orphan containers.
func runCleanup(cmd *cobra.Command, args []string) error {
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

	// Find orphan containers
	var orphans []runtime.ContainerInfo
	for _, c := range containers {
		if isOrphan(c) {
			orphans = append(orphans, c)
		}
	}

	if len(orphans) == 0 {
		// Silent exit when no orphans
		return nil
	}

	var toDelete []runtime.ContainerInfo

	if cleanupAll {
		// --all flag: skip interaction
		toDelete = orphans
	} else {
		// Display orphan containers
		fmt.Printf("Found %d orphan container(s):\n\n", len(orphans))
		for i, c := range orphans {
			reason := getOrphanReason(c)
			projectPath := c.ProjectPath
			if projectPath == "" {
				projectPath = "(no path)"
			}
			fmt.Printf("  [%d] %s\n", i+1, c.Name)
			fmt.Printf("      Path: %s\n", projectPath)
			fmt.Printf("      Reason: %s\n", reason)
			fmt.Println()
		}

		// Interactive selection
		fmt.Println("Select containers to delete (comma-separated numbers, or Enter for all):")
		fmt.Print("> ")

		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			// Ctrl+C or EOF
			fmt.Println("\nCancelled.")
			return nil
		}

		input = strings.TrimSpace(input)
		if input == "" {
			// Empty input = delete all
			toDelete = orphans
		} else {
			// Parse numbers
			parts := strings.Split(input, ",")
			seen := make(map[int]bool)
			for _, part := range parts {
				part = strings.TrimSpace(part)
				num, err := strconv.Atoi(part)
				if err != nil {
					fmt.Printf("Invalid number: %s\n", part)
					continue
				}
				if num < 1 || num > len(orphans) {
					fmt.Printf("Number out of range: %d\n", num)
					continue
				}
				if !seen[num] {
					seen[num] = true
					toDelete = append(toDelete, orphans[num-1])
				}
			}
		}

		if len(toDelete) == 0 {
			fmt.Println("No containers selected.")
			return nil
		}
	}

	// Delete containers
	deleted := 0
	for _, c := range toDelete {
		fmt.Printf("Removing %s... ", c.Name)
		if err := rt.RemoveContainer(ctx, c.Name); err != nil {
			fmt.Printf("failed: %v\n", err)
		} else {
			fmt.Println("done")
			deleted++
		}
	}

	fmt.Printf("\nRemoved %d container(s).\n", deleted)
	return nil
}

// isOrphan checks if a container is orphaned.
func isOrphan(c runtime.ContainerInfo) bool {
	// No path label = orphan
	if c.ProjectPath == "" {
		return true
	}

	// Directory doesn't exist = orphan
	if _, err := os.Stat(c.ProjectPath); os.IsNotExist(err) {
		return true
	}

	// No state file = orphan
	stateFile := state.StateFilePath(c.ProjectPath)
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		return true
	}

	// Optionally: verify project ID matches
	st, err := state.Load(c.ProjectPath)
	if err != nil || st == nil {
		return true
	}
	if st.ProjectID != c.ProjectID {
		return true
	}

	return false
}

// getOrphanReason returns a human-readable reason why the container is orphaned.
func getOrphanReason(c runtime.ContainerInfo) string {
	if c.ProjectPath == "" {
		return "no project path label"
	}

	if _, err := os.Stat(c.ProjectPath); os.IsNotExist(err) {
		return "project directory does not exist"
	}

	stateFile := state.StateFilePath(c.ProjectPath)
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		return "state file (.alca/state.json) does not exist"
	}

	st, err := state.Load(c.ProjectPath)
	if err != nil {
		return "failed to load state file"
	}
	if st == nil {
		return "state file is empty"
	}
	if st.ProjectID != c.ProjectID {
		return "project ID mismatch"
	}

	return "unknown"
}
