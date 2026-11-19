package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
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

func TestFirstFrameEveryStyleAndSize(t *testing.T) {
	for _, st := range styles {
		for _, sz := range [][2]int{{1, 1}, {10, 4}, {20, 5}, {40, 12}, {80, 24}, {200, 60}} {
			m := fresh(t)
			m.style = st
			m.View()
			run(m, sz[0], sz[1])
			run(m, sz[0], sz[1], "t", "down", "?", "x", "right")
		}
	}
}

func TestCardsMoveOpenSwitch(t *testing.T) {
	m := fresh(t)
	o := run(m, 110, 30)
	if !strings.Contains(o, "Nvim") || !strings.Contains(o, "Git") {
		t.Fatalf("cards:\n%s", o)
	}
	m.sel = 0
	run(m, 110, 30, "right", "right", "right") // Files
	if m.apps[m.sel].name != "Files" {
		t.Fatalf("sel = %s", m.apps[m.sel].name)
	}
	cmd := m.open(m.apps[m.sel])
	if sw, ok := cmd().(app.SwitchMsg); !ok || sw.ID != "files" {
		t.Fatalf("Files should switch to the Files part, got %#v", cmd())
	}
	o = run(m, 110, 30, "v") // VS Code by its key: in dump mode it says what it would run
	if !strings.Contains(o, "would run: code .") {
		t.Fatalf("key v:\n%s", o)
	}
}

// the picked parts are apps on Home by themselves; the others aren't there
func TestPartApps(t *testing.T) {
	defer app.SaveParts([]string{"home", "files", "notes", "git"})
	names := func(apps []appItem) string {
		var n []string
		for _, a := range apps {
			n = append(n, a.name+"/"+a.key)
		}
		return strings.Join(n, " ")
	}
	base := []appItem{{name: "Nvim", key: "n", run: "nvim ."}, {name: "Files", key: "f", run: "projects"}, {name: "Notes", key: "o", run: "dotes n"}, {name: "Shell", key: "s"}}
	if got := names(withParts(base, []string{"home", "files", "git"})); got != "Nvim/n Files/f Git/g Shell/s" {
		t.Fatalf("got %s", got)
	}
	if got := names(withParts(base, []string{"home", "notes"})); got != "Nvim/n Notes/o Shell/s" {
		t.Fatalf("got %s", got)
	}
}

func TestThemesAndPalette(t *testing.T) {
	m := fresh(t)
	run(m, 100, 26, "t", "down", "down", "enter")
	if m.style != "palette" {
		t.Fatalf("style = %s", m.style)
	}
	o := run(m, 100, 26, "g", "i")
	if !strings.Contains(o, "1/6") || !strings.Contains(o, "Git") {
		t.Fatalf("palette filter:\n%s", o)
	}
	o = run(m, 100, 26, "esc", "esc") // clear, then quit is fine
	if m.filter != "" {
		t.Fatalf("esc clears the filter")
	}
	_ = o
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
	if !strings.Contains(o, "Asr in 1m 00s") || !strings.Contains(o, "Maghrib") {
		t.Fatalf("prayer box:\n%s", o)
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
	if c.style != "info" || c.github != "someone" || c.prayer == nil || c.prayer.method != 21 || len(c.apps) != 3 {
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
