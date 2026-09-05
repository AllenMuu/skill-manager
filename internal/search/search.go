// Package search finds catalog skills by their searchable metadata and body text.
package search

import (
	"sort"
	"strings"
	"unicode"

	"github.com/AllenMuu/skill-manager/internal/catalog"
)

// Match returns skills whose identifier, name, description, tags, or SKILL.md
// body contain query after case- and separator-normalization. Results are
// ordered by identifier and the input catalog is not modified.
func Match(query string, skills []catalog.Skill) []catalog.Skill {
	normalizedQuery := normalize(query)
	matches := make([]catalog.Skill, 0)
	for _, skill := range skills {
		if normalizedQuery == "" || containsNormalized(searchableText(skill), normalizedQuery) {
			matches = append(matches, skill)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Identifier < matches[j].Identifier
	})
	return matches
}

func searchableText(skill catalog.Skill) string {
	fields := []string{skill.Identifier, skill.Name, skill.Description, skill.Body}
	fields = append(fields, skill.Tags...)
	return normalize(strings.Join(fields, " "))
}

func normalize(value string) string {
	var normalized strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+' || r == '#' {
			normalized.WriteRune(r)
			continue
		}
		normalized.WriteByte(' ')
	}
	return strings.Join(strings.Fields(normalized.String()), " ")
}

func containsNormalized(text, query string) bool {
	if strings.ContainsAny(query, "+#") || len([]rune(query)) == 1 {
		return strings.Contains(" "+text+" ", " "+query+" ")
	}
	return strings.Contains(text, query)
}
