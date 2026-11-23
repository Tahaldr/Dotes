package files

// Spreadsheets full screen (F on a .csv, .tsv, .xlsx or .xlsm): a grid with a cell cursor,
// like a small read-only Excel. Sheets in tabs, column letters and row numbers, the whole
// cell (and its formula) at the bottom, find, filter, sort by a column, copy a cell or a
// row, and save a sheet as csv.

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"github.com/xuri/excelize/v2"
)

const cellMax = 40 // the widest a column gets (the whole cell shows at the bottom)

type sheet struct {
	name string
	rows [][]string
}

type grid struct {
	file   *excelize.File // for formulas (nil for csv)
	sheets []sheet
	cur    int
	header bool // the first row stays on top and out of sorting

	view      []int // the sheet's rows shown, after filter and sort
	r, c      int   // the cursor: a row of view, a column
	top, left int
	sortCol   int // -1 = file order
	sortDesc  bool
	filter    string
	find      string
	widths    []int

	// where things were drawn (mouse)
	tabXs   [][2]int
	tabLeft int // the first sheet tab shown

	// enter: the whole cell in a popup
	cellOpen   bool
	cellScroll int
	lastClick  [2]int
	lastClickT time.Time
	colXs      [][3]int // x start, x end, column
	rowY       int      // screen row of view[top]
	tabY       int
}

func isSheetExt(ext string) bool { return ext == ".xlsx" || ext == ".xlsm" }

// loadSheets reads every sheet of an Excel file
func loadSheets(path string, maxRows int) (*excelize.File, []sheet, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, nil, err
	}
	var out []sheet
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			continue
		}
		if len(rows) > maxRows {
			rows = rows[:maxRows]
		}
		out = append(out, sheet{name: name, rows: rows})
	}
	return f, out, nil
}

func csvRows(text string, tab bool, maxRows int) [][]string {
	r := csv.NewReader(strings.NewReader(text))
	if tab {
		r.Comma = '\t'
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var rows [][]string
	for len(rows) < maxRows {
		rec, err := r.Read()
		if err != nil {
			if _, ok := err.(*csv.ParseError); ok && len(rows) > 0 {
				continue
			}
			break
		}
		rows = append(rows, rec)
	}
	return rows
}

func newGrid(f *excelize.File, sheets []sheet) *grid {
	g := &grid{file: f, sheets: sheets, header: true, sortCol: -1}
	g.rebuild()
	return g
}

func (g *grid) sh() *sheet { return &g.sheets[g.cur] }

func (g *grid) cols() int {
	n := 0
	for _, r := range g.sh().rows {
		n = max(n, len(r))
	}
	return n
}

func (g *grid) cell(row, col int) string {
	rows := g.sh().rows
	if row < 0 || row >= len(rows) || col < 0 || col >= len(rows[row]) {
		return ""
	}
	return rows[row][col]
}

// oneLine: a cell as one line
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, " ", " ")), " ")
}

// rebuild: the rows shown (filter, sort) and the column widths
func (g *grid) rebuild() {
	rows := g.sh().rows
	first := 0
	if g.header && len(rows) > 1 {
		first = 1
	}
	g.view = g.view[:0]
	for i := first; i < len(rows); i++ {
		if g.filter != "" && !rowMatches(rows[i], g.filter) {
			continue
		}
		g.view = append(g.view, i)
	}
	if c := g.sortCol; c >= 0 {
		sort.SliceStable(g.view, func(a, b int) bool {
			x, y := g.cell(g.view[a], c), g.cell(g.view[b], c)
			if g.sortDesc {
				x, y = y, x
			}
			return cellLess(x, y)
		})
	}
	n := g.cols()
	g.widths = make([]int, n)
	for i := range g.widths {
		g.widths[i] = max(3, width(colName(i)))
	}
	for _, r := range rows {
		for i, v := range r {
			g.widths[i] = max(g.widths[i], min(cellMax, width(oneLine(v))))
		}
	}
	g.r = max(0, min(g.r, len(g.view)-1))
	g.c = max(0, min(g.c, n-1))
}

func rowMatches(row []string, q string) bool {
	for _, v := range row {
		if smartMatch(v, q) {
			return true
		}
	}
	return false
}

