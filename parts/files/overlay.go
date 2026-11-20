package files

// Popups drawn over the screen: the input (like Nvim's cmdline popup), y / n questions,
// and the task list. Same look as TermGit's.

import (
	"fmt"
	"path/filepath"
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
	onChange    func(m *model, s string) // live: filter / find as you type
	onCancel    func(m *model)
	onTab       func(m *model)

	// Go to: suggestions under the line (↑↓ pick one, tab takes it)
	sugs   []pathSug
	sugSel int // -1 = none picked
	more   int // how many more matched
	known  []string
	sugFor string // the text they're for
}

type question struct {
	label  string
	danger bool
	onYes  func(m *model) tea.Cmd
}

type yankState struct {
	paths []string
	cut   bool
}

// input opens the text popup; cur < 0 puts the cursor at the end
func (m *model) input(title, hint, text string, cur int, onOK func(m *model, s string) tea.Cmd) {
	r := []rune(text)
	if cur < 0 || cur > len(r) {
		cur = len(r)
	}
	m.prompt = &prompt{title: title, hint: hint, text: r, pos: cur, onOK: onOK}
}

func (m *model) ask(label string, danger bool, onYes func(m *model) tea.Cmd) {
	m.question = &question{label: label, danger: danger, onYes: onYes}
}

func (m *model) promptKey(k tea.KeyPressMsg) tea.Cmd {
	p := m.prompt
	before := string(p.text)
	defer func() {
		if m.prompt == p && p.onChange != nil && string(p.text) != before {
			p.onChange(m, string(p.text))
		}
	}()
	switch k.String() {
	case "esc", "ctrl+c":
		m.prompt = nil
		if p.onCancel != nil {
			p.onCancel(m)
		}
	case "up", "down":
		if n := len(p.sugs); n > 0 {
			if k.String() == "down" {
				p.sugSel = (p.sugSel + 1) % n
			} else {
				p.sugSel = (max(p.sugSel, 0) + n - 1) % n
			}
		}
	case "tab":
		if p.onTab != nil {
			p.onTab(m)
		}
	case "enter":
		m.prompt = nil
		if p.sugSel >= 0 && p.sugSel < len(p.sugs) { // a picked suggestion: go there
			return p.onOK(m, p.sugs[p.sugSel].path)
		}
		return p.onOK(m, string(p.text))
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
	default:
		if t := k.Text; t != "" && !strings.ContainsAny(t, "\r\n") {
			m.insertText(t)
		}
	}
	return nil
}

func (m *model) insertText(t string) {
	p := m.prompt
	r := []rune(strings.NewReplacer("\r", "", "\n", "").Replace(t))
	p.text = append(p.text[:p.pos:p.pos], append(r, p.text[p.pos:]...)...)
	p.pos += len(r)
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
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && width(line)+1+width(word) > w {
			out = append(out, st.Render(line))
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		out = append(out, st.Render(line))
	}
	return out
}

