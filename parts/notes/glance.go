package notes

// Glance: a few lines about your todos for Home: what's late or due soon, else what's open
// in the list you edited last.

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// GlanceItem is one todo on Home.
type GlanceItem struct {
	Text, List string
	When       string         // "today", "2d late", "Fri" ("" = no date)
	WhenStyle  lipgloss.Style // its color
	Path       string         // the list's file
}

// GlanceInfo: counts and the first few todos to show.
type GlanceInfo struct {
	Lists, Notes int
	Open, Done   int // todos in every list (not archived)
	Late, Today  int
	Items        []GlanceItem
	NothingDue   bool // Items are just open todos, no dates
	Notebook     string
	List         string // the list shown ("" = what's due from every list)
	ListPath     string
}

// TodoLists: the titles of your todo lists, last edited first (Home's list picker).
func TodoLists() []string {
	if cfg.notebook == "" {
		LoadConfig("")
	}
	var out []string
	for _, d := range listDocs() {
		if d.kind == "todos" {
			out = append(out, d.title)
		}
	}
	return out
}

// Glance reads the notebook (small Markdown files: fast). Call it from Update, not in the
// background (it uses the Notes settings). list = the title of one todo list to show (its
// open todos, dated ones first); "" or a list that's gone = what's due from every list.
func Glance(n int, list string) GlanceInfo {
	if cfg.notebook == "" {
		LoadConfig("")
	}
	docs := listDocs()
	g := GlanceInfo{Notebook: prettyPath(cfg.notebook)}
	now := today()
	m := &model{docs: docs}
	m.buildAgenda()
	for _, d := range docs {
		if d.kind == "notes" {
			g.Notes++
			continue
		}
		g.Lists++
		g.Done += d.done
		g.Open += d.total - d.done
	}
	for _, it := range m.agenda {
		switch it.group {
		case 0:
			g.Late++
		case 1:
			g.Today++
		}
		if len(g.Items) < n {
			l, st := dueLabel(it.due, false, now)
			g.Items = append(g.Items, GlanceItem{Text: it.text, List: it.list, When: l, WhenStyle: st, Path: it.path})
		}
	}
	if list != "" {
		for _, d := range docs {
			if d.kind == "todos" && strings.EqualFold(d.title, list) {
				return listGlance(g, d, n, now)
			}
		}
	}
	if len(g.Items) == 0 { // nothing dated: what's open in the list edited last
		g.NothingDue = true
		for _, d := range docs {
			if d.kind != "todos" || d.total == d.done {
				continue
			}
			doc, err := loadDoc(d.path)
			if err != nil {
				continue
			}
			for i := 0; i < doc.archiveStart() && len(g.Items) < n; i++ {
				if t, ok := parseTodo(doc.lines[i].s); ok && !t.done {
					plain, _, _ := splitTags(t.text)
					g.Items = append(g.Items, GlanceItem{Text: strings.TrimSpace(plain), List: d.title, Path: d.path})
				}
			}
			break
		}
	}
	return g
}

// Ready: the Notes settings are loaded (Glance won't fall back to the default notebook).
func Ready() bool { return cfg.notebook != "" }

// listGlance: one list's open todos (late and due first, then the rest in order)
func listGlance(g GlanceInfo, d docInfo, n int, now time.Time) GlanceInfo {
	g.List, g.ListPath, g.Items, g.Late, g.Today, g.NothingDue = d.title, d.path, nil, 0, 0, false
	g.Open = d.total - d.done
	doc, err := loadDoc(d.path)
	if err != nil {
		return g
	}
	var dated, plain []GlanceItem
	for i := 0; i < doc.archiveStart(); i++ {
		t, ok := parseTodo(doc.lines[i].s)
		if !ok || t.done {
			continue
		}
		text, _, due := splitTags(t.text)
		it := GlanceItem{Text: strings.TrimSpace(text), List: d.title, Path: d.path}
		if dt, ok := parseDate(due); ok {
			it.When, it.WhenStyle = dueLabel(due, false, now)
			switch {
			case dt.Before(now):
				g.Late++
			case dt.Equal(now):
				g.Today++
			}
			dated = append(dated, it)
		} else {
			plain = append(plain, it)
		}
	}
	for _, it := range append(dated, plain...) {
		if len(g.Items) < n {
			g.Items = append(g.Items, it)
		}
	}
	return g
}
