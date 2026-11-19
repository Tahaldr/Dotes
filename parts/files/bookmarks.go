package files

// Bookmarks: b saves (or removes) the current folder, ' lists them.
// Saved in the Dotes config folder as files-bookmarks.txt, one folder per line.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Tahaldr/dotes/internal/app"
)

func bookmarkFile() string { return app.File("files-bookmarks.txt") }

func loadBookmarks() []string {
	b, err := os.ReadFile(bookmarkFile())
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func (m *model) saveBookmarks() {
	if m.dump {
		return // tests never touch your real bookmarks
	}
	os.MkdirAll(filepath.Dir(bookmarkFile()), 0o755)
	os.WriteFile(bookmarkFile(), []byte(strings.Join(m.bookmarks, "\n")+"\n"), 0o644)
}

func (m *model) bookmarkIndex(dir string) int {
	for i, b := range m.bookmarks {
		if strings.EqualFold(b, dir) {
			return i
		}
	}
	return -1
}

func (m *model) toggleBookmark() {
	if m.virtual != "" {
		m.setNote("bookmarks are for folders: leave this list first (h)", true)
		return
	}
	if i := m.bookmarkIndex(m.cwd); i >= 0 {
		m.bookmarks = append(m.bookmarks[:i], m.bookmarks[i+1:]...)
		m.setNote("bookmark removed: "+prettyPath(m.cwd), false)
	} else {
		m.bookmarks = append(m.bookmarks, m.cwd)
		m.setNote("bookmarked "+prettyPath(m.cwd)+" · ' to jump", false)
	}
	m.saveBookmarks()
}

func (m *model) marksKey(k tea.KeyPressMsg) {
	s := k.String()
	n := len(m.bookmarks)
	switch s {
	case "'", "esc", "q":
		m.showMarks = false
	case "j", "down":
		if n > 0 {
			m.markCur = (m.markCur + 1) % n
		}
	case "k", "up":
		if n > 0 {
			m.markCur = (m.markCur - 1 + n) % n
		}
	case "d", "delete":
		if m.markCur < n {
			m.bookmarks = append(m.bookmarks[:m.markCur], m.bookmarks[m.markCur+1:]...)
			m.markCur = max(0, min(m.markCur, len(m.bookmarks)-1))
			m.saveBookmarks()
		}
	case "enter", "l":
		if m.markCur < n {
			m.showMarks = false
			m.jumpTo(m.bookmarks[m.markCur])
		}
	default:
		if i, err := strconv.Atoi(s); err == nil && i >= 1 && i <= n && i <= 9 {
			m.showMarks = false
			m.jumpTo(m.bookmarks[i-1])
		}
	}
}

func (m *model) marksPopup(lines []string) {
	if !m.showMarks {
		return
	}
	w := m.popupWidth(50, 80)
	inner := w - 4
	var body []string
	if len(m.bookmarks) == 0 {
		body = append(body, fg(cMut).Render("No bookmarks yet: b saves the folder you're in."))
	}
	for i, b := range m.bookmarks {
		num := "  "
		if i < 9 {
			num = strconv.Itoa(i+1) + " "
		}
		name := filepath.Base(b)
		if isRoot(b) {
			name = b
		}
		st, mark := fg(cTxt), "  "
		if i == m.markCur {
			st, mark = fg(cAcc).Bold(true), fg(cAcc).Render("❯ ")
		}
		row := mark + fg(cHint).Render(num) + fg(cAcc).Render(icFolder) + " " + st.Render(name)
		where := fg(cMut).Render(prettyPath(filepath.Dir(b)))
		if isRoot(b) {
			where = ""
		}
		gap := inner - width(row) - width(where)
		if gap < 2 {
			where = trunc(where, max(0, inner-width(row)-2))
			gap = 2
		}
		body = append(body, row+strings.Repeat(" ", gap)+where)
	}
	body = append(body, fg(cDim).Render(strings.Repeat("─", inner)), keyHints("1-9/enter", "jump", "j/k", "move", "d", "remove", "esc", "close"))
	box := popupBox(fg(cAcc).Bold(true).Render(icBookmark+" Bookmarks"), w, body, fg(cAcc))
	put(lines, (m.w-w)/2, 2, box, w)
}
