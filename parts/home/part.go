// Package home is Dotes' Home part: dashboard, app launcher, project picker and prayer
// times (was TermNook).
package home

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/ui"
)

// Options for starting the part.
type Options struct {
	CwdFile  string // where the folder to cd to goes (the shell function)
	Projects bool   // open the project picker straight away (dotes h projects)
	Dump     bool
}

const Usage = `  dotes h                           Home: dashboard, app launcher (also what plain  dotes  opens)
  dotes h quick                     one-line app picker under your prompt
  dotes h projects                  pick a project and open it
  dotes h salat                     today's prayer times and the next one, under your prompt`

// New makes the Home part.
func New(o Options) tea.Model {
	m := newModel()
	m.cwdFile = o.CwdFile
	if o.Dump {
		m.dump = true
		return m
	}
	m.live = true
	if o.Projects {
		return &projectsOnly{m}
	}
	m.inWindow = true // the project picker is the Files part next to Home
	return m
}

// projectsOnly: dotes h projects. The picker opens at once; Dotes closes after.
type projectsOnly struct{ *model }

func (p *projectsOnly) Init() tea.Cmd { return p.pickProject() }
func (p *projectsOnly) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := p.model.Update(msg)
	if _, ok := msg.(pickedMsg); ok && cmd == nil { // nothing picked
		return p, tea.Quit
	}
	return p, cmd
}

// Salat prints the prayer box under the prompt (downloads the month first if needed).
func Salat() {
	m := newModel()
	if m.cfg.prayer == nil {
		fmt.Println("Prayer times are off. Set  prayer = City | Country | method  in home.txt (dotes config).")
		return
	}
	if m.prayer == nil {
		if r, ok := fetchPrayer(m.cfg.prayer)().(prayerMsg); ok && r.data != nil {
			m.prayer = r.data
		}
	}
	m.now = time.Now()
	w := 120
	if s := os.Getenv("COLUMNS"); s != "" {
		fmt.Sscan(s, &w)
	}
	box := m.prayerBox(w, m.now.Format("Monday, Jan 2  ·  15:04"))
	if box == nil {
		fmt.Println("Couldn't get prayer times for " + m.cfg.prayer.city + " (no internet?)")
		return
	}
	fmt.Println(strings.Join(box, "\n"))
}

// ---- the quick picker: one row under the prompt, erased when done ----
//
//	 Nvim   VS Code   Git   Files       ←→ move  enter open  esc cancel
//	        ━━━━━━━  code

type quick struct {
	m      *model
	items  []appItem
	sel    int
	w      int
	picked *appItem
	done   bool
}

// Quick runs the one-line picker; it returns what to open (nil = cancelled).
func Quick() *appItem {
	m := newModel()
	q := &quick{m: m, w: 120}
	for _, a := range m.apps {
		if a.run != "" {
			q.items = append(q.items, a)
		}
	}
	if len(q.items) == 0 {
		fmt.Println("No apps with a command in home.txt.")
		return nil
	}
	if _, err := tea.NewProgram(q).Run(); err != nil {
		return nil
	}
	if q.picked != nil && m.cfg.recent {
		remember(q.picked.name)
	}
	return q.picked
}

func (q *quick) Init() tea.Cmd { return nil }
func (q *quick) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		q.w = msg.Width
	case tea.KeyPressMsg:
		n := len(q.items)
		switch s := msg.String(); s {
		case "left", "up", "shift+tab":
			q.sel = (q.sel + n - 1) % n
		case "right", "down", "tab":
			q.sel = (q.sel + 1) % n
		case "enter":
			q.picked = &q.items[q.sel]
		case "esc", "q", "ctrl+c":
		default:
			for i := range q.items {
				if q.items[i].key != "" && strings.EqualFold(s, q.items[i].key) {
					q.picked = &q.items[i]
				}
			}
			if q.picked == nil {
				return q, nil
			}
		}
		if s := msg.String(); q.picked != nil || s == "esc" || s == "q" || s == "ctrl+c" {
			q.done = true
			return q, tea.Quit
		}
	}
	return q, nil
}

func (q *quick) View() tea.View {
	if q.done {
		return tea.NewView("")
	}
	t := q.m.cfg.th
	const gap, lead = 3, 2
	n := len(q.items)
	span := func(a, b int) int {
		s := 0
		for i := a; i <= b; i++ {
			s += width(q.items[i].name)
			if i > a {
				s += gap
			}
		}
		return s
	}
	avail := q.w - lead - 4
	first := 0
	for first < q.sel && span(first, q.sel)+4 > avail {
		first++
	}
	last := q.sel
	for last+1 < n && span(first, last+1)+4 <= avail {
		last++
	}
	line, pos, selStart := strings.Repeat(" ", lead), lead, lead
	if first > 0 {
		line += fg(t.mut).Render("… ")
		pos += 2
	}
	for i := first; i <= last; i++ {
		it := q.items[i]
		nm := it.name
		at := -1
		if it.key != "" {
			at = strings.Index(strings.ToLower(nm), it.key)
		}
		pre, ltr, post := nm, "", ""
		if at >= 0 {
			pre, ltr, post = nm[:at], nm[at:at+1], nm[at+1:]
		}
		if i == q.sel {
			selStart = pos
			st := fg(t.acc).Bold(true)
			line += st.Render(pre) + st.Underline(true).Render(ltr) + st.Render(post)
		} else {
			line += fg(t.mut).Render(pre) + fg(t.txt).Render(ltr) + fg(t.mut).Render(post)
		}
		pos += width(nm)
		if i < last {
			line += strings.Repeat(" ", gap)
			pos += gap
		}
	}
	if last < n-1 {
		line += " " + fg(t.mut).Render("…")
		pos += 2
	}
	help := "←→ move  enter open  esc cancel"
	if q.m.hintsOn && pos+4+width(help) < q.w {
		line += "    " + fg(t.mut).Render(help)
	}
	it := q.items[q.sel]
	under := strings.Repeat(" ", selStart) + fg(t.acc).Render(strings.Repeat("━", width(it.name)))
	if it.hint != "" && selStart+width(it.name)+2+width(it.hint) < q.w {
		under += "  " + fg(t.selHint).Render(it.hint)
	}
	return tea.NewView(line + "\n" + under)
}

// Run and Name of a picked quick item, for the caller.
func (a *appItem) Run() string  { return a.run }
func (a *appItem) Name() string { return a.name }

// PartOf: "dotes g" → "git" (empty when the command isn't a Dotes part).
func PartOf(run string) string { return partOfRun(run) }

// Command builds what runs an app's command in a folder.
var Command = command

// ---- test helpers ----

func drain(m tea.Model, first tea.Cmd) { ui.Drain(m, first) }

func parseInput(a string) tea.Msg { return ui.ParseInput(a) }
