package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/sync"
)

func init() {
	syncCheckCmd.Flags().String("template", "", "Go template for output format")
}

var syncCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for sync conflicts",
	Long:  "Check for file sync conflicts. Exit 0 if no conflicts, exit 1 if conflicts exist.",
	RunE:  runSyncCheck,
}

type syncCheckData struct {
	Conflicts []sync.ConflictInfo `json:"conflicts"`
	Count     int                 `json:"count"`
}

func runSyncCheck(cmd *cobra.Command, args []string) error {
	_, _ = fmt.Fprint(cmd.OutOrStderr(), experimentalWarning)
	_, _ = fmt.Fprintln(cmd.OutOrStderr())

	cwd, err := getCwd()
	if err != nil {
		return err
	}

	deps := newCLIReadDeps()
	env, runtimeEnv := deps.Env, deps.RuntimeEnv

	st, err := loadRequiredState(env, cwd)
	if err != nil {
		return err
	}

	syncEnv := sync.NewSyncEnv(afero.NewOsFs(), deps.CmdRunner, runtime.NewMutagenSyncClient(runtimeEnv))

	cacheData, err := sync.SyncUpdateCache(context.Background(), syncEnv, st.ProjectID, cwd)
	if err != nil {
		return fmt.Errorf("failed to check sync conflicts: %w", err)
	}

	tplStr, _ := cmd.Flags().GetString("template")

	if tplStr != "" {
		data := syncCheckData{
			Conflicts: cacheData.Conflicts,
			Count:     len(cacheData.Conflicts),
		}
		if err := renderSyncCheckTemplate(cmd, tplStr, data); err != nil {
			return err
		}
		if data.Count > 0 {
			return fmt.Errorf("%d sync conflicts found", data.Count)
		}
		return nil
	}

	if len(cacheData.Conflicts) > 0 {
		return fmt.Errorf("%d sync conflicts found", len(cacheData.Conflicts))
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No sync conflicts.")
	return nil
}

func renderSyncCheckTemplate(cmd *cobra.Command, tplStr string, data syncCheckData) error {
	funcMap := template.FuncMap{
		"json": func(v any) (string, error) {
			b, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
	}

	tpl, err := template.New("synccheck").Funcs(funcMap).Parse(tplStr)
	if err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	return tpl.Execute(cmd.OutOrStdout(), data)
}
