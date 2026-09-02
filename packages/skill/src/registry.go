package skill

import (
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Body        string `json:"body"`
}

func Discover(root string) ([]Skill, error) {
	var result []Skill
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := filepath.Base(filepath.Dir(path))
		contents := string(data)
		description := skillDescription(contents)
		result = append(result, Skill{Name: name, Description: description, Path: path, Body: skillBody(contents)})
		return nil
	})
	return result, err
}

func skillDescription(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) == "---" {
				break
			}
			key, field, ok := strings.Cut(line, ":")
			if ok && strings.TrimSpace(key) == "description" {
				return strings.Trim(strings.TrimSpace(field), `"'`)
			}
		}
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

func skillBody(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if !strings.HasPrefix(strings.TrimSpace(value), "---") {
		return strings.TrimSpace(value)
	}
	lines := strings.Split(value, "\n")
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			return strings.TrimSpace(strings.Join(lines[index+1:], "\n"))
		}
	}
	return strings.TrimSpace(value)
}
