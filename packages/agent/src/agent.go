package agent

type Info struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Mode        string   `json:"mode"`
	Prompt      string   `json:"prompt,omitempty"`
	Tools       []string `json:"tools,omitempty"`
}

func Builtins() []Info {
	return []Info{
		{Name: "build", Description: "Implement changes and run tools", Mode: "build", Prompt: "You are in build mode. Inspect carefully, make requested changes, and verify them.", Tools: []string{"*"}},
		{Name: "plan", Description: "Inspect the project and propose a plan", Mode: "plan", Prompt: "You are in plan mode. Inspect and reason about the project; do not modify files or run commands.", Tools: []string{"read", "list", "glob", "grep", "question"}},
	}
}

func Find(name string) (Info, bool) {
	for _, item := range Builtins() {
		if item.Name == name {
			return item, true
		}
	}
	return Info{}, false
}

func (i Info) AllowsTool(name string) bool {
	for _, allowed := range i.Tools {
		if allowed == "*" || allowed == name {
			return true
		}
	}
	return false
}
