package home

// The dashboard: the Dotes tab row and where you are, the logo when there's room, your apps
// as cards (rows of up to 5, evened out), and a glance at the other parts: what's due in
// Notes, which repos need you in Git, your recent projects in Files, and prayer times.
//
//	 󰋜 Home                                Thu 19 Nov · 16:40      Home · Files · Notes · Git
//	 󰉋 ~\Documents\Coding\Dotes   master
//
//	          ╭──────────╮ ╭──────────╮ ╭──────────╮ ╭──────────╮
//	          │    󰕰     │ │         │ │    󰦔     │ │    󰨞     │
//	          │  Herdr   │ │   Nvim   │ │  Player  │ │ VS Code  │
//	          │workspace │ │  editor  │ │  cliamp  │ │   code   │
//	          ╰─── h ────╯ ╰─── n ────╯ ╰─── p ────╯ ╰─── v ────╯
//
//	╭─ 󰠮 Notes ──── 2 due ╮ ╭─  Git ── 1 failing ╮ ╭─ 󰉋 Projects ──╮ ╭─ 󰥔 Maghrib 1h 32m ╮
//	│ ○ buy milk   today  │ │ ● Dotes  3 ↑1      │ │ Dotes         │ │ Fajr 05:34 …     │
//	╰─────────────────────╯ ╰────────────────────╯ ╰───────────────╯ ╰──────────────────╯

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Tahaldr/dotes/internal/rtl"
	"github.com/Tahaldr/dotes/internal/ui"
)

func fg(c color.Color) lipgloss.Style { return ui.FG(c) }
func width(s string) int              { return ui.Width(s) }

const (
	icHome    = "\U000F02DC" // 󰋜
	icFolderO = "\U000F0770" // 󰝰
	icBranch  = ""          //
	icNotes   = "\U000F082E" // 󰠮
	icGit     = ""          //
	icProj    = "\U000F024B" // 󰉋
	icSalat   = "\U000F0954" // 󰥔
	icDot     = "●"
	icTodo    = "\U000F0131" // 󰄱
)

// body: lines already centered in the window, zones with y relative to the first line
type body struct {
	lines []string
	zones []zone
}

// center returns the left padding that centers a block bw cells wide
func center(bw, w int) int { return max(0, (w-bw)/2) }

func blockW(lines []string) int {
	bw := 0
	for _, l := range lines {
		bw = max(bw, width(l))
	}
	return bw
}

// centerBlock pads every line of a block by the same amount (they keep their alignment)
func centerBlock(lines []string, w int) ([]string, int) {
	p := center(blockW(lines), w)
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.Repeat(" ", p) + l
	}
	return out, p
}

func centerIn(s string, w int) string {
	l := max(0, (w-width(s))/2)
	return strings.Repeat(" ", l) + s + strings.Repeat(" ", max(0, w-width(s)-l))
}

func maxLen(apps []appItem, hint bool) int {
	n := 0
	for _, a := range apps {
		if hint {
			n = max(n, width(a.hint))
		} else {
			n = max(n, width(a.name))
		}
	}
	return n
}

// ---- cards ----

const cardH = 5

// card: the icon, the name and its hint; the key sits in the bottom edge
func (m *model) card(a appItem, on bool, cw int) []string {
	t := m.cfg.th
	b, name, hint, key := fg(t.selBg), fg(t.txt), fg(t.mut), fg(t.mut)
	if on {
		b, name, hint, key = fg(t.acc), fg(t.acc).Bold(true), fg(t.selHint), fg(t.acc).Bold(true)
	}
	v := b.Render("│")
	mid := func(s string, st lipgloss.Style) string { return v + st.Render(centerIn(ui.Trunc(s, cw-2), cw)) + v }
	icon := a.icon
	if icon == "" {
		icon = "•"
	}
	bottom := b.Render("╰" + strings.Repeat("─", cw) + "╯")
	if a.key != "" {
		k := " " + a.key + " "
		l := (cw - width(k)) / 2
		bottom = b.Render("╰"+strings.Repeat("─", l)) + key.Render(k) + b.Render(strings.Repeat("─", cw-l-width(k))+"╯")
	}
	return []string{b.Render("╭" + strings.Repeat("─", cw) + "╮"), mid(icon, fg(t.acc)), mid(a.name, name), mid(a.hint, hint), bottom}
}

