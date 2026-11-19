package notes

// Popups drawn over the screen: the text input (titles), y / n questions and the which-key
// popup. The same look as TermFiles and TermGit. The text editing keys are shared with
// editing a todo in its row.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type prompt struct {
	title, hint string
	text        []rune
	pos         int
	onOK        func(m *model, s string) tea.Cmd
	onCancel    func(m *model, s string)
}

type question struct {
	label  string
	danger bool
	onYes  func(m *model) tea.Cmd
}

func newPrompt(text string, cur int) *prompt {
	r := []rune(text)
	if cur < 0 || cur > len(r) {
		cur = len(r)
	}
	return &prompt{text: r, pos: cur}
}

// input opens the text popup; cur < 0 puts the cursor at the end
func (m *model) input(title, hint, text string, onOK func(m *model, s string) tea.Cmd) {
	p := newPrompt(text, -1)
	p.title, p.hint, p.onOK = title, hint, onOK
	m.prompt = p
}

func (m *model) ask(label string, danger bool, onYes func(m *model) tea.Cmd) {
	m.question = &question{label: label, danger: danger, onYes: onYes}
}

// editKey handles a key for a text being typed; done = enter / esc / ctrl+c ended it
func (p *prompt) editKey(k tea.KeyPressMsg) (done bool, how string) {
	switch s := k.String(); s {
	case "esc", "ctrl+c", "enter":
		return true, s
	case "left", "ctrl+b":
		p.pos = max(0, p.pos-1)
	case "right", "ctrl+f":
		p.pos = min(len(p.text), p.pos+1)
	case "home", "ctrl+a":
		p.pos = 0
	case "end", "ctrl+e":
		p.pos = len(p.text)
	case "backspace", "ctrl+h":
		if p.pos > 0 {
			p.text = append(p.text[:p.pos-1], p.text[p.pos:]...)
			p.pos--
		}
	case "delete", "ctrl+d":
		if p.pos < len(p.text) {
			p.text = append(p.text[:p.pos], p.text[p.pos+1:]...)
		}
	case "ctrl+u":
		p.text, p.pos = p.text[p.pos:], 0
	case "ctrl+k":
		p.text = p.text[:p.pos]
	case "ctrl+w", "ctrl+backspace":
		i := p.pos
		for i > 0 && p.text[i-1] == ' ' {
			i--
		}
		for i > 0 && !strings.ContainsRune(` /\.-_`, p.text[i-1]) {
			i--
		}
		if i == p.pos && i > 0 {
			i--
		}
		p.text = append(p.text[:i], p.text[p.pos:]...)
		p.pos = i
	case "ctrl+left", "alt+b":
		for p.pos > 0 && p.text[p.pos-1] == ' ' {
			p.pos--
		}
		for p.pos > 0 && p.text[p.pos-1] != ' ' {
			p.pos--
		}
	case "ctrl+right", "alt+f":
		for p.pos < len(p.text) && p.text[p.pos] != ' ' {
			p.pos++
		}
		for p.pos < len(p.text) && p.text[p.pos] == ' ' {
			p.pos++
		}
	default:
		if t := k.Text; t != "" {
			p.insert(t)
		}
	}
	return false, ""
}

func (p *prompt) insert(t string) {
	r := []rune(strings.NewReplacer("\r", "", "\n", " ", "\t", " ").Replace(t))
	p.text = append(p.text[:p.pos:p.pos], append(r, p.text[p.pos:]...)...)
	p.pos += len(r)
}

// field draws the text with a block cursor, kept in view within w columns
func (p *prompt) field(w int, st lipgloss.Style) string {
	before := string(p.text[:p.pos])
	at, after := " ", ""
	if p.pos < len(p.text) {
		at, after = string(p.text[p.pos]), string(p.text[p.pos+1:])
	}
	cursor := lipgloss.NewStyle().Background(cAcc).Foreground(cBg).Render(at)
	s := st.Render(before) + cursor + st.Render(after)
	if width(s) > w {
		if width(st.Render(before))+1 > w { // keep the cursor in view
			s = ansi.TruncateLeft(s, width(st.Render(before))+1-w+1, "…")
		}
		s = trunc(s, w)
	}
	return s
}

func (m *model) promptKey(k tea.KeyPressMsg) tea.Cmd {
	p := m.prompt
	done, how := p.editKey(k)
	if !done {
		return nil
	}
	m.prompt = nil
	if how == "enter" {
		return p.onOK(m, strings.TrimSpace(string(p.text)))
	}
	if p.onCancel != nil {
		p.onCancel(m, string(p.text))
	}
	return nil
}

