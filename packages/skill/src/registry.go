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
		description := strings.TrimSpace(string(data))
		result = append(result, Skill{Name: name, Description: description, Path: path, Body: string(data)})
		return nil
	})
	return result, err
}
