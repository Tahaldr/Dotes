// Package ui holds the look every Dotes part shares.
package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Muted onedark colors (NvChad's onedark).
var (
	Acc    = lipgloss.Color("#61afef") // accent: hovered item, keys
	Txt    = lipgloss.Color("#abb2bf") // normal text
	Mut    = lipgloss.Color("#565c64") // hints, secondary text
	Hint   = lipgloss.Color("#818792") // a bit brighter than muted
	Dim    = lipgloss.Color("#3e4451") // dividers
	Bg     = lipgloss.Color("#282c34") // dark text on chips
	Green  = lipgloss.Color("#98c379")
	Red    = lipgloss.Color("#e06c75")
	Yellow = lipgloss.Color("#e5c07b")
	Orange = lipgloss.Color("#d19a66")
	Purple = lipgloss.Color("#c678dd")
	Cyan   = lipgloss.Color("#56b6c2")
)

func FG(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

func Width(s string) int { return ansi.StringWidth(s) }

func Trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// Pad cuts or fills s to exactly w columns
func Pad(s string, w int) string {
	s = Trunc(s, w)
	if n := w - Width(s); n > 0 {
		s += strings.Repeat(" ", n)
	}
	return s
}

// Spread puts left and right on one line of width w
func Spread(left, right string, w int) string {
	gap := w - Width(left) - Width(right)
	if gap < 1 {
		right = Trunc(right, max(0, w-Width(left)-1))
		gap = w - Width(left) - Width(right)
	}
	return Pad(left+strings.Repeat(" ", max(0, gap))+right, w)
}