// cellLess: numbers by value, empty cells last, the rest in natural order
func cellLess(a, b string) bool {
	fa, ea := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(a), ",", ""), 64)
	fb, eb := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(b), ",", ""), 64)
	switch {
	case ea == nil && eb == nil:
		return fa < fb
	case strings.TrimSpace(a) == "":
		return false
	case strings.TrimSpace(b) == "":
		return true
	}
	return natLess(strings.ToLower(a), strings.ToLower(b))
}

func isNumber(s string) bool {
	s = strings.TrimSpace(strings.TrimSuffix(strings.ReplaceAll(s, ",", ""), "%"))
	_, err := strconv.ParseFloat(s, 64)
	return err == nil && s != ""
}

// colName: 0 → A, 25 → Z, 26 → AA
func colName(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

func (g *grid) setSheet(i int) {
	if i < 0 || i >= len(g.sheets) || i == g.cur {
		return
	}
	g.cur = i
	g.r, g.c, g.top, g.left, g.sortCol, g.filter = 0, 0, 0, 0, -1, ""
	g.rebuild()
}

// ---------------------------------------------------------------------------
// drawing
// ---------------------------------------------------------------------------

// render: h lines of width w, starting at screen row y0
func (g *grid) render(w, h, y0 int) []string {
	out := make([]string, 0, h)
	sh := g.sh()

	// sheet tabs
	g.tabY, g.tabXs = y0, nil
	var info []string
	if g.filter != "" {
		info = append(info, fg(cYellow).Render(icFilter+" "+g.filter))
	}
	if g.sortCol >= 0 {
		arrow := "↓"
		if g.sortDesc {
			arrow = "↑"
		}
		info = append(info, fg(cHint).Render(icSort+" "+colName(g.sortCol)+" "+arrow))
	}
	info = append(info, fg(cMut).Render(plural(len(g.view), "row")+" · "+plural(len(g.widths), "column")))
	right := strings.Join(info, "   ") + " "
	out = append(out, spread(g.tabRow(w-width(right)-2), right, w))

	if len(sh.rows) == 0 {
		out = append(out, "", " "+fg(cMut).Render("(empty sheet)"))
		for len(out) < h {
			out = append(out, "")
		}
		return out
	}

	// which columns fit, keeping the cursor's in sight
	numW := max(3, len(strconv.Itoa(len(sh.rows))))
	room := w - numW - 2
	fits := func(left int) int { // the last column that fits from left
		used, last := 0, left
		for i := left; i < len(g.widths); i++ {
			used += g.widths[i] + 3
			if used > room && i > left {
				break
			}
			last = i
		}
		return last
	}
	if g.c < g.left {
		g.left = g.c
	}
	for g.left < g.c && fits(g.left) < g.c {
		g.left++
	}
	last := fits(g.left)

	sep := fg(cDim).Render(" │ ")
	rowLine := func(num string, numSt lipgloss.Style, cells func(i int) string) string {
		l := " " + numSt.Render(fmt.Sprintf("%*s", numW, num)) + sep
		for i := g.left; i <= last; i++ {
			if i > g.left {
				l += sep
			}
			l += cells(i)
		}
		return l
	}
	g.colXs = nil
	cx := 1 + numW + 3
	for i := g.left; i <= last; i++ {
		g.colXs = append(g.colXs, [3]int{cx, cx + g.widths[i], i})
		cx += g.widths[i] + 3
	}
	fit := func(v string, w int, st lipgloss.Style, right bool) string {
		v = trunc(oneLine(v), w)
		if right {
			return st.Render(strings.Repeat(" ", w-width(v)) + v)
		}
		return st.Render(v) + strings.Repeat(" ", w-width(v))
	}

	// column letters
	out = append(out, rowLine("", fg(cDim), func(i int) string {
		st := fg(cMut)
		if i == g.c {
			st = fg(cAcc).Bold(true)
		}
		return fit(colName(i), g.widths[i], st, false)
	}))
	// the header row, kept on top
	if g.header && len(sh.rows) > 1 {
		out = append(out, rowLine("1", fg(cDim), func(i int) string {
			return fit(g.cell(0, i), g.widths[i], fg(cAcc).Bold(true), false)
		}))
		rule := " " + strings.Repeat("─", numW) + "─┼─"
		for i := g.left; i <= last; i++ {
			if i > g.left {
				rule += "─┼─"
			}
			rule += strings.Repeat("─", g.widths[i])
		}
		out = append(out, fg(cDim).Render(trunc(rule, w)))
	}

	// the rows
	bodyH := max(1, h-len(out)-2)
	if g.r < g.top {
		g.top = g.r
	}
	if g.r >= g.top+bodyH {
		g.top = g.r - bodyH + 1
	}
	g.top = max(0, min(g.top, len(g.view)-bodyH))
	g.rowY = y0 + len(out)
	for k := 0; k < bodyH; k++ {
		vi := g.top + k
		if vi >= len(g.view) {
			if len(g.view) == 0 && k == 0 {
				out = append(out, " "+fg(cMut).Render("no row matches "+g.filter+" · esc clears the filter"))
				continue
			}
			out = append(out, "")
			continue
		}
		row := g.view[vi]
		numSt := fg(cDim)
		if vi == g.r {
			numSt = fg(cAcc)
		}
		out = append(out, rowLine(strconv.Itoa(row+1), numSt, func(i int) string {
			v := g.cell(row, i)
			st := fg(cTxt)
			switch {
			case vi == g.r && i == g.c:
				st = lipgloss.NewStyle().Background(cAcc).Foreground(cBg)
			case g.find != "" && smartMatch(v, g.find):
				st = fg(cYellow)
			case vi == g.r:
				st = fg(cHint)
			}
			return fit(v, g.widths[i], st, isNumber(v))
		}))
	}

	// the cell under the cursor, whole
	out = append(out, fg(cDim).Render(strings.Repeat("─", w)))
	out = append(out, g.cellBar(w))
	return out
}

// tabRow: the sheet tabs that fit in w, scrolled so the current one is always in sight
// (‹ › show there are more)
func (g *grid) tabRow(w int) string {
	g.tabXs = make([][2]int, len(g.sheets))
	labels := make([]string, len(g.sheets))
	for i, s := range g.sheets {
		labels[i] = " " + trunc(s.name, 30) + " "
	}
	fits := func(from, to int) bool { // tabs from..to and the arrows, in w
		n := 2 + 2 // " ‹" and " ›"
		for i := from; i <= to; i++ {
			n += width(labels[i]) + 1
		}
		return n <= w
	}
	g.tabLeft = max(0, min(g.tabLeft, g.cur))
	for g.tabLeft < g.cur && !fits(g.tabLeft, g.cur) {
		g.tabLeft++
	}
	out, x := " ", 1
	if g.tabLeft > 0 {
		out, x = " "+fg(cAcc).Render("‹")+" ", 3
	}
	last := g.tabLeft
	for i := g.tabLeft; i < len(g.sheets); i++ {
		if i > g.tabLeft && !fits(g.tabLeft, i) {
			break
		}
		st := fg(cMut)
		if i == g.cur {
			st = lipgloss.NewStyle().Background(cAcc).Foreground(cBg).Bold(true)
		}
		g.tabXs[i] = [2]int{x, x + width(labels[i])}
		out += st.Render(labels[i]) + " "
		x += width(labels[i]) + 1
		last = i
	}
	if last < len(g.sheets)-1 {
		out += fg(cAcc).Render("›") + fg(cMut).Render(fmt.Sprintf(" %d more", len(g.sheets)-1-last))
	}
	return out
}

func (g *grid) curRow() int {
	if g.r < 0 || g.r >= len(g.view) {
		return -1
	}
	return g.view[g.r]
}

func (g *grid) cellBar(w int) string {
	row := g.curRow()
	if row < 0 {
		return ""
	}
	name := colName(g.c) + strconv.Itoa(row+1)
	l := " " + fg(cAcc).Bold(true).Render(name) + "  "
	if g.header && row > 0 && g.c < len(g.sh().rows[0]) {
		if h := oneLine(g.cell(0, g.c)); h != "" {
			l += fg(cMut).Render(trunc(h, 30)+" ") + " "
		}
	}
	v := oneLine(g.cell(row, g.c))
	if g.file != nil {
		if f, err := g.file.GetCellFormula(g.sh().name, name); err == nil && f != "" {
			l += fg(cPurple).Render("="+f) + fg(cMut).Render("  →  ")
		}
	}
	if v == "" {
		v = fg(cMut).Render("(empty)")
	}
	return trunc(l+v, w)
}

// ---------------------------------------------------------------------------
// keys and mouse
// ---------------------------------------------------------------------------

func (m *model) gridKey(s string) tea.Cmd {
	g := m.rd.p.grid
	if g.cellOpen {
		switch s {
		case "esc", "q", "enter", "space", " ", "v":
			g.cellOpen = false
		case "j", "down":
			g.cellScroll++
		case "k", "up":
			g.cellScroll = max(0, g.cellScroll-1)
		case "ctrl+d", "pgdown":
			g.cellScroll += 10
		case "ctrl+u", "pgup":
			g.cellScroll = max(0, g.cellScroll-10)
		case "y":
			m.gridCopy(g.cell(g.curRow(), g.c), "cell")
		case "h", "left", "l", "right", "tab", "shift+tab": // the next cell, popup still open
			g.cellOpen = false
			m.gridKey(s)
			g.cellOpen, g.cellScroll = true, 0
		}
		return nil
	}
	page := max(1, m.bodyH-6)
	switch s {
	case "j", "down":
		g.r = min(len(g.view)-1, g.r+1)
	case "k", "up":
		g.r = max(0, g.r-1)
	case "l", "right":
		g.c = min(len(g.widths)-1, g.c+1)
	case "h", "left":
		g.c = max(0, g.c-1)
	case "enter", "space", " ", "v":
		if g.curRow() >= 0 {
			g.cellOpen, g.cellScroll = true, 0
		}
	case "ctrl+d", "pgdown":
		g.r = min(len(g.view)-1, g.r+page)
	case "ctrl+u", "pgup":
		g.r = max(0, g.r-page)
	case "g", "home":
		g.r = 0
	case "G", "end":
		g.r = len(g.view) - 1
	case "0", "^", "H":
		g.c = 0
	case "$", "L":
		g.c = len(g.widths) - 1
	case "tab", "]":
		g.setSheet((g.cur + 1) % len(g.sheets))
	case "shift+tab", "[":
		g.setSheet((g.cur - 1 + len(g.sheets)) % len(g.sheets))
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		g.setSheet(int(s[0] - '1'))
	case "/":
		m.gridFind()
	case "n":
		m.gridNext(1)
	case "N":
		m.gridNext(-1)
	case "f":
		m.gridFilter()
	case "s":
		if g.sortCol == g.c {
			g.sortDesc = !g.sortDesc
		} else {
			g.sortCol, g.sortDesc = g.c, false
		}
		m.gridKeepRow(func() { g.rebuild() })
	case "t":
		g.header = !g.header
		m.gridKeepRow(func() { g.rebuild() })
		if g.header {
			m.setNote("row 1 is the header: it stays on top", false)
		} else {
			m.setNote("row 1 is a normal row", false)
		}
	case "y":
		m.gridCopy(oneLine(g.cell(g.curRow(), g.c)), "cell")
	case "Y":
		if row := g.curRow(); row >= 0 {
			m.gridCopy(strings.Join(g.sh().rows[row], "\t"), "row")
		}
	case "x":
		m.gridExport()
	case "esc":
		switch {
		case g.filter != "" || g.sortCol >= 0 || g.find != "":
			g.filter, g.sortCol, g.find = "", -1, ""
			m.gridKeepRow(func() { g.rebuild() })
		default:
			m.closeReader()
		}
	default:
		return m.readerKey(s)
	}
	return nil
}

// gridKeepRow: the cursor stays on the same sheet row through a re-sort / filter
func (m *model) gridKeepRow(f func()) {
	g := m.rd.p.grid
	row := g.curRow()
	f()
	for i, v := range g.view {
		if v == row {
			g.r = i
			return
		}
	}
}

func (m *model) gridFind() {
	g := m.rd.p.grid
	r0, c0 := g.r, g.c
	m.input("Find in the sheet", "a capital letter makes it case-sensitive · n / N next / previous", g.find, -1, func(m *model, s string) tea.Cmd {
		g.find = s
		return nil
	})
	m.prompt.onChange = func(m *model, s string) {
		g.find = s
		g.r, g.c = r0, c0
		if s != "" {
			m.gridNextFrom(r0, c0-1, 1)
		}
	}
	m.prompt.onCancel = func(m *model) { g.find, g.r, g.c = "", r0, c0 }
}

func (m *model) gridNext(d int) {
	g := m.rd.p.grid
	if g.find == "" {
		m.setNote("/ finds text in the sheet first", false)
		return
	}
	if !m.gridNextFrom(g.r, g.c, d) {
		m.setNote("nothing matches "+g.find, true)
	}
}

// gridNextFrom: the next cell matching find after (r, c), going around the sheet
func (m *model) gridNextFrom(r, c, d int) bool {
	g := m.rd.p.grid
	n := len(g.widths)
	total := len(g.view) * n
	if total == 0 {
		return false
	}
	pos := r*n + c
	for k := 1; k <= total; k++ {
		p := ((pos+d*k)%total + total) % total // around the sheet
		rr, cc := p/n, p%n
		if smartMatch(g.cell(g.view[rr], cc), g.find) {
			g.r, g.c = rr, cc
			return true
		}
	}
	return false
}

func (m *model) gridFilter() {
	g := m.rd.p.grid
	old := g.filter
	apply := func(m *model, s string) {
		g.filter = s
		g.r, g.top = 0, 0
		g.rebuild()
	}
	m.input("Filter rows", "only rows with a cell containing this · esc in the sheet clears it", old, -1, func(m *model, s string) tea.Cmd {
		apply(m, s)
		return nil
	})
	m.prompt.onChange = apply
	m.prompt.onCancel = func(m *model) { apply(m, old) }
}

func (m *model) gridCopy(text, what string) {
	if !m.dump {
		if err := clipboard.WriteAll(text); err != nil {
			m.setNote("clipboard: "+err.Error(), true)
			return
		}
	}
	if text == "" {
		m.setNote("copied an empty "+what, false)
		return
	}
	m.setNote("copied the "+what+": "+trunc(strings.ReplaceAll(text, "\t", " · "), 60), false)
}

// gridExport saves the sheet as it's shown (filter and sort included) as a csv next to the file
func (m *model) gridExport() {
	g := m.rd.p.grid
	sh := g.sh()
	base := strings.TrimSuffix(m.rd.e.name, filepath.Ext(m.rd.e.name))
	if len(g.sheets) > 1 || g.file != nil {
		base += " - " + sh.name
	}
	dir := filepath.Dir(m.rd.e.path)
	path := freeName(dir, base+".csv")
	name := filepath.Base(path)
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // Excel then reads Arabic and accents right
	w := csv.NewWriter(&buf)
	if g.header && len(sh.rows) > 1 {
		w.Write(sh.rows[0])
	}
	for _, r := range g.view {
		w.Write(sh.rows[r])
	}
	w.Flush()
	if m.dump {
		m.setNote("would save "+name, false)
		return
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		m.setNote("couldn't save: "+err.Error(), true)
		return
	}
	m.setNote("saved "+name, false)
}

func (m *model) gridClick(x, y int) {
	g := m.rd.p.grid
	if y == g.tabY {
		for i, t := range g.tabXs {
			if x >= t[0] && x < t[1] {
				g.setSheet(i)
			}
		}
		return
	}
	if y < g.rowY {
		return
	}
	vi := g.top + y - g.rowY
	if vi >= len(g.view) {
		return
	}
	for _, c := range g.colXs {
		if x >= c[0]-1 && x <= c[1]+1 {
			at := [2]int{vi, c[2]}
			if at == g.lastClick && time.Since(g.lastClickT) < 450*time.Millisecond || (m.dump && g.r == vi && g.c == c[2]) {
				g.cellOpen, g.cellScroll = true, 0 // double-click: the whole cell
			}
			g.lastClick, g.lastClickT = at, time.Now()
			g.r, g.c = vi, c[2]
			return
		}
	}
}

func (m *model) gridWheel(w tea.MouseWheelMsg) {
	g := m.rd.p.grid
	if g.cellOpen {
		if w.Button == tea.MouseWheelDown {
			g.cellScroll += 3
		} else if w.Button == tea.MouseWheelUp {
			g.cellScroll = max(0, g.cellScroll-3)
		}
		return
	}
	side := w.Mod&tea.ModShift != 0
	switch {
	case w.Button == tea.MouseWheelRight || (side && w.Button == tea.MouseWheelDown):
		g.c = min(len(g.widths)-1, g.c+1)
	case w.Button == tea.MouseWheelLeft || (side && w.Button == tea.MouseWheelUp):
		g.c = max(0, g.c-1)
	case w.Button == tea.MouseWheelDown:
		g.r = min(len(g.view)-1, g.r+3)
	case w.Button == tea.MouseWheelUp:
		g.r = max(0, g.r-3)
	}
}

// sheetPreview: the side column for an Excel file: its sheets, then the first one as a table
func sheetPreview(p *preview, e entry) {
	f, sheets, err := loadSheets(e.path, 60)
	if err != nil {
		p.lines = append(infoCard(e, "Excel sheet"), readError(p, err)...)
		return
	}
	f.Close()
	var b strings.Builder
	names := make([]string, len(sheets))
	for i, s := range sheets {
		names[i] = s.name
	}
	b.WriteString("**" + strconv.Itoa(len(sheets)) + " sheets:** " + strings.Join(names, " · ") + "  \n*F reads them full screen*\n\n")
	if len(sheets) > 0 {
		var csvText bytes.Buffer
		w := csv.NewWriter(&csvText)
		w.WriteAll(sheets[0].rows)
		b.WriteString(csvAsMarkdown(csvText.String(), false))
	}
	p.kind, p.src = "md", b.String()
}

// cellPopup: the cell under the cursor, whole, with its own line breaks and spaces
func (m *model) cellPopup(lines []string) {
	if m.rd == nil || m.rd.p.grid == nil || !m.rd.p.grid.cellOpen || m.help {
		return
	}
	g := m.rd.p.grid
	row := g.curRow()
	if row < 0 {
		g.cellOpen = false
		return
	}
	w := m.popupWidth(60, 110)
	inner := w - 4
	name := colName(g.c) + strconv.Itoa(row+1)
	title := fg(cAcc).Bold(true).Render(name)
	if g.header && row > 0 {
		if h := oneLine(g.cell(0, g.c)); h != "" {
			title += fg(cMut).Render("  " + trunc(h, inner-10))
		}
	}
	var body []string
	if g.file != nil {
		if f, err := g.file.GetCellFormula(g.sh().name, name); err == nil && f != "" {
			for _, l := range strings.Split(ansi.Wrap("="+f, inner, " ,;("), "\n") {
				body = append(body, fg(cPurple).Render(l))
			}
			body = append(body, fg(cDim).Render(strings.Repeat("─", inner)))
		}
	}
	text := strings.ReplaceAll(strings.ReplaceAll(g.cell(row, g.c), "\r\n", "\n"), "\t", "    ")
	if text == "" {
		body = append(body, fg(cMut).Render("(empty)"))
	}
	for _, l := range strings.Split(text, "\n") {
		// a wrapped line keeps the line's indent (and lines up after a "- " / "• " bullet)
		rest := strings.TrimLeft(l, " ")
		ind := len(l) - len(rest)
		hang := ind
		for _, b := range []string{"- ", "• ", "* "} {
			if strings.HasPrefix(rest, b) {
				hang += 2
			}
		}
		hang = min(hang, inner/2)
		for i, part := range strings.Split(ansi.Wrap(rest, inner-hang, " -/,;"), "\n") {
			lead := strings.Repeat(" ", min(ind, inner/2))
			if i > 0 {
				lead = strings.Repeat(" ", hang)
			}
			body = append(body, fg(cTxt).Render(lead+strings.TrimLeft(part, " ")))
		}
	}
	room := max(3, m.h-9)
	g.cellScroll = max(0, min(g.cellScroll, len(body)-room))
	more := ""
	if len(body) > room {
		more = fmt.Sprintf("  %d–%d of %d lines", g.cellScroll+1, g.cellScroll+room, len(body))
		body = body[g.cellScroll : g.cellScroll+room]
	}
	body = append(body, fg(cDim).Render(strings.Repeat("─", inner)),
		keyHints("esc", "close", "j/k", "scroll", "h/l", "next cell", "y", "copy")+fg(cMut).Render(more))
	box := popupBox(title, w, body, fg(cAcc))
	put(lines, (m.w-w)/2, max(2, (m.h-len(box))/2), box, w)
}

// readError: why a file can't be read; a file another app is busy with is tried again
func readError(p *preview, err error) []string {
	msg := err.Error()
	if pe, ok := err.(*os.PathError); ok {
		msg = pe.Err.Error()
	}
	if busy(err) {
		p.retry = true
		return []string{"", "  " + fg(cYellow).Render("another app is using it right now (Excel, a download,"),
			"  " + fg(cYellow).Render("OneDrive…): trying again")}
	}
	out := []string{""}
	for _, l := range wrapText("can't read: "+msg, 50, fg(cRed)) {
		out = append(out, "  "+l)
	}
	return out
}
