package tui

import (
	"strings"
	"testing"
)

func TestHighlightCode(t *testing.T) {
	code := `package main

import "fmt"

func main() {
	// print message
	msg := "hello"
	fmt.Println(msg)
}
`
	hl := HighlightCode(code, "go")
	if !strings.Contains(hl, "package") || !strings.Contains(hl, "func") {
		t.Fatalf("unexpected highlighted output: %s", hl)
	}
}

func TestRenderThinkingBlock(t *testing.T) {
	thought := "Thinking about the solution step 1\nStep 2"
	rendered := RenderThinkingBlock(thought)
	if !strings.Contains(rendered, "Thinking / Reasoning:") || !strings.Contains(rendered, "Step 2") {
		t.Fatalf("unexpected thinking block: %s", rendered)
	}
}

func TestJobMonitor(t *testing.T) {
	m := NewJobMonitor()
	m.AddJob("job-1", "Security Scanner", "explore")
	m.UpdateJob("job-1", JobRunning, "Scanning /api routes...")

	jobs := m.ActiveJobs()
	if len(jobs) != 1 || jobs[0].Name != "Security Scanner" {
		t.Fatalf("unexpected jobs: %#v", jobs)
	}

	pane := m.RenderSplitPane(80)
	if !strings.Contains(pane, "Security Scanner") || !strings.Contains(pane, "Scanning /api routes...") {
		t.Fatalf("unexpected pane render: %s", pane)
	}

	m.UpdateJob("job-1", JobCompleted, "Finished scan")
	paneDone := m.RenderSplitPane(80)
	if !strings.Contains(paneDone, "Done") {
		t.Fatalf("unexpected completed status: %s", paneDone)
	}
}
