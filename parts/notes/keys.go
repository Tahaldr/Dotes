package notes

// Every key, in one table: the hint bar, the ~ help page and the which-key popup all come
// from here. ctx says where a key works: "lists" (the board of cards), "todos" (inside a todo
// list), "note" (reading a note), "today" (the Today tab), "edit" (typing a todo) or "" (everywhere).

import (
	"strings"

	"github.com/Tahaldr/dotes/internal/app"
)

// Nerd Font icons
const (
	icTodos    = "\U000F0132" // 󰄲
	icNotes    = "\U000F082E" // 󰠮
	icBox      = "\U000F0131" // 󰄱
	icBoxDone  = "\U000F0135" // 󰄵
	icFold     = "\U000F0142" // 󰅂
	icArchive  = "\U000F003C" // 󰀼
	icHeading  = "\U000F0270" // 󰉰
	icCalendar = "\U000F00ED" // 󰃭
	icMove     = "\U000F0450" // 󰑐
	icSearch   = ""          //
	icEdit     = ""          //
	icInfo     = ""          //
	icWarn     = ""          //
	icBook     = "\U000F00BE" // 󰂾
)

type keyHelp struct {
	ctx, group, keys, desc string
	hint                   string // short label for the hint bar ("" = help page only)
}

var keyTable = []keyHelp{
	{"lists", "Board", "h j k l  arrows", "move between the cards", "move"},
	{"todos", "Move", "j / k  ↓ / ↑", "down / up", "move"},
	{"today", "Move", "j / k  ↓ / ↑", "down / up", "move"},
	{"", "Move", "gg / G  home / end", "top / bottom", ""},
	{"", "Move", "ctrl+d / ctrl+u", "half a page down / up", ""},
	{"", "Move", "mouse", "click a card to pick it, double-click to open, click a box to tick it, click ‹ to go back", ""},
	{"", "Move", "tab  1 / 2 / 3", "tabs: Todo lists / Notes / Today (or click one)", ""},
	{"", "Move", "/", "find in every todo list and note (enter goes there)", "find"},

	{"lists", "Board", "enter / space", "open the card: a todo list, or a note to read", "open"},
	{"lists", "Board", "a", "new todo list or note (in the open tab)", "new"},
	{"lists", "Board", "tab", "next tab", "tabs"},
	{"lists", "Board", "r", "rename (the title and the file)", "rename"},
	{"lists", "Board", "d", "delete (asks first, goes to the Recycle Bin)", "delete"},
	{"lists", "Board", "e", "open the file in your editor", ""},

	{"note", "Note", "j / k  ctrl+d / ctrl+u", "scroll", "scroll"},
	{"note", "Note", "enter / e", "open it in your editor (where you've scrolled to)", "edit"},
	{"note", "Note", "r / d", "rename / delete it", ""},
	{"note", "Note", "gf", "open a path from the note in Files (a menu when there are more; or click one)", "open path"},
	{"note", "Note", "h / esc", "back to the board", "back"},

	{"todos", "Todos", "enter / space / x", "tick / untick (sub-todos follow, a parent ticks when all its are)", "tick"},
	{"todos", "Todos", "o / O", "new todo below / above", "new"},
	{"todos", "Todos", "s", "new sub-todo under this one", "sub-todo"},
	{"todos", "Todos", "i / a  cc", "edit the text: cursor at start / end, or start empty (priority and date stay)", "edit"},
	{"todos", "Todos", "t", "due date: today, tomorrow, fri, +3, +2w, 11-20, 2026-11-20 (empty removes)", "due"},
	{"todos", "Todos", "!", "priority: none → low → medium → high", "priority"},
	{"todos", "Todos", "dd", "delete the todo with its sub-todos (p puts it back somewhere)", "delete"},
	{"todos", "Todos", "yy", "copy the todo with its sub-todos", ""},
	{"todos", "Todos", "p / P", "paste below / above", ""},
	{"todos", "Todos", "m", "move the todo (with its sub-todos) to another list", "move to"},
	{"todos", "Todos", "> / <", "make it a sub-todo / take it out one level", "indent"},
	{"todos", "Todos", "J / K", "move it down / up (with its sub-todos)", ""},
	{"todos", "Todos", "S", "sort by priority, high first (sub-todos too)", ""},
	{"todos", "Todos", "tab", "fold / unfold its sub-todos", "fold"},
	{"todos", "Todos", "shift+tab", "fold / unfold all", ""},
	{"todos", "Todos", "A", "archive: move the done ones under ## Archive", "archive"},
	{"todos", "Todos", "u / ctrl+r", "undo / redo", "undo"},
	{"todos", "Todos", "E", "open the file in your editor on this line", ""},
	{"todos", "Todos", "gf", "open the path in this todo in Files (or click it; paths on this PC are underlined)", "open path"},
	{"todos", "Todos", "h / esc", "back to the board", "back"},

	{"today", "Today", "x / space", "tick it (it's saved in its list)", "tick"},
	{"today", "Today", "enter", "go to it in its list", "go there"},
	{"today", "Today", "t", "change its due date", "due"},
	{"today", "Today", "tab", "next tab", "tabs"},

	{"edit", "Typing", "enter", "save, and start the next todo (when adding)", "save + next"},
	{"edit", "Typing", "esc", "save and stop (an empty new todo goes away)", "done"},
	{"edit", "Typing", "ctrl+c", "stop without saving the change", "cancel"},
	{"edit", "Typing", "tab / shift+tab", "make it a sub-todo / take it out one level", "indent"},
	{"edit", "Typing", "~/  C:\\", "start a path: its folder's files pop up, tab / shift+tab put one in, enter keeps it", "path"},
	{"edit", "Typing", "ctrl+w  ctrl+u", "delete a word / everything before the cursor", ""},

	{"", "Other", "~ / F1", "this page (h here hides / shows the key hints)", "all keys"},
	{"", "Other", "g d y c", "first key of a combo: a popup shows what can follow", ""},
	{"", "Other", "q", "quit (everything is saved as you go)", "quit"},
}