// grid: up to 5 cards a row, the rows evened out (7 → 4 + 3, 6 → 3 + 3)
func (m *model) grid(w int) body {
	n, gap := len(m.apps), 1
	if n == 0 {
		return body{lines: []string{fg(m.cfg.th.mut).Render("no apps yet · add some in home.txt (dotes config)")}}
	}
	cw := min(16, max(10, max(maxLen(m.apps, false), maxLen(m.apps, true))+2))
	fit := max(1, (w-2+gap)/(cw+2+gap))
	per := min(5, fit, n)
	rows := (n + per - 1) / per
	per = (n + rows - 1) / rows
	m.perRow = per
	var b body
	for s := 0; s < n; s += per {
		if s > 0 {
			b.lines = append(b.lines, "")
		}
		end := min(n, s+per)
		rowLines := make([]string, cardH)
		for i := s; i < end; i++ {
			c := m.card(m.apps[i], i == m.sel, cw)
			for r := range rowLines {
				if i > s {
					rowLines[r] += strings.Repeat(" ", gap)
				}
				rowLines[r] += c[r]
			}
		}
		rowW := (end-s)*(cw+2) + (end-s-1)*gap
		p := center(rowW, w)
		y0 := len(b.lines)
		for i := s; i < end; i++ {
			x := p + (i-s)*(cw+2+gap)
			b.zones = append(b.zones, zone{x, x + cw + 1, y0, y0 + cardH - 1, i})
		}
		for _, r := range rowLines {
			b.lines = append(b.lines, strings.Repeat(" ", p)+r)
		}
	}
	return b
}

// ---- the glance panels ----

type panel struct {
	icon, title string
	badge       string // right of the title (styled)
	lines       []string
	part, path  string   // a click goes there
	rowPaths    []string // a click on a line opens that (same length as lines, "" = the panel's)
}

const panelRows = 3

// panelBox: a quiet box, the title in its top edge and a badge on the right of it
func (m *model) panelBox(p panel, w int) []string {
	t := m.cfg.th
	b := fg(t.selBg)
	inner := w - 2
	title := fg(t.acc).Render(p.icon) + " " + fg(t.txt).Bold(true).Render(p.title)
	top := b.Render("╭─ ") + title + " "
	if p.badge != "" {
		fill := inner - 3 - width(title) - 1 - width(p.badge) - 2
		top += b.Render(strings.Repeat("─", max(1, fill))) + " " + p.badge + " " + b.Render("─╮")
	} else {
		top += b.Render(strings.Repeat("─", max(0, inner-3-width(title)-1)) + "╮")
	}
	out := []string{ui.Trunc(top, w)}
	for i := 0; i < panelRows; i++ {
		l := ""
		if i < len(p.lines) {
			l = p.lines[i]
		}
		out = append(out, b.Render("│")+" "+ui.Pad(l, inner-2)+" "+b.Render("│"))
	}
	return append(out, b.Render("╰"+strings.Repeat("─", inner)+"╯"))
}

// spread2: left text, right text, on one line of w
func spread2(l, r string, w int) string {
	if r == "" {
		return ui.Trunc(l, w)
	}
	l = ui.Trunc(l, max(1, w-width(r)-1))
	return l + strings.Repeat(" ", max(1, w-width(l)-width(r))) + r
}

