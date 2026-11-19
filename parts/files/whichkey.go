package files

// Which-key: after the first key of a two-key combo (g, c, ",", m) a popup lists what can
// follow, like which-key.nvim. And the hint bar can be hidden (h on the ~ page), like TermNook.

import "github.com/Tahaldr/dotes/internal/app"

var comboKeys = map[string][][2]string{
	"g": {{"g", "top"}, {"h", "home"}, {"d", "Downloads"}, {"D", "Documents"}, {"c", "config folder"},
		{"space", "type a path"}, {"f", "follow link"}, {"t", "Recycle Bin"}, {"p", "recent projects"}},
	"c": {{"c", "copy path"}, {"d", "copy folder path"}, {"f", "copy name"}, {"n", "copy name, no extension"}},
	",": {{"m", "modified"}, {"b", "created"}, {"s", "size"}, {"a", "name A-Z"}, {"n", "natural name"},
		{"e", "extension"}, {"r", "random"}, {"shift", "reversed"}},
	"t": {{"t", "new tab"}, {"r", "rename tab"}},
	"m": {{"s", "size"}, {"p", "permissions"}, {"b", "created"}, {"m", "modified"}, {"o", "owner"}, {"n", "nothing"}},
}

var comboTitle = map[string]string{"t": "Tabs", "g": "Go to", "c": "Copy", ",": "Sort by", "m": "Rows show"}

// whichKeyPopup sits above the status bar while a combo is waiting for its second key
func (m *model) whichKeyPopup(lines []string) {
	items, ok := comboKeys[m.pending]
	if !ok || !m.hintsOn { // hidden hints hide this too
		return
	}
	cellW := 0
	for _, it := range items {
		cellW = max(cellW, width(it[0])+2+width(it[1]))
	}
	cellW += 4
	w := min(m.w-2, max(40, cellW*min(len(items), 4)+4))
	cols := max(1, (w-4)/cellW)
	var body []string
	row := ""
	for i, it := range items {
		row += pad(fg(cAcc).Bold(true).Render(it[0])+"  "+fg(cTxt).Render(it[1]), cellW)
		if (i+1)%cols == 0 || i == len(items)-1 {
			body = append(body, row)
			row = ""
		}
	}
	body = append(body, fg(cMut).Render("esc cancel"))
	title := fg(cAcc).Bold(true).Render(m.pending + "  " + comboTitle[m.pending])
	box := popupBox(title, w, body, fg(cAcc))
	y := m.bodyY + m.bodyH - len(box)
	put(lines, m.w-w-1, max(1, y), box, w)
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
