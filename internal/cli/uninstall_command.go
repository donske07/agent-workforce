package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func uninstallCommand(opts *CommonOptions) *cobra.Command {
	local := *opts
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove package-owned Agent Workforce installation artifacts.",
		Long:  "Remove files tracked in the install manifest. Unrelated Forge files are never removed.",
		Example: strings.TrimSpace(`agent-workforce uninstall
agent-workforce uninstall --yes --remove-state`),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, err := loadManifest()
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("load install manifest: %w", err)
			}
			if manifest == nil {
				if local.JSON {
					if err := printJSON(map[string]any{"ok": false, "error": "manifest missing"}); err != nil {
						return err
					}
				}
				return errors.New("not installed: manifest missing")
			}
			if !local.Yes && !local.JSON {
				ok, err := confirm("Uninstall Agent Workforce package-owned files?", false)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
			}
			removed, errs := removeManifestFiles(manifest, forgeConfigDir(local.ForgeConfig))
			if !local.SkipMCPRemove {
				mcpPath := mcpConfigPath()
				if exists(mcpPath) {
					if err := removeMCPConfig(); err == nil {
						removed = append(removed, mcpPath)
					} else {
						errs = append(errs, err.Error())
					}
				}
			}
			if cleanupActions, err := cleanupLegacy(agentDir(forgeConfigDir(local.ForgeConfig)), skillDir(forgeConfigDir(local.ForgeConfig)), false); err != nil {
				errs = append(errs, err.Error())
			} else {
				for _, action := range cleanupActions {
					if path, ok := action["remove"].(string); ok {
						removed = append(removed, path)
					}
				}
			}
			if local.RemoveState {
				if err := os.RemoveAll(stateDir()); err != nil {
					errs = append(errs, err.Error())
				}
			}
			if local.JSON {
				if err := printJSON(map[string]any{"removed": removed, "errors": errs}); err != nil {
					return err
				}
				if len(errs) > 0 {
					return errors.New(strings.Join(errs, "; "))
				}
				return nil
			}
			fmt.Printf("Removed %d files.\n", len(removed))
			if len(errs) > 0 {
				return errors.New(strings.Join(errs, "; "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&local.Yes, "yes", false, "Run non-interactively.")
	cmd.Flags().BoolVar(&local.SkipMCPRemove, "skip-mcp-remove", false, "Skip removing generated MCP config.")
	cmd.Flags().BoolVar(&local.RemoveState, "remove-state", false, "Remove ~/.agent-workforce state after uninstalling files.")
	cmd.Flags().BoolVar(&local.JSON, "json", false, "Print JSON output.")
	return cmd
}