func (m *model) panels(iw int) []panel {
	t := m.cfg.th
	mu, tx := fg(t.mut), fg(t.txt)
	var out []panel
	if m.on["notes"] && m.notesG != nil {
		g := m.notesG
		p := panel{icon: icNotes, title: "Notes", part: "notes"}
		switch {
		case g.Late > 0:
			p.badge = fg(ui.Red).Render(fmt.Sprintf("%d late", g.Late))
		case g.Today > 0:
			p.badge = fg(ui.Yellow).Render(fmt.Sprintf("%d today", g.Today))
		case g.Open > 0:
			p.badge = mu.Render(fmt.Sprintf("%d open", g.Open))
		}
		for _, it := range g.Items {
			when := ""
			if it.When != "" {
				when = it.WhenStyle.Render(it.When)
			}
			p.lines = append(p.lines, spread2(fg(t.mut).Render(icTodo)+" "+tx.Render(it.Text), when, iw))
		}
		if len(p.lines) == 0 {
			p.lines = []string{mu.Render("nothing to do"), mu.Render(fmt.Sprintf("%d lists · %d notes", g.Lists, g.Notes))}
		} else if g.NothingDue && len(p.lines) < panelRows {
			p.lines = append(p.lines, mu.Render("nothing due · from "+g.Items[0].List))
		}
		out = append(out, p)
	}
	if m.on["git"] && m.gitG != nil {
		g := m.gitG
		p := panel{icon: icGit, title: "Git", part: "git"}
		if n := len(g.Failing); n > 0 {
			p.badge = fg(ui.Red).Render(fmt.Sprintf("✗ %d failing", n))
		} else if n := len(g.Repos); n > 0 {
			p.badge = fg(ui.Orange).Render(fmt.Sprintf("%d to do", n))
		} else {
			p.badge = fg(ui.Green).Render("✓ clean")
		}
		for _, f := range g.Failing {
			if len(p.lines) >= 1 && len(g.Repos) > 0 {
				break // one failing run, the rest for the repos
			}
			p.lines = append(p.lines, fg(ui.Red).Render("✗ ")+tx.Render(f))
			p.rowPaths = append(p.rowPaths, "")
		}
		for _, r := range g.Repos {
			if len(p.lines) >= panelRows {
				break
			}
			var st []string
			if r.Dirty > 0 {
				st = append(st, fg(ui.Orange).Render(fmt.Sprintf("●%d", r.Dirty)))
			}
			if r.Ahead > 0 {
				st = append(st, fg(ui.Green).Render(fmt.Sprintf("↑%d", r.Ahead)))
			}
			if r.Behind > 0 {
				st = append(st, fg(ui.Cyan).Render(fmt.Sprintf("↓%d", r.Behind)))
			}
			p.lines = append(p.lines, spread2(tx.Render(r.Name), strings.Join(st, " "), iw))
			p.rowPaths = append(p.rowPaths, r.Dir)
		}
		if len(p.lines) == 0 {
			p.lines = []string{mu.Render("nothing to commit or push"), mu.Render(fmt.Sprintf("%d repos on this PC", g.Local))}
		}
		out = append(out, p)
	}
	if m.on["files"] && len(m.recent) > 0 {
		p := panel{icon: icProj, title: "Projects", part: "files", badge: mu.Render("recent")}
		for _, d := range m.recent {
			if len(p.lines) >= panelRows {
				break
			}
			name := filepath.Base(d)
			p.lines = append(p.lines, spread2(tx.Render(name), mu.Render(ui.Trunc(filepath.Base(filepath.Dir(d)), iw/2)), iw))
			p.rowPaths = append(p.rowPaths, d)
		}
		out = append(out, p)
	}
	if st := m.prayer.state(m.now); m.cfg.prayer != nil && st != nil {
		// the next three prayers, the next one bright; the countdown on the edge
		p := panel{icon: icSalat, title: "Salat", badge: fg(t.acc).Render("in " + st.in)}
		next := st.next
		if next < 0 {
			next = 0 // after Isha: tomorrow's Fajr
		}
		for k := 0; k < panelRows; k++ {
			i := (next + k) % len(prayerNames)
			name, at := prayerNames[i], st.at[i].Format("15:04")
			if k == 0 {
				p.lines = append(p.lines, spread2(fg(t.acc).Bold(true).Render(icDot+" "+name), fg(t.acc).Bold(true).Render(at), iw))
			} else {
				p.lines = append(p.lines, spread2(mu.Render("  "+name), mu.Render(at), iw))
			}
		}
		out = append(out, p)
	}
	return out
}

