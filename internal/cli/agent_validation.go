package cli

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/donske07/agent-workforce/internal/assets"
)

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
			if value, ok := strings.CutPrefix(line, "  - "); ok {
				item := strings.TrimSpace(value)
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
