package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/internal/ui"
	"github.com/Tahaldr/dotes/parts/git"
	"github.com/Tahaldr/dotes/parts/notes"
	"github.com/charmbracelet/x/ansi"
)

func TestMain(m *testing.M) {
	d, _ := os.MkdirTemp("", "dotes-test-config")
	os.Setenv("DOTES_CONFIG", d)
	code := m.Run()
	os.RemoveAll(d)
	os.Exit(code)
}

func run(m *model, w, h int, keys ...string) string {
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	for _, k := range keys {
		m.View()
		drain(m, func() tea.Msg { return parseInput(k) })
	}
	return ansi.Strip(m.View().Content)
}

func fresh(t *testing.T) *model {
	t.Helper()
	os.Remove(configFile())
	os.Remove(app.File("home-style.txt"))
	app.SaveParts([]string{"home", "files", "notes", "git"})
	m := newModel()
	m.dump = true
	return m
}

func TestFirstFrameEverySize(t *testing.T) {
	for _, sz := range [][2]int{{1, 1}, {10, 4}, {20, 5}, {40, 12}, {80, 24}, {200, 60}} {
		m := fresh(t)
		m.View()
		run(m, sz[0], sz[1])
		run(m, sz[0], sz[1], "down", "F1", "x", "right")
	}
}

func TestCardsMoveOpen(t *testing.T) {
	m := fresh(t)
	o := run(m, 110, 30)
	if !strings.Contains(o, "Nvim") || strings.Contains(o, "pick one") || !strings.Contains(o, "Home") {
		t.Fatalf("cards:\n%s", o)
	}
	if strings.Contains(o, "file manager") || strings.Contains(o, "todos and notes") {
		t.Fatalf("the parts are in the tab row, not cards:\n%s", o)
	}
	o = run(m, 110, 30, "v") // VS Code by its key: in dump mode it says what it would run
	if !strings.Contains(o, "would run: code .") {
		t.Fatalf("key v:\n%s", o)
	}
}

// up to 5 cards a row, the rows evened out
func TestGridRows(t *testing.T) {
	m := fresh(t)
	for _, c := range []struct{ n, per int }{{3, 3}, {5, 5}, {6, 3}, {7, 4}, {11, 4}} {
		m.apps = nil
		for i := 0; i < c.n; i++ {
			m.apps = append(m.apps, appItem{name: "App" + itoa(i), run: "x"})
		}
		m.grid(200)
		if m.perRow != c.per {
			t.Errorf("%d apps: %d a row, want %d", c.n, m.perRow, c.per)
		}
	}
}

// the parts aren't cards (they're in the tab row); the picker needs Files
func TestPartApps(t *testing.T) {
	names := func(apps []appItem) string {
		var n []string
		for _, a := range apps {
			n = append(n, a.name+"/"+a.key)
		}
		return strings.Join(n, " ")
	}
	base := []appItem{{name: "Nvim", key: "n", run: "nvim ."}, {name: "Projects", key: "f", run: "projects"}, {name: "Notes", key: "o", run: "dotes n"}, {name: "Shell", key: "s"}}
	if got := names(withParts(base, []string{"home", "files", "git"})); got != "Nvim/n Projects/f Shell/s" {
		t.Fatalf("got %s", got)
	}
	if got := names(withParts(base, []string{"home", "notes"})); got != "Nvim/n Shell/s" {
		t.Fatalf("got %s", got)
	}
}

// the glance: a panel per picked part, a click on a line goes right there
func TestGlance(t *testing.T) {
	m := fresh(t)
	m.notesG = &notes.GlanceInfo{Open: 4, Today: 1, Items: []notes.GlanceItem{{Text: "buy milk", When: "today", WhenStyle: fg(ui.Yellow)}}}
	m.gitG = &git.GlanceInfo{Local: 9, Repos: []git.GlanceRepo{{Name: "Dotes", Dir: `C:\x\Dotes`, Dirty: 3, Ahead: 1}}}
	m.recent = []string{filepath.Join("C:", "code", "Shop")}
	o := run(m, 130, 34)
	for _, want := range []string{"Notes", "1 today", "buy milk", "Git", "Dotes", "●3", "↑1", "Projects", "Shop"} {
		if !strings.Contains(o, want) {
			t.Fatalf("no %q in the glance:\n%s", want, o)
		}
	}
	var line *pzone
	for i := range m.pzones {
		if m.pzones[i].path == `C:\x\Dotes` {
			line = &m.pzones[i]
		}
	}
	if line == nil {
		t.Fatalf("no zone for the Dotes line: %+v", m.pzones)
	}
	cmd := m.click(line.x0+2, line.y0)
	if sw, ok := cmd().(app.SwitchMsg); !ok || sw.ID != "git" || sw.Path != `C:\x\Dotes` {
		t.Fatalf("a click on the repo should open it in Git: %#v", cmd())
	}
}

