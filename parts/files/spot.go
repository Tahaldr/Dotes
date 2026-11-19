package files

// Tab: everything about the file under the cursor (folder sizes are counted in the background).

import (
	"bytes"
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type spotState struct {
	path   string
	rows   [][2]string
	scroll int
}

type spotSizeMsg struct {
	path         string
	size         int64
	files, dirs  int
	stoppedEarly bool
}

func (m *model) openSpot() tea.Cmd {
	c := m.current()
	if c == nil {
		return nil
	}
	e := *c
	sp := &spotState{path: e.path}
	add := func(k, v string) {
		if v != "" {
			sp.rows = append(sp.rows, [2]string{k, v})
		}
	}
	fi, err := os.Lstat(e.path)
	add("name", filepath.Base(e.path))
	add("folder", prettyPath(filepath.Dir(e.path)))
	if err != nil {
		add("error", errText(err))
		m.spot = sp
		return nil
	}
	ext := strings.ToLower(filepath.Ext(e.name))
	kind := "File"
	switch {
	case e.drive:
		kind = "Drive"
	case e.isLink && e.isDir:
		kind = "Link to a folder"
	case e.isLink:
		kind = "Link to a file"
	case e.isDir && e.isRepo:
		kind = "Folder (git repo)"
	case e.isDir:
		kind = "Folder"
	case binaryExt[ext] != "":
		kind = binaryExt[ext]
	}
	add("kind", kind)
	var cmd tea.Cmd
	if e.isDir {
		add("size", "counting…")
		p := e.path
		cmd = func() tea.Msg { return folderSize(p) }
	} else {
		add("size", fmt.Sprintf("%s (%d bytes)", humanSize(fi.Size()), fi.Size()))
	}
	if e.isLink {
		if t, err := os.Readlink(e.path); err == nil {
			add("points to", t)
		}
	}
	add("created", fileBtime(fi).Format("2006-01-02 15:04:05"))
	add("modified", fi.ModTime().Format("2006-01-02 15:04:05"))
	if a := fileAtime(fi); !a.IsZero() {
		add("opened", a.Format("2006-01-02 15:04:05"))
	}
	add("mode", fi.Mode().String())
	add("attributes", fileAttrs(fi))
	add("owner", fileOwner(e.path))
	if mk, ok := m.gitMarks[strings.ToLower(e.path)]; ok {
		add("git", map[string]string{"M": "changed", "A": "added", "?": "new (not tracked)", "D": "deleted",
			"R": "renamed", "C": "copied", "U": "conflict", "!": "ignored", "•": "has changes inside", "T": "type changed"}[mk])
	}
	if !e.isDir {
		if imageExt[ext] {
			if f, err := os.Open(e.path); err == nil {
				if c, format, err := image.DecodeConfig(f); err == nil {
					add("pixels", fmt.Sprintf("%d × %d (%s)", c.Width, c.Height, format))
				}
				f.Close()
			}
		} else if data, err := readHead(e.path); err == nil {
			if text, ok := asText(data); ok {
				enc := "UTF-8"
				switch {
				case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
					enc = "UTF-8 with BOM"
				case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
					enc = "UTF-16"
				}
				ends := "LF (Unix)"
				if strings.Contains(text, "\r\n") {
					ends = "CRLF (Windows)"
				}
				n := strings.Count(text, "\n")
				if !strings.HasSuffix(text, "\n") && text != "" {
					n++
				}
				more := ""
				if fi.Size() > previewBytes {
					more = "+ (only the start was read)"
				}
				add("text", fmt.Sprintf("%d%s lines · %s · %s", n, more, enc, ends))
			}
		}
	}
	m.spot = sp
	return cmd
}

// folderSize adds up a folder (stops after 200 000 entries so huge trees can't hang)
func folderSize(dir string) spotSizeMsg {
	r := spotSizeMsg{path: dir}
	seen := 0
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return nil
		}
		seen++
		if seen > 200000 {
			r.stoppedEarly = true
			return filepath.SkipAll
		}
		if d.IsDir() {
			r.dirs++
			return nil
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() {
			r.files++
			r.size += info.Size()
		}
		return nil
	})
	return r
}

func (m *model) spotSize(msg spotSizeMsg) {
	if m.spot == nil || m.spot.path != msg.path {
		return
	}
	v := fmt.Sprintf("%s · %s · %s", humanSize(msg.size), plural(msg.files, "file"), plural(msg.dirs, "folder"))
	if msg.stoppedEarly {
		v = "more than " + v
	}
	for i := range m.spot.rows {
		if m.spot.rows[i][0] == "size" {
			m.spot.rows[i][1] = v
		}
	}
}

func (m *model) spotKey(k tea.KeyPressMsg) {
	switch k.String() {
	case "tab", "esc", "q":
		m.spot = nil
	case "j", "down":
		m.spot.scroll++
	case "k", "up":
		m.spot.scroll = max(0, m.spot.scroll-1)
	case "c":
		m.pending = "c" // cc / cd / cf / cn still work from here
		m.spot = nil
	}
}

func (m *model) spotPopup(lines []string) {
	sp := m.spot
	if sp == nil {
		return
	}
	w := m.popupWidth(56, 90)
	inner := w - 4
	var body []string
	for _, r := range sp.rows {
		k := fg(cMut).Render(pad(r[0], 11))
		vals := wrapText(r[1], inner-11, fg(cTxt))
		if len(vals) == 0 {
			vals = []string{""}
		}
		for i, v := range vals {
			if i == 0 {
				body = append(body, k+v)
			} else {
				body = append(body, strings.Repeat(" ", 11)+v)
			}
		}
	}
	room := max(3, m.h-8)
	sp.scroll = max(0, min(sp.scroll, len(body)-room))
	if len(body) > room {
		body = body[sp.scroll : sp.scroll+room]
	}
	body = append(body, fg(cDim).Render(strings.Repeat("─", inner)), keyHints("tab/esc", "close", "j/k", "scroll", "c", "copy…"))
	box := popupBox(fg(cAcc).Bold(true).Render(icInfo+" "+filepath.Base(sp.path)), w, body, fg(cAcc))
	put(lines, (m.w-w)/2, 2, box, w)
}
