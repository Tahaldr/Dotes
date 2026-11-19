# Dotes

One terminal app with the parts you pick. Every part has the same look, the same keys and
one settings folder, and you switch between them in the same window.

| Part | What it is | Status |
|---|---|---|
| **Home** | your apps as cards, and a glance at the other parts: what's due, repos to commit or push, failing CI, recent projects, prayer times | ready |
| **Files** | a yazi-style file manager | ready |
| **Notes** | todo lists and notes | ready |
| **Git** | GitHub and Git: repos, changes, PRs, issues, CI | ready |

## Install

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/Tahaldr/Dotes/master/install.ps1 | iex
```

macOS / Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/Tahaldr/Dotes/master/install.sh | sh
```

The installer asks which parts you want and offers to add the shell function to your profile.
`dotes update` gets newer versions later. The Git part logs in through the
[GitHub CLI](https://cli.github.com) (`dotes g login`).

The first time you run `dotes` it asks which parts you want. The others stay hidden; change
your pick any time with `dotes setup`.

## Use

```
dotes                 open your first part (Home)
dotes h quick         one-line app picker under your prompt
dotes h projects      pick a project and open it
dotes h salat         today's prayer times
dotes f [folder]      Files
dotes n               Notes  (dotes n add "buy milk" -d fri, dotes n today, dotes n lists)
dotes g [tab|repo|.]  Git    (dotes g login, dotes g doctor, dotes g FILE reads a file)
dotes setup           pick your parts
dotes config          open the settings (dotes.txt)
dotes update          get the newest version
dotes shell pwsh      the shell function for your profile (also bash, zsh)
```

Inside, every part has the same first line: its name and tabs on the left, and
`Home · Files · Notes · Git` on the right (click one, or `alt+1`–`alt+9`). `q` goes back to
Home (a part you opened straight from the prompt, like `dotes f`, quits instead). `F1` shows
every key of the part you're in.

Everything is live: a file saved in Nvim, a commit or pull from another terminal, a list
changed in another window shows at once, without refreshing.

The parts link to each other:

- Files: `gi` opens the repo you're in, in Git; `gn` opens the project's note in Notes
- Git: `ctrl+f` shows the repo folder (or the highlighted file) in Files
- Notes: type `~/` or `C:\` in a todo and the folder's files pop up (`tab` puts one in);
  paths on your PC show as links, and a click or `gf` opens them in Files

## Settings

`%APPDATA%\dotes\` on Windows, `~/.config/dotes/` elsewhere:

- `dotes.txt`: shared settings (your parts, key hints, the part names on the first line)
- `files.txt`: Files settings (and `files-bookmarks.txt`)
- `notes.txt`: Notes settings (your notebook folder, editor, inbox list)
- `git.txt`: Git settings (start tab, where to clone, editor, git tool)
- `home.txt`: Home settings (apps, logo, prayer times, project folders), e.g.
  `prayer = Casablanca | Morocco | 21` for prayer times with a countdown

Coming from TermNook, TermFiles, TermNotes or TermGit? Their settings and bookmarks are copied over the first time.

Every option is explained in the file itself. Updates never touch these files.

## The shell function

Quitting Files opened with `dotes f` (or opening a project) can leave your shell in that folder, and `x`
on Home closes the terminal tab. That needs a small function in your shell; the installer
adds it, or add the line yourself:

```powershell
# $PROFILE  (--start also opens Home in every new tab)
dotes.exe shell pwsh --start | Out-String | Invoke-Expression
```

```sh
# ~/.bashrc or ~/.zshrc
eval "$(command dotes shell bash --start)"
```

`dotes shell pwsh` prints the function, if you'd rather paste it.

## Build

```
go test ./...
go build -o dotes.exe .
```

MIT license.
