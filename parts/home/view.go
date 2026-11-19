package home

// The four layouts (cards, info, palette, list), the themes popup and the keys page.
// Everything is centered like TermNook's dashboard.

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/internal/rtl"
	"github.com/Tahaldr/dotes/internal/ui"
	"github.com/charmbracelet/x/ansi"
)

func fg(c color.Color) lipgloss.Style { return ui.FG(c) }
func width(s string) int              { return ui.Width(s) }

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

// ---- cards: boxes side by side, wrapping into more rows when needed ----

func (m *model) card(a appItem, on bool, cw int) []string {
	t := m.cfg.th
	b, kc := fg(t.selBg), fg(t.mut)
	if on {
		b, kc = fg(t.acc), fg(t.selHint)
	}
	name := a.name
	if width(name) > cw {
		name = ansi.Truncate(name, cw, "…")
	}
	v := b.Render("│")
	mid := func(s string, st lipgloss.Style) string { return v + st.Render(centerIn(s, cw)) + v }
	hz := strings.Repeat("─", cw)
	return []string{b.Render("╭" + hz + "╮"), mid("", fg(t.txt)), mid(a.icon, fg(t.acc)), mid("", fg(t.txt)),
		mid(name, fg(t.txt)), mid(a.key, kc), mid("", fg(t.txt)), b.Render("╰" + hz + "╯")}
}

func (m *model) cards(w int) body {
	n, gap := len(m.apps), 2
	cw := min(22, max(10, maxLen(m.apps, false)+2))
	per := max(1, (w-2+gap)/(cw+2+gap))
	m.perRow = per
	var b body
	for s := 0; s < n; s += per {
		if s > 0 {
			b.lines = append(b.lines, "")
		}
		rows := make([]string, 8)
		end := min(n, s+per)
		for i := s; i < end; i++ {
			c := m.card(m.apps[i], i == m.sel, cw)
			for r := range rows {
				if i > s {
					rows[r] += strings.Repeat(" ", gap)
				}
				rows[r] += c[r]
			}
		}
		rowW := (end-s)*(cw+2) + (end-s-1)*gap
		p := center(rowW, w)
		y0 := len(b.lines)
		for i := s; i < end; i++ {
			x := p + (i-s)*(cw+2+gap)
			b.zones = append(b.zones, zone{x, x + cw + 1, y0, y0 + 7, i})
		}
		for _, r := range rows {
			b.lines = append(b.lines, strings.Repeat(" ", p)+r)
		}
	}
	return b
}

// ---- info: the logo beside live info, apps as a wrapping bar of chips ----

func (m *model) chip(a appItem, on bool) ([]string, int) {
	t := m.cfg.th
	content := ""
	if a.icon != "" {
		content += fg(t.acc).Render(a.icon) + " "
	}
	content += fg(t.txt).Render(a.name)
	if a.key != "" {
		kc := t.mut
		if on {
			kc = t.selHint
		}
		content += " " + fg(kc).Render(a.key)
	}
	cw := width(content)
	if on {
		hz := strings.Repeat("─", cw+2)
		b := fg(t.acc)
		return []string{b.Render("╭" + hz + "╮"), b.Render("│") + " " + content + " " + b.Render("│"), b.Render("╰" + hz + "╯")}, cw + 4
	}
	sp := strings.Repeat(" ", cw+4)
	return []string{sp, "  " + content + "  ", sp}, cw + 4
}