// glance lays the panels out in rows that fit (each panel 26 to 40 wide)
func (m *model) glance(w int) body {
	n := len(m.panels(30))
	if n == 0 {
		return body{}
	}
	const gap, minW, maxW = 1, 26, 40
	per := max(1, min(n, (w-2+gap)/(minW+gap)))
	rows := (n + per - 1) / per
	per = (n + rows - 1) / rows
	pw := min(maxW, (w-2-(per-1)*gap)/per)
	ps := m.panels(pw - 4)
	var b body
	for s := 0; s < len(ps); s += per {
		if s > 0 {
			b.lines = append(b.lines, "")
		}
		end := min(len(ps), s+per)
		rowW := (end-s)*pw + (end-s-1)*gap
		x0 := center(rowW, w)
		lines := make([]string, panelRows+2)
		y0 := len(b.lines)
		for i := s; i < end; i++ {
			box := m.panelBox(ps[i], pw)
			for r := range lines {
				if i > s {
					lines[r] += strings.Repeat(" ", gap)
				}
				lines[r] += box[r]
			}
			x := x0 + (i-s)*(pw+gap)
			m.pzones = append(m.pzones, pzone{zone: zone{x, x + pw - 1, y0, y0 + panelRows + 1, -1}, part: ps[i].part})
			for r, rp := range ps[i].rowPaths {
				if rp != "" {
					m.pzones = append(m.pzones, pzone{zone: zone{x, x + pw - 1, y0 + 1 + r, y0 + 1 + r, -1}, part: ps[i].part, path: rp})
				}
			}
		}
		for _, l := range lines {
			b.lines = append(b.lines, strings.Repeat(" ", x0)+l)
		}
	}
	return b
}

// ---- the logo with a two-tone gradient ----

func (m *model) logoLines() []string {
	lines := m.cfg.logo
	n := len(lines)
	out := make([]string, n)
	r1, g1, b1, _ := m.cfg.th.gradFrom.RGBA()
	r2, g2, b2, _ := m.cfg.th.gradTo.RGBA()
	for i, l := range lines {
		f := 0.0
		if n > 1 {
			f = float64(i) / float64(n-1)
		}
		mix := func(a, b uint32) uint8 { return uint8((float64(a>>8) + (float64(b>>8)-float64(a>>8))*f) + 0.5) }
		c := color.RGBA{mix(r1, r2), mix(g1, g2), mix(b1, b2), 255}
		out[i] = fg(c).Render(l)
	}
	return out
}

// ---- the whole screen ----

func (m *model) View() tea.View {
	var content string
	switch {
	case m.w < 20 || m.h < 5:
		content = fg(ui.Mut).Render("Home needs a bigger window.")
	case m.help:
		content = m.keysPage()
	default:
		content = m.dashboard()
	}
	v := tea.NewView(rtl.Frame(content))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "Dotes · Home"
	return v
}

// header: the tab row and where you are (the same two lines as the other parts)
func (m *model) header(w int) []string {
	t := m.cfg.th
	row, _ := ui.TabRow(icHome, "Home", nil, 0, fg(t.mut).Render(m.now.Format("Mon 2 Jan  ·  15:04")), w)
	here := " " + fg(t.acc).Render(icFolderO) + "  " + fg(t.selHint).Render(prettyPath(m.cwd))
	if m.branch != "" {
		here += "   " + fg(ui.Purple).Render(icBranch+" "+m.branch)
	}
	return []string{row, ui.Pad(here, w)}
}

