// Package catalog discovers directory skills in a local skill library.
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill is an eligible directory skill discovered in a skill library.
type Skill struct {
	Identifier    string
	SourcePath    string
	Name          string
	Description   string
	Body          string
	Tags          []string
	Compatibility []string
	Provenance    string
}

// Diagnostic explains why a library entry is not eligible for management.
type Diagnostic struct {
	Path    string
	Message string
}

// Discover reads immediate child directories of root that contain valid SKILL.md
// files. It only reads metadata files; it never executes skill-provided content.
func Discover(root string) ([]Skill, []Diagnostic, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve skill library path: %w", err)
	}
	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("read skill library: %w", err)
	}

	var skills []Skill
	var diagnostics []Diagnostic
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(absRoot, entry.Name())
		skill, err := readSkill(path, entry.Name())
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: err.Error()})
			continue
		}
		skills = append(skills, skill)
	}
	return skills, diagnostics, nil
}

func readSkill(path, identifier string) (Skill, error) {
	contents, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return Skill{}, fmt.Errorf("missing SKILL.md")
		}
		return Skill{}, fmt.Errorf("read SKILL.md: %w", err)
	}

	name, description, body, err := parseSkillMarkdown(string(contents))
	if err != nil {
		return Skill{}, err
	}
	skill := Skill{
		Identifier:  identifier,
		SourcePath:  path,
		Name:        name,
		Description: description,
		Body:        body,
	}
	metadataPath := filepath.Join(path, ".skill-manager.yaml")
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return skill, nil
		}
		return Skill{}, fmt.Errorf("read companion metadata: %w", err)
	}
	tags, compatibility, provenance, err := parseCompanionMetadata(string(metadata))
	if err != nil {
		return Skill{}, fmt.Errorf("parse companion metadata: %w", err)
	}
	skill.Tags = tags
	skill.Compatibility = compatibility
	skill.Provenance = provenance
	return skill, nil
}

func parseSkillMarkdown(contents string) (name, description, body string, err error) {
	lines := strings.SplitAfter(contents, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", "", fmt.Errorf("SKILL.md must begin with frontmatter")
	}
	frontmatterEnd := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			frontmatterEnd = i
			break
		}
	}
	if frontmatterEnd < 0 {
		return "", "", "", fmt.Errorf("SKILL.md frontmatter is not closed")
	}
	var frontmatter struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:frontmatterEnd], "")), &frontmatter); err != nil {
		return "", "", "", fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	name = frontmatter.Name
	description = frontmatter.Description
	if name == "" || description == "" {
		return "", "", "", fmt.Errorf("SKILL.md frontmatter requires name and description")
	}
	return name, description, strings.Join(lines[frontmatterEnd+1:], ""), nil
}

func parseCompanionMetadata(contents string) (tags, compatibility []string, provenance string, err error) {
	var metadata struct {
		Tags          []string `yaml:"tags"`
		Compatibility []string `yaml:"compatibility"`
		Provenance    string   `yaml:"provenance"`
	}
	if err := yaml.Unmarshal([]byte(contents), &metadata); err != nil {
		return nil, nil, "", err
	}
	return metadata.Tags, metadata.Compatibility, metadata.Provenance, nil
}
