package files

// F: read the file under the cursor full screen, like Git's README reader. It reads much more
// of the file than the side preview, draws tables (csv / tsv) with every column at its real
// width, and scrolls sideways (h / l) for what's wider than the window.

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type reader struct {
	e         entry
	p         preview
	loading   bool
	top, left int
	lines     []string // drawn for width w
	w         int
	wraps     bool // markdown: wrapped to the window, nothing to scroll sideways
}

type readerMsg struct {
	path string
	p    preview
}

// openReader: F on a file
func (m *model) openReader() tea.Cmd {
	c := m.current()
	if c == nil || c.isDir || c.drive {
		m.setNote("F reads a file full screen: put the cursor on a file", false)
		return nil
	}
	m.rd = &reader{e: *c, loading: true}
	return m.readerLoad()
}

func (m *model) readerLoad() tea.Cmd {
	e, hidden := m.rd.e, m.showHidden
	load := func() tea.Msg { return readerMsg{e.path, makePreviewLim(e, hidden, fullLimits)} }
	if m.dump {
		m.update(load())
		return nil
	}
	return load
}

// readerCheck: the file changed on disk (an editor, another window): read it again
func (m *model) readerCheck() tea.Cmd {
	r := m.rd
	if r == nil || r.loading {
		return nil
	}
	fi, err := os.Stat(r.e.path)
	if err != nil || (fi.ModTime().Equal(r.e.mod) && fi.Size() == r.e.size) {
		return nil
	}
	r.e.mod, r.e.size = fi.ModTime(), fi.Size()
	r.loading = true
	return m.readerLoad()
}

func (m *model) readerGot(msg readerMsg) {
	r := m.rd
	if r == nil || r.e.path != msg.path {
		return
	}
	r.p, r.loading, r.lines, r.w = msg.p, false, nil, 0
}

// readerLines: the file drawn for width w (kept until the width or the file changes)
func (m *model) readerLines(w, h int) []string {
	r := m.rd
	if r.lines != nil && r.w == w && r.p.kind != "image" {
		return r.lines
	}
	r.w, r.wraps = w, false
	switch r.p.kind {
	case "md":
		r.lines, r.wraps = renderMarkdown(r.p.src, w), true
	case "csv":
		r.lines = csvTable(r.p.src, strings.Contains(strings.SplitN(r.p.src, "\n", 2)[0], "\t"))
	case "image":
		if m.gfx == "sixel" && !m.popupOpen() {
			r.lines = m.sixelPreview(&r.p, w, h, 1, m.bodyY)
		} else {
			r.lines = r.p.richLines(w, h)
		}
		r.wraps = true
	case "":
		r.lines = r.p.lines
	default:
		r.lines, r.wraps = r.p.richLines(w, h), true
	}
	return r.lines
}

func (m *model) readerView() string {
	m.imgWant = imgSpot{}
	r := m.rd
	hints := []string{" " + keyHints("j/k", "scroll", "h/l", "sideways", "g/G", "top/end", "e", "edit", "o", "open", "E", "Explorer", "q", "back")}
	if !m.hintsOn {
		hints = []string{" " + keyHints("q", "back", "F1", "keys")}
	}
	m.bodyY = 2
	m.bodyH = max(1, m.h-m.bodyY-len(hints)-1)
	h, w := m.bodyH, m.w-2
	ic := iconFor(&r.e)
	title := " " + fg(ic.col).Render(ic.glyph) + "  " + fg(cHint).Render(prettyPath(filepath.Dir(r.e.path))+string(filepath.Separator)) +
		fg(cAcc).Bold(true).Render(r.e.name)

	body := make([]string, h)
	right := ""
	if r.loading {
		body[0] = " " + fg(cMut).Render("reading…")
	} else {
		lines := m.readerLines(w, h)
		widest := 0
		if !r.wraps {
			for _, l := range lines {
				widest = max(widest, ansi.StringWidth(l))
			}
		}
		r.left = max(0, min(r.left, widest-w))
		r.top = max(0, min(r.top, len(lines)-h))
		for i := 0; i < h && r.top+i < len(lines); i++ {
			l := lines[r.top+i]
			if r.left > 0 {
				l = ansi.Cut(l, r.left, r.left+w)
			}
			body[i] = " " + trunc(l, w)
		}
		if len(lines) > h {
			right = fmt.Sprintf("%d%%", min(100, (r.top+h)*100/len(lines)))
		}
		if widest > w {
			right = fmt.Sprintf("col %d  ", r.left+1) + right
		}
	}

	right = fg(cMut).Render(right) + " "
	if room := m.w - width(right) - 2; width(title) > room { // a long path: keep the name, cut the folder
		head := " " + fg(ic.col).Render(ic.glyph) + "  "
		dir := []rune(prettyPath(filepath.Dir(r.e.path)) + string(filepath.Separator))
		keep := room - width(head) - width(r.e.name)
		d := ""
		if keep > 1 {
			d = "…" + string(dir[max(0, len(dir)-keep+1):])
		}
		title = trunc(head+fg(cHint).Render(d)+fg(cAcc).Bold(true).Render(r.e.name), room)
	}
	out := []string{m.tabBar(), spread(title, right, m.w)}
	out = append(out, body...)
	out = append(out, m.statusBar())
	for _, l := range hints {
		out = append(out, trunc(l, m.w))
	}
	return strings.Join(out, "\n")
}

