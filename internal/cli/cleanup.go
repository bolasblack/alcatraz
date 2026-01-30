package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/state"
	"github.com/bolasblack/alcatraz/internal/util"
)

var cleanupAll bool

// orphanContainer holds container info with the reason it's orphaned.
// This avoids calling checkOrphanStatus twice for display purposes.
type orphanContainer struct {
	info   runtime.ContainerInfo
	reason string
}

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
	cwd, err := getCwd()
	if err != nil {
		return err
	}

	// Create env for read-only file operations
	env := util.NewReadonlyOsEnv()

	// Load config (optional) and select runtime
	_, rt, err := loadConfigAndRuntimeOptional(env, cwd)
	if err != nil {
		return err
	}

	ctx := context.Background()
	containers, err := rt.ListContainers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	// Find orphan containers with their reasons
	var orphans []orphanContainer
	for _, c := range containers {
		isOrphan, reason := checkOrphanStatus(env, c)
		if isOrphan {
			orphans = append(orphans, orphanContainer{info: c, reason: reason})
		}
	}

	if len(orphans) == 0 {
		// Silent exit when no orphans
		return nil
	}

	var toDelete []runtime.ContainerInfo

	if cleanupAll {
		// --all flag: skip interaction
		toDelete = orphansToContainerInfos(orphans)
	} else {
		toDelete = selectOrphansInteractively(orphans)
	}

	if len(toDelete) == 0 {
		fmt.Println("No containers selected.")
		return nil
	}

	// Delete containers
	deleted := deleteContainers(ctx, rt, toDelete)
	fmt.Println("") // spacing after inline progress
	util.ProgressDone(os.Stdout, "Removed %d container(s).\n", deleted)
	return nil
}

// checkOrphanStatus checks if a container is orphaned and returns the reason.
// Returns (isOrphan, reason).
func checkOrphanStatus(env *util.Env, c runtime.ContainerInfo) (bool, string) {
	// No path label = orphan
	if c.ProjectPath == "" {
		return true, "no project path label"
	}

	// Directory doesn't exist = orphan
	if _, err := env.Fs.Stat(c.ProjectPath); os.IsNotExist(err) {
		return true, "project directory does not exist"
	}

	// No state file = orphan
	stateFile := state.StateFilePath(c.ProjectPath)
	if _, err := env.Fs.Stat(stateFile); os.IsNotExist(err) {
		return true, "state file (.alca/state.json) does not exist"
	}

	// Verify project ID matches
	st, err := state.Load(env, c.ProjectPath)
	if err != nil {
		return true, "failed to load state file"
	}
	if st == nil {
		return true, "state file is empty"
	}
	if st.ProjectID != c.ProjectID {
		return true, "project ID mismatch"
	}

	return false, ""
}

// selectOrphansInteractively displays orphans and prompts for selection.
func selectOrphansInteractively(orphans []orphanContainer) []runtime.ContainerInfo {
	fmt.Printf("Found %d orphan container(s):\n\n", len(orphans))
	for i, o := range orphans {
		projectPath := o.info.ProjectPath
		if projectPath == "" {
			projectPath = "(no path)"
		}
		fmt.Printf("  [%d] %s\n", i+1, o.info.Name)
		fmt.Printf("      Path: %s\n", projectPath)
		fmt.Printf("      Reason: %s\n", o.reason)
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
		return orphansToContainerInfos(orphans)
	}

	// Parse numbers
	return parseContainerSelection(input, orphans)
}

// parseContainerSelection parses user input and returns selected containers.
func parseContainerSelection(input string, orphans []orphanContainer) []runtime.ContainerInfo {
	var selected []runtime.ContainerInfo
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
			selected = append(selected, orphans[num-1].info)
		}
	}

	return selected
}

// orphansToContainerInfos extracts ContainerInfo from orphanContainer slice.
func orphansToContainerInfos(orphans []orphanContainer) []runtime.ContainerInfo {
	infos := make([]runtime.ContainerInfo, len(orphans))
	for i, o := range orphans {
		infos[i] = o.info
	}
	return infos
}

// deleteContainers removes the given containers and returns the count of successfully deleted.
func deleteContainers(ctx context.Context, rt runtime.Runtime, containers []runtime.ContainerInfo) int {
	deleted := 0
	for _, c := range containers {
		util.ProgressStep(os.Stdout, "Removing %s... ", c.Name)
		if err := rt.RemoveContainer(ctx, c.Name); err != nil {
			util.Progress(os.Stdout, "failed: %v\n", err)
		} else {
			util.Progress(os.Stdout, "done\n")
			deleted++
		}
	}
	return deleted
}
