package tui

import (
	"regexp"
	"strings"
)

var (
	goKeywords    = []string{"break", "default", "func", "interface", "select", "case", "defer", "go", "map", "struct", "chan", "else", "goto", "package", "switch", "const", "fallthrough", "if", "range", "type", "continue", "for", "import", "return", "var"}
	pyKeywords    = []string{"def", "class", "import", "from", "return", "if", "elif", "else", "for", "while", "try", "except", "with", "as", "pass", "lambda", "yield", "async", "await"}
	jsKeywords    = []string{"function", "const", "let", "var", "return", "if", "else", "for", "while", "import", "export", "from", "class", "extends", "async", "await", "try", "catch", "new", "this"}
	typeNames     = []string{"string", "int", "int64", "bool", "error", "any", "float64", "byte", "rune", "void", "Promise", "Array", "Object", "Number", "String", "Boolean"}
	stringPattern = regexp.MustCompile(`"([^"\\]|\\.)*"|'([^'\\]|\\.)*'|` + "`([^`])*`")
	commentPattern = regexp.MustCompile(`(//.*|#.*)`)
)

// HighlightCode applies syntax highlighting to code snippets based on detected/specified language.
func HighlightCode(code, lang string) string {
	lines := strings.Split(code, "\n")
	var out []string

	for _, line := range lines {
		out = append(out, highlightLine(line, lang))
	}
	return strings.Join(out, "\n")
}

func highlightLine(line, lang string) string {
	if strings.TrimSpace(line) == "" {
		return line
	}

	// Highlight single line comments first
	if commentPattern.MatchString(line) {
		return commentPattern.ReplaceAllStringFunc(line, func(c string) string {
			return Style(c, FgGray, Italic)
		})
	}

	res := line

	// Highlight strings
	res = stringPattern.ReplaceAllStringFunc(res, func(s string) string {
		return Style(s, FgBrightYellow)
	})

	// Highlight keywords
	keywords := goKeywords
	if lang == "python" || lang == "py" {
		keywords = pyKeywords
	} else if lang == "javascript" || lang == "typescript" || lang == "js" || lang == "ts" {
		keywords = jsKeywords
	}

	for _, kw := range keywords {
		pattern := `\b` + kw + `\b`
		re := regexp.MustCompile(pattern)
		res = re.ReplaceAllStringFunc(res, func(k string) string {
			return Style(k, Bold, FgBrightMagenta)
		})
	}

	// Highlight types
	for _, tp := range typeNames {
		pattern := `\b` + tp + `\b`
		re := regexp.MustCompile(pattern)
		res = re.ReplaceAllStringFunc(res, func(t string) string {
			return Style(t, Bold, FgBrightCyan)
		})
	}

	return res
}

// RenderThinkingBlock renders LLM internal reasoning/thinking steps in a distinct muted/italic box.
func RenderThinkingBlock(thought string) string {
	if strings.TrimSpace(thought) == "" {
		return ""
	}
	var out strings.Builder
	out.WriteString(Style("🧠 Thinking / Reasoning:", Bold, FgGray) + "\n")
	lines := strings.Split(strings.TrimSpace(thought), "\n")
	for _, l := range lines {
		out.WriteString(Style("  │ ", FgGray) + Style(l, Italic, FgGray) + "\n")
	}
	return out.String()
}
