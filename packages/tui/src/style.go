package tui

import (
	"strings"
)

// ANSI color and style sequences for zero-dependency high-performance terminal rendering.
const (
	Reset       = "\033[0m"
	Bold        = "\033[1m"
	Dim         = "\033[2m"
	Italic      = "\033[3m"
	Underline   = "\033[4m"
	Inverse     = "\033[7m"

	FgBlack     = "\033[30m"
	FgRed       = "\033[31m"
	FgGreen     = "\033[32m"
	FgYellow    = "\033[33m"
	FgBlue      = "\033[34m"
	FgMagenta   = "\033[35m"
	FgCyan      = "\033[36m"
	FgWhite     = "\033[37m"
	FgGray      = "\033[90m"
	FgBrightRed = "\033[91m"
	FgBrightGreen = "\033[92m"
	FgBrightYellow= "\033[93m"
	FgBrightBlue  = "\033[94m"
	FgBrightMagenta = "\033[95m"
	FgBrightCyan  = "\033[96m"
	FgBrightWhite = "\033[97m"

	BgBlack     = "\033[40m"
	BgRed       = "\033[41m"
	BgGreen     = "\033[42m"
	BgYellow    = "\033[43m"
	BgBlue      = "\033[44m"
	BgMagenta   = "\033[45m"
	BgCyan      = "\033[46m"
	BgWhite     = "\033[47m"
	BgGray      = "\033[100m"
)

// Style applies ANSI styles to a string and resets at the end.
func Style(text string, codes ...string) string {
	if text == "" || len(codes) == 0 {
		return text
	}
	return strings.Join(codes, "") + text + Reset
}

// StripANSI removes ANSI escape codes from string for layout/width calculation.
func StripANSI(str string) string {
	var b strings.Builder
	b.Grow(len(str))
	inEscape := false
	for i := range len(str) {
		c := str[i]
		if c == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inEscape = false
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// RenderMarkdownBlock provides light terminal formatting for markdown elements like code blocks, bullets, and titles.
func RenderMarkdownBlock(content string) string {
	lines := strings.Split(content, "\n")
	var formatted []string
	inCodeBlock := false
	codeLang := ""
	var codeBlockLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				// Flush highlighted code block
				highlighted := HighlightCode(strings.Join(codeBlockLines, "\n"), codeLang)
				for _, hLine := range strings.Split(highlighted, "\n") {
					formatted = append(formatted, "    "+hLine)
				}
				codeBlockLines = nil
				inCodeBlock = false
				formatted = append(formatted, Style("  ```", FgCyan, Dim))
			} else {
				inCodeBlock = true
				codeLang = strings.TrimPrefix(trimmed, "```")
				formatted = append(formatted, Style("  "+trimmed, FgCyan, Dim))
			}
			continue
		}
		if inCodeBlock {
			codeBlockLines = append(codeBlockLines, line)
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			formatted = append(formatted, Style(line, Bold, FgBrightCyan, Underline))
		} else if strings.HasPrefix(trimmed, "## ") {
			formatted = append(formatted, Style(line, Bold, FgCyan))
		} else if strings.HasPrefix(trimmed, "### ") {
			formatted = append(formatted, Style(line, Bold, FgBrightWhite))
		} else if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			formatted = append(formatted, Style("  •", FgYellow)+" "+line[2:])
		} else if strings.HasPrefix(trimmed, "> ") {
			formatted = append(formatted, Style("  │ "+line[2:], FgGray, Italic))
		} else {
			formatted = append(formatted, line)
		}
	}
	if inCodeBlock && len(codeBlockLines) > 0 {
		highlighted := HighlightCode(strings.Join(codeBlockLines, "\n"), codeLang)
		for _, hLine := range strings.Split(highlighted, "\n") {
			formatted = append(formatted, "    "+hLine)
		}
	}
	return strings.Join(formatted, "\n")
}

// RenderDiffLine highlights a git/snapshot diff line.
func RenderDiffLine(line string) string {
	if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
		return Style(line, FgGreen)
	}
	if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
		return Style(line, FgRed)
	}
	if strings.HasPrefix(line, "@@") {
		return Style(line, FgCyan, Dim)
	}
	if strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
		return Style(line, Bold, FgGray)
	}
	return line
}
