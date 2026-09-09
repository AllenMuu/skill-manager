package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AllenMuu/skill-manager/internal/catalog"
	"github.com/AllenMuu/skill-manager/internal/config"
	"github.com/AllenMuu/skill-manager/internal/resource"
	"github.com/AllenMuu/skill-manager/internal/subagent"
	"github.com/spf13/cobra"
)

type subAgentOptions struct {
	root    string
	library string
	json    bool
}

type subAgentValidationReport struct {
	Valid       bool                     `json:"valid"`
	Diagnostics []subAgentDiagnosticJSON `json:"diagnostics"`
}

type subAgentDiagnosticJSON struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func newSubAgentsCommand(rootOptions *rootOptions) *cobra.Command {
	options := &subAgentOptions{}
	command := &cobra.Command{
		Use:     "subagents",
		Aliases: []string{"subagent"},
		Short:   "Inspect canonical SubAgent definitions",
	}
	command.PersistentFlags().StringVar(&options.root, "root", "", "path to the Agent Manager data root")
	command.PersistentFlags().StringVar(&options.library, "library", "", "path to the Skill library used to verify references")
	command.PersistentFlags().BoolVar(&options.json, "json", false, "write machine-readable JSON")
	command.AddCommand(newSubAgentListCommand(rootOptions, options))
	command.AddCommand(newSubAgentShowCommand(rootOptions, options))
	command.AddCommand(newSubAgentValidateCommand(rootOptions, options))
	return command
}

func newSubAgentListCommand(rootOptions *rootOptions, options *subAgentOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List canonical SubAgent definitions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, definitions, diagnostics, err := discoverSubAgents(rootOptions, options)
			if err != nil {
				return err
			}
			if options.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(definitions)
			}
			for _, definition := range definitions {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\tcompatibility: %s\n", definition.ID, definition.Name, definition.Role, strings.Join(definition.Compatibility.Agents, ", ")); err != nil {
					return err
				}
			}
			writeSubAgentDiagnostics(cmd, diagnostics)
			return nil
		},
	}
}

func newSubAgentShowCommand(rootOptions *rootOptions, options *subAgentOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show one canonical SubAgent definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, definitions, diagnostics, err := discoverSubAgents(rootOptions, options)
			if err != nil {
				return err
			}
			for _, definition := range definitions {
				if definition.ID != args[0] {
					continue
				}
				if options.json {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(definition)
				}
				return renderSubAgentHuman(cmd, definition)
			}
			return unknownSubAgentError(args[0], definitions, diagnostics)
		},
	}
}

func newSubAgentValidateCommand(rootOptions *rootOptions, options *subAgentOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "validate [id]",
		Short: "Validate canonical SubAgent definitions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, definitions, diagnostics, err := discoverSubAgents(rootOptions, options)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				for _, definition := range definitions {
					if definition.ID == args[0] {
						if options.json {
							return json.NewEncoder(cmd.OutOrStdout()).Encode(subAgentValidationReport{Valid: true, Diagnostics: []subAgentDiagnosticJSON{}})
						}
						_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: valid\n", definition.ID)
						return err
					}
				}
				selected := diagnosticsForID(args[0], diagnostics)
				if len(selected) > 0 {
					if options.json {
						return json.NewEncoder(cmd.OutOrStdout()).Encode(subAgentValidationReport{Valid: false, Diagnostics: diagnosticsJSON(selected)})
					}
					writeSubAgentDiagnostics(cmd, selected)
					return nil
				}
				return unknownSubAgentError(args[0], definitions, diagnostics)
			}
			if options.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(subAgentValidationReport{Valid: len(diagnostics) == 0, Diagnostics: diagnosticsJSON(diagnostics)})
			}
			if len(diagnostics) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "all SubAgent definitions are valid")
				return err
			}
			writeSubAgentDiagnostics(cmd, diagnostics)
			return nil
		},
	}
}

