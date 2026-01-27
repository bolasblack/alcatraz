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
	Use:           "alca",
	Short:         "Alcatraz - Lightweight container isolation for AI coding assistants",
	Long:          `Alcatraz (alca) provides lightweight container isolation for AI coding assistants.
It wraps AI agent processes in configurable containers for enhanced security.`,
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// GetRootCmd returns the root command for documentation generation.
func GetRootCmd() *cobra.Command {
	return rootCmd
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(experimentalCmd)
	rootCmd.AddCommand(networkHelperCmd)
}
