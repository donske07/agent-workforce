package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func agentsActivateCommand(opts *CommonOptions, activate bool) *cobra.Command {
	verb := "deactivate"
	value := false
	if activate {
		verb = "activate"
		value = true
	}
	cmd := &cobra.Command{Use: verb + " <agent-id>", Short: titleFromID(verb) + " an agent.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		config := forgeConfigDir(opts.ForgeConfig)
		inv, err := inventory(config)
		if err != nil {
			return err
		}
		if !agentExists(inv, id) {
			return fmt.Errorf("agent not found: %s", id)
		}
		state, err := loadAgentState()
		if err != nil {
			return err
		}
		state[id] = value
		if err := saveAgentState(state); err != nil {
			return err
		}
		_, err = syncAgents(config, false)
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSON(map[string]any{"id": id, "active": value})
		}
		fmt.Printf("%s active=%v\n", id, value)
		return nil
	}}
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Print JSON output.")
	return cmd
}

func agentsDeleteCommand(opts *CommonOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "delete <agent-id>", Short: "Delete a custom agent. Bundled agents cannot be deleted.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if !agentIDPattern.MatchString(id) {
			return fmt.Errorf("invalid agent id: %s", id)
		}
		if isBundled(id) {
			return fmt.Errorf("bundled agents cannot be deleted: %s. Use 'agents manage' or 'agents deactivate' instead", id)
		}
		if !opts.Yes && !opts.JSON {
			ok, err := confirm("Delete custom agent "+id+"?", false)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
		removed := []string{}
		errs := []string{}
		paths := []string{customAgentPath(id), filepath.Join(agentDir(forgeConfigDir(opts.ForgeConfig)), id+".md"), filepath.Join(disabledAgentDir(agentDir(forgeConfigDir(opts.ForgeConfig))), id+".md")}
		removePaths(paths, &removed, &errs)
		if len(errs) > 0 {
			if opts.JSON {
				if err := printJSON(map[string]any{"removed": removed, "errors": errs}); err != nil {
					return err
				}
			}
			return errors.New(strings.Join(errs, "; "))
		}
		state, err := loadAgentState()
		if err != nil {
			return err
		}
		delete(state, id)
		if err := saveAgentState(state); err != nil {
			return err
		}
		if opts.JSON {
			return printJSON(map[string]any{"removed": removed})
		}
		fmt.Printf("Deleted %s (%d files).\n", id, len(removed))
		return nil
	}}
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "Run non-interactively.")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Print JSON output.")
	return cmd
}

func agentsAddCommand(opts *CommonOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "add <agent-id>", Short: "Create a new custom agent using the ForgeCode-configured editor.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if !agentIDPattern.MatchString(id) {
			return fmt.Errorf("invalid agent id: %s", id)
		}
		editor, err := resolveForgeEditor(forgeConfigDir(opts.ForgeConfig))
		if err != nil {
			return err
		}
		path := customAgentPath(id)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			content := fmt.Sprintf("---\nid: %s\ntitle: %s\ndescription: Custom workforce agent for targeted project assistance.\ntools:\n  - read\n  - fs_search\n  - sem_search\n---\n\n# %s\n\nYou are a custom Agent Workforce specialist. Help with targeted project analysis and implementation tasks within the scope the user provides. Verify codebase facts before making claims, keep responses concise, and summarize changed files and validation when work is completed.\n", id, titleFromID(id), titleFromID(id))
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return err
			}
		}
		if err := launchEditor(editor, path); err != nil {
			return err
		}
		if errs := validateAgentFile(id, path); len(errs) > 0 {
			return errors.New(strings.Join(errs, "\n"))
		}
		_, err = syncAgents(forgeConfigDir(opts.ForgeConfig), false)
		if err != nil {
			return err
		}
		fmt.Printf("Added custom agent %s.\n", id)
		return nil
	}}
	return cmd
}

func agentsEditCommand(opts *CommonOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "edit [agent-id]", Short: "Open interactive single-select editor screen or edit one agent directly.", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		config := forgeConfigDir(opts.ForgeConfig)
		if len(args) == 1 {
			return editAgent(config, args[0])
		}
		agents, err := inventory(config)
		if err != nil {
			return err
		}
		selected, ok, err := runEditSelector(agents)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return editAgent(config, selected)
	}}
	return cmd
}

func agentsManageCommand(opts *CommonOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "manage", Short: "Open interactive activation manager.", RunE: func(cmd *cobra.Command, args []string) error {
		config := forgeConfigDir(opts.ForgeConfig)
		agents, err := inventory(config)
		if err != nil {
			return err
		}
		staged, confirmed, err := runManageSelector(agents)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
		if err := saveAgentState(staged); err != nil {
			return err
		}
		_, err = syncAgents(config, false)
		return err
	}}
	return cmd
}