func (m *model) dashboard() string {
	t := m.cfg.th
	w, h := m.w, m.h
	m.pzones = m.pzones[:0]
	head := m.header(w)
	cards := m.grid(w)
	gl := m.glance(w)

	help := "←→ move   enter open   q back to the prompt   x close tab   F1 keys"
	if m.perRow < len(m.apps) {
		help = "arrows move   enter open   q back to the prompt   x close tab   F1 keys"
	}
	if !m.hintsOn {
		help = "F1 keys"
	}
	var foot []string
	if m.note != "" {
		c := t.mut
		if m.noteErr {
			c = ui.Red
		}
		foot = append(foot, centerIn(fg(c).Render(m.note), w))
	}
	if len(m.cfg.warnings) > 0 {
		msg := "home.txt: " + m.cfg.warnings[0]
		if len(m.cfg.warnings) > 1 {
			msg += fmt.Sprintf("  (+%d more)", len(m.cfg.warnings)-1)
		}
		foot = append(foot, centerIn(fg(ui.Red).Render(msg), w))
	}
	foot = append(foot, centerIn(hintLine(help, t), w))

	room := h - len(head) - len(foot)
	// what fits, most important first: the cards, then the glance, then the logo
	type placed struct {
		lines  []string
		zones  []zone
		glance bool
	}
	var blocks []placed
	used := len(cards.lines)
	blocks = append(blocks, placed{cards.lines, cards.zones, false})
	if len(gl.lines) > 0 && used+2+len(gl.lines) <= room {
		blocks = append(blocks, placed{gl.lines, nil, true})
		used += 2 + len(gl.lines)
	} else {
		m.pzones = m.pzones[:0]
	}
	if logo := m.logoLines(); len(logo) > 0 && used+2+len(logo) <= room-2 {
		l, _ := centerBlock(logo, w)
		blocks = append([]placed{{l, nil, false}}, blocks...)
		used += 2 + len(logo)
	}
	top := len(head) + max(0, (room-used)/2)
	screen := make([]string, 0, h)
	screen = append(screen, head...)
	for len(screen) < top {
		screen = append(screen, "")
	}
	m.zones = m.zones[:0]
	for i, bl := range blocks {
		if i > 0 {
			screen = append(screen, "", "")
		}
		y := len(screen)
		for _, z := range bl.zones {
			z.y0, z.y1 = z.y0+y, z.y1+y
			m.zones = append(m.zones, z)
		}
		if bl.glance {
			for k := range m.pzones {
				m.pzones[k].y0 += y
				m.pzones[k].y1 += y
			}
		}
		screen = append(screen, bl.lines...)
	}
	for len(screen) < h-len(foot) {
		screen = append(screen, "")
	}
	screen = append(screen[:min(len(screen), h-len(foot))], foot...)
	for i := range screen {
		screen[i] = ui.Pad(screen[i], w)
	}
	return strings.Join(screen[:min(len(screen), h)], "\n")
}

// hintLine: "key what   key what" with the keys a bit brighter
func hintLine(s string, t theme) string {
	var out []string
	for _, part := range strings.Split(s, "   ") {
		k, d, _ := strings.Cut(part, " ")
		out = append(out, fg(t.selHint).Render(k)+" "+fg(t.mut).Render(d))
	}
	return strings.Join(out, "   ")
}

func (m *model) keysPage() string {
	t := m.cfg.th
	k := func(s string) string { return fg(t.txt).Render(s) }
	mu := func(s string) string { return fg(t.mut).Render(s) }
	h := func(s string) string { return fg(t.gradTo).Render(s) }
	var keys []string
	for _, a := range m.apps {
		if a.key != "" {
			keys = append(keys, a.key)
		}
	}
	first := "the first app"
	var alts []string
	for i, o := range m.cfg.openWith {
		if i == 0 {
			first = o.name
		} else if o.key != "" {
			alts = append(alts, k("alt+"+o.key)+mu(" "+o.name))
		}
	}
	state := "shown"
	if !m.hintsOn {
		state = "hidden (only the keys key)"
	}
	lines := []string{"", "  " + fg(t.acc).Render("Home keys"), "",
		"  " + h("DASHBOARD"),
		"    " + k("arrows") + mu(" / ") + k("tab") + mu(" move   ") + k("enter") + mu(" / click open   ") + k(strings.Join(keys, " ")) + mu(" open directly   ") + k("q") + mu(" / ") + k("esc") + mu(" back to the prompt"),
		"    " + k("x") + mu(" close the terminal tab   ") + k("alt+1-9") + mu(" / the names at the top: the other Dotes parts   ") + k("F1") + mu(" this page"),
		"    " + mu("click a panel to open its part, a line in it to go right there (a repo, a project)"),
		"",
		"  " + h("PROJECT PICKER") + mu("  (an app with  projects  as its command)"),
		"    " + mu("Files opens on your recent projects:   ") + k("enter") + mu(" open in "+first+"   ") + strings.Join(alts, "   ") + "   " + k("l") + mu(" go in   ") + k("q") + mu(" back to Home"),
		"",
		"  " + mu("hint lines: ") + k(state),
		"", "  " + k("h") + mu(map[bool]string{true: " hide", false: " show"}[m.hintsOn]+" hint lines   any other key: back"),
	}
	for i := range lines {
		lines[i] = ui.Pad(lines[i], m.w)
	}
	for len(lines) < m.h {
		lines = append(lines, strings.Repeat(" ", m.w))
	}
	return strings.Join(lines[:m.h], "\n")
}
