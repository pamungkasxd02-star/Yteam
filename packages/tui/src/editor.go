package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Editor struct {
	value         []rune
	cursor        int
	history       []string
	historyCursor int
	saved         string
}

func NewEditor() *Editor           { return &Editor{historyCursor: -1} }
func (e *Editor) String() string   { return string(e.value) }
func (e *Editor) Empty() bool      { return strings.TrimSpace(e.String()) == "" }
func (e *Editor) Cursor() int      { return e.cursor }
func (e *Editor) Set(value string) { e.value = []rune(value); e.cursor = len(e.value) }
func (e *Editor) Reset()           { e.value = nil; e.cursor = 0; e.historyCursor = -1; e.saved = "" }
func (e *Editor) Insert(value string) {
	runes := []rune(value)
	if len(runes) == 0 {
		return
	}
	e.value = append(e.value[:e.cursor], append(runes, e.value[e.cursor:]...)...)
	e.cursor += len(runes)
}
func (e *Editor) Newline() { e.Insert("\n") }
func (e *Editor) Backspace() {
	if e.cursor == 0 {
		return
	}
	start := previousClusterStart(e.value, e.cursor)
	e.value = append(e.value[:start], e.value[e.cursor:]...)
	e.cursor = start
}
func (e *Editor) Delete() {
	if e.cursor >= len(e.value) {
		return
	}
	end := nextClusterEnd(e.value, e.cursor)
	e.value = append(e.value[:e.cursor], e.value[end:]...)
}

func (e *Editor) WordLeft() {
	for e.cursor > 0 && isEditorSpace(e.value[e.cursor-1]) {
		e.cursor = previousClusterStart(e.value, e.cursor)
	}
	for e.cursor > 0 && !isEditorSpace(e.value[e.cursor-1]) {
		e.cursor = previousClusterStart(e.value, e.cursor)
	}
}

func (e *Editor) WordRight() {
	for e.cursor < len(e.value) && isEditorSpace(e.value[e.cursor]) {
		e.cursor = nextClusterEnd(e.value, e.cursor)
	}
	for e.cursor < len(e.value) && !isEditorSpace(e.value[e.cursor]) {
		e.cursor = nextClusterEnd(e.value, e.cursor)
	}
}

func (e *Editor) DeleteWordBackward() {
	end := e.cursor
	e.WordLeft()
	e.value = append(e.value[:e.cursor], e.value[end:]...)
}

func (e *Editor) DeleteWordForward() {
	start := e.cursor
	e.WordRight()
	e.value = append(e.value[:start], e.value[e.cursor:]...)
	e.cursor = start
}

func isEditorSpace(value rune) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}

func (e *Editor) Left() {
	if e.cursor > 0 {
		e.cursor = previousClusterStart(e.value, e.cursor)
	}
}
func (e *Editor) Right() {
	if e.cursor < len(e.value) {
		e.cursor = nextClusterEnd(e.value, e.cursor)
	}
}
func (e *Editor) Home() { e.cursor = lineStart(e.value, e.cursor) }
func (e *Editor) End()  { e.cursor = lineEnd(e.value, e.cursor) }
func (e *Editor) Up()   { e.moveLine(-1) }
func (e *Editor) Down() { e.moveLine(1) }
func (e *Editor) AddHistory(value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if len(e.history) == 0 || e.history[len(e.history)-1] != value {
		e.history = append(e.history, value)
	}
	e.historyCursor = -1
}
func (e *Editor) HistoryUp() bool {
	if len(e.history) == 0 {
		return false
	}
	if e.historyCursor == -1 {
		e.saved = e.String()
		e.historyCursor = len(e.history)
	}
	if e.historyCursor > 0 {
		e.historyCursor--
	}
	e.Set(e.history[e.historyCursor])
	return true
}
func (e *Editor) HistoryDown() bool {
	if e.historyCursor == -1 {
		return false
	}
	if e.historyCursor < len(e.history)-1 {
		e.historyCursor++
		e.Set(e.history[e.historyCursor])
		return true
	}
	e.historyCursor = -1
	e.Set(e.saved)
	return true
}

// openExternalEditor implements OpenCode's VISUAL/EDITOR prompt flow. The
// caller owns terminal suspension/restoration around this operation.
func openExternalEditor(ctx context.Context, value, cwd, tempDir string, stdin io.Reader, stdout, stderr io.Writer) (string, error) {
	editor := firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"))
	if editor == "" {
		return "", fmt.Errorf("VISUAL or EDITOR is not set")
	}
	if strings.TrimSpace(tempDir) == "" {
		return "", fmt.Errorf("application home is not configured for editor temporary files")
	}
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(tempDir, "yteam-prompt-*.md")
	if err != nil {
		return "", err
	}
	path := temporary.Name()
	defer os.Remove(path)
	if _, err := temporary.WriteString(value); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	args, err := splitEditorCommand(editor)
	if err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, args[0], append(args[1:], path)...)
	if cwd != "" {
		if resolved, resolveErr := filepath.Abs(cwd); resolveErr == nil {
			if info, statErr := os.Stat(resolved); statErr == nil && info.IsDir() {
				command.Dir = resolved
			}
		}
	}
	command.Stdin, command.Stdout, command.Stderr = stdin, stdout, stderr
	if err := command.Run(); err != nil {
		return "", err
	}
	result, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func splitEditorCommand(value string) ([]string, error) {
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			parts = append(parts, current.String())
			current.Reset()
		}
	}
	for _, char := range value {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case ' ', '\t', '\r', '\n':
			flush()
		default:
			current.WriteRune(char)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in editor command")
	}
	flush()
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty editor command")
	}
	return parts, nil
}

func (e *Editor) moveLine(direction int) {
	start := lineStart(e.value, e.cursor)
	column := clusterColumn(e.value, start, e.cursor)
	if direction < 0 {
		if start == 0 {
			return
		}
		previousEnd := start - 1
		previousStart := lineStart(e.value, previousEnd)
		e.cursor = lineColumnCursor(e.value, previousStart, previousEnd, column)
		return
	}
	end := lineEnd(e.value, e.cursor)
	if end == len(e.value) {
		return
	}
	nextStart := end + 1
	nextEnd := lineEnd(e.value, nextStart)
	e.cursor = lineColumnCursor(e.value, nextStart, nextEnd, column)
}

func clusterColumn(value []rune, start, cursor int) int {
	column := 0
	for _, unit := range textUnits(value[start:cursor]) {
		column += unit.Width
	}
	return column
}

func lineColumnCursor(value []rune, start, end, column int) int {
	if column <= 0 {
		return start
	}
	for _, unit := range textUnits(value[start:end]) {
		if column < unit.Width {
			return start + unit.Start
		}
		if column == unit.Width {
			return start + unit.End
		}
		column -= unit.Width
	}
	return end
}

func lineStart(value []rune, cursor int) int {
	for cursor > 0 && value[cursor-1] != '\n' {
		cursor--
	}
	return cursor
}
func lineEnd(value []rune, cursor int) int {
	for cursor < len(value) && value[cursor] != '\n' {
		cursor++
	}
	return cursor
}

func previousClusterStart(value []rune, cursor int) int {
	units := textUnits(value)
	for index := len(units) - 1; index >= 0; index-- {
		if units[index].End == cursor {
			return units[index].Start
		}
	}
	return max(0, cursor-1)
}

func nextClusterEnd(value []rune, cursor int) int {
	for _, unit := range textUnits(value) {
		if unit.Start == cursor {
			return unit.End
		}
	}
	return min(len(value), cursor+1)
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
