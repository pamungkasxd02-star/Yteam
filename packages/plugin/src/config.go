package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type ConfigFile struct {
	Plugins map[string]Config `json:"plugins"`
}

func LoadConfigs(home string) (map[string]Config, error) {
	path := strings.TrimSpace(os.Getenv("YTEAM_PLUGIN_CONFIG"))
	if path == "" && strings.TrimSpace(home) != "" {
		path = filepath.Join(home, "plugins.json")
	}
	configs := map[string]Config{}
	if path == "" {
		return configs, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return configs, nil
	}
	if err != nil {
		return nil, err
	}
	var file ConfigFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	for name, cfg := range file.Plugins {
		if strings.TrimSpace(name) == "" || len(cfg.Command) == 0 {
			return nil, errors.New("plugin name and command are required")
		}
		configs[name] = cfg
	}
	return configs, nil
}
