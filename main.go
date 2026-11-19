// Dotes: one terminal app with the parts you pick — Home, Files, Notes, Git.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/parts/files"
	"github.com/Tahaldr/dotes/shell"
	"github.com/charmbracelet/x/ansi"
)

var usage = `Dotes ` + app.Version + ` - one terminal app with the parts you pick

  dotes                             open your first part (alt+1-9 or a click on the top bar switches)
` + files.Usage + `
  dotes setup                       pick the parts you want (the others stay hidden)
  dotes config                      open the settings folder's dotes.txt in your editor
  dotes update [--check]            get the newest version
  dotes --version

Every part: ~ shows its keys. Settings: ` + app.Dir()

func main() {
	args := os.Args[1:]
	o := shell.Options{FilesPath: "."}
	// --cwd-file works with every command (the shell function passes it)
	for i := 0; i < len(args); i++ {
		if args[i] == "--cwd-file" && i+1 < len(args) {
			o.Files.CwdFile = args[i+1]
			args = append(args[:i:i], args[i+2:]...)
			i--
		} else if strings.HasPrefix(args[i], "--cwd-file=") {
			o.Files.CwdFile = strings.TrimPrefix(args[i], "--cwd-file=")
			args = append(args[:i:i], args[i+1:]...)
			i--
		}
	}
	cmd := ""
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "":
	case "--version", "-v", "version":
		fmt.Println("dotes " + app.Version)
		return
	case "--help", "-h", "help":
		fmt.Println(usage)
		return
	case "update":
		app.SelfUpdate(len(args) > 0 && args[0] == "--check")
		return
	case "config":
		app.LoadShared() // makes dotes.txt if it's missing
		openEditor(app.File("dotes.txt"))
		return
	case "setup":
		o.Setup = true
	case "--dump":
		dump(args)
		return
	case "f", "files":
		o.Start = "files"
		for i := 0; i < len(args); i++ {
			a := args[i]
			switch {
			case a == "--sixel-test" && i+1 < len(args):
				files.SixelTest(args[i+1])
				return
			case a == "--pick" && i+1 < len(args):
				o.Files.Pick, o.NoBar = args[i+1], true
				i++
			case a == "--projects" && i+1 < len(args):
				o.Files.Projects = args[i+1]
				i++
			case a == "--openers" && i+1 < len(args):
				o.Files.Openers = args[i+1]
				i++
			case strings.HasPrefix(a, "-"):
				fail("unknown option " + a)
			default:
				o.FilesPath = a
			}
		}
	default:
		for _, p := range app.Parts {
			if cmd == p.ID || cmd == p.Short {
				if !p.Ready {
					fmt.Println(p.Name + " comes in a later Dotes version. Files is ready now:  dotes f")
					return
				}
				o.Start = p.ID
			}
		}
		if o.Start == "" {
			fail("unknown command " + cmd)
		}
	}
	if _, err := tea.NewProgram(shell.New(o)).Run(); err != nil {
		crashed(err)
	}
	if o.Setup {
		fmt.Println("Saved. Your parts: " + strings.Join(app.LoadShared().Parts, ", ") + ". Run  dotes  to open them.")
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "dotes: "+msg+"\n\n"+usage)
	os.Exit(2)
}

// openEditor opens a file in $EDITOR, else nvim, else Notepad / vi
func openEditor(path string) {
	ed := os.Getenv("EDITOR")
	if ed == "" {
		ed = "nvim"
		if _, err := exec.LookPath(ed); err != nil {
			ed = "notepad"
			if _, err := exec.LookPath(ed); err != nil {
				ed = "vi"
			}
		}
	}
	c := exec.Command(ed, path)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		fmt.Println("Couldn't open an editor. The file is " + path)
	}
}

// crashed keeps the error on screen (and in a file) instead of the window just closing
func crashed(err error) {
	dir, _ := os.UserCacheDir()
	log := filepath.Join(dir, "dotes-crash.log")
	os.WriteFile(log, []byte(time.Now().Format(time.RFC3339)+"\n"+err.Error()+"\n"), 0o644)
	fmt.Fprintln(os.Stderr, "\ndotes stopped because of an error:\n  "+err.Error())
	fmt.Fprintln(os.Stderr, "\nIt's saved in "+log+". Press enter to close.")
	bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(1)
}

// dump: dotes --dump WxH [files|setup] [dir:PATH] keys...  prints one screen without a terminal
func dump(args []string) {
	w, h := 120, 30
	if len(args) > 0 {
		if p := strings.SplitN(args[0], "x", 2); len(p) == 2 {
			if a, err := strconv.Atoi(p[0]); err == nil {
				w = a
				h, _ = strconv.Atoi(p[1])
				args = args[1:]
			}
		}
	}
	o := shell.Options{FilesPath: ".", Dump: true}
	if len(args) > 0 && (args[0] == "files" || args[0] == "setup") {
		if args[0] == "setup" {
			o.Setup = true
		} else {
			o.Start = args[0]
		}
		args = args[1:]
	}
	if len(args) > 0 && strings.HasPrefix(args[0], "dir:") {
		o.FilesPath = args[0][4:]
		args = args[1:]
	}
	m := shell.New(o)
	_, first := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	files.Drain(m, tea.Batch(first, m.Init()))
	for _, a := range args {
		m.View() // lay out so clicks land
		files.Drain(m, func() tea.Msg { return files.ParseInput(a) })
	}
	out := m.View().Content
	if os.Getenv("DOTES_RAW") == "" {
		out = ansi.Strip(out)
	}
	fmt.Println(out)
}
