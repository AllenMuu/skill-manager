package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AllenMuu/skill-manager/internal/adapter"
	"github.com/spf13/cobra"
)

type agentInventoryItem struct {
	ID            string   `json:"id"`
	Availability  string   `json:"availability"`
	ResourceKinds []string `json:"resourceKinds"`
	Capabilities  []string `json:"capabilities"`
}

func newAgentsCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "agents", Short: "List supported agent adapters", RunE: func(cmd *cobra.Command, _ []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		items := make([]agentInventoryItem, 0, len(adapter.Supported()))
		for _, a := range adapter.Supported() {
			item := agentInventoryItem{ID: string(a.Target()), Availability: adapterAvailability(a, home)}
			for _, kind := range a.ResourceKinds() {
				item.ResourceKinds = append(item.ResourceKinds, string(kind))
				for _, capability := range a.Capabilities(kind) {
					item.Capabilities = append(item.Capabilities, string(capability))
				}
			}
			items = append(items, item)
		}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%v\t%v\n", item.ID, item.Availability, item.ResourceKinds, item.Capabilities); err != nil {
				return err
			}
		}
		return nil
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return cmd
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
