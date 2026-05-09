package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agent-workforce/agent-workforce/internal/assets"
	"github.com/agent-workforce/agent-workforce/internal/office"
	"github.com/agent-workforce/agent-workforce/internal/product"
)

const (
	PackageName           = product.PackageName
	Version               = product.Version
	MCPCommand            = product.MCPCommand
	DisabledAgentsDirName = product.DisabledAgentsDirName
)

var bundledAgents = product.AgentIDs()

var legacyAgents = []string{
	"moderation-backend-architect",
	"moderation-backend-programmer",
	"moderation-frontend-architect",
	"moderation-frontend-programmer",
	"moderation-model-architect",
	"moderation-pipeline-architect",
	"moderation-policy-architect",
	"moderation-taxonomy-architect",
	"moderation-data-privacy-architect",
	"moderation-qa-architect",
	"moderation-qa-programmer",
	"mcp-agent-platform-architect",
	"backend-architect",
	"frontend-architect",
	"ai-model-architect",
	"workflow-architect",
	"policy-architect",
	"data-architect",
	"privacy-security-architect",
	"qa-architect",
}

var bundledSkills = []string{"delegate-to-workforce-experts"}
var legacySkills = []string{"delegate-to-moderation-experts"}
var agentIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)
var supportedAgentTools = map[string]bool{
	"read":              true,
	"write":             true,
	"fs_search":         true,
	"sem_search":        true,
	"remove":            true,
	"patch":             true,
	"multi_patch":       true,
	"undo":              true,
	"shell":             true,
	"fetch":             true,
	"followup":          true,
	"skill":             true,
	"plan":              true,
	"todo_write":        true,
	"todo_read":         true,
	"agent_workforce_*": true,
}

type CommonOptions struct {
	Yes                 bool
	ForgeConfig         string
	ForgeBin            string
	SkipForgeValidation bool
	SkipMCPImport       bool
	SkipMCPRemove       bool
	DryRun              bool
	JSON                bool
	RemoveState         bool
	OfficeURL           string
}

type Manifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	Package       string           `json:"package"`
	Version       string           `json:"packageVersion"`
	InstalledAt   string           `json:"installedAt"`
	UpdatedAt     string           `json:"updatedAt"`
	Forge         map[string]any   `json:"forge"`
	MCP           map[string]any   `json:"mcp"`
	Agents        []ManifestRecord `json:"agents"`
	Skills        []ManifestRecord `json:"skills"`
	LegacyCleanup []map[string]any `json:"legacyCleanup,omitempty"`
}

type ManifestRecord struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Source string `json:"source,omitempty"`
	Active *bool  `json:"active,omitempty"`
}

type AgentInfo struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Active    bool   `json:"active"`
	Deletable bool   `json:"deletable"`
	Path      string `json:"path"`
	Title     string `json:"title,omitempty"`
}

func Execute() error {
	opts := &CommonOptions{OfficeURL: product.DefaultOfficeURL()}
	root := &cobra.Command{
		Use:   "agent-workforce",
		Short: "Install and operate a global Agent Workforce for ForgeCode.",
		Long: strings.TrimSpace(`Install and operate a global Agent Workforce for ForgeCode.

This CLI manages global agent installation, global skill installation, generated MCP config,
the Pixel Agent Office runtime, and agent lifecycle/editing workflows.

Global config path resolution:
1) --forge-config
2) FORGE_CONFIG environment variable
3) ~/forge (default)`),
		Example: strings.TrimSpace(`agent-workforce install
agent-workforce new
agent-workforce new -p "Review this repo"
agent-workforce doctor
agent-workforce office`),
	}
	root.PersistentFlags().StringVar(&opts.ForgeConfig, "forge-config", "", "Forge config directory. Defaults to FORGE_CONFIG or ~/forge.")

	root.AddCommand(newCommand(opts))
	root.AddCommand(installCommand(opts, false))
	root.AddCommand(installCommand(opts, true))
	root.AddCommand(uninstallCommand(opts))
	root.AddCommand(doctorCommand(opts))
	root.AddCommand(statusCommand(opts))
	root.AddCommand(officeCommand())
	root.AddCommand(agentsCommand(opts))
	return root.Execute()
}

func newCommand(opts *CommonOptions) *cobra.Command {
	local := *opts
	cmd := &cobra.Command{
		Use:                "new [forge args...]",
		Short:              "Start a new Forge session with the Agent Workforce coordinator.",
		Long:               "Start ForgeCode with the Agent Workforce coordinator agent and forward any remaining arguments to Forge.",
		Example:            "agent-workforce new\nagent-workforce new -p \"Review this repo\"\nagent-workforce new --forge-bin /path/to/forge -p \"Plan this feature\"",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			forgeBinFlag, forgeArgs, helpRequested, err := parseNewCommandArgs(args)
			if err != nil {
				return err
			}
			if helpRequested {
				return cmd.Help()
			}
			if forgeBinFlag != "" {
				local.ForgeBin = forgeBinFlag
			}
			forgeBin, err := resolveForgeBin(local.ForgeBin)
			if err != nil {
				return fmt.Errorf("forge binary resolution failed: %w", err)
			}
			return launchCoordinatorForge(forgeBin, forgeArgs)
		},
	}
	cmd.Flags().StringVar(&local.ForgeBin, "forge-bin", "", "Forge binary path. Defaults to FORGE_BIN or PATH lookup.")
	return cmd
}

