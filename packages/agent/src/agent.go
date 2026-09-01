package agent

type Info struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Mode        string `json:"mode"`
}

func Builtins() []Info {
	return []Info{
		{Name: "build", Description: "Implement changes and run tools", Mode: "build"},
		{Name: "plan", Description: "Inspect the project and propose a plan", Mode: "plan"},
	}
}
