package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/donske07/agent-workforce/internal/product"
)

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