// hintKey is the short key shown in the bar: "j/k" from "j / k  ↓ / ↑"
func hintKey(keys string) string {
	k := keys
	if i := strings.Index(k, "  "); i > 0 {
		k = k[:i]
	}
	return strings.ReplaceAll(k, " / ", "/")
}

type hintItem struct{ k, d string }

// hintRows lays out the hints for ctx in rows of width w (at most max rows); "~ all keys" stays
func hintRows(ctx string, w, maxRows int) []string {
	var items []hintItem
	var last hintItem
	for _, h := range keyTable {
		if h.hint == "" || (h.ctx != ctx && h.ctx != "") || (ctx == "edit" && h.ctx == "") {
			continue
		}
		it := hintItem{hintKey(h.keys), h.hint}
		if strings.HasPrefix(h.keys, "~") {
			last = it
			continue
		}
		items = append(items, it)
	}
	if ctx != "edit" {
		items = append(items, last)
	}
	render := func(it hintItem) string { return fg(cHint).Render(it.k) + " " + fg(cMut).Render(it.d) }
	var rows []string
	row, rw := " ", 1
	for i, it := range items {
		s := render(it)
		sw := width(s) + 3
		lastRow := len(rows) == maxRows-1
		room := w - rw
		if lastRow && i < len(items)-1 && last.k != "" {
			room -= width(render(last)) + 3 // keep space for "~ all keys"
		}
		if sw > room {
			if lastRow {
				if ctx != "edit" {
					row += render(last)
				}
				return append(rows, row)
			}
			rows = append(rows, row)
			row, rw = " ", 1
		}
		row += s + "   "
		rw += sw
	}
	return append(rows, row)
}

// ---- which-key: after the first key of a combo, a popup lists what can follow ----

var comboKeys = map[string][][2]string{
	"g": {{"g", "top"}, {"f", "open the path in Files"}},
	"d": {{"d", "delete the todo"}},
	"y": {{"y", "copy the todo"}},
	"c": {{"c", "change: retype the text"}},
}

var comboTitle = map[string]string{"g": "Go to", "d": "Delete", "y": "Copy", "c": "Change"}

func (m *model) whichKeyPopup(lines []string) {
	items, ok := comboKeys[m.pending]
	if !ok || !m.hintsOn { // hidden hints hide this too
		return
	}
	var body []string
	for _, it := range items {
		body = append(body, fg(cAcc).Bold(true).Render(it[0])+"  "+fg(cTxt).Render(it[1]))
	}
	body = append(body, fg(cMut).Render("esc cancel"))
	w := min(m.w-2, 36)
	box := popupBox(fg(cAcc).Bold(true).Render(m.pending+"  "+comboTitle[m.pending]), w, body, fg(cAcc))
	put(lines, m.w-w-1, max(1, m.bodyY+m.bodyH-len(box)), box, w)
}

// ---- hint bar on / off, saved in dotes.txt (shared by every part) ----

func (m *model) toggleHints() {
	m.hintsOn = !m.hintsOn
	if m.dump {
		return
	}
	v := "on"
	if !m.hintsOn {
		v = "off"
	}
	app.SetShared("hints", v)
}
