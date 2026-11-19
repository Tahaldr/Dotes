package git

// The dotes g command: what it can open, login, doctor, config, help.

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/cli/go-gh/v2/pkg/auth"
)

const Usage = `  dotes g                           Git (on the start tab from git.txt)
  dotes g TAB                       open on a tab: home, repos, issues, prs, actions, inbox, explore
  dotes g .                         the GitHub repo of this folder (its Changes if it has some)
  dotes g OWNER/REPO                a repo, e.g. dotes g charmbracelet/bubbletea
  dotes g FILE                      read a file full screen (markdown or code); reloads when it changes
  dotes g search QUERY              search GitHub repos
  dotes g user LOGIN                someone's profile
  dotes g login / logout            log in to GitHub (through the GitHub CLI) with what Git needs
  dotes g doctor                    check that everything Git uses is set up
  dotes g config [path]             open (or print where) git.txt is`

// scopes Git uses: repo (code, issues, PRs), read:org (org repos), workflow (CI, pushing
// workflow files), delete_repo (D on a repo), user:follow (F on a profile), notifications
var loginScopes = "repo,read:org,workflow,delete_repo,user:follow,notifications,gist"

var nwoRe = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)

// Launch is what dotes g ... asked to open.
type Launch struct {
	first func(m *model) tea.Cmd // runs once the window size is known
	solo  *model                 // a file read full screen
}

// Command handles dotes g ARGS: commands that don't open the app (login, doctor, ...)
// run here and return nil; the rest return what to open.
func Command(args []string) *Launch {
	if len(args) == 0 {
		return start(nil)
	}
	a := strings.ToLower(args[0])
	switch a {
	case "-h", "--help", "help":
		fmt.Println(Usage)
	case "login":
		login()
	case "logout":
		run("gh", "auth", "logout", "-h", "github.com")
	case "doctor":
		doctor()
	case "config", "--config":
		if len(args) > 1 && args[1] == "path" || a == "--config" {
			fmt.Println(configPath())
			return nil
		}
		if _, err := os.Stat(configPath()); err != nil {
			writeDefaultConfig(configPath(), cfg)
		}
		c := editorCmd(configPath())
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			fail(err)
		}
	case "home", "repos", "issues", "prs", "actions", "inbox", "explore":
		cfg.startTab = a
		return start(nil)
	case "search":
		q := strings.Join(args[1:], " ")
		return start(func(m *model) tea.Cmd {
			m.setTab(tabExplore)
			m.exp.mode, m.exp.query[1] = 1, q
			return m.expSelected()
		})
	case "user":
		if len(args) < 2 {
			fail(fmt.Errorf("which user? dotes g user LOGIN"))
		}
		login := args[1]
		return start(func(m *model) tea.Cmd { return m.openProfile(login) })
	case ".":
		dir, _ := os.Getwd()
		nwo := remoteOf(dir)
		if nwo == "" {
			fail(fmt.Errorf("this folder isn't a clone of a GitHub repo (no github.com remote)"))
		}
		return start(func(m *model) tea.Cmd { return m.openDir(dir) })
	default:
		if nwoRe.MatchString(args[0]) {
			if _, err := os.Stat(args[0]); err != nil { // not a file: a repo
				nwo := args[0]
				return start(func(m *model) tea.Cmd { return m.gotoItem(nwo, secCode, "", func(exItem) bool { return true }, nil) })
			}
		}
		if strings.HasPrefix(args[0], "-") {
			fail(fmt.Errorf("unknown option %s\n\n%s", args[0], Usage))
		}
		rd, err := loadFileReader(args[0])
		if err != nil {
			fail(err)
		}
		return &Launch{solo: newSoloModel(rd)}
	}
	return nil
}

// openDir opens the repo a folder on this PC is a clone of (dotes g . , Files' gi):
// Changes when something isn't committed, else Code.
func (m *model) openDir(dir string) tea.Cmd {
	nwo := remoteOf(dir)
	if nwo == "" {
		return m.setStatus(tildePath(dir)+" isn't a clone of a GitHub repo (no github.com remote)", true)
	}
	m.local[strings.ToLower(nwo)] = dir
	sec := secCode
	if out, _ := gitRun(dir, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		sec = secChanges
	}
	return tea.Batch(m.scanLocalInfo(), m.gotoItem(nwo, sec, "", func(exItem) bool { return true }, nil))
}

// start checks the login, then says what to open; first (if any) runs once the window size is known
func start(first func(m *model) tea.Cmd) *Launch {
	if !ready() {
		os.Exit(1)
	}
	return &Launch{first: first}
}

