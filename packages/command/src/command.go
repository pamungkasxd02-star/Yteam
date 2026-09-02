package command

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Source string

const (
	SourceCommand Source = "command"
	SourceMCP     Source = "mcp"
	SourceSkill   Source = "skill"
)

type Info struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Agent       string   `json:"agent,omitempty"`
	Model       string   `json:"model,omitempty"`
	Variant     string   `json:"variant,omitempty"`
	Source      Source   `json:"source,omitempty"`
	Template    string   `json:"template"`
	Subtask     bool     `json:"subtask,omitempty"`
	Hints       []string `json:"hints"`
}

const (
	DefaultInit   = "init"
	DefaultReview = "review"
)

var argumentPattern = regexp.MustCompile(`\$[1-9]`)

func Hints(template string) []string {
	seen := map[string]bool{}
	for _, match := range argumentPattern.FindAllString(template, -1) {
		seen[match] = true
	}
	if strings.Contains(template, "$ARGUMENTS") {
		seen["$ARGUMENTS"] = true
	}
	result := make([]string, 0, len(seen))
	for item := range seen {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func Expand(template string, args []string) string {
	result := strings.ReplaceAll(template, "$ARGUMENTS", strings.Join(args, " "))
	for index, value := range args {
		result = strings.ReplaceAll(result, fmt.Sprintf("$%d", index+1), value)
	}
	return result
}

func Builtins(root string) []Info {
	return []Info{
		{Name: DefaultInit, Description: "guided AGENTS.md setup", Source: SourceCommand, Template: initTemplate(root), Hints: Hints(initTemplate(root))},
		{Name: DefaultReview, Description: "review changes [commit|branch|pr], defaults to uncommitted", Source: SourceCommand, Template: reviewTemplate(root), Subtask: true, Hints: Hints(reviewTemplate(root))},
	}
}

func Discover(root string) ([]Info, error) {
	result := Builtins(root)
	seen := map[string]bool{DefaultInit: true, DefaultReview: true}
	paths := []string{filepath.Join(root, "command"), filepath.Join(root, "commands"), filepath.Join(root, ".opencode", "command"), filepath.Join(root, ".opencode", "commands")}
	for _, directory := range paths {
		if err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) {
				return filepath.SkipDir
			}
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				return nil
			}
			item, err := loadMarkdown(path, directory)
			if err != nil {
				return err
			}
			if seen[item.Name] {
				return nil
			}
			seen[item.Name] = true
			result = append(result, item)
			return nil
		}); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func loadMarkdown(path, directory string) (Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	front, body, err := splitFrontmatter(string(data))
	if err != nil {
		return Info{}, fmt.Errorf("invalid command %s: %w", path, err)
	}
	relative, err := filepath.Rel(directory, path)
	if err != nil {
		return Info{}, err
	}
	name := strings.TrimSuffix(filepath.ToSlash(relative), filepath.Ext(relative))
	name = strings.TrimPrefix(name, "./")
	info := Info{Name: name, Source: SourceCommand, Template: strings.TrimSpace(body)}
	for key, value := range front {
		switch key {
		case "description":
			info.Description = value
		case "agent":
			info.Agent = value
		case "model":
			info.Model = value
		case "variant":
			info.Variant = value
		case "subtask":
			info.Subtask = strings.EqualFold(value, "true")
		case "template":
			if info.Template == "" {
				info.Template = value
			}
		}
	}
	if info.Template == "" {
		return Info{}, errors.New("command template is empty")
	}
	info.Hints = Hints(info.Template)
	return info, nil
}

func splitFrontmatter(input string) (map[string]string, string, error) {
	front := map[string]string{}
	reader := bufio.NewScanner(strings.NewReader(input))
	if !reader.Scan() || strings.TrimSpace(reader.Text()) != "---" {
		return front, input, nil
	}
	for reader.Scan() {
		line := reader.Text()
		if strings.TrimSpace(line) == "---" {
			body := []string{}
			for reader.Scan() {
				body = append(body, reader.Text())
			}
			return front, strings.Join(body, "\n"), reader.Err()
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, "", errors.New("frontmatter must contain key: value lines")
		}
		front[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return nil, "", errors.New("frontmatter is not closed")
}

func initTemplate(root string) string {
	return "Create or update `AGENTS.md` for this repository.\n\nThe repository root is: " + root + "\n\nRead the project README, manifests, build and test configuration, and existing instruction files before writing only high-signal guidance."
}

func reviewTemplate(root string) string {
	return "Review the repository changes and provide actionable feedback.\n\nRepository root: " + root + "\n\nInspect the diff and the full modified files. Focus on bugs, security, behavior changes, and reproducibility."
}
