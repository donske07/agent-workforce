package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/donske07/agent-workforce/internal/office"
	"github.com/donske07/agent-workforce/internal/product"
)

func statusCommand(opts *CommonOptions) *cobra.Command {
	local := *opts
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current Agent Workforce install status from manifest data.",
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, err := loadManifest()
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("load install manifest: %w", err)
			}
			if local.JSON {
				return printJSON(map[string]any{"manifest": manifest})
			}
			if manifest == nil {
				fmt.Println("Agent Workforce is not installed.")
				return nil
			}
			fmt.Println("Agent Workforce installed")
			fmt.Println("Version:", manifest.Version)
			fmt.Println("Agents:", len(manifest.Agents))
			fmt.Println("Skills:", len(manifest.Skills))
			return nil
		},
	}
	cmd.Flags().BoolVar(&local.JSON, "json", false, "Print JSON output.")
	return cmd
}

func doctorCommand(opts *CommonOptions) *cobra.Command {
	local := *opts
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run installation diagnostics.",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := forgeConfigDir(local.ForgeConfig)
			checks := []map[string]any{}
			checks = append(checks, check("forge config directory", exists(config), config))
			_, forgeErr := resolveForgeBin(local.ForgeBin)
			checks = append(checks, check("forge binary", forgeErr == nil || local.SkipForgeValidation, errText(forgeErr)))
			inv, invErr := inventory(config)
			agentInventoryOK := invErr == nil && len(inv) >= len(bundledAgents)
			agentInventoryDetail := fmt.Sprintf("%d agents", len(inv))
			if invErr != nil {
				agentInventoryDetail = invErr.Error()
			}
			checks = append(checks, check("agent inventory", agentInventoryOK, agentInventoryDetail))
			_, mcpErr := exec.LookPath(MCPCommand)
			checks = append(checks, check("mcp command", mcpErr == nil, errText(mcpErr)))
			ok := true
			for _, c := range checks {
				if c["ok"] == false {
					ok = false
				}
			}
			if local.JSON {
				return printJSON(map[string]any{"ok": ok, "checks": checks})
			}
			for _, c := range checks {
				fmt.Printf("[%s] %s %s\n", passFail(c["ok"].(bool)), c["name"], c["detail"])
			}
			if !ok {
				return errors.New("doctor checks failed")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&local.ForgeBin, "forge-bin", "", "Forge binary path.")
	cmd.Flags().BoolVar(&local.SkipForgeValidation, "skip-forge-validation", false, "Skip Forge binary validation.")
	cmd.Flags().BoolVar(&local.JSON, "json", false, "Print JSON output.")
	return cmd
}

func officeCommand() *cobra.Command {
	var host string
	var port int
	cmd := &cobra.Command{
		Use:   "office",
		Short: "Start the Pixel Agent Office web server.",
		Long:  "Start the embedded Pixel Agent Office server exposing /, /health, /events, and /agent-state.",
		RunE:  func(cmd *cobra.Command, args []string) error { return office.Run(host, port) },
	}
	cmd.Flags().StringVar(&host, "host", product.DefaultOfficeHost, "Host/interface to bind.")
	cmd.Flags().IntVar(&port, "port", product.DefaultOfficePort, "Port to bind.")
	return cmd
}