func (m *model) info(w int) body {
	t := m.cfg.th
	type row struct{ k, v string }
	rows := []row{{"time", m.now.Format("15:04")}, {"date", m.now.Format("Monday, Jan 2")}, {"folder", prettyPath(m.cwd)}}
	if m.cfg.github != "" {
		u := "https://github.com/" + m.cfg.github
		rows = append(rows, row{"github", "\x1b]8;;" + u + "\x1b\\" + m.cfg.github + "\x1b]8;;\x1b\\"})
	}
	if br := gitBranch(m.cwd); br != "" {
		rows = append(rows, row{"branch", " " + br})
	}
	rows = append(rows, row{"dotes", app.Version})
	var info []string
	for _, r := range rows {
		info = append(info, fg(t.acc).Render(fmt.Sprintf("%-8s", r.k))+fg(t.txt).Render(r.v))
	}
	logo := m.logoLines()
	gap := 5
	logoW := blockW(logo)
	var top []string
	if len(logo) > 0 && w >= logoW+gap+blockW(info)+2 {
		hgt := max(len(logo), len(info))
		fi, fl := (hgt-len(info))/2, (hgt-len(logo))/2
		for r := 0; r < hgt; r++ {
			l, ri := "", ""
			if j := r - fl; j >= 0 && j < len(logo) {
				l = logo[j]
			}
			if j := r - fi; j >= 0 && j < len(info) {
				ri = info[j]
			}
			top = append(top, ui.Pad(l, logoW)+strings.Repeat(" ", gap)+ri)
		}
	} else {
		top = append(top, logo...)
		if len(logo) > 0 {
			top = append(top, "")
		}
		top = append(top, info...)
	}
	var b body
	tl, _ := centerBlock(top, w)
	b.lines = append(b.lines, tl...)
	// chips, wrapped into rows that fit, each row centered on its own
	type chipT struct {
		lines []string
		w     int
	}
	var chipRows [][]chipT
	var cur []chipT
	cw := 0
	for i, a := range m.apps {
		l, ww := m.chip(a, i == m.sel)
		if len(cur) > 0 && cw+1+ww > w-2 {
			chipRows, cur, cw = append(chipRows, cur), nil, 0
		}
		if len(cur) > 0 {
			cw++
		}
		cw += ww
		cur = append(cur, chipT{l, ww})
	}
	if len(cur) > 0 {
		chipRows = append(chipRows, cur)
	}
	barW := 0
	var bar []string
	var zones []zone
	idx := 0
	for _, r := range chipRows {
		rw := -1
		for _, c := range r {
			rw += c.w + 1
		}
		barW = max(barW, rw)
		p := center(rw, w)
		y0 := len(bar)
		x := p
		for _, c := range r {
			zones = append(zones, zone{x, x + c.w - 1, y0, y0 + 2, idx})
			x += c.w + 1
			idx++
		}
		for k := 0; k < 3; k++ {
			var parts []string
			for _, c := range r {
				parts = append(parts, c.lines[k])
			}
			bar = append(bar, strings.Repeat(" ", p)+strings.Join(parts, " "))
		}
	}
	dw := min(w-2, max(blockW(top), barW))
	b.lines = append(b.lines, "", strings.Repeat(" ", center(dw, w))+fg(t.selBg).Render(strings.Repeat("─", max(0, dw))))
	off := len(b.lines)
	for _, z := range zones {
		z.y0, z.y1 = z.y0+off, z.y1+off
		b.zones = append(b.zones, z)
	}
	b.lines = append(b.lines, bar...)
	return b
}

// ---- palette: type to filter, the selected row gets a border ----