func (m *model) readerKey(s string) tea.Cmd {
	r := m.rd
	switch s {
	case "q", "esc", "F", "backspace":
		m.rd = nil
	case "j", "down":
		r.top++
	case "k", "up":
		r.top = max(0, r.top-1)
	case "ctrl+d", "space", " ":
		r.top += m.half()
	case "ctrl+u":
		r.top = max(0, r.top-m.half())
	case "ctrl+f", "pgdown":
		r.top += m.page()
	case "ctrl+b", "pgup":
		r.top = max(0, r.top-m.page())
	case "g", "home":
		r.top, r.left = 0, 0
	case "G", "end":
		r.top = 1 << 30
	case "l", "right":
		r.left += 8
	case "h", "left":
		r.left = max(0, r.left-8)
	case "L", "$":
		r.left = 1 << 30
	case "H", "0":
		r.left = 0
	case "e":
		return m.editFile(r.e, r.top+1)
	case "o", "enter":
		if m.dump {
			m.setNote("would open "+r.e.name, false)
			return nil
		}
		if err := openDefault(r.e.path); err != nil {
			m.setNote("couldn't open "+r.e.name+": "+err.Error(), true)
		}
	case "E":
		m.showInExplorer(r.e.path)
	case "~", "f1":
		m.help = true
	case "ctrl+c":
		m.rd = nil
	}
	return nil
}

func (m *model) readerWheel(w tea.MouseWheelMsg) {
	r := m.rd
	switch w.Button {
	case tea.MouseWheelDown:
		if w.Mod&tea.ModShift != 0 {
			r.left += 8
		} else {
			r.top += 3
		}
	case tea.MouseWheelUp:
		if w.Mod&tea.ModShift != 0 {
			r.left = max(0, r.left-8)
		} else {
			r.top = max(0, r.top-3)
		}
	case tea.MouseWheelRight:
		r.left += 8
	case tea.MouseWheelLeft:
		r.left = max(0, r.left-8)
	}
}

// showInExplorer opens Windows Explorer with the file picked (a folder: opened)
func (m *model) showInExplorer(path string) {
	if m.dump {
		m.setNote("would show "+filepath.Base(path)+" in Explorer", false)
		return
	}
	if err := exec.Command("explorer.exe", "/select,", path).Start(); err != nil {
		m.setNote("couldn't open Explorer: "+err.Error(), true)
		return
	}
	m.setNote("shown in Explorer: "+filepath.Base(path), false)
}

// csvTable draws csv / tsv text as a table, every column as wide as its widest cell
// (at most 120), with row numbers: made to scroll sideways, not to fit the window
func csvTable(text string, tab bool) []string {
	r := csv.NewReader(strings.NewReader(text))
	if tab {
		r.Comma = '\t'
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var rows [][]string
	var widths []int
	for {
		rec, err := r.Read()
		if err != nil {
			if pe, ok := err.(*csv.ParseError); ok && pe.Err != nil && len(rec) == 0 && len(rows) > 0 {
				continue
			}
			break
		}
		for i := range rec {
			rec[i] = strings.Join(strings.Fields(rec[i]), " ") // one line per cell
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], min(120, ansi.StringWidth(rec[i])))
		}
		rows = append(rows, rec)
	}
	if len(rows) == 0 {
		return []string{fg(cMut).Render("(no rows)")}
	}
	num := len(fmt.Sprint(len(rows)))
	sep := fg(cDim).Render(" │ ")
	out := make([]string, 0, len(rows)+1)
	for ri, rec := range rows {
		cells := make([]string, len(widths))
		for i := range widths {
			v := ""
			if i < len(rec) {
				v = rec[i]
			}
			st := fg(cTxt)
			if ri == 0 {
				st = fg(cAcc).Bold(true)
			}
			cells[i] = pad(st.Render(trunc(v, widths[i])), widths[i])
		}
		n := fmt.Sprintf("%*d", num, ri)
		if ri == 0 {
			n = strings.Repeat(" ", num)
		}
		out = append(out, fg(cDim).Render(n)+sep+strings.Join(cells, sep))
		if ri == 0 {
			parts := make([]string, len(widths))
			for i, w := range widths {
				parts[i] = strings.Repeat("─", w)
			}
			out = append(out, fg(cDim).Render(strings.Repeat("─", num)+"─┼─"+strings.Join(parts, "─┼─")))
		}
	}
	return out
}