func parseNewCommandArgs(args []string) (string, []string, bool, error) {
	forgeArgs := []string{}
	forgeBin := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			forgeArgs = append(forgeArgs, args[i+1:]...)
			return forgeBin, forgeArgs, false, nil
		case arg == "--help" || arg == "-h":
			return forgeBin, forgeArgs, true, nil
		case arg == "--forge-bin":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", nil, false, errors.New("--forge-bin requires a value")
			}
			i++
			forgeBin = args[i]
		case strings.HasPrefix(arg, "--forge-bin="):
			forgeBin = strings.TrimPrefix(arg, "--forge-bin=")
			if forgeBin == "" {
				return "", nil, false, errors.New("--forge-bin requires a value")
			}
		default:
			forgeArgs = append(forgeArgs, arg)
		}
	}
	return forgeBin, forgeArgs, false, nil
}

var coordinatorForgeRunner = runExternalCommand

func launchCoordinatorForge(forgeBin string, forgeArgs []string) error {
	args := append([]string{"--agent", product.CoordinatorAgentID}, forgeArgs...)
	return coordinatorForgeRunner(forgeBin, args)
}

func runExternalCommand(name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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

func uninstallCommand(opts *CommonOptions) *cobra.Command {
	local := *opts
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove package-owned Agent Workforce installation artifacts.",
		Long:  "Remove files tracked in the install manifest. Unrelated Forge files are never removed.",
		Example: strings.TrimSpace(`agent-workforce uninstall
agent-workforce uninstall --yes --remove-state`),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, _ := loadManifest()
			if manifest == nil {
				if local.JSON {
					return printJSON(map[string]any{"ok": false, "error": "manifest missing"})
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
			removed := []string{}
			errs := []string{}
			for _, rec := range append(manifest.Agents, manifest.Skills...) {
				if rec.Path == "" {
					continue
				}
				if err := os.Remove(rec.Path); err == nil {
					removed = append(removed, rec.Path)
				} else if !os.IsNotExist(err) {
					errs = append(errs, err.Error())
				}
			}
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
				return printJSON(map[string]any{"removed": removed, "errors": errs})
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

func statusCommand(opts *CommonOptions) *cobra.Command {
	local := *opts
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current Agent Workforce install status from manifest data.",
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, _ := loadManifest()
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

func agentsActivateCommand(opts *CommonOptions, activate bool) *cobra.Command {
	verb := "deactivate"
	value := false
	if activate {
		verb = "activate"
		value = true
	}
	cmd := &cobra.Command{Use: verb + " <agent-id>", Short: strings.Title(verb) + " an agent.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
		paths := []string{customAgentPath(id), filepath.Join(agentDir(forgeConfigDir(opts.ForgeConfig)), id+".md"), filepath.Join(disabledAgentDir(agentDir(forgeConfigDir(opts.ForgeConfig))), id+".md")}
		for _, p := range paths {
			if err := os.Remove(p); err == nil {
				removed = append(removed, p)
			}
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

func validateAgentFile(expectedID, path string) []string {
	var data []byte
	var err error
	if strings.HasPrefix(path, "files/") {
		data, err = fs.ReadFile(assets.Files, path)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	fields := parseFrontmatter(string(data))
	tools := parseFrontmatterList(string(data), "tools")
	errs := []string{}
	id := fields["id"]
	if id == "" {
		errs = append(errs, path+": missing id")
	} else if id != expectedID {
		errs = append(errs, path+": frontmatter id does not match expected id")
	}
	if fields["title"] == "" {
		errs = append(errs, path+": missing title")
	}
	if fields["description"] == "" {
		errs = append(errs, path+": missing description")
	}
	if len(tools) == 0 {
		errs = append(errs, path+": missing tools")
	}
	for _, tool := range tools {
		if !isSupportedAgentTool(tool) {
			errs = append(errs, fmt.Sprintf("%s: unsupported tool %q", path, tool))
		}
	}
	return errs
}

func parseFrontmatter(content string) map[string]string {
	fields := map[string]string{}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fields
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			fields[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return fields
}

func parseFrontmatterList(content, key string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}
	items := []string{}
	inList := false
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		if inList {
			if strings.HasPrefix(line, "  - ") {
				item := strings.TrimSpace(strings.TrimPrefix(line, "  - "))
				if item != "" {
					items = append(items, item)
				}
				continue
			}
			if trimmed == "" {
				continue
			}
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inList = false
			}
		}
		if trimmed == key+":" {
			inList = true
		}
	}
	return items
}

func isSupportedAgentTool(tool string) bool {
	tool = strings.TrimSpace(tool)
	if supportedAgentTools[tool] {
		return true
	}
	if strings.HasPrefix(tool, "agent_workforce_") {
		return true
	}
	return strings.HasPrefix(tool, "mcp_agent_workforce_") && strings.Contains(tool, "_tool_agent_workforce_")
}
