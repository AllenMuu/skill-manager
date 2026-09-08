package search_test

import (
	"reflect"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/catalog"
	"github.com/AllenMuu/skill-manager/internal/search"
)

func TestMatchFindsNormalizedTextInEachSearchableField(t *testing.T) {
	skills := []catalog.Skill{
		{Identifier: "go-helper", Description: "Formatting assistance"},
		{Identifier: "database-tools", Description: "Schema work"},
		{Identifier: "release-notes", Description: "Write releases", Tags: []string{"product-management"}},
		{Identifier: "incident-helper", Description: "Respond to incidents", Body: "Use a postmortem template."},
	}

	tests := []struct {
		name        string
		query       string
		identifiers []string
	}{
		{name: "identifier", query: "GO HELPER", identifiers: []string{"go-helper"}},
		{name: "description", query: "formatting", identifiers: []string{"go-helper"}},
		{name: "tag", query: "product management", identifiers: []string{"release-notes"}},
		{name: "body", query: "POSTMORTEM", identifiers: []string{"incident-helper"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := search.Match(tt.query, skills)
			if identifiers := skillIdentifiers(got); !reflect.DeepEqual(identifiers, tt.identifiers) {
				t.Errorf("Match(%q) identifiers = %v, want %v", tt.query, identifiers, tt.identifiers)
			}
		})
	}
}

func TestMatchReturnsIdentifierOrderedCatalogSkills(t *testing.T) {
	skills := []catalog.Skill{
		{Identifier: "zebra", Description: "Go helpers"},
		{Identifier: "alpha", Description: "Go helpers"},
	}

	got := search.Match("go", skills)
	if identifiers := skillIdentifiers(got); !reflect.DeepEqual(identifiers, []string{"alpha", "zebra"}) {
		t.Errorf("Match() identifiers = %v, want [alpha zebra]", identifiers)
	}
}

func TestMatchKeepsCSharpAndCppDistinctFromC(t *testing.T) {
	skills := []catalog.Skill{
		{Identifier: "c-language", Description: "C programming"},
		{Identifier: "cpp-language", Description: "C++ programming"},
		{Identifier: "csharp-language", Description: "C# programming"},
	}

	tests := []struct {
		query       string
		identifiers []string
	}{
		{query: "c", identifiers: []string{"c-language"}},
		{query: "c++", identifiers: []string{"cpp-language"}},
		{query: "c#", identifiers: []string{"csharp-language"}},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			if got := skillIdentifiers(search.Match(tt.query, skills)); !reflect.DeepEqual(got, tt.identifiers) {
				t.Errorf("Match(%q) identifiers = %v, want %v", tt.query, got, tt.identifiers)
			}
		})
	}
}

func TestMatchRetainsSubstringMatchingForOrdinaryTerms(t *testing.T) {
	skills := []catalog.Skill{
		{Identifier: "go-helper", Description: "Formatting assistance"},
	}

	tests := []struct {
		query string
	}{
		{query: "format"},
		{query: "go-hel"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			if got := skillIdentifiers(search.Match(tt.query, skills)); !reflect.DeepEqual(got, []string{"go-helper"}) {
				t.Errorf("Match(%q) identifiers = %v, want [go-helper]", tt.query, got)
			}
		})
	}
}

func skillIdentifiers(skills []catalog.Skill) []string {
	identifiers := make([]string, len(skills))
	for i, skill := range skills {
		identifiers[i] = skill.Identifier
	}
	return identifiers
}
