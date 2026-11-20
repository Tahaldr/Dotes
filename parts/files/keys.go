package files

// Every key, in one table: the hint bar and the ~ help page are both drawn from it.

import "strings"

type keyHelp struct {
	group, keys, desc string
	hint              string // short label for the hint bar ("" = help page only)
}

var keyTable = []keyHelp{
	{"Move", "j / k  ↓ / ↑", "down / up", "move"},
	{"Move", "gg / G  home / end", "top / bottom", ""},
	{"Move", "ctrl+d / ctrl+u", "half a page down / up", ""},
	{"Move", "ctrl+f / ctrl+b  pgdn / pgup", "a page down / up", ""},
	{"Move", "shift+pgdn / shift+pgup", "half a page down / up", ""},
	{"Move", "mouse", "click to move, double-click to open, wheel to scroll", ""},
	{"Go", "h / l  ← / →", "parent folder / into the folder (or open the file)", "leave/enter"},
	{"Go", "H / L", "back / forward in history", ""},
	{"Go", "gh", "home folder", ""},
	{"Go", "gd", "Downloads", ""},
	{"Go", "gD", "Documents", ""},
	{"Go", "gc", "config folder (%APPDATA% / ~/.config)", ""},
	{"Go", "g space", "type a path to go to (suggestions as you type, tab completes, ↑↓ picks)", ""},
	{"Go", "gf", "follow a link to where it points", ""},
	{"Go", "gt", "open the Recycle Bin", ""},
	{"Go", "gp", "recent projects (when opened by dotes h projects)", ""},
	{"Go", "gi", "open this repo in Git (the folder under the cursor, or this one)", ""},
	{"Go", "gn", "this project's note in Notes (made the first time)", ""},
	{"Pick", "enter", "projects picker: open the file or folder in the first app", ""},
	{"Pick", "alt+letter", "projects picker: open it in that app (O lists them)", ""},
	{"Pick", "l", "projects picker: go into the folder instead", ""},
	{"Go", "h at C:\\", "show all drives", ""},
	{"Go", "z", "jump with fzf to anything below this folder", "jump"},
	{"Go", "Z", "jump with zoxide to a folder you use often", ""},
	{"Go", "b", "bookmark this folder (again removes it)", ""},
	{"Go", "'", "bookmarks: 1-9 jumps", "bookmarks"},
	{"Find", "f", "filter: only show names containing… (esc clears)", "filter"},
	{"Find", "/ or ?", "find a name, forward / backward", "find"},
	{"Find", "n / N", "next / previous match", ""},
	{"Find", "s", "search names below this folder (fd)", "search"},
	{"Find", "S", "search text in files below this folder (ripgrep)", ""},
	{"Find", "ctrl+s", "stop a running search", ""},
	{"Find", "h / esc", "leave the search results", ""},
	{"Sort", ",m / ,M", "modified, newest first / reversed", ""},
	{"Sort", ",b / ,B", "created, newest first / reversed", ""},
	{"Sort", ",s / ,S", "size, biggest first / reversed", ""},
	{"Sort", ",a / ,A", "name A to Z / Z to A", ""},
	{"Sort", ",n / ,N", "natural name order (2 before 10) / reversed", "sort"},
	{"Sort", ",e / ,E", "extension / reversed", ""},
	{"Sort", ",r", "random", ""},
	{"Sort", "ms mp mb mm mo mn", "rows show: size, permissions, created, modified, owner, nothing", ""},
	{"Select", "space", "select / unselect, then move down", "select"},
	{"Select", "v", "visual mode: select a range as you move (v or esc ends it)", ""},
	{"Select", "V", "visual unset: unselect a range as you move", ""},
	{"Select", "ctrl+a", "select everything here", ""},
	{"Select", "ctrl+r", "invert the selection here", ""},
	{"Select", "esc", "end visual mode, then clear the selection", ""},
	{"Files", "enter / o", "open: folders go in, text opens in your editor, the rest in its default app", "open"},
	{"Files", "y / x", "copy / cut the selection (or the file under the cursor)", "copy/cut"},
	{"Files", "p", "paste here (a taken name becomes \"name (1)\")", "paste"},
	{"Files", "P", "paste here and overwrite what has the same name", ""},
	{"Files", "Y / X", "cancel the copy / cut", ""},
	{"Files", "d  delete", "move to the Recycle Bin (asks first, u brings it back)", "trash"},
	{"Files", "D  shift+delete", "delete forever (asks first, can't be undone)", ""},
	{"Files", "a", "create a file, or a folder if the name ends with /", "create"},
	{"Files", "A", "create many: one name per line in your editor", ""},
	{"Files", "r  F2", "rename (with a selection: rename them all in your editor)", "rename"},
	{"Files", "- / _", "link to what's copied: absolute / relative path", ""},
	{"Files", "ctrl+-", "hard link to what's copied", ""},
	{"Files", "O  shift+enter", "open with…: editor, default app, choose an app, Explorer, VS Code, your own", "open with"},
	{"Files", "e", "extract the archive here (zip; 7z, rar, tar with 7-Zip)", ""},
	{"Files", "C", "compress the selection into a .zip (or .7z / .tar.gz with 7-Zip)", ""},
	{"Files", ";", "run a shell command here (%h = file under the cursor, %s = selected)", ""},
	{"Files", ":", "run a shell command and wait for enter before coming back", ""},
	{"Files", "u", "undo the last rename, move, paste, create or trash", "undo"},
	{"Copy", "cc", "copy the path", ""},
	{"Copy", "cd", "copy the folder's path", ""},
	{"Copy", "cf", "copy the file name", ""},
	{"Copy", "cn", "copy the name without extension", ""},
	{"View", ".", "show / hide hidden files", "hidden"},
	{"View", "J / K", "scroll the preview down / up", ""},
	{"View", "tab", "everything about the file: size of folders, dates, owner, git, text lines…", "info"},
	{"Tabs", "tt", "new tab here", ""},
	{"Tabs", "tr", "rename the tab", ""},
	{"Tabs", "1-9", "go to tab 1 to 9", ""},
	{"Tabs", "[ / ]", "previous / next tab", ""},
	{"Tabs", "{ / }", "move the tab left / right", ""},
	{"Tabs", "ctrl+c", "close the tab (quits on the last one)", ""},
	{"Other", "w", "tasks: copies, moves and deletes with progress (x cancels one)", "tasks"},
	{"Other", "F1 / ~", "this page (h here hides / shows the key hints)", "all keys"},
	{"Other", "g c , m", "first key of a combo: a popup shows what can follow", ""},
	{"Other", "q", "back to Home (opened with dotes f: quit, and the shell goes to this folder)", "home"},
	{"Other", "Q", "quit and stay where you were", ""},
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

// contextHints come first in the bar: what you can do right now
func (m *model) contextHints() []hintItem {
	var out []hintItem
	if m.picking() && len(m.pick.openers) > 0 {
		out = append(out, hintItem{"enter", m.pick.openers[0].name})
		for _, o := range m.pick.openers[1:] {
			if o.key != "" {
				out = append(out, hintItem{"alt+" + o.key, o.name})
			}
		}
	}
	switch {
	case m.visual == visSel:
		out = append(out, hintItem{"v/esc", "end visual"})
	case m.visual == visUnset:
		out = append(out, hintItem{"V/esc", "end unset"})
	case len(m.sel) > 0:
		out = append(out, hintItem{"esc", "clear selection"})
	}
	if m.yank != nil {
		out = append(out, hintItem{"p", "paste"}, hintItem{"P", "overwrite"}, hintItem{"Y", "cancel copy"})
	}
	return out
}

// hintRows lays the hints out in rows of width w (at most max rows); "~ all keys" is always kept
func hintRows(w, maxRows int, extra []hintItem) []string {
	items := append([]hintItem{}, extra...)
	var last hintItem
	for _, h := range keyTable {
		if h.hint == "" {
			continue
		}
		it := hintItem{hintKey(h.keys), h.hint}
		if strings.HasPrefix(h.keys, "F1") {
			last = it
			continue
		}
		dup := false
		for _, e := range extra {
			if e.k == it.k {
				dup = true
			}
		}
		if !dup {
			items = append(items, it)
		}
	}
	items = append(items, last)
	render := func(it hintItem) string { return fg(cHint).Render(it.k) + " " + fg(cMut).Render(it.d) }
	var rows []string
	row, rw := " ", 1
	for i, it := range items {
		s := render(it)
		sw := width(s) + 3
		lastRow := len(rows) == maxRows-1
		room := w - rw
		if lastRow && i < len(items)-1 {
			room -= width(render(last)) + 3 // keep space for "~ all keys"
		}
		if sw > room {
			if lastRow {
				rows = append(rows, row+render(last))
				return rows
			}
			rows = append(rows, row)
			row, rw = " ", 1
		}
		row += s + "   "
		rw += sw
	}
	return append(rows, row)
}