func (m *model) overlays(content string) string {
	lines := strings.Split(content, "\n")
	for len(lines) < m.h {
		lines = append(lines, "")
	}
	m.whichKeyPopup(lines)
	m.tasksPopup(lines)
	m.marksPopup(lines)
	m.spotPopup(lines)
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
	before := string(p.text[:p.pos])
	at, after := " ", ""
	if p.pos < len(p.text) {
		at, after = string(p.text[p.pos]), string(p.text[p.pos+1:])
	}
	cursor := lipgloss.NewStyle().Background(cAcc).Foreground(cBg).Render(at)
	input := fg(cAcc).Bold(true).Render("❯ ") + fg(cTxt).Render(before) + cursor + fg(cTxt).Render(after)
	// long text: keep the cursor in view
	if width(input) > inner {
		input = ansi.TruncateLeft(input, width(input)-inner+1, "…")
	}
	body := []string{input}
	if p.hint != "" {
		body = append(body, wrapText(p.hint, inner, fg(cMut))...)
	}
	for i, s := range p.sugs {
		ic := icon{icFile, cHint}
		if s.isDir {
			ic = icon{icFolder, cAcc}
		}
		name, mark, st := s.name, "  ", fg(cTxt)
		if s.isDir {
			name += string(filepath.Separator)
		}
		if i == p.sugSel {
			mark, st = fg(cAcc).Bold(true).Render("❯ "), fg(cAcc).Bold(true)
		}
		row := mark + fg(ic.col).Render(ic.glyph) + " " + st.Render(name)
		if s.where != "" {
			row += fg(cMut).Render("  " + s.where)
		}
		if i == p.sugSel {
			row = lipgloss.NewStyle().Background(cDim).Render(ansi.Truncate(row, inner, "…") + strings.Repeat(" ", max(0, inner-width(row))))
		}
		body = append(body, ansi.Truncate(row, inner, "…"))
	}
	if p.more > 0 {
		body = append(body, fg(cMut).Render(fmt.Sprintf("  … %d more · keep typing", p.more)))
	}
	keys := keyHints("enter", "ok", "esc", "cancel", "ctrl+w", "delete word")
	if p.title == "Go to" {
		keys = keyHints("enter", "go", "tab", "complete", "↑↓", "pick", "esc", "cancel")
	}
	body = append(body, fg(cDim).Render(strings.Repeat("─", inner)), keys)
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

// tasksPopup: w shows every copy / move / delete with a progress bar
func (m *model) tasksPopup(lines []string) {
	if !m.showTasks {
		return
	}
	w := m.popupWidth(56, 90)
	inner := w - 4
	var body []string
	if len(m.tasks) == 0 {
		body = append(body, fg(cMut).Render("No tasks yet: copies, moves and deletes show up here."))
	}
	m.taskCur = max(0, min(m.taskCur, len(m.tasks)-1))
	for i := len(m.tasks) - 1; i >= 0 && len(body) < 3*10; i-- { // newest first
		t := m.tasks[i]
		on := i == m.taskCur
		mark := "  "
		tc := fg(cTxt)
		if on {
			mark, tc = fg(cAcc).Render("❯ "), fg(cAcc).Bold(true)
		}
		state := fmt.Sprintf("%d%%", t.percent())
		sc := fg(cHint)
		switch {
		case t.ended.Load() && t.err == errCanceled:
			state, sc = "canceled", fg(cMut)
		case t.ended.Load() && t.err != nil:
			state, sc = "failed", fg(cRed)
		case t.ended.Load():
			state, sc = "done", fg(cGreen)
		}
		head := mark + tc.Render(t.title())
		body = append(body, head+strings.Repeat(" ", max(1, inner-width(head)-width(state)))+sc.Render(state))
		barW := inner - 4
		fill := barW * t.percent() / 100
		bar := fg(cAcc).Render(strings.Repeat("━", fill)) + fg(cDim).Render(strings.Repeat("━", barW-fill))
		body = append(body, "  "+bar)
		sub := ""
		switch {
		case t.err != nil && t.err != errCanceled:
			sub = fg(cRed).Render(trunc(t.err.Error(), inner-2))
		case !t.ended.Load():
			sub = fg(cMut).Render(trunc(prettyPath(t.cur()), inner-2))
		case t.kind == taskCopy || t.kind == taskMove:
			sub = fg(cMut).Render(humanSize(t.total.Load()))
		}
		body = append(body, "  "+sub)
	}
	body = append(body, fg(cDim).Render(strings.Repeat("─", inner)), keyHints("j/k", "move", "x", "cancel", "esc", "close"))
	title := icTasks + " Tasks"
	if n := m.running(); n > 0 {
		title += fmt.Sprintf(" · %d running", n)
	}
	box := popupBox(fg(cAcc).Bold(true).Render(title), w, body, fg(cAcc))
	put(lines, (m.w-w)/2, 2, box, w)
}

func (m *model) tasksKey(k tea.KeyPressMsg) {
	switch k.String() {
	case "w", "esc", "q":
		m.showTasks = false
	case "k", "up": // the list is newest first
		m.taskCur = min(len(m.tasks)-1, m.taskCur+1)
	case "j", "down":
		m.taskCur = max(0, m.taskCur-1)
	case "x":
		if m.taskCur < len(m.tasks) {
			m.tasks[m.taskCur].cancel.Store(true)
		}
	}
}
