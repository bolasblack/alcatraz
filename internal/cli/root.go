package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version is set at build time via ldflags
	Version = "dev"
)

var rootCmd = &cobra.Command{
	Use:   "alca",
	Short: "Alcatraz - Lightweight process isolation for Claude Code",
	Long: `Alcatraz (alca) provides lightweight process isolation using macOS sandbox-exec.
It wraps Claude Code processes in configurable sandboxes for enhanced security.`,
	Version: Version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(experimentalCmd)
}
