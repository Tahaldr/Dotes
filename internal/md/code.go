package md

// Code colors for Markdown code blocks (shared by every part).

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var codeStyle = styles.Get("onedark")

// colorCode splits code into colored lines (bg = background color, or nil)
func colorCode(lexer chroma.Lexer, text string, bg color.Color) []string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\t", "    ")
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	base := lipgloss.NewStyle().Foreground(cTxt)
	if bg != nil {
		base = base.Background(bg)
	}
	it, err := lexer.Tokenise(nil, text)
	if err != nil {
		out := strings.Split(text, "\n")
		for i := range out {
			out[i] = base.Render(out[i])
		}
		return out
	}
	var lines []string
	cur := ""
	for _, tok := range it.Tokens() {
		st := base
		if e := codeStyle.Get(tok.Type); e.Colour.IsSet() {
			st = st.Foreground(lipgloss.Color(e.Colour.String()))
		}
		for i, p := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				lines = append(lines, cur)
				cur = ""
			}
			if p != "" {
				cur += st.Render(p)
			}
		}
	}
	return append(lines, cur)
}

// ColorCode splits code into colored lines (bg = background color, or nil).
func ColorCode(lexer chroma.Lexer, text string, bg color.Color) []string {
	return colorCode(lexer, text, bg)
}
