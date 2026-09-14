package cli

import (
	"encoding/json"
	"fmt"

	"github.com/AllenMuu/skill-manager/internal/adapter"
	"github.com/AllenMuu/skill-manager/internal/config"
	"github.com/AllenMuu/skill-manager/internal/memory"
	"github.com/spf13/cobra"
)

func newMemoryCommand(options *rootOptions) *cobra.Command {
	memoryCmd := &cobra.Command{Use: "memory", Short: "Configure and diagnose shared Memory providers"}
	memoryCmd.AddCommand(newMemoryConfigureCommand(options), newMemoryStatusCommand(options))
	return memoryCmd
}

func newMemoryConfigureCommand(options *rootOptions) *cobra.Command {
	var provider, id, refKind, refName string
	var scopes, capabilities []string
	cmd := &cobra.Command{Use: "configure", Short: "Explicitly configure a shared Memory provider", RunE: func(cmd *cobra.Command, _ []string) error {
		if options.configPath == "" {
			return fmt.Errorf("memory provider configuration requires --config")
		}
		cfg := memory.ProviderConfig{Version: "v1", ID: id, Provider: provider, Configuration: memory.ConfigReference{Kind: refKind, Name: refName}}
		for _, value := range scopes {
			cfg.Scopes = append(cfg.Scopes, memory.Scope(value))
		}
		for _, value := range capabilities {
			cfg.Capabilities = append(cfg.Capabilities, memory.Capability(value))
		}
		if err := config.SaveMemory(options.configPath, cfg); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "memory provider configured")
		return nil
	}}
	flags := cmd.Flags()
	flags.StringVar(&provider, "provider", "graphiti", "provider adapter")
	flags.StringVar(&id, "id", "shared-memory", "provider configuration id")
	flags.StringVar(&refKind, "reference-kind", "env", "external reference kind")
	flags.StringVar(&refName, "reference", "GRAPHITI_URL", "external configuration reference name")
	flags.StringSliceVar(&scopes, "scope", []string{"user", "project"}, "supported scope (repeat or comma-separate)")
	flags.StringSliceVar(&capabilities, "capability", []string{"read", "search"}, "provider capability (repeat or comma-separate)")
	return cmd
}

func newMemoryStatusCommand(options *rootOptions) *cobra.Command {
	var asJSON, allowNetwork bool
	cmd := &cobra.Command{Use: "status", Short: "Show shared Memory provider and per-agent capability status", RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(options.configPath)
		if err != nil {
			return err
		}
		if cfg.Memory == nil {
			return renderMemoryStatus(cmd, memory.StatusSummary{Provider: memory.DiscoveryResult{Status: memory.ProviderUnavailable, Reason: "no Memory provider is configured", NextAction: "run memory configure with --config"}, Agents: []memory.AgentStatus{}}, asJSON)
		}
		access := make([]memory.AgentAccess, 0, len(adapter.Supported()))
		for _, target := range adapter.Supported() {
			// Agent adapters currently declare no verified shared-memory
			// integration, so status must report these mappings unsupported.
			access = append(access, memory.AgentAccess{Agent: string(target.Target())})
		}
		status := memory.SummarizeStatus(*cfg.Memory, access, memory.DiscoveryOptions{AllowNetwork: allowNetwork})
		return renderMemoryStatus(cmd, status, asJSON)
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	cmd.Flags().BoolVar(&allowNetwork, "network", false, "explicitly probe the provider health endpoint")
	return cmd
}

func renderMemoryStatus(cmd *cobra.Command, status memory.StatusSummary, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(status)
	}
	p := status.Provider
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "provider\t%s\t%s\n", p.Provider, p.Status); err != nil {
		return err
	}
	if p.Reason != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "reason\t%s\n", p.Reason); err != nil {
			return err
		}
	}
	if p.NextAction != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "next\t%s\n", p.NextAction); err != nil {
			return err
		}
	}
	for _, agent := range status.Agents {
		for _, capability := range agent.Capabilities {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agent\t%s\t%s\t%s\t%s\n", agent.Agent, capability.Capability, capability.Scope, capability.Status); err != nil {
				return err
			}
		}
	}
	return nil
}
