package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func agentsCommand(opts *CommonOptions) *cobra.Command {
	local := *opts
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Manage agent lifecycle and interactive editing.",
		Long: strings.TrimSpace(`Manage bundled and custom agents.

Interactive commands use Up/Down arrows to move and Enter to toggle/select rows.
Bundled agents can be activated/deactivated but cannot be deleted. Custom agents can be deleted.`),
	}
	cmd.AddCommand(agentsListCommand(&local), agentsSyncCommand(&local), agentsValidateCommand(&local), agentsActivateCommand(&local, true), agentsActivateCommand(&local, false), agentsDeleteCommand(&local), agentsAddCommand(&local), agentsEditCommand(&local), agentsManageCommand(&local))
	return cmd
}

func agentsListCommand(opts *CommonOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "List available agents across bundled/custom/installed layers.", RunE: func(cmd *cobra.Command, args []string) error {
		config := forgeConfigDir(opts.ForgeConfig)
		agents, err := inventory(config)
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSON(map[string]any{"agents": agents})
		}
		fmt.Printf("%-10s %-8s %-9s %s\n", "STATUS", "SOURCE", "DELETABLE", "ID")
		for _, a := range agents {
			status := "inactive"
			if a.Active {
				status = "active"
			}
			deletable := "no"
			if a.Deletable {
				deletable = "yes"
			}
			fmt.Printf("%-10s %-8s %-9s %s\n", status, a.Source, deletable, a.ID)
		}
		return nil
	}}
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Print JSON output.")
	return cmd
}

func agentsSyncCommand(opts *CommonOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "sync", Short: "Sync active agent set into global Forge runtime location.", RunE: func(cmd *cobra.Command, args []string) error {
		actions, err := syncAgents(forgeConfigDir(opts.ForgeConfig), opts.DryRun)
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSON(map[string]any{"actions": actions})
		}
		fmt.Printf("%d sync actions.\n", len(actions))
		return nil
	}}
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview actions without writing files.")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Print JSON output.")
	return cmd
}

func agentsValidateCommand(opts *CommonOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "validate [agent-id]", Short: "Validate one or all agent files.", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		agents, err := inventory(forgeConfigDir(opts.ForgeConfig))
		if err != nil {
			return err
		}
		errs := []string{}
		found := len(args) == 0
		for _, a := range agents {
			if len(args) == 1 && a.ID != args[0] {
				continue
			}
			found = true
			errs = append(errs, validateAgentFile(a.ID, a.Path)...)
		}
		if !found {
			errs = append(errs, "agent not found: "+args[0])
		}
		if opts.JSON {
			return printJSON(map[string]any{"ok": len(errs) == 0, "errors": errs})
		}
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e)
			}
			return errors.New("validation failed")
		}
		fmt.Println("Agent validation passed.")
		return nil
	}}
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Print JSON output.")
	return cmd
}
