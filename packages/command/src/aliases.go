package command

import "strings"

// Alias describes a user-facing slash alias and its canonical OpenCode
// command. Keeping this table in one package prevents CLI, REPL, TUI, and
// autocomplete from drifting apart.
type Alias struct {
	Name        string
	Canonical   string
	Description string
}

var aliases = []Alias{
	{Name: "q", Canonical: "exit", Description: "Exit"},
	{Name: "quit", Canonical: "exit", Description: "Exit"},
	{Name: "resume", Canonical: "sessions", Description: "Switch session"},
	{Name: "continue", Canonical: "sessions", Description: "Switch session"},
	{Name: "clear", Canonical: "new", Description: "Create a session"},
	{Name: "variant", Canonical: "variants", Description: "Choose a model variant"},
	{Name: "agent", Canonical: "agents", Description: "Choose an agent"},
	{Name: "debug", Canonical: "status", Description: "Show status"},
}

func Aliases() []Alias { return append([]Alias(nil), aliases...) }

func Canonical(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "/")
	for _, alias := range aliases {
		if value == alias.Name {
			return alias.Canonical
		}
	}
	return value
}

func AliasItems() []struct {
	ID          string
	Label       string
	Description string
} {
	result := make([]struct {
		ID          string
		Label       string
		Description string
	}, 0, len(aliases))
	for _, alias := range aliases {
		result = append(result, struct {
			ID          string
			Label       string
			Description string
		}{ID: "/" + alias.Name, Label: "/" + alias.Name, Description: alias.Description})
	}
	return result
}