func discoverSubAgents(rootOptions *rootOptions, options *subAgentOptions) (subagent.Registry, []subagent.Definition, []subagent.Diagnostic, error) {
	var checker subagent.SkillReferenceChecker
	library := options.library
	implicitDefaultLibrary := library == "" && rootOptions.configPath == ""
	if library == "" {
		configured, err := config.Load(rootOptions.configPath)
		if err != nil {
			return subagent.Registry{}, nil, nil, err
		}
		library = configured.LibraryPath
	}
	if _, err := os.Stat(library); err != nil {
		if os.IsNotExist(err) {
			if !implicitDefaultLibrary {
				return subagent.Registry{}, nil, nil, fmt.Errorf("read skill library: %w", err)
			}
			// The default library is optional until a definition references a
			// Skill. Registry validation will report that reference explicitly.
			registry, registryErr := registryForOptions(options, nil)
			if registryErr != nil {
				return subagent.Registry{}, nil, nil, registryErr
			}
			definitions, diagnostics, discoverErr := registry.Discover()
			return registry, definitions, diagnostics, discoverErr
		}
		return subagent.Registry{}, nil, nil, fmt.Errorf("inspect skill library: %w", err)
	}
	skills, _, err := catalog.Discover(library)
	if err != nil {
		return subagent.Registry{}, nil, nil, err
	}
	checker = func(identifier string) bool {
		for _, skill := range skills {
			if skill.Identifier == identifier {
				return true
			}
		}
		return false
	}
	registry, err := registryForOptions(options, checker)
	if err != nil {
		return subagent.Registry{}, nil, nil, err
	}
	definitions, diagnostics, err := registry.Discover()
	return registry, definitions, diagnostics, err
}

func registryForOptions(options *subAgentOptions, checker subagent.SkillReferenceChecker) (subagent.Registry, error) {
	if options.root == "" {
		return subagent.NewDefaultRegistry(checker)
	}
	return subagent.NewRegistry(options.root, checker)
}

func renderSubAgentHuman(cmd *cobra.Command, definition subagent.Definition) error {
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "id: %s\nversion: %s\nname: %s\nrole: %s\ninstructions: %s\nskills: %s\ncompatibility: %s\nrequired capabilities: %s\n", definition.ID, definition.Version, definition.Name, definition.Role, definition.Instructions, strings.Join(definition.Skills, ", "), strings.Join(definition.Compatibility.Agents, ", "), strings.Join(capabilityStrings(definition.RequiredCapabilities), ", "))
	return err
}

func capabilityStrings(capabilities []resource.Capability) []string {
	values := make([]string, len(capabilities))
	for i, capability := range capabilities {
		values[i] = string(capability)
	}
	return values
}

func diagnosticsJSON(diagnostics []subagent.Diagnostic) []subAgentDiagnosticJSON {
	values := make([]subAgentDiagnosticJSON, len(diagnostics))
	for i, diagnostic := range diagnostics {
		values[i] = subAgentDiagnosticJSON{Path: diagnostic.Path, Message: diagnostic.Message}
	}
	return values
}

func writeSubAgentDiagnostics(cmd *cobra.Command, diagnostics []subagent.Diagnostic) {
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(cmd.ErrOrStderr(), "diagnostic:", diagnostic.Error())
	}
}

func unknownSubAgentError(id string, definitions []subagent.Definition, diagnostics []subagent.Diagnostic) error {
	ids := make([]string, len(definitions))
	for i, definition := range definitions {
		ids[i] = definition.ID
	}
	sort.Strings(ids)
	message := fmt.Sprintf("SubAgent %q not found; available: %s", id, strings.Join(ids, ", "))
	if len(diagnostics) > 0 {
		message += "; run 'subagents validate' for invalid definitions"
	}
	return errors.New(message)
}

func diagnosticsForID(id string, diagnostics []subagent.Diagnostic) []subagent.Diagnostic {
	selected := make([]subagent.Diagnostic, 0)
	for _, diagnostic := range diagnostics {
		base := filepath.Base(diagnostic.Path)
		if strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml") == id {
			selected = append(selected, diagnostic)
		}
	}
	return selected
}
