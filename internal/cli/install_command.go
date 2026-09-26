package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/donske07/agent-workforce/internal/product"
)

func installCommand(opts *CommonOptions, update bool) *cobra.Command {
	local := *opts
	use := "install"
	short := "Interactively install the global Agent Workforce runtime."
	if update {
		use = "update"
		short = "Update previously installed Agent Workforce assets."
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long: strings.TrimSpace(short + `

Installs or re-syncs bundled agent definition files, bundled skill files, generated
MCP config metadata, and the local manifest at ~/.agent-workforce/manifest.json.
This command is safe by default and only writes Agent Workforce-owned assets.`),
		Example: fmt.Sprintf("agent-workforce %s --yes\nagent-workforce %s --yes --skip-mcp-import --forge-config \"$HOME/forge\"", use, use),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !local.Yes && !local.JSON {
				ok, err := confirm("Continue with Agent Workforce "+use+"?", true)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
			}
			if !local.SkipForgeValidation {
				forgeBin, err := resolveForgeBin(local.ForgeBin)
				if err != nil {
					return fmt.Errorf("forge binary validation failed: %w", err)
				}
				local.ForgeBin = forgeBin
			}
			manifest, err := installAssets(local)
			if err != nil {
				return err
			}
			if local.JSON {
				return printJSON(map[string]any{"ok": true, "dryRun": local.DryRun, "manifest": manifest})
			}
			if local.DryRun {
				fmt.Printf("Agent Workforce %s dry run complete. No files were written.\n", use)
				fmt.Println("Forge config:", forgeConfigDir(local.ForgeConfig))
				return nil
			}
			status := "installed"
			if update {
				status = "updated"
			}
			fmt.Printf("Agent Workforce %s successfully.\n", status)
			fmt.Println("Forge config:", forgeConfigDir(local.ForgeConfig))
			fmt.Println("Next: agent-workforce doctor && agent-workforce office")
			return nil
		},
	}
	addInstallFlags(cmd, &local)
	return cmd
}

func addInstallFlags(cmd *cobra.Command, opts *CommonOptions) {
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "Run non-interactively and accept safe defaults.")
	cmd.Flags().StringVar(&opts.ForgeBin, "forge-bin", "", "Forge binary path. Defaults to FORGE_BIN or PATH lookup.")
	cmd.Flags().BoolVar(&opts.SkipForgeValidation, "skip-forge-validation", false, "Skip checking the Forge binary.")
	cmd.Flags().BoolVar(&opts.SkipMCPImport, "skip-mcp-import", false, "Do not write generated MCP config.")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview actions without writing files.")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Print machine-readable JSON output.")
	cmd.Flags().StringVar(&opts.OfficeURL, "office-url", product.DefaultOfficeURL(), "Pixel office URL used in generated MCP config.")
}
