package home

// Settings in the Dotes config folder (home.txt), made with every option explained on first start.
// The first time, a TermNook config.psd1 (~/.termnook) is turned into home.txt.

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/internal/ui"
)

type appItem struct {
	name, hint, key, icon string
	run                   string // a command; "dotes f/n/g" switches part, "projects" opens the project picker, "" = back to the prompt
}

type prayerCfg struct {
	city, country string
	method        int
}

type theme struct {
	gradFrom, gradTo, acc, txt, mut, selBg, selHint, hot color.Color
}

type config struct {
	style    string // cards, info, palette, list
	logo     []string
	github   string
	recent   bool
	prayer   *prayerCfg
	apps     []appItem
	roots    []string // project folders
	openWith []appItem
	search   string
	skip     []string
	th       theme
	warnings []string
}

var styles = []string{"cards", "info", "palette", "list"}
var styleNames = map[string]string{"cards": "Card row", "info": "Info panel", "palette": "Command palette", "list": "Classic list"}

const defaultConfig = `# Home settings (Dotes): key = value. Lines starting with # are ignored.
# Changes apply the next time Home opens. Shared settings (hints, parts) are in dotes.txt.

# layout: cards, info, palette or list (t inside switches and remembers it)
style = cards

# the logo: a text file in this folder (empty = none)
logo = home-logo.txt

# your GitHub user, shown as a link in the info layout (empty hides it)
github =

# put the apps you used last first: on / off
recent = off

# today's prayer times with a countdown (from api.aladhan.com once a month). Empty = off.
#   prayer = City | Country | method   (methods: https://aladhan.com/calculation-methods)
# prayer = Casablanca | Morocco | 21
prayer =

# your apps, in order:  app = Name | hint | key | icon | command
#   key   one letter that opens it ('t', 'q', 'x' are Home's own)
#   icon  a Nerd Font code, e.g. f36f (https://www.nerdfonts.com/cheat-sheet), or empty
#   command  what to run, as you'd type it. Special ones:
#            dotes f / dotes n / dotes g   switch to that Dotes part
#            projects                      the project picker
#            (empty)                       back to the prompt
app = Nvim | editor | n | f36f | nvim .
app = VS Code | code | v | f0a1e | code .
app = Projects | pick one | p | f07b | projects
app = Files | file manager | f | f024b | dotes f
app = Notes | todos and notes | o | f082e | dotes n
app = Git | github | g | f02a2 | dotes g

# project picker: folders searched for projects (with .git, package.json, go.mod, ...), ; between
project_roots = ~/code ; ~/projects ; ~/source/repos ; ~/Documents/GitHub
# apps (names from above) a project can open in: enter = the first, alt+key the others. Shell = just go there
project_open_with = Nvim, VS Code, Shell
# where typed paths start
project_search = ~
# folder names hidden until you type the start of their name
project_skip = AppData, node_modules, scoop, vendor

# colors (#rrggbb), NvChad's onedark by default
color_grad_from = #61afef
color_grad_to = #c678dd
color_accent = #61afef
color_text = #abb2bf
color_muted = #565c64
color_sel_bg = #3e4451
color_sel_hint = #818792
color_hot = #e7c787
`

func configFile() string { return app.File("home.txt") }

var reserved = map[string]bool{"t": true, "q": true, "x": true}

