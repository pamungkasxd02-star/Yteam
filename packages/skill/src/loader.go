package skill

import (
	"fmt"
	"strings"
)

func SystemContext(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var out strings.Builder
	fmt.Fprintln(&out, "Available project skills:")
	for _, item := range skills {
		fmt.Fprintf(&out, "- %s: %s\n", item.Name, item.Description)
	}
	return out.String()
}
