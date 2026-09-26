package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func removeManifestFiles(manifest *Manifest, config string) ([]string, []string) {
	removed := []string{}
	errs := []string{}
	agentsRoot := agentDir(config)
	for _, record := range manifest.Agents {
		paths, err := uninstallAgentPaths(record, agentsRoot)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		removePaths(paths, &removed, &errs)
	}
	for _, record := range manifest.Skills {
		if record.Path == "" {
			continue
		}
		if !pathWithin(skillDir(config), record.Path) {
			errs = append(errs, fmt.Sprintf("refusing to remove skill path outside Forge skills directory: %s", record.Path))
			continue
		}
		removePaths([]string{record.Path}, &removed, &errs)
	}
	return removed, errs
}

func uninstallAgentPaths(record ManifestRecord, agentsRoot string) ([]string, error) {
	if !agentIDPattern.MatchString(record.ID) {
		return nil, fmt.Errorf("refusing to remove invalid agent id: %s", record.ID)
	}
	if record.Source == "custom" {
		return []string{
			filepath.Join(agentsRoot, record.ID+".md"),
			filepath.Join(disabledAgentDir(agentsRoot), record.ID+".md"),
		}, nil
	}
	if record.Source != "" && record.Source != "bundled" {
		return nil, fmt.Errorf("refusing to remove agent with unknown source %q", record.Source)
	}
	if record.Path == "" {
		return nil, nil
	}
	if !pathWithin(agentsRoot, record.Path) {
		return nil, fmt.Errorf("refusing to remove agent path outside Forge agents directory: %s", record.Path)
	}
	return []string{record.Path}, nil
}

func removePaths(paths []string, removed, errs *[]string) {
	for _, candidate := range paths {
		if err := os.Remove(candidate); err == nil {
			*removed = append(*removed, candidate)
		} else if !os.IsNotExist(err) {
			*errs = append(*errs, err.Error())
		}
	}
}

func pathWithin(root, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