func loadConfig() config {
	if _, err := os.Stat(configFile()); err != nil {
		importTermNook()
	}
	c := config{style: "cards", recent: false, search: home(""), th: theme{
		gradFrom: ui.Acc, gradTo: ui.Purple, acc: ui.Acc, txt: ui.Txt, mut: ui.Mut, selBg: ui.Dim, selHint: ui.Hint,
		hot: lipgloss.Color("#e7c787")}}
	logoName := "home-logo.txt"
	seen := map[string]string{}
	var openWith []string
	for _, kv := range app.ReadPairs(configFile(), defaultConfig) {
		k, v := kv[0], kv[1]
		switch k {
		case "style":
			if styleNames[v] != "" {
				c.style = v
			} else {
				c.warnings = append(c.warnings, "style '"+v+"' is unknown, using cards")
			}
		case "logo":
			logoName = v
		case "github":
			c.github = v
		case "recent":
			c.recent = v == "on" || v == "true" || v == "yes"
		case "prayer":
			p := splitBar(v, 3)
			if p[0] != "" {
				pc := &prayerCfg{city: p[0], country: p[1], method: 3}
				if n, err := strconv.Atoi(p[2]); err == nil {
					pc.method = n
				}
				c.prayer = pc
			}
		case "app":
			p := splitBar(v, 5)
			if p[0] == "" {
				c.warnings = append(c.warnings, "an app without a name was skipped")
				continue
			}
			a := appItem{name: p[0], hint: p[1], key: strings.ToLower(p[2]), run: p[4]}
			switch {
			case len([]rune(a.key)) > 1:
				c.warnings = append(c.warnings, a.name+": key must be one letter")
				a.key = ""
			case reserved[a.key]:
				c.warnings = append(c.warnings, a.name+": key '"+a.key+"' is Home's own")
				a.key = ""
			case a.key != "" && seen[a.key] != "":
				c.warnings = append(c.warnings, a.name+": key '"+a.key+"' is already used by "+seen[a.key])
				a.key = ""
			}
			if a.key != "" {
				seen[a.key] = a.name
			}
			if p[3] != "" {
				if cp, err := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(p[3]), "0x"), 16, 32); err == nil {
					a.icon = string(rune(cp))
				} else {
					c.warnings = append(c.warnings, a.name+": icon '"+p[3]+"' is not a hex code")
				}
			}
			c.apps = append(c.apps, a)
		case "project_roots":
			for _, r := range splitList(v, ";") {
				if p := home(r); isDir(p) {
					c.roots = append(c.roots, p)
				}
			}
		case "project_open_with":
			openWith = splitList(v, ",")
		case "project_search":
			c.search = home(v)
		case "project_skip":
			c.skip = splitList(v, ",")
		default:
			if strings.HasPrefix(k, "color_") {
				if !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(v) {
					c.warnings = append(c.warnings, k+" '"+v+"' is not a #rrggbb color")
					continue
				}
				col := lipgloss.Color(v)
				switch k {
				case "color_grad_from":
					c.th.gradFrom = col
				case "color_grad_to":
					c.th.gradTo = col
				case "color_accent":
					c.th.acc = col
				case "color_text":
					c.th.txt = col
				case "color_muted":
					c.th.mut = col
				case "color_sel_bg":
					c.th.selBg = col
				case "color_sel_hint":
					c.th.selHint = col
				case "color_hot":
					c.th.hot = col
				}
			}
		}
	}
	if len(c.apps) == 0 {
		c.warnings = append(c.warnings, "no apps in home.txt")
		c.apps = []appItem{{name: "Shell", hint: "prompt", key: "s"}}
	}
	for _, n := range openWith {
		found := false
		for _, a := range c.apps {
			if strings.EqualFold(a.name, n) {
				c.openWith, found = append(c.openWith, a), true
				break
			}
		}
		if !found && strings.EqualFold(n, "Shell") {
			c.openWith, found = append(c.openWith, appItem{name: "Shell", hint: "prompt", key: "s", icon: ""}), true
		}
		if !found {
			c.warnings = append(c.warnings, "project_open_with: no app named '"+n+"'")
		}
	}
	if len(c.openWith) == 0 { // apps that open "." are the natural ones
		for _, a := range c.apps {
			if strings.HasSuffix(a.run, " .") {
				c.openWith = append(c.openWith, a)
			}
		}
	}
	if !isDir(c.search) {
		c.search = home("")
	}
	if logoName != "" {
		c.logo = readLogo(app.File(logoName))
		if c.logo == nil && logoName == "home-logo.txt" {
			c.logo = strings.Split(defaultLogo, "\n")
		} else if c.logo == nil {
			c.warnings = append(c.warnings, "logo file '"+logoName+"' not found")
		}
	}
	return c
}

const defaultLogo = "█▀▄ █▀█ ▀█▀ █▀▀ █▀\n█▄▀ █▄█  █  ██▄ ▄█"

func readLogo(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimPrefix(string(b), string(rune(0xFEFF))), "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func splitBar(v string, n int) []string {
	p := strings.SplitN(v, "|", n)
	for len(p) < n {
		p = append(p, "")
	}
	for i := range p {
		p[i] = strings.TrimSpace(p[i])
	}
	return p
}

func splitList(v, sep string) []string {
	var out []string
	for _, s := range strings.Split(v, sep) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func home(p string) string {
	h, _ := os.UserHomeDir()
	switch {
	case p == "" || p == "~":
		return h
	case strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`):
		return filepath.Join(h, p[2:])
	}
	return filepath.Clean(os.ExpandEnv(p))
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// ---- moving over from TermNook (~/.termnook/config.psd1 and its files) ----

var (
	psStr  = func(k string) *regexp.Regexp { return regexp.MustCompile(`(?m)^\s*` + k + `\s*=\s*'([^']*)'`) }
	psApp  = regexp.MustCompile(`@\{\s*Name\s*=\s*'([^']*)'([^}]*)\}`)
	psPart = func(k string) *regexp.Regexp {
		return regexp.MustCompile(k + `\s*=\s*(?:'([^']*)'|(0x[0-9A-Fa-f]+|\d+))`)
	}
	psList = func(k string) *regexp.Regexp { return regexp.MustCompile(k + `\s*=\s*@\(([^)]*)\)`) }
)

// what TermNook ran, as Dotes does it
var hintMap = map[string]string{"termgit": "dotes g", "termnotes": "dotes n", "termfiles": "projects"}

