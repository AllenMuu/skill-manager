package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AllenMuu/skill-manager/internal/catalog"
	"github.com/AllenMuu/skill-manager/internal/config"
	"github.com/AllenMuu/skill-manager/internal/search"
	"github.com/spf13/cobra"
)

type searchOutput struct {
	Identifier    string   `json:"identifier"`
	Description   string   `json:"description"`
	Tags          []string `json:"tags"`
	Compatibility []string `json:"compatibility"`
}

func newSearchCommand(options *rootOptions) *cobra.Command {
	var libraryPath string
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the local skill catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configured, err := config.Load(options.configPath)
			if err != nil {
				return err
			}
			if libraryPath != "" {
				configured.LibraryPath = libraryPath
			}

			skills, _, err := catalog.Discover(configured.LibraryPath)
			if err != nil {
				return err
			}
			matches := search.Match(args[0], skills)
			if jsonOutput {
				return renderSearchJSON(cmd, matches)
			}
			return renderSearchHuman(cmd, matches)
		},
	}
	command.Flags().StringVar(&libraryPath, "library", "", "path to skill library")
	command.Flags().BoolVar(&jsonOutput, "json", false, "render machine-readable JSON")
	return command
}

func renderSearchJSON(cmd *cobra.Command, skills []catalog.Skill) error {
	results := make([]searchOutput, 0, len(skills))
	for _, skill := range skills {
		results = append(results, searchOutput{
			Identifier:    skill.Identifier,
			Description:   skill.Description,
			Tags:          nonNilStrings(skill.Tags),
			Compatibility: nonNilStrings(skill.Compatibility),
		})
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	return encoder.Encode(results)
}

func renderSearchHuman(cmd *cobra.Command, skills []catalog.Skill) error {
	for _, skill := range skills {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\ttags: %s\tcompatibility: %s\n", skill.Identifier, skill.Description, strings.Join(skill.Tags, ", "), strings.Join(skill.Compatibility, ", ")); err != nil {
			return err
		}
	}
	return nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
