package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func normalizePasteText(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
}

func pastedVirtualText(value string) (string, bool) {
	value = normalizePasteText(value)
	lines := strings.Count(value, "\n") + 1
	if lines < 3 && len([]rune(value)) <= 150 {
		return value, false
	}
	return "[Pasted ~" + strconv.Itoa(lines) + " lines]", true
}

func expandTrackedPastedText(value string, parts []schema.MessagePart) string {
	type replacement struct {
		start int
		end   int
		text  string
	}
	replacements := make([]replacement, 0, len(parts))
	for _, part := range parts {
		if part.Type != "text" || part.Source == nil || part.Source.Text == nil {
			continue
		}
		source := part.Source.Text
		if source.Start < 0 || source.End < source.Start || source.End > len([]rune(value)) {
			continue
		}
		replacements = append(replacements, replacement{start: source.Start, end: source.End, text: part.Text})
	}
	sort.SliceStable(replacements, func(i, j int) bool { return replacements[i].start > replacements[j].start })
	for _, item := range replacements {
		runes := []rune(value)
		value = string(append(append(append([]rune{}, runes[:item.start]...), []rune(item.text)...), runes[item.end:]...))
	}
	return value
}

func clonePromptParts(parts []schema.MessagePart) []schema.MessagePart {
	if len(parts) == 0 {
		return nil
	}
	result := make([]schema.MessagePart, len(parts))
	for index, part := range parts {
		result[index] = part
		if part.Source != nil {
			source := *part.Source
			result[index].Source = &source
			if part.Source.Text != nil {
				text := *part.Source.Text
				result[index].Source.Text = &text
			}
		}
	}
	return result
}

// rebasePromptParts applies the same essential range behavior as prompt
// extmarks: edits outside a virtual part move its range, while edits that
// overlap its marker remove the part because its source text is no longer
// represented faithfully in the prompt.
func rebasePromptParts(parts []schema.MessagePart, start, end int, inserted string) []schema.MessagePart {
	if len(parts) == 0 {
		return nil
	}
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	delta := len([]rune(inserted)) - (end - start)
	result := make([]schema.MessagePart, 0, len(parts))
	for _, part := range clonePromptParts(parts) {
		partStart, partEnd, ok := promptPartRange(part)
		if !ok {
			result = append(result, part)
			continue
		}
		if start == end {
			switch {
			case start <= partStart:
				partStart += len([]rune(inserted))
				partEnd += len([]rune(inserted))
			case start == partEnd:
				partEnd += len([]rune(inserted))
			case start > partEnd:
				// The edit is after this part.
			case start > partStart && start < partEnd:
				continue
			}
		} else {
			if start < partEnd && end > partStart {
				continue
			}
			if end <= partStart || start >= partEnd {
				partStart += delta
				partEnd += delta
			}
		}
		setPromptPartRange(&part, partStart, partEnd)
		result = append(result, part)
	}
	return result
}

func promptPartRange(part schema.MessagePart) (int, int, bool) {
	if part.Source == nil {
		return 0, 0, false
	}
	if part.Source.Text != nil {
		return part.Source.Text.Start, part.Source.Text.End, true
	}
	if part.Source.End >= part.Source.Start && part.Source.Value != "" {
		return part.Source.Start, part.Source.End, true
	}
	return 0, 0, false
}

func setPromptPartRange(part *schema.MessagePart, start, end int) {
	if part.Source == nil {
		return
	}
	if part.Source.Text != nil {
		part.Source.Text.Start = start
		part.Source.Text.End = end
		return
	}
	part.Source.Start = start
	part.Source.End = end
}
