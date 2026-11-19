# Dotes

One terminal app with the parts you pick. Every part has the same look, the same keys and
one settings folder, and you switch between them in the same window.

| Part | What it is | Status |
|---|---|---|
| **Home** | dashboard, app launcher, projects, prayer times | ready |
| **Files** | a yazi-style file manager | ready |
| **Notes** | todo lists and notes | ready |
| **Git** | GitHub and Git: repos, changes, PRs, issues, CI | ready |

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
```

Inside: `alt+1`–`alt+9` (or a click on the top bar) switches parts, `~` shows every key of
the part you're in.

## Settings

`%APPDATA%\dotes\` on Windows, `~/.config/dotes/` elsewhere:

- `dotes.txt`: shared settings (your parts, key hints, the top bar)
- `files.txt`: Files settings (and `files-bookmarks.txt`)
- `notes.txt`: Notes settings (your notebook folder, editor, inbox list)
- `git.txt`: Git settings (start tab, where to clone, editor, git tool)
- `home.txt`: Home settings (apps, layout, logo, prayer times, project folders)

Coming from TermNook, TermFiles, TermNotes or TermGit? Their settings and bookmarks are copied over the first time.

Every option is explained in the file itself. Updates never touch these files.

## Go to the folder you left (PowerShell)

Quitting Files with `q` (or opening a project) can leave your shell in that folder, and `x`
on Home closes the terminal tab. Add this to your `$PROFILE`:

```powershell
function dotes {
    $tmp = [IO.Path]::GetTempFileName()
    dotes.exe --cwd-file $tmp @args
    $code = $LASTEXITCODE
    $dir = Get-Content -LiteralPath $tmp -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $tmp -ErrorAction SilentlyContinue
    if ($dir -and $dir -ne $PWD.Path -and (Test-Path -LiteralPath $dir)) { Set-Location -LiteralPath $dir }
    if ($code -eq 3) { [Environment]::Exit(0) }   # x on Home: close this tab
}
```

## Build

```
go test ./...
go build -o dotes.exe .
```

MIT license.
