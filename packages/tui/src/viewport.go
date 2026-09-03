package tui

import "strings"
// Viewport is the small, renderer-independent equivalent of OpenCode's
// session scrollbox. It keeps the newest output visible until the user moves
// away from the bottom, then preserves that position while the transcript
// changes.
type Viewport struct {
	Width        int
	Height       int
	Lines        []string
	Offset       int
	FollowBottom bool
}

func NewViewport(width, height int) *Viewport {
	return &Viewport{Width: width, Height: height, FollowBottom: true}
}

func (v *Viewport) SetSize(width, height int) {
	if width > 0 {
		v.Width = width
	}
	if height > 0 {
		v.Height = height
	} else {
		v.Height = 1
	}
	if v.FollowBottom {
		v.toBottom()
	} else {
		v.clamp()
	}
}

func (v *Viewport) SetLines(lines []string) {
	v.Lines = append(v.Lines[:0], lines...)
	if v.FollowBottom {
		v.toBottom()
		return
	}
	v.clamp()
}

func (v *Viewport) Page(direction int) {
	step := v.Height
	if step < 1 {
		step = 1
	}
	v.FollowBottom = false
	v.Offset += direction * step
	v.clamp()
}

func (v *Viewport) Line(direction int) {
	v.FollowBottom = false
	v.Offset += direction
	v.clamp()
}

func (v *Viewport) HalfPage(direction int) {
	step := v.Height / 2
	if step < 1 {
		step = 1
	}
	v.FollowBottom = false
	v.Offset += direction * step
	v.clamp()
}

func (v *Viewport) ToTop() {
	v.FollowBottom = false
	v.Offset = 0
}

func (v *Viewport) First() { v.ToTop() }

func (v *Viewport) Last() { v.ToBottom() }

func (v *Viewport) ToBottom() {
	v.FollowBottom = true
	v.toBottom()
}

func (v *Viewport) Visible() []string {
	v.clamp()
	end := v.Offset + v.Height
	if end > len(v.Lines) {
		end = len(v.Lines)
	}
	return v.Lines[v.Offset:end]
}

func (v *Viewport) toBottom() {
	v.Offset = len(v.Lines) - v.Height
	if v.Offset < 0 {
		v.Offset = 0
	}
}

func (v *Viewport) clamp() {
	if v.Offset < 0 {
		v.Offset = 0
	}
	maxOffset := len(v.Lines) - v.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if v.Offset > maxOffset {
		v.Offset = maxOffset
	}
}

func wrapText(value string, width int) []string {
	if width < 1 {
		width = 1
	}
	result := []string{}
	line := ""
	lineWidth := 0
	flush := func() {
		result = append(result, line)
		line = ""
		lineWidth = 0
	}
	for _, unit := range displayUnits(value) {
		text := []rune(value)[unit.Start:unit.End]
		part := string(text)
		if part == "\n" || part == "\r\n" {
			flush()
			continue
		}
		if lineWidth > 0 && lineWidth+unit.Width > width {
			flush()
		}
		line += part
		lineWidth += unit.Width
	}
	if line != "" || len(result) == 0 {
		result = append(result, line)
	}
	return result
}
func transcriptLines(messages []MessageView, width int) []string {
	lines := []string{}
	for _, message := range messages {
		header := message.Role
		switch message.Role {
		case "user":
			header = Style("👤 You", Bold, FgBrightCyan)
		case "assistant":
			header = Style("🤖 Assistant", Bold, FgBrightGreen)
		case "tool":
			header = Style("🛠️ Tool ("+message.Content+")", Bold, FgBrightYellow)
		case "system":
			header = Style("⚙️ System", Bold, FgMagenta)
		default:
			header = Style(message.Role, Bold)
		}
		lines = append(lines, header)
		if message.Reasoning != "" {
			renderedThinking := RenderThinkingBlock(message.Reasoning)
			for _, part := range strings.Split(renderedThinking, "\n") {
				if part != "" {
					lines = append(lines, wrapText(part, width)...)
				}
			}
		}
		if message.Role != "tool" || message.Content != "" {
			rendered := RenderMarkdownBlock(message.Content)
			for _, part := range strings.Split(rendered, "\n") {
				lines = append(lines, wrapText(part, width)...)
			}
		}
		lines = append(lines, "")
	}
	return lines
}

// MessageView avoids coupling viewport layout to the durable session model.
type MessageView struct {
	Role      string
	Content   string
	Reasoning string
}
