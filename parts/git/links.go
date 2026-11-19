package git

// Links you can ctrl+click: markdown links (marked with OSC 8 while drawing) and any
// http(s):// address on screen. GitHub links open inside Dotes (repo, file, issue,
// PR, profile); links to other files in a repo open that file; the rest the browser.

import (
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func ghCmd(args ...string) *exec.Cmd { return exec.Command("gh", args...) }

// osc8 marks text as a link to u (taken off again before the screen is drawn)
func osc8(u, text string) string {
	if u == "" {
		return text
	}
	return "\x1b]8;;" + u + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

type linkZone struct {
	x0, x1, y int
	url       string
}

var rawURL = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `)\]│]+`)

// takeLinks finds the links on the screen, remembers where they are, and removes the
// OSC 8 marks (so the terminal doesn't also act on them)
func takeLinks(content string, hx, hy int) (string, []linkZone) {
	var zones []linkZone
	lines := strings.Split(content, "\n")
	for y, line := range lines {
		if !strings.Contains(line, "\x1b]8;") && !strings.Contains(line, "://") {
			continue
		}
		var b strings.Builder
		col, open, start := 0, "", 0
		i := 0
		for i < len(line) {
			if strings.HasPrefix(line[i:], "\x1b]8;") {
				end := strings.Index(line[i:], "\x1b\\")
				if end < 0 {
					break
				}
				seq := line[i+4 : i+end] // params;url
				u := ""
				if k := strings.Index(seq, ";"); k >= 0 {
					u = seq[k+1:]
				}
				if u != "" {
					open, start = u, col
				} else if open != "" {
					zones = append(zones, linkZone{start, col - 1, y, open})
					open = ""
				}
				i += end + 2
				continue
			}
			if line[i] == 0x1b { // other escape sequences: copy, no width
				j := i + 1
				if j < len(line) && line[j] == '[' {
					j++
					for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
						j++
					}
					j++
				}
				b.WriteString(line[i:min(j, len(line))])
				i = j
				continue
			}
			// one character
			r, size := utf8.DecodeRuneInString(line[i:])
			b.WriteString(line[i : i+size])
			col += ansi.StringWidth(string(r))
			i += size
		}
		if open != "" {
			zones = append(zones, linkZone{start, col - 1, y, open})
		}
		clean := b.String()
		// plain addresses (found below) and marked links: underline the one under the pointer
		plainHere := ansi.Strip(clean)
		for _, loc := range rawURL.FindAllStringIndex(plainHere, -1) {
			x0 := ansi.StringWidth(plainHere[:loc[0]])
			zones = append(zones, linkZone{x0, x0 + ansi.StringWidth(plainHere[loc[0]:loc[1]]) - 1, y, strings.TrimRight(plainHere[loc[0]:loc[1]], ".,;:")})
		}
		if y == hy {
			for _, z := range zones {
				if z.y == y && hx >= z.x0 && hx <= z.x1 {
					clean = underlineCols(clean, z.x0, z.x1)
					break
				}
			}
		}
		lines[y] = clean
	}
	return strings.Join(lines, "\n"), zones
}

func (m *model) linkAt(x, y int) string {
	for _, z := range m.linkZones {
		if z.y == y && x >= z.x0 && x <= z.x1 {
			return z.url
		}
	}
	return ""
}

// followLink opens a link: GitHub ones inside Dotes, files of the same repo, else the browser
func (m *model) followLink(link string) tea.Cmd {
	rd := m.rd
	if strings.HasPrefix(link, imgScheme) { // an image: opens outside, stay where you are
		return m.openImage(strings.TrimPrefix(link, imgScheme), rd)
	}
	m.rd = nil // GitHub links open a page in Dotes: leave full screen
	cmd := m.followLink2(link)
	if !m.homeLeft {
		m.rd = rd // it went to the browser: stay where you were
	}
	return cmd
}

