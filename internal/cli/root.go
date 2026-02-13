package cli

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version, Commit, and Date are set at build time via ldflags
	Version = "dev"
	Commit  = ""
	Date    = ""
)

var rootCmd = &cobra.Command{
	Use:   "alca",
	Short: "Alcatraz - Run code agents unrestricted, but fearlessly",
	Long: `Alcatraz (alca) — Run code agents unrestricted, but fearlessly.

Wraps AI code agents in containers with file and network isolation,
so you can run agents without guardrails and keep your system safe.`,
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
	if os.Getenv("ALCA_DEBUG") == "" {
		log.SetOutput(os.Stderr)
	}

	rootCmd.SetVersionTemplate(fmt.Sprintf("alca version %s\ncommit: %s\ndate: %s\n", Version, Commit, Date))

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