var runMap = map[string]string{"termgit": "dotes g", "termnotes": "dotes n", "termfiles": "dotes f", "tf": "dotes f",
	"nook -p": "projects", "nook -projects": "projects", "nook --projects": "projects"}

func importTermNook() {
	if os.Getenv("DOTES_CONFIG") != "" { // tests: never read the real old settings
		return
	}
	importFrom(home("~/.termnook"))
}

func importFrom(dir string) {
	b, err := os.ReadFile(filepath.Join(dir, "config.psd1"))
	if err != nil {
		return
	}
	s := string(b)
	get := func(k string) string {
		if m := psStr(k).FindStringSubmatch(s); m != nil {
			return m[1]
		}
		return ""
	}
	list := func(k string) []string {
		m := psList(k).FindStringSubmatch(s)
		if m == nil {
			return nil
		}
		var out []string
		for _, q := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(m[1], -1) {
			out = append(out, q[1])
		}
		return out
	}
	out := defaultConfig
	set := func(key, val string) {
		re := regexp.MustCompile(`(?m)^` + key + ` =.*$`)
		out = re.ReplaceAllLiteralString(out, key+" = "+val)
	}
	if v := get("Style"); v != "" {
		set("style", v)
	}
	if st, err := os.ReadFile(filepath.Join(dir, "style.txt")); err == nil && styleNames[strings.TrimSpace(string(st))] != "" {
		set("style", strings.TrimSpace(string(st)))
	}
	if v := get("GitHub"); v != "" {
		set("github", v)
	}
	if regexp.MustCompile(`(?m)^\s*Recent\s*=\s*\$true`).MatchString(s) {
		set("recent", "on")
	}
	if m := regexp.MustCompile(`Prayer\s*=\s*@\{([^}]*)\}`).FindStringSubmatch(s); m != nil {
		city, country, method := partOf(m[1], "City"), partOf(m[1], "Country"), partOf(m[1], "Method")
		if city != "" {
			if method == "" {
				method = "3"
			}
			out = strings.Replace(out, "\nprayer =\n", "\nprayer = "+city+" | "+country+" | "+method+"\n", 1)
		}
	}
	if apps := psApp.FindAllStringSubmatch(s, -1); len(apps) > 0 {
		var lines []string
		for _, a := range apps {
			body := a[2]
			icon := partOf(body, "Icon")
			icon = strings.TrimPrefix(strings.ToLower(icon), "0x")
			run, hint := partOf(body, "Run"), partOf(body, "Hint")
			if r, ok := runMap[strings.TrimSpace(run)]; ok {
				run = r
			}
			if h, ok := hintMap[strings.TrimSpace(hint)]; ok { // the old app names
				hint = h
			}
			lines = append(lines, fmt.Sprintf("app = %s | %s | %s | %s | %s", a[1], hint, partOf(body, "Key"), icon, run))
		}
		out = regexp.MustCompile(`(?m)^app = .*\n`).ReplaceAllString(out, "")
		out = strings.Replace(out, "#            (empty)                       back to the prompt\n",
			"#            (empty)                       back to the prompt\n"+strings.Join(lines, "\n")+"\n", 1)
	}
	if r := list("Roots"); r != nil {
		set("project_roots", strings.Join(r, " ; "))
	}
	if r := list("OpenWith"); r != nil {
		set("project_open_with", strings.Join(r, ", "))
	}
	if v := get("Search"); v != "" {
		set("project_search", v)
	}
	if r := list("Skip"); r != nil {
		set("project_skip", strings.Join(r, ", "))
	}
	for _, k := range []string{"GradFrom", "GradTo", "Accent", "Text", "Muted", "SelBg", "SelHint", "Hot"} {
		if v := get(k); v != "" {
			key := map[string]string{"GradFrom": "color_grad_from", "GradTo": "color_grad_to", "Accent": "color_accent",
				"Text": "color_text", "Muted": "color_muted", "SelBg": "color_sel_bg", "SelHint": "color_sel_hint", "Hot": "color_hot"}[k]
			set(key, v)
		}
	}
	// the logo and this month's prayer times come along
	if v := get("Logo"); v != "" && v != "logo.txt" {
		if lb, err := os.ReadFile(filepath.Join(dir, v)); err == nil {
			os.MkdirAll(app.Dir(), 0o755)
			os.WriteFile(app.File("home-logo.txt"), lb, 0o644)
		}
	}
	if pb, err := os.ReadFile(filepath.Join(dir, "prayer.json")); err == nil {
		os.WriteFile(app.File("home-prayer.json"), pb, 0o644)
	}
	os.MkdirAll(app.Dir(), 0o755)
	os.WriteFile(configFile(), []byte(out), 0o644)
}

// partOf: Key = 'text' or Key = 0x1234 / 12 inside a PowerShell @{ } body
func partOf(body, key string) string {
	if m := psPart(key).FindStringSubmatch(body); m != nil {
		if m[1] != "" {
			return m[1]
		}
		return m[2]
	}
	return ""
}
