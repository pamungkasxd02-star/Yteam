package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestTUIDiffAndGitCommands(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := runtime.New(config.Config{Home: home, Model: "test-model"}, root, store, current, provider.New("http://127.0.0.1:1", ""))

	var output bytes.Buffer
	ui := New(app, bytes.NewBuffer(nil), &output)

	handled, err := ui.command(context.Background(), "/diff")
	if !handled || err != nil {
		t.Fatalf("handled=%v, err=%v", handled, err)
	}

	handled, err = ui.command(context.Background(), "/git")
	if !handled || err != nil {
		t.Fatalf("handled=%v, err=%v", handled, err)
	}
}

func TestRenderMarkdownBlockAndDiffLine(t *testing.T) {
	md := "# Title\n```go\nfmt.Println(\"hello\")\n```\n- item 1\n> quote"
	rendered := RenderMarkdownBlock(md)
	if !strings.Contains(rendered, "Title") || !strings.Contains(rendered, "fmt.Println") {
		t.Fatalf("unexpected rendered markdown: %s", rendered)
	}

	diffAdd := RenderDiffLine("+added line")
	if !strings.Contains(diffAdd, "added line") {
		t.Fatalf("diff line rendering failed: %s", diffAdd)
	}

	diffRem := RenderDiffLine("-removed line")
	if !strings.Contains(diffRem, "removed line") {
		t.Fatalf("diff line rendering failed: %s", diffRem)
	}
}

func TestStripANSI(t *testing.T) {
	styled := Style("hello world", Bold, FgBrightGreen)
	stripped := StripANSI(styled)
	if stripped != "hello world" {
		t.Fatalf("expected 'hello world', got %q", stripped)
	}
}

func TestFuzzyMatchScoring(t *testing.T) {
	matched, score1 := fuzzyMatch("mod", "/models")
	if !matched || score1 <= 0 {
		t.Fatalf("expected fuzzy match for 'mod' on '/models', got %v (score %d)", matched, score1)
	}

	matched2, score2 := fuzzyMatch("xyz", "/models")
	if matched2 || score2 != 0 {
		t.Fatalf("expected no fuzzy match for 'xyz' on '/models', got %v", matched2)
	}
}
