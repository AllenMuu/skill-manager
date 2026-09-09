package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AllenMuu/skill-manager/internal/adapter"
	"github.com/spf13/cobra"
)

type agentInventoryItem struct {
	ID            string   `json:"id"`
	Status        string   `json:"status"`
	Availability  string   `json:"availability"`
	ResourceKinds []string `json:"resourceKinds"`
	Capabilities  []string `json:"capabilities"`
}

func newAgentsCommand() *cobra.Command {
	var asJSON bool
	var project string
	cmd := &cobra.Command{Use: "agents", Short: "List supported agent adapters", RunE: func(cmd *cobra.Command, _ []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		items := make([]agentInventoryItem, 0, len(adapter.Supported()))
		for _, a := range adapter.Supported() {
			item := agentInventoryItem{ID: string(a.Target()), Status: "supported", Availability: adapterAvailability(a, home), ResourceKinds: []string{}, Capabilities: []string{}}
			for _, kind := range a.ResourceKinds() {
				item.ResourceKinds = append(item.ResourceKinds, string(kind))
				for _, capability := range a.Capabilities(kind) {
					item.Capabilities = append(item.Capabilities, string(capability))
				}
			}
			items = append(items, item)
		}
		unsupported, err := unsupportedAgentInventory(home, project)
		if err != nil {
			return err
		}
		items = append(items, unsupported...)
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%v\t%v\n", item.ID, item.Status, item.Availability, item.ResourceKinds, item.Capabilities); err != nil {
				return err
			}
		}
		return nil
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	cmd.Flags().StringVar(&project, "project", "", "project directory whose agent locations should be inventoried")
	return cmd
}

func unsupportedAgentInventory(home, project string) ([]agentInventoryItem, error) {
	seen := make(map[string]bool)
	for _, a := range adapter.Supported() {
		rel, err := filepath.Rel(home, a.GlobalSkillPath(home, "placeholder"))
		if err == nil {
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) > 0 {
				seen[parts[0]] = true
			}
		}
	}
	seen[".agents"] = true
	seen[".git"] = true
	seen[".skill-manager"] = true

	roots := []string{home}
	if project != "" {
		absolute, err := filepath.Abs(project)
		if err != nil {
			return nil, err
		}
		roots = append(roots, absolute)
	}
	items := make([]agentInventoryItem, 0)
	found := make(map[string]bool)
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || len(entry.Name()) < 2 || entry.Name()[0] != '.' || seen[entry.Name()] {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, entry.Name(), "skills")); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			id := strings.TrimPrefix(entry.Name(), ".")
			if found[id] {
				continue
			}
			found[id] = true
			items = append(items, agentInventoryItem{ID: id, Status: "unsupported", Availability: "unsupported", ResourceKinds: []string{}, Capabilities: []string{}})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func adapterAvailability(a adapter.Adapter, home string) string {
	if _, err := exec.LookPath(agentCommand(a.Target())); err == nil {
		return "detected"
	}
	configRoot := filepath.Dir(filepath.Dir(a.GlobalSkillPath(home, "placeholder")))
	info, err := os.Stat(configRoot)
	if err == nil && info.IsDir() {
		return "configured"
	}
	return "unavailable"
}

func agentCommand(target adapter.Target) string {
	if target == adapter.ClaudeCode {
		return "claude"
	}
	return string(target)
}
