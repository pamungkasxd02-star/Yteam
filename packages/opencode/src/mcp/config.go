package mcp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RemoteConfigFile struct {
	Servers map[string]RemoteConfig `json:"servers"`
}

// LoadRemoteConfigs reads the optional MCP file below application home. The
// environment override is useful for CI and containers with no mounted file.
func LoadRemoteConfigs(home string) (map[string]RemoteConfig, error) {
	configs := map[string]RemoteConfig{}
	path := strings.TrimSpace(os.Getenv("YTEAM_MCP_CONFIG"))
	if path == "" && strings.TrimSpace(home) != "" {
		path = filepath.Join(home, "mcp.json")
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err == nil {
			var file RemoteConfigFile
			if err := json.Unmarshal(data, &file); err != nil {
				return nil, err
			}
			for name, cfg := range file.Servers {
				if strings.TrimSpace(name) == "" || strings.TrimSpace(cfg.URL) == "" {
					return nil, errors.New("MCP server name and URL are required")
				}
				configs[name] = cfg
			}
		}
	}
	if url := strings.TrimSpace(os.Getenv("YTEAM_MCP_URL")); url != "" {
		cfg := RemoteConfig{URL: url, Headers: map[string]string{}}
		if raw := strings.TrimSpace(os.Getenv("YTEAM_MCP_HEADERS")); raw != "" {
			if err := json.Unmarshal([]byte(raw), &cfg.Headers); err != nil {
				return nil, err
			}
		}
		if raw := strings.TrimSpace(os.Getenv("YTEAM_MCP_TIMEOUT")); raw != "" {
			duration, err := time.ParseDuration(raw)
			if err != nil {
				return nil, err
			}
			cfg.Timeout = duration
		} else if raw := strings.TrimSpace(os.Getenv("YTEAM_MCP_TIMEOUT_SECONDS")); raw != "" {
			seconds, err := strconv.Atoi(raw)
			if err != nil || seconds <= 0 {
				return nil, errors.New("invalid YTEAM_MCP_TIMEOUT_SECONDS")
			}
			cfg.Timeout = time.Duration(seconds) * time.Second
		}
		configs["default"] = cfg
	}
	return configs, nil
}
