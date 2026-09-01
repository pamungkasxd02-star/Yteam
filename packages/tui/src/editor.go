package tui

import "strings"

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
	e.value = append(e.value[:e.cursor-1], e.value[e.cursor:]...)
	e.cursor--
}
func (e *Editor) Delete() {
	if e.cursor >= len(e.value) {
		return
	}
	e.value = append(e.value[:e.cursor], e.value[e.cursor+1:]...)
}
func (e *Editor) Left() {
	if e.cursor > 0 {
		e.cursor--
	}
}
func (e *Editor) Right() {
	if e.cursor < len(e.value) {
		e.cursor++
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

func (e *Editor) moveLine(direction int) {
	start := lineStart(e.value, e.cursor)
	column := e.cursor - start
	if direction < 0 {
		if start == 0 {
			return
		}
		previousEnd := start - 1
		previousStart := lineStart(e.value, previousEnd)
		e.cursor = previousStart + min(column, previousEnd-previousStart)
		return
	}
	end := lineEnd(e.value, e.cursor)
	if end == len(e.value) {
		return
	}
	nextStart := end + 1
	nextEnd := lineEnd(e.value, nextStart)
	e.cursor = nextStart + min(column, nextEnd-nextStart)
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
func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
