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
		{Name: "build", Description: "The default agent. Executes tools based on configured permissions.", Mode: "primary", Prompt: "You are in build mode. Inspect carefully, make requested changes, and verify them.", Tools: []string{"*"}},
		{Name: "plan", Description: "Plan mode. Disallows all edit tools.", Mode: "primary", Prompt: "You are in plan mode. Inspect and reason about the project; do not modify files or run commands.", Tools: []string{"read", "list", "glob", "grep", "question"}},
		{Name: "explore", Description: "Fast agent specialized for exploring codebases, patterns and keywords.", Mode: "subagent", Prompt: "You are an exploratory agent. Search codebases quickly using read, list, glob, grep and explain the structure.", Tools: []string{"read", "list", "glob", "grep", "bash"}},
		{Name: "general", Description: "General-purpose agent for researching complex questions and executing multi-step tasks.", Mode: "subagent", Prompt: "You are a general research agent. Analyze complex questions and coordinate solutions.", Tools: []string{"*"}},
		{Name: "compaction", Description: "Summarizes session message histories when context fills up.", Mode: "primary", Prompt: "Summarize the above session compactly preserving key context and findings.", Tools: []string{}},
		{Name: "title", Description: "Generates concise session titles from initial prompts.", Mode: "primary", Prompt: "Generate a 2-5 word concise descriptive title for this conversation.", Tools: []string{}},
		{Name: "summary", Description: "Generates quick session summaries.", Mode: "primary", Prompt: "Provide a 1-sentence summary of the conversation.", Tools: []string{}},
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