func TestClickOpens(t *testing.T) {
	m := fresh(t)
	run(m, 110, 30)
	z := m.zones[1]
	run(m, 110, 30, "click:"+itoa(z.x0+2)+","+itoa(z.y0+2))
	if m.sel != 1 {
		t.Fatalf("click selects: sel=%d zones=%v", m.sel, m.zones)
	}
	o := run(m, 110, 30, "click:"+itoa(z.x0+2)+","+itoa(z.y0+2))
	if !strings.Contains(o, "would run: code .") {
		t.Fatalf("second click opens:\n%s", o)
	}
}

func itoa(n int) string {
	return strings.TrimSpace(strings.Repeat(" ", 0) + string(rune('0'+n/100%10)) + string(rune('0'+n/10%10)) + string(rune('0'+n%10)))
}

func TestPrayerState(t *testing.T) {
	d := &prayerData{Days: map[string]map[string]string{
		"2026-11-19": {"Fajr": "05:30", "Dhuhr": "12:20", "Asr": "15:10", "Maghrib": "17:35", "Isha": "18:50"},
		"2026-11-20": {"Fajr": "05:31", "Dhuhr": "12:20", "Asr": "15:10", "Maghrib": "17:35", "Isha": "18:50"},
	}}
	at := func(hm string) time.Time {
		x, _ := time.ParseInLocation("2006-01-02 15:04", "2026-11-19 "+hm, time.Local)
		return x
	}
	if st := d.state(at("14:00")); st.nextName != "Asr" || st.in != "1h 10m" {
		t.Fatalf("at 14:00: %+v", st)
	}
	if st := d.state(at("20:00")); st.nextName != "Fajr" || st.next != -1 {
		t.Fatalf("after Isha: %+v", st)
	}
	m := fresh(t)
	m.prayer, m.now = d, at("15:09")
	m.cfg.prayer = &prayerCfg{city: "X"}
	o := run(m, 110, 34)
	if !strings.Contains(o, "in 1m 00s") || !strings.Contains(o, "Asr") || !strings.Contains(o, "Maghrib") {
		t.Fatalf("prayer panel:\n%s", o)
	}
}

func TestImportTermNook(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.psd1"), []byte(`@{
    Style  = 'info'
    Logo   = 'my-logo.txt'
    GitHub = 'someone'
    Recent = $false
    Prayer = @{ City = 'Casablanca'; Country = 'Morocco'; Method = 21 }
    Apps = @(
        @{ Name = 'Nvim';    Hint = 'editor';    Key = 'n'; Icon = 0xF36F;  Run = 'nvim .' }
        @{ Name = 'GitHub';  Hint = 'termgit';   Key = 'g'; Icon = 0xF09B;  Run = 'termgit' }
        @{ Name = 'Files';   Hint = 'termfiles'; Key = 'f'; Icon = 0xF07B;  Run = 'nook -p' }
    )
    Projects = @{
        Roots    = @('~\code', '~\apps')
        OpenWith = @('Nvim', 'Shell')
        Search   = '~'
        Skip     = @('AppData', 'go')
    }
}`), 0o644)
	os.WriteFile(filepath.Join(dir, "my-logo.txt"), []byte("LOGO\n"), 0o644)
	os.Remove(configFile())
	importFrom(dir)
	c := loadConfig()
	if c.github != "someone" || c.prayer == nil || c.prayer.method != 21 || len(c.apps) != 3 {
		t.Fatalf("imported: %+v", c)
	}
	if c.apps[1].run != "dotes g" || c.apps[2].run != "projects" || c.apps[0].icon != "\uf36f" {
		t.Fatalf("apps: %+v", c.apps)
	}
	if len(c.openWith) != 2 || c.openWith[1].name != "Shell" || strings.Join(c.skip, ",") != "AppData,go" || c.logo[0] != "LOGO" {
		t.Fatalf("projects / logo: %+v %v %v", c.openWith, c.skip, c.logo)
	}
	os.Remove(configFile())
	os.Remove(app.File("home-logo.txt"))
}