func (m *model) followLink2(link string) tea.Cmd {
	m.homeLeft = false
	if strings.HasPrefix(link, "#") || strings.HasPrefix(link, "mailto:") {
		return m.setStatus("That link points inside the page", false)
	}
	u, err := url.Parse(link)
	if err != nil {
		return m.openURL(link)
	}
	if u.Scheme == "" { // a link to another file in this repo
		if ex := m.ex; ex != nil {
			base := ex.dir
			if it, ok := m.exSel(); ok && it.kind == "file" {
				base = parentDir(it.ent.Path)
			}
			target := strings.TrimPrefix(path.Clean(path.Join(base, u.Path)), "/")
			m.homeLeft = true
			return m.gotoFile(ex.repo.NameWithOwner, target)
		}
		return m.setStatus("Can't tell which repo "+link+" belongs to", true)
	}
	if u.Host != "github.com" && u.Host != "www.github.com" {
		return m.openURL(link)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch {
	case len(parts) == 1 && parts[0] != "":
		m.homeLeft = true
		return m.openProfile(parts[0])
	case len(parts) >= 4 && parts[2] == "issues":
		n, _ := strconv.Atoi(parts[3])
		m.homeLeft = true
		return m.gotoIssue(parts[0]+"/"+parts[1], n, true)
	case len(parts) >= 4 && parts[2] == "pull":
		n, _ := strconv.Atoi(parts[3])
		m.homeLeft = true
		return m.gotoPR(parts[0]+"/"+parts[1], n, true)
	case len(parts) >= 5 && (parts[2] == "blob" || parts[2] == "tree"):
		m.homeLeft = true
		return m.gotoFile(parts[0]+"/"+parts[1], strings.Join(parts[4:], "/"))
	case len(parts) == 2:
		m.homeLeft = true
		return m.gotoItem(parts[0]+"/"+parts[1], secCode, "", func(exItem) bool { return true }, nil)
	}
	return m.openURL(link)
}

// gotoFile opens repo nwo's Code at that file (or folder)
func (m *model) gotoFile(nwo, p string) tea.Cmd {
	cmd := m.gotoItem(nwo, secCode, p, func(it exItem) bool { return (it.kind == "file" || it.kind == "dir") && it.ent.Path == p }, nil)
	if m.ex != nil {
		if m.ex.ref == "" {
			m.ex.ref = "HEAD"
		}
		m.ex.dir = parentDir(p)
		return tea.Batch(cmd, m.exSelected())
	}
	return cmd
}

// underlineCols underlines screen columns x0..x1 of a line (keeping its colors)
func underlineCols(line string, x0, x1 int) string {
	var b strings.Builder
	col, on := 0, false
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			j := i + 1
			if j < len(line) && line[j] == '[' {
				j++
				for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
					j++
				}
				j++
			}
			b.WriteString(line[i:min(j, len(line))])
			if on { // a style reset in the middle: underline again
				b.WriteString("\x1b[4m")
			}
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		if col == x0 && !on {
			b.WriteString("\x1b[4m")
			on = true
		}
		b.WriteString(line[i : i+size])
		col += ansi.StringWidth(string(r))
		i += size
		if on && col > x1 {
			b.WriteString("\x1b[24m")
			on = false
		}
	}
	if on {
		b.WriteString("\x1b[24m")
	}
	return b.String()
}

// ---- images: Dotes doesn't draw them, ctrl+click shows them outside ----

const imgScheme = "dotes-git-img:"

// imgLink marks an image's address so a ctrl+click knows it's an image
func imgLink(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	return imgScheme + src
}

// openImage shows an image: web addresses in the browser, images of a GitHub repo on
// GitHub's page for that file, images next to a file on this PC in the PC's image viewer.
// Nothing gets downloaded.
func (m *model) openImage(src string, rd *reader) tea.Cmd {
	if u, err := url.Parse(src); err == nil && u.Scheme != "" {
		if u.Scheme == "http" || u.Scheme == "https" {
			return m.openURL(src)
		}
		return m.setStatus("Can't open that image ("+src+")", true)
	}
	clean := strings.SplitN(strings.SplitN(src, "#", 2)[0], "?", 2)[0]
	if dec, err := url.PathUnescape(clean); err == nil {
		clean = dec
	}
	if rd != nil && rd.path != "" { // dotes g file.md: the image is on this PC
		p := clean
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(rd.path), filepath.FromSlash(p))
		}
		if _, err := os.Stat(p); err != nil {
			return m.setStatus("Image not found: "+tildePath(p), true)
		}
		return m.openLocal(p)
	}
	// which repo: the README in full screen, the repo you're in, or the one highlighted in a list
	nwo, ref, base := "", "HEAD", ""
	switch {
	case rd != nil && rd.nwo != "":
		nwo = rd.nwo
	case m.ex != nil:
		nwo, base = m.ex.repo.NameWithOwner, m.ex.dir
		if m.ex.ref != "" {
			ref = m.ex.ref
		}
		if it, ok := m.exSel(); ok && it.kind == "file" {
			base = parentDir(it.ent.Path)
		}
	case m.selected() != nil:
		nwo = m.selected().NameWithOwner
	default:
		if it, ok := m.expSel(); ok && it.repo != nil {
			nwo = it.repo.NameWithOwner
		}
	}
	if nwo == "" {
		return m.setStatus("Can't tell which repo "+src+" belongs to", true)
	}
	target := path.Clean(path.Join("/", base, clean))
	if strings.HasPrefix(clean, "/") { // from the top of the repo
		target = path.Clean(clean)
	}
	return m.openURL("https://github.com/" + nwo + "/blob/" + ref + (&url.URL{Path: target}).EscapedPath())
}

// openLocal opens a file on this PC with its usual app (Photos for images on Windows)
func (m *model) openLocal(p string) tea.Cmd {
	if m.dump {
		return m.setStatus("Would open "+p, false)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", p)
	case "darwin":
		cmd = exec.Command("open", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	if err := cmd.Start(); err != nil {
		return m.setStatus("Couldn't open "+tildePath(p)+": "+err.Error(), true)
	}
	go cmd.Wait()
	return m.setStatus("Opened "+filepath.Base(p), false)
}
