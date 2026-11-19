package notes

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/Tahaldr/dotes/internal/ui"
)

// Colors come from the shared Dotes look.
var (
	cAcc    = ui.Acc
	cTxt    = ui.Txt
	cMut    = ui.Mut
	cHint   = ui.Hint
	cDim    = ui.Dim
	cBg     = ui.Bg
	cGreen  = ui.Green
	cRed    = ui.Red
	cYellow = ui.Yellow
	cOrange = ui.Orange
	cPurple = ui.Purple
	cCyan   = ui.Cyan
)

func fg(c color.Color) lipgloss.Style { return ui.FG(c) }
func width(s string) int              { return ui.Width(s) }
func trunc(s string, w int) string    { return ui.Trunc(s, w) }
func pad(s string, w int) string      { return ui.Pad(s, w) }
