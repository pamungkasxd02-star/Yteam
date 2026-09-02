package tui

import (
	"unicode"
	"unicode/utf8"
)

// TextUnit is one user-visible text cluster. Start and End are rune indexes,
// which matches Editor's internal representation; Width is terminal-cell
// width. Keeping the cluster boundaries separate from runes prevents cursor
// movement from splitting combining marks and joined emoji sequences.
type TextUnit struct {
	Start int
	End   int
	Width int
}

func textUnits(value []rune) []TextUnit {
	units := make([]TextUnit, 0, len(value))
	for index := 0; index < len(value); {
		end := index + 1
		if value[index] == '\r' && end < len(value) && value[end] == '\n' {
			end++
		} else if isRegionalIndicator(value[index]) && end < len(value) && isRegionalIndicator(value[end]) {
			end++
		}
		for end < len(value) && isGraphemeExtend(value[end]) {
			end++
		}
		for end < len(value) && value[end] == '\u200d' {
			end++
			if end < len(value) {
				end++
				for end < len(value) && isGraphemeExtend(value[end]) {
					end++
				}
			}
		}
		units = append(units, TextUnit{Start: index, End: end, Width: runeClusterWidth(value[index:end])})
		index = end
	}
	return units
}

func isGraphemeExtend(value rune) bool {
	return unicode.Is(unicode.Mn, value) || unicode.Is(unicode.Me, value) ||
		value == '\ufe0e' || value == '\ufe0f' ||
		(value >= 0x1f3fb && value <= 0x1f3ff) || value == '\u20e3'
}

func isRegionalIndicator(value rune) bool { return value >= 0x1f1e6 && value <= 0x1f1ff }

func runeClusterWidth(value []rune) int {
	if len(value) == 0 {
		return 0
	}
	if value[0] == '\n' || value[0] == '\r' {
		return 1
	}
	if value[0] == '\t' {
		return 1
	}
	for _, item := range value {
		if item == '\u200d' {
			return 2
		}
	}
	width := runeWidth(value[0])
	if isRegionalIndicator(value[0]) && len(value) > 1 {
		return 2
	}
	return width
}

func runeWidth(value rune) int {
	if value == '\n' || value == '\r' || value == '\t' || unicode.IsControl(value) ||
		unicode.Is(unicode.Mn, value) || unicode.Is(unicode.Me, value) {
		return 0
	}
	if isWideRune(value) {
		return 2
	}
	return 1
}

// This is the same conservative width policy needed by a terminal editor:
// combining characters occupy no cells, CJK/full-width characters and emoji
// occupy two, and ordinary printable characters occupy one.
func isWideRune(value rune) bool {
	return (value >= 0x1100 && (value <= 0x115f || value == 0x2329 || value == 0x232a ||
		(value >= 0x2e80 && value <= 0xa4cf) || (value >= 0xac00 && value <= 0xd7a3) ||
		(value >= 0xf900 && value <= 0xfaff) || (value >= 0xfe10 && value <= 0xfe19) ||
		(value >= 0xfe30 && value <= 0xfe6f) || (value >= 0xff00 && value <= 0xff60) ||
		(value >= 0xffe0 && value <= 0xffe6))) ||
		(value >= 0x1f000 && value <= 0x1faff)
}

func displayWidth(value string) int {
	runes := []rune(value)
	width := 0
	for _, unit := range textUnits(runes) {
		width += unit.Width
	}
	return width
}

func displayOffsetIndex(value string, offset int) int {
	if offset <= 0 {
		return 0
	}
	runes := []rune(value)
	width := 0
	for _, unit := range textUnits(runes) {
		if width+unit.Width > offset {
			return runeIndexToByte(value, unit.Start)
		}
		width += unit.Width
	}
	return len(value)
}

func runeIndexToByte(value string, index int) int {
	if index <= 0 {
		return 0
	}
	seen, byteIndex := 0, 0
	for byteIndex < len(value) && seen < index {
		_, size := utf8.DecodeRuneInString(value[byteIndex:])
		byteIndex += size
		seen++
	}
	return byteIndex
}

func displaySlice(value string, start, end int) string {
	if end < start {
		end = start
	}
	return value[displayOffsetIndex(value, start):displayOffsetIndex(value, end)]
}

func displayCharAt(value string, offset int) string {
	runes := []rune(value)
	width := 0
	for _, unit := range textUnits(runes) {
		if offset == width || offset < width+unit.Width {
			return string(runes[unit.Start:unit.End])
		}
		width += unit.Width
	}
	return ""
}

func displayUnits(value string) []TextUnit { return textUnits([]rune(value)) }