func (m *model) questionKey(k tea.KeyPressMsg) tea.Cmd {
	q := m.question
	switch k.String() {
	case "y", "Y", "enter":
		m.question = nil
		return q.onYes(m)
	case "n", "N", "esc", "q", "ctrl+c":
		m.question = nil
	}
	return nil
}

// ---------------------------------------------------------------------------
// drawing
// ---------------------------------------------------------------------------

// put draws box at x, y over the screen lines
func put(lines []string, x, y int, box []string, w int) {
	for i, b := range box {
		row := y + i
		if row < 0 || row >= len(lines) {
			continue
		}
		base := lines[row]
		left := ansi.Truncate(base, x, "")
		if n := x - ansi.StringWidth(left); n > 0 {
			left += strings.Repeat(" ", n)
		}
		right := ansi.TruncateLeft(base, x+w, "")
		lines[row] = left + "\x1b[0m" + b + "\x1b[0m" + right
	}
}

// popupBox is a rounded box with the title in its top border
func popupBox(title string, w int, body []string, border lipgloss.Style) []string {
	inner := w - 2
	top := "─"
	if title != "" {
		top += " " + title + " "
	}
	top = trunc(top, inner)
	top += strings.Repeat("─", max(0, inner-width(top)))
	out := []string{border.Render("╭") + border.Render(top) + border.Render("╮")}
	for _, l := range body {
		out = append(out, border.Render("│")+" "+pad(l, inner-2)+" "+border.Render("│"))
	}
	return append(out, border.Render("╰"+strings.Repeat("─", inner)+"╯"))
}

func (m *model) popupWidth(lo, hi int) int { return max(min(lo, m.w-4), min(hi, m.w*60/100)) }

func keyHints(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, fg(cHint).Render(pairs[i])+" "+fg(cMut).Render(pairs[i+1]))
	}
	return strings.Join(parts, "   ")
}

// wrapText breaks plain text into lines of at most w columns
func wrapText(s string, w int, st lipgloss.Style) []string {
	var out []string
	for _, l := range wrapPlain(s, w) {
		out = append(out, st.Render(l))
	}
	return out
}

// wrapPlain breaks text at spaces into lines of at most w columns (long words are cut)
func wrapPlain(s string, w int) []string {
	w = max(1, w)
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		for width(word) > w {
			if line != "" {
				out = append(out, line)
				line = ""
			}
			cut := ansi.Truncate(word, w, "")
			out = append(out, cut)
			word = word[len(cut):]
		}
		if line != "" && width(line)+1+width(word) > w {
			out = append(out, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" || len(out) == 0 {
		out = append(out, line)
	}
	return out
}

func (m *model) overlays(content string) string {
	lines := strings.Split(content, "\n")
	for len(lines) < m.h {
		lines = append(lines, "")
	}
	m.whichKeyPopup(lines)
	m.suggestPopup(lines)
	m.finderPopup(lines)
	m.menuPopup(lines)
	m.inputPopup(lines)
	m.questionPopup(lines)
	return strings.Join(lines, "\n")
}

func (m *model) inputPopup(lines []string) {
	p := m.prompt
	if p == nil {
		return
	}
	w := m.popupWidth(50, 80)
	inner := w - 4
	body := []string{fg(cAcc).Bold(true).Render("❯ ") + p.field(inner-2, fg(cTxt))}
	if p.hint != "" {
		body = append(body, wrapText(p.hint, inner, fg(cMut))...)
	}
	body = append(body, fg(cDim).Render(strings.Repeat("─", inner)), keyHints("enter", "ok", "esc", "cancel", "ctrl+w", "delete word"))
	box := popupBox(fg(cAcc).Bold(true).Render(icEdit+" "+p.title), w, body, fg(cAcc))
	put(lines, (m.w-w)/2, 2, box, w)
}

func (m *model) questionPopup(lines []string) {
	q := m.question
	if q == nil {
		return
	}
	w := m.popupWidth(46, 70)
	inner := w - 4
	border, title, yesBg := fg(cAcc), icInfo+" Confirm", cAcc
	if q.danger {
		border, title, yesBg = fg(cRed), icWarn+" Careful", cRed
	}
	body := []string{""}
	body = append(body, wrapText(q.label, inner, fg(cTxt))...)
	body = append(body, "")
	yes := lipgloss.NewStyle().Background(yesBg).Foreground(cBg).Bold(true).Render("  y  Yes  ")
	no := lipgloss.NewStyle().Background(cDim).Foreground(cTxt).Render("  n  No  ")
	buttons := yes + "  " + no + fg(cMut).Render("   esc cancel")
	body = append(body, strings.Repeat(" ", max(0, (inner-width(buttons))/2))+buttons, "")
	box := popupBox(border.Bold(true).Render(title), w, body, border)
	put(lines, (m.w-w)/2, max(2, (m.h-len(box))/2-2), box, w)
}
