package files

// F: read the file under the cursor full screen, like Git's README reader. It reads much more
// of the file than the side preview, draws tables (csv / tsv) with every column at its real
// width, and scrolls sideways (h / l) for what's wider than the window.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	tries     int  // reads of a busy file
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
	load := func() tea.Msg { return readerMsg{e.path, makeFullPreview(e, hidden)} }
	if m.dump {
		m.update(load())
		return nil
	}
	return load
}

// makeFullPreview: the file read for full screen; spreadsheets become a grid
func makeFullPreview(e entry, hidden bool) preview {
	ext := strings.ToLower(filepath.Ext(e.name))
	if isSheetExt(ext) {
		p := preview{key: previewKey(e, hidden)}
		f, sheets, err := loadSheets(e.path, 1_000_000)
		if err != nil {
			p.lines = readError(&p, err)
			return p
		}
		if len(sheets) == 0 {
			sheets = []sheet{{name: "Sheet1"}}
		}
		p.kind, p.grid = "sheet", newGrid(f, sheets)
		return p
	}
	p := makePreviewLim(e, hidden, fullLimits)
	if p.kind == "csv" {
		tab := ext == ".tsv" || strings.Contains(strings.SplitN(p.src, "\n", 2)[0], "\t")
		p.grid = newGrid(nil, []sheet{{name: e.name, rows: csvRows(p.src, tab, 1_000_000)}})
	}
	return p
}

// closeReader: back to the columns
func (m *model) closeReader() {
	if m.rd != nil && m.rd.p.grid != nil && m.rd.p.grid.file != nil {
		m.rd.p.grid.file.Close()
	}
	m.rd = nil
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
	if old := r.p.grid; old != nil && msg.p.grid != nil { // read again: stay where you were
		g := msg.p.grid
		g.header, g.filter, g.find, g.sortCol, g.sortDesc = old.header, old.filter, old.find, old.sortCol, old.sortDesc
		g.cur = min(old.cur, len(g.sheets)-1)
		g.rebuild()
		g.r, g.c = min(old.r, max(0, len(g.view)-1)), min(old.c, max(0, len(g.widths)-1))
		if old.file != nil {
			old.file.Close()
		}
	}
	r.p, r.loading, r.lines, r.w = msg.p, false, nil, 0
	if msg.p.retry && r.tries < 60 { // busy: the next check reads it again
		r.tries++
		r.e.mod = time.Time{}
	}
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
	if r.p.grid != nil && !r.loading {
		hints = []string{" " + keyHints("hjkl", "move", "tab / [ ]", "sheets", "/ n", "find", "f", "filter", "s", "sort by column", "t", "header row"),
			" " + keyHints("y / Y", "copy cell / row", "x", "save sheet as csv", "e", "edit", "o", "open", "E", "Explorer", "esc", "clear", "q", "back")}
	}
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
	} else if g := r.p.grid; g != nil {
		body = g.render(m.w-1, h, m.bodyY)
		for i := range body {
			body[i] = trunc(body[i], m.w)
		}
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
		m.closeReader()
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
		m.closeReader()
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
