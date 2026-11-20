package home

// Prayer times (prayer = City | Country | method in home.txt). A month of times comes from
// api.aladhan.com once and is kept in home-prayer.json, so Home never waits on the network.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
)

var prayerNames = []string{"Fajr", "Dhuhr", "Asr", "Maghrib", "Isha"}

type prayerData struct {
	Key  string                       `json:"Key"`
	Days map[string]map[string]string `json:"Days"` // "2006-01-02" → name → "15:04"
}

type prayerMsg struct {
	data *prayerData
	err  error
}

func prayerFile() string { return app.File("home-prayer.json") }

func prayerKey(p *prayerCfg, t time.Time) string {
	return fmt.Sprintf("%s|%s|%d|%s", p.city, p.country, p.method, t.Format("2006-01"))
}

// loadPrayer reads the saved month (nil when it's another month or place)
func loadPrayer(p *prayerCfg, now time.Time) *prayerData {
	b, err := os.ReadFile(prayerFile())
	if err != nil {
		return nil
	}
	var d prayerData
	if json.Unmarshal(b, &d) != nil || d.Key != prayerKey(p, now) {
		return nil
	}
	return &d
}

// fetchPrayer downloads this month's times in the background
func fetchPrayer(p *prayerCfg) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		u := fmt.Sprintf("https://api.aladhan.com/v1/calendarByCity/%d/%d?city=%s&country=%s&method=%d",
			now.Year(), int(now.Month()), url.QueryEscape(p.city), url.QueryEscape(p.country), p.method)
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(u)
		if err != nil {
			return prayerMsg{err: err}
		}
		defer resp.Body.Close()
		var r struct {
			Data []struct {
				Timings map[string]string `json:"timings"`
				Date    struct {
					Gregorian struct {
						Date string `json:"date"` // dd-MM-yyyy
					} `json:"gregorian"`
				} `json:"date"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			return prayerMsg{err: err}
		}
		d := &prayerData{Key: prayerKey(p, now), Days: map[string]map[string]string{}}
		for _, day := range r.Data {
			g := strings.Split(day.Date.Gregorian.Date, "-")
			if len(g) != 3 {
				continue
			}
			t := map[string]string{}
			for _, n := range prayerNames {
				t[n] = strings.Fields(day.Timings[n] + " ")[0]
			}
			d.Days[g[2]+"-"+g[1]+"-"+g[0]] = t
		}
		if len(d.Days) == 0 {
			return prayerMsg{err: fmt.Errorf("no times")}
		}
		if b, err := json.MarshalIndent(d, "", "  "); err == nil {
			os.WriteFile(prayerFile(), b, 0o644)
		}
		return prayerMsg{data: d}
	}
}

type prayerState struct {
	at       []time.Time
	next     int // index of the next one today (-1 = tomorrow's Fajr)
	nextName string
	in       string
	word     string // "in" (counting down) or "since" (it came in a few minutes ago)
}

// sinceFor: how long a prayer that just came in stays shown as "since" (the time
// between the adhan and the iqama; Maghrib's is short)
func sinceFor(name string) time.Duration {
	if name == "Maghrib" {
		return 10 * time.Minute
	}
	return 15 * time.Minute
}

func (d *prayerData) state(now time.Time) *prayerState {
	if d == nil {
		return nil
	}
	day := now.Format("2006-01-02")
	today := d.Days[day]
	if today == nil {
		return nil
	}
	parse := func(date, hm string) time.Time {
		t, _ := time.ParseInLocation("2006-01-02 15:04", date+" "+hm, now.Location())
		return t
	}
	st := &prayerState{next: -1}
	for _, n := range prayerNames {
		st.at = append(st.at, parse(day, today[n]))
	}
	for i, t := range st.at {
		if t.After(now) {
			st.next = i
			break
		}
	}
	var nextAt time.Time
	if st.next >= 0 {
		nextAt = st.at[st.next]
		st.nextName = prayerNames[st.next]
	} else { // after Isha: tomorrow's Fajr
		tom := now.AddDate(0, 0, 1).Format("2006-01-02")
		f := today["Fajr"]
		if tm := d.Days[tom]; tm != nil {
			f = tm["Fajr"]
		}
		nextAt, st.nextName = parse(tom, f), "Fajr"
	}
	st.word = "in"
	last := st.next - 1 // the one that came in last (today)
	if st.next < 0 {
		last = len(prayerNames) - 1
	}
	secs := int(nextAt.Sub(now).Seconds() + 0.999)
	if last >= 0 {
		if ago := now.Sub(st.at[last]); ago >= 0 && ago < sinceFor(prayerNames[last]) {
			st.next, st.nextName, st.word = last, prayerNames[last], "since"
			secs = int(ago.Seconds())
		}
	}
	switch {
	case secs >= 3600:
		st.in = fmt.Sprintf("%dh %02dm", secs/3600, secs%3600/60)
	case secs >= 60:
		st.in = fmt.Sprintf("%dm %02ds", secs/60, secs%60)
	default:
		st.in = fmt.Sprintf("%ds", secs)
	}
	return st
}

// prayerBox: a quiet box with the day as its title and the countdown in its bottom edge:
//
//	╭─ Thursday, Nov 6 · 14:21 ──────────────────────╮
//	│    Fajr    Dhuhr     Asr    Maghrib    Isha    │
//	│   05:24    12:19    15:13    17:39     18:54   │
//	╰─────────────── Asr in 34m 12s ─────────────────╯
//
// Narrow windows get one line.
func (m *model) prayerBox(w int, title string) []string {
	st := m.prayer.state(m.now)
	if st == nil {
		return nil
	}
	t := m.cfg.th
	count := fg(t.mut).Render(st.nextName + " " + st.word + " " + st.in)
	const col = 9
	inner := col*5 + 2
	if w < inner+4 {
		return []string{fg(t.mut).Render("  ") + count}
	}
	center := func(s string, w int) string {
		l := (w - width(s)) / 2
		return strings.Repeat(" ", max(0, l)) + s + strings.Repeat(" ", max(0, w-width(s)-l))
	}
	names, times := "", ""
	for i, n := range prayerNames {
		c := t.mut
		if i == st.next {
			c = t.acc
		}
		names += fg(c).Render(center(n, col))
		times += fg(c).Render(center(st.at[i].Format("15:04"), col))
	}
	b := fg(t.selBg)
	side := b.Render("│")
	top := b.Render("╭" + strings.Repeat("─", inner) + "╮")
	if title != "" {
		top = b.Render("╭─ ") + fg(t.mut).Render(title) + b.Render(" "+strings.Repeat("─", max(0, inner-width(title)-3))+"╮")
	}
	cw := width(count) + 2
	l := (inner - cw) / 2
	bottom := b.Render("╰"+strings.Repeat("─", l)) + " " + count + " " + b.Render(strings.Repeat("─", max(0, inner-cw-l))+"╯")
	return []string{top, side + " " + names + " " + side, side + " " + times + " " + side, bottom}
}
