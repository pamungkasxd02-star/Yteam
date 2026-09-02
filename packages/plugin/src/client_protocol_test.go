package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestPluginSubprocessProtocolLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := Start(ctx, Config{Command: []string{os.Args[0], "-test.run=TestPluginProtocolHelperProcess"}, Environment: map[string]string{"YTEAM_PLUGIN_HELPER": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Initialize(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	tools, err := client.Tools(ctx)
	if err != nil || len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %#v, err=%v", tools, err)
	}
	result, err := client.CallTool(ctx, "echo", map[string]any{"value": "hello"})
	if err != nil || result != "hello" {
		t.Fatalf("result = %q, err=%v", result, err)
	}
}

func TestPluginProtocolHelperProcess(t *testing.T) {
	if os.Getenv("YTEAM_PLUGIN_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     int64          `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			continue
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"name": "demo"}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "echo", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			arguments, _ := request.Params["arguments"].(map[string]any)
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": arguments["value"]}}}
		default:
			result = map[string]any{}
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}
}
