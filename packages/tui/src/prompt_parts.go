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