// New makes the Git part. l = nil opens it normally (alt+N in Dotes).
func New(l *Launch, dump bool) tea.Model {
	if l != nil && l.solo != nil {
		l.solo.dump = dump
		return l.solo
	}
	if !dump && !loggedIn() {
		return notReady{}
	}
	m := newModel()
	m.applyConfig()
	if l != nil {
		m.first = l.first
	}
	m.dump = dump
	return m
}

// LoadConfig reads git.txt (made on first use).
func LoadConfig() {
	cfg = loadConfig()
	cloneRoot, localRoots = cfg.cloneInto, cfg.lookIn
}

// FileLaunch opens a file in the reader (dotes --dump git file:PATH).
func FileLaunch(path string) (*Launch, error) {
	rd, err := loadFileReader(path)
	if err != nil {
		return nil, err
	}
	return &Launch{solo: newSoloModel(rd)}, nil
}

func remoteOf(dir string) string {
	out, err := gitRun(dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	if m := remoteRe.FindStringSubmatch("url = " + strings.TrimSpace(out)); m != nil {
		return m[1] + "/" + m[2]
	}
	return ""
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "dotes:", err)
	os.Exit(1)
}

func run(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func ask(q string) bool {
	fmt.Print(q + " [Y/n] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "" || line == "y" || line == "yes"
}

func haveGh() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func loggedIn() bool {
	if app.Demo {
		return true
	}
	tok, _ := auth.TokenForHost("github.com")
	return tok != ""
}

const installGh = `Dotes logs in through the GitHub CLI ("gh"), which isn't installed yet.
Install it, then run  dotes g login :

  Windows   winget install --id GitHub.cli
  macOS     brew install gh
  Linux     see https://github.com/cli/cli#installation
`

// ready checks the login before opening Git, and offers to log in
func ready() bool {
	if loggedIn() {
		return true
	}
	if !haveGh() {
		fmt.Print(installGh)
		os.Exit(1)
	}
	fmt.Println("Git needs you to log in to GitHub first (it uses the GitHub CLI; Dotes never stores a token).")
	if !ask("Log in now?") {
		os.Exit(1)
	}
	login()
	return loggedIn()
}

func login() {
	if !haveGh() {
		fmt.Print(installGh)
		os.Exit(1)
	}
	if loggedIn() {
		fmt.Println("Already logged in. Adding the permissions Git uses (a browser page may open)…")
		if err := run("gh", "auth", "refresh", "-h", "github.com", "-s", loginScopes); err != nil {
			fail(err)
		}
		return
	}
	if err := run("gh", "auth", "login", "-h", "github.com", "-p", "https", "-w", "-s", loginScopes); err != nil {
		fail(err)
	}
	run("gh", "auth", "setup-git") // so git pull / push use the same login
	fmt.Println("Logged in. Run  dotes g  to start.")
}

func doctor() {
	ok := func(good bool, what, fix string) {
		if good {
			fmt.Println("  ✓ " + what)
		} else {
			fmt.Println("  ✗ " + what + "\n      " + fix)
		}
	}
	fmt.Printf("Dotes %s · Git\n\n", version)
	ok(haveGh(), "GitHub CLI (gh) installed", "install it: winget install --id GitHub.cli · brew install gh · https://github.com/cli/cli#installation")
	ok(loggedIn(), "logged in to GitHub", "run  dotes g login")
	_, err := exec.LookPath("git")
	ok(err == nil, "git installed (for repos on this PC)", "install it: https://git-scm.com/downloads")
	if haveGh() && loggedIn() {
		out, _ := exec.Command("gh", "auth", "status", "-h", "github.com").CombinedOutput()
		s := string(out)
		for _, sc := range []string{"repo", "read:org", "workflow", "delete_repo", "user:follow"} {
			ok(strings.Contains(s, "'"+sc+"'"), "permission "+sc, "run  dotes g login  to add it")
		}
	}
	ed := strings.Fields(cfg.editor + " nvim")[0]
	_, err = exec.LookPath(ed)
	ok(err == nil, "editor ("+ed+") found", "install it, or set  editor =  in the settings (dotes g config)")
	gt := strings.Fields(cfg.gitTool + " lazygit")[0]
	_, err = exec.LookPath(gt)
	ok(err == nil, "git tool ("+gt+", the L key) found", "optional: install it, or set  git_tool =  in the settings")
	if nv := app.NewerSaved(); nv != "" {
		ok(false, "newest version", "Dotes "+strings.TrimPrefix(nv, "v")+" is out: run  dotes update")
	}
	fmt.Println("\n  settings: " + configPath())
	fmt.Println("  cache:    " + cacheDir())
	fmt.Println("  clones:   " + cfg.cloneInto)
	fmt.Println("\nIcons need a Nerd Font in your terminal: https://www.nerdfonts.com")
}
