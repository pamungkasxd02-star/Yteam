package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// JobStatus represents the state of an asynchronous subagent or tool job.
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

// JobInfo tracks background worker activity.
type JobInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Agent     string    `json:"agent"`
	Status    JobStatus `json:"status"`
	Progress  string    `json:"progress,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// JobMonitor tracks and renders live background jobs in TUI split/sidebar views.
type JobMonitor struct {
	mu   sync.RWMutex
	jobs map[string]JobInfo
}

func NewJobMonitor() *JobMonitor {
	return &JobMonitor{
		jobs: make(map[string]JobInfo),
	}
}

func (m *JobMonitor) AddJob(id, name, agent string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[id] = JobInfo{
		ID:        id,
		Name:      name,
		Agent:     agent,
		Status:    JobRunning,
		StartedAt: time.Now().UTC(),
	}
}

func (m *JobMonitor) UpdateJob(id string, status JobStatus, progress string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[id]; ok {
		j.Status = status
		j.Progress = progress
		m.jobs[id] = j
	}
}

func (m *JobMonitor) ActiveJobs() []JobInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []JobInfo
	for _, j := range m.jobs {
		list = append(list, j)
	}
	return list
}

func (m *JobMonitor) RenderSplitPane(width int) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.jobs) == 0 {
		return ""
	}

	var out strings.Builder
	out.WriteString(Style("⚡ Active Background Jobs & Subagents:", Bold, FgBrightCyan) + "\n")
	for _, j := range m.jobs {
		statusMarker := Style("● Running", FgBrightYellow)
		if j.Status == JobCompleted {
			statusMarker = Style("✓ Done", FgBrightGreen)
		} else if j.Status == JobFailed {
			statusMarker = Style("✗ Failed", FgBrightRed)
		}
		out.WriteString(fmt.Sprintf("  [%s] %s (%s) — %s\n", Style(j.ID, Dim), Style(j.Name, Bold), Style(j.Agent, FgCyan), statusMarker))
		if j.Progress != "" {
			out.WriteString(fmt.Sprintf("    %s %s\n", Style("↳", FgGray), Style(j.Progress, Italic, FgGray)))
		}
	}
	return out.String()
}
