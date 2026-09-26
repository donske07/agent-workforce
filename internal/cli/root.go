package cli

import (
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/donske07/agent-workforce/internal/product"
)

const (
	PackageName           = product.PackageName
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
	return rootCommand(&CommonOptions{OfficeURL: product.DefaultOfficeURL()}).Execute()
}

func rootCommand(opts *CommonOptions) *cobra.Command {
	root := &cobra.Command{
		Use:     "agent-workforce",
		Version: product.Version,
		Short:   "Install and operate a global Agent Workforce for ForgeCode.",
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
	return root
}