func (m *model) match() {
	f := strings.ToLower(m.filter)
	type hit struct{ i, pos int }
	var hits []hit
	for i, a := range m.apps {
		if f == "" || strings.Contains(strings.ToLower(a.name+" "+a.hint), f) {
			pos := strings.Index(strings.ToLower(a.name), f)
			if pos < 0 {
				pos = 99
			}
			hits = append(hits, hit{i, pos})
		}
	}
	for i := 1; i < len(hits); i++ { // best name match first, else config order
		for j := i; j > 0 && (hits[j].pos < hits[j-1].pos); j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	m.found = m.found[:0]
	for _, h := range hits {
		m.found = append(m.found, h.i)
	}
	m.sel = min(m.sel, max(0, len(m.found)-1))
}

func (m *model) palette(w int) body {
	t := m.cfg.th
	m.match()
	nameCol := max(14, maxLen(m.apps, false)+3)
	hintCol := max(12, maxLen(m.apps, true)+2)
	iw := 2 + 1 + 2 + nameCol + hintCol
	var L []string
	prompt := fg(t.gradTo).Render("❯ ")
	if m.filter != "" {
		prompt += fg(t.txt).Render(m.filter) + fg(t.acc).Render("▏")
	} else {
		prompt += fg(t.acc).Render("▏") + fg(t.mut).Render("type to search")
	}
	L = append(L, " "+prompt)
	rule := fg(t.selBg).Render(strings.Repeat("─", iw+2))
	L = append(L, rule)
	type rz struct{ y0, y1, idx int }
	var rzs []rz
	used := 0
	for p, ai := range m.found {
		a := m.apps[ai]
		on := p == m.sel
		nm := a.name
		name := fg(t.txt).Render(nm)
		if at := strings.Index(strings.ToLower(nm), strings.ToLower(m.filter)); m.filter != "" && at >= 0 {
			e := at + len(m.filter)
			name = fg(t.txt).Render(nm[:at]) + fg(t.acc).Render(nm[at:e]) + fg(t.txt).Render(nm[e:])
		}
		hc := t.mut
		if on {
			hc = t.selHint
		}
		icon := a.icon
		if icon == "" {
			icon = " "
		}
		content := ui.Pad("  "+fg(t.acc).Render(icon)+"  "+name+strings.Repeat(" ", max(0, nameCol-width(nm)))+fg(hc).Render(a.hint), iw)
		if on {
			b := fg(t.acc)
			rzs = append(rzs, rz{len(L), len(L) + 2, p})
			L = append(L, b.Render("╭"+strings.Repeat("─", iw)+"╮"), b.Render("│")+content+b.Render("│"), b.Render("╰"+strings.Repeat("─", iw)+"╯"))
			used += 3
		} else {
			rzs = append(rzs, rz{len(L), len(L), p})
			L = append(L, " "+content+" ")
			used++
		}
	}
	if len(m.found) == 0 {
		L = append(L, fg(t.mut).Render("  no match, esc to clear"))
		used = 1
	}
	for i := used; i < len(m.apps)+2; i++ { // the same height while filtering
		L = append(L, "")
	}
	L = append(L, rule)
	cnt := fmt.Sprintf("%d/%d", len(m.found), len(m.apps))
	L = append(L, strings.Repeat(" ", max(0, iw+2-len(cnt)))+fg(t.mut).Render(cnt))
	lines, p := centerBlock(L, w)
	b := body{lines: lines}
	for _, r := range rzs {
		b.zones = append(b.zones, zone{p, p + iw + 1, r.y0, r.y1, r.idx})
	}
	return b
}

// ---- list: the original vertical menu ----

func (m *model) list(w int) body {
	t := m.cfg.th
	nameCol := max(16, maxLen(m.apps, false)+3)
	hintCol := max(14, maxLen(m.apps, true)+3)
	var L []string
	for i, a := range m.apps {
		icon, key := a.icon, a.key
		if icon == "" {
			icon = " "
		}
		if key == "" {
			key = " "
		}
		name, hint := ui.Pad(a.name, nameCol), ui.Pad(a.hint, hintCol)
		if i == m.sel {
			bg := lipgloss.NewStyle().Background(t.selBg)
			L = append(L, bg.Foreground(t.acc).Render("  "+icon+"  "+name)+bg.Foreground(t.selHint).Render(hint)+bg.Foreground(t.acc).Render(key+"  "))
		} else {
			L = append(L, fg(t.acc).Render("  "+icon+"  ")+fg(t.txt).Render(name)+fg(t.mut).Render(hint+key+"  "))
		}
		if i < len(m.apps)-1 {
			L = append(L, "")
		}
	}
	lines, p := centerBlock(L, w)
	b := body{lines: lines}
	bw := blockW(L)
	for i := range m.apps {
		b.zones = append(b.zones, zone{p, p + bw - 1, i * 2, i * 2, i})
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

func (m *model) dashboard() string {
	t := m.cfg.th
	w, h := m.w, m.h
	var b body
	switch m.style {
	case "cards":
		b = m.cards(w)
	case "info":
		b = m.info(w)
	case "palette":
		b = m.palette(w)
	default:
		b = m.list(w)
	}
	n := len(m.apps)
	help := map[string]string{
		"list":    "↑↓ move   enter open   q back to the prompt   x close tab",
		"palette": "type to filter   ↑↓ move   enter open   esc clear / back   ctrl+x close tab",
	}[m.style]
	if help == "" {
		help = "←→ move   enter open   q back to the prompt   x close tab"
		if m.perRow < n {
			help = "arrows move   enter open   q back to the prompt   x close tab"
		}
	}
	hot := "t themes"
	if m.style == "palette" {
		hot = "ctrl+t themes"
	}
	keysKey := "F1" // the keys page is F1 in every part
	foot := []string{""}
	date := m.now.Format("Monday, Jan 2  ·  15:04")
	title := date
	if m.style == "info" {
		title = ""
	}
	if pb := m.prayerBox(w, title); pb != nil {
		l, _ := centerBlock(pb, w)
		foot = append(foot, l...)
		foot = append(foot, "")
	} else if m.style != "info" {
		foot = append(foot, centerIn(fg(t.mut).Render(date), w))
	}
	hl := fg(t.mut).Render(keysKey + " keys")
	if m.hintsOn {
		hl = fg(t.mut).Render(help+"   ") + fg(t.hot).Render(hot) + fg(t.mut).Render("   "+keysKey+" keys")
	}
	foot = append(foot, centerIn(hl, w))
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
	var head []string
	if m.style != "info" && len(m.cfg.logo) > 0 && len(m.cfg.logo)+2+len(b.lines)+len(foot) <= h {
		head, _ = centerBlock(m.logoLines(), w)
		head = append(head, "", "")
	}
	all := append(append(append([]string{}, head...), b.lines...), foot...)
	if len(all) > h {
		all = all[:h]
	}
	top := max(0, (h-len(all))/2)
	m.zones = m.zones[:0]
	for _, z := range b.zones {
		z.y0, z.y1 = z.y0+top+len(head), z.y1+top+len(head)
		m.zones = append(m.zones, z)
	}
	screen := make([]string, h)
	for i := range screen {
		if j := i - top; j >= 0 && j < len(all) {
			screen[i] = ui.Pad(all[j], w)
		} else {
			screen[i] = strings.Repeat(" ", w)
		}
	}
	if m.popup {
		m.themesPopup(screen)
	}
	return strings.Join(screen, "\n")
}

// themesPopup draws the layout picker over the middle of the screen
func (m *model) themesPopup(screen []string) {
	t := m.cfg.th
	iw := 34
	hot := fg(t.hot)
	v := hot.Render("│")
	title := " themes "
	box := []string{hot.Render("╭─" + title + strings.Repeat("─", iw-1-len(title)) + "╮"), v + strings.Repeat(" ", iw) + v}
	for i, st := range styles {
		label := fmt.Sprintf("%-20s", styleNames[st])
		cur := ""
		if st == m.style {
			cur = "current"
		}
		row := "    " + fg(t.txt).Render(label)
		if i == m.popSel {
			row = "  " + fg(t.acc).Render("❯ "+label)
		}
		box = append(box, v+row+fg(t.mut).Render(fmt.Sprintf("%-*s", iw-24, cur))+v)
	}
	box = append(box, v+strings.Repeat(" ", iw)+v, v+fg(t.mut).Render(fmt.Sprintf("%-*s", iw, "  ↑↓ move  enter apply  esc close"))+v,
		hot.Render("╰"+strings.Repeat("─", iw)+"╯"))
	y0 := max(0, (len(screen)-len(box))/2)
	x0 := max(0, (m.w-iw-2)/2)
	for i, l := range box {
		if y := y0 + i; y < len(screen) {
			s := screen[y]
			screen[y] = ansi.Truncate(s, x0, "") + l + ansi.TruncateLeft(s, x0+iw+2, "")
		}
	}
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
		"    " + fg(t.hot).Render("t") + mu(" themes   ") + fg(t.hot).Render("x") + mu(" close the terminal tab   (") + fg(t.hot).Render("ctrl+t") + mu(" / ") + fg(t.hot).Render("ctrl+x") + mu(" in the palette)"),
		"    " + k("? ~ F1") + mu(" this page   ") + k("alt+1-9") + mu(" other Dotes parts"),
		"",
		"  " + h("PROJECT PICKER") + mu("  (an app with  projects  as its command)"),
		"    " + mu("Files opens on your recent projects:   ") + k("enter") + mu(" open in "+first+"   ") + strings.Join(alts, "   ") + "   " + k("l") + mu(" go in   ") + k("q") + mu(" back to the prompt in that folder"),
		"",
		"  " + mu("hint lines: ") + k(state),
		"", "  " + fg(t.hot).Render("h") + mu(map[bool]string{true: " hide", false: " show"}[m.hintsOn]+" hint lines   any other key: back"),
	}
	for i := range lines {
		lines[i] = ui.Pad(lines[i], m.w)
	}
	for len(lines) < m.h {
		lines = append(lines, strings.Repeat(" ", m.w))
	}
	return strings.Join(lines[:m.h], "\n")
}
