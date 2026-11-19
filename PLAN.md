# Dotes — plan

One terminal app that holds the Term apps as **parts**. The user picks which parts
they want; the rest stay hidden. It must feel like ONE program: one command, one
config folder, one look, instant switching between parts.

## Decisions (2026-11-19)

| # | Topic | Decision |
|---|---|---|
| 1 | Parts | **Home** (TermNook, rewritten in Go), **Files** (TermFiles), **Notes** (TermNotes), **Git** (TermGit) |
| 2 | Packaging | One program `dotes.exe` (`dotes` on Mac/Linux) with every part built in; parts not picked are hidden (~30 MB vs 49 MB for the 3 apps today) |
| 3 | Change picks later | `dotes setup` re-opens the part picker |
| 4 | Commands | `dotes` (Home, or the first picked part) · `dotes f` / `files` · `dotes n` / `notes` · `dotes g` / `git` · `dotes setup` · `dotes update` · `dotes config` |
| 5 | Config | `%APPDATA%\dotes\` (`~/.config/dotes/` elsewhere): `dotes.txt` (shared: theme, hints, picked parts) + `home.txt`, `files.txt`, `notes.txt`, `git.txt` |
| 6 | Switching | Top tab bar `Home │ Files │ Notes │ Git` (picked parts only), one key + click to switch. Removable if the user dislikes it |
| 7 | Links between parts | Yes, after the merge works (batch 5), plus Notes **path mentions with suggestions** |
| 8 | Old apps | No migration needed (only the user's 2 PCs). Remove old apps, install Dotes |
| 9 | Old repos | Make all 4 old Term repos private once Dotes ships (batch 7) |
| 10 | Install | `install.ps1` (and `install.sh`) that asks which parts you want |
| 11 | Platforms | Windows, Mac, Linux (6 builds, like TermGit) |
| 12 | Repo | New public repo `Tahaldr/Dotes` (name is free on the user's account) |

## Code layout

All 3 apps already use the same libraries (Bubble Tea v2.0.9, Lipgloss v2.0.6), so
they move in as Go packages with little rewriting.

```
Dotes/
  main.go              ← command parsing (dotes f, dotes setup, ...)
  shell/               ← the outer app: tab bar, part switching, first-run picker
  internal/ui/         ← shared: theme, overlay popups, which-key, hint bar, ~ help, mdrender, rtl
  internal/config/     ← dotes.txt + per-part files
  internal/update/     ← one `dotes update` + "⬆ version" chip
  parts/home/          ← TermNook in Go
  parts/files/         ← TermFiles
  parts/notes/         ← TermNotes
  parts/git/           ← TermGit
```

Each part exposes the same small interface (Init / Update / View / Keys / Title), so
the shell can host, switch and hide them. Duplicated files today (theme.go,
overlay.go, mdrender.go, update.go, config.go in every app) become one shared copy.

## Batches

One batch at a time; next only when the user says.

1. ✅ **Shell + Files** (v0.1.0). New repo, module `github.com/Tahaldr/dotes`. Shell with tab bar,
   shared `internal/ui`, config folder + `dotes.txt`, first-run part picker / `dotes setup`.
   TermFiles moved in as the Files part (`dotes f`, `--pick`, `--cwd-file`, cd-on-quit
   shell function). Tests: first frame at every size for shell + Files.
2. ✅ **Notes part** (v0.2.0). TermNotes moved in (`dotes n`, `dotes n add/today/lists`), shared
   overlay/which-key/hints with Files. Same Notebook files as the nvim popup.
3. ✅ **Git part** (v0.3.0; recent repos on Home moved from alt+1-5 to shift+1-5, alt+digits switch parts). TermGit moved in (`dotes g`, all screens, `dotes g file.md` reader).
   Biggest batch (~17k lines). Keep TestFirstFrameEveryTab.
4. ✅ **Home part** (v0.4.0; settings moved from TermNook automatically, x closes the tab through the shell function). TermNook rewritten in Go: dashboard layouts (cards/info/palette/list),
   apps launcher, quick picker, project picker (uses the Files part), prayer times,
   logo, recent. A small profile function so `dotes` can cd the shell and launch apps.
5. ✅ **Links between parts + Notes path mentions** (v0.5.0: Files gi → Git, gn → project note; Git ctrl+f → Files; Notes gf → Files, path suggestions while typing, paths shown as links).
   - Files → "open in Git" on a repo folder; Files → project notes.
   - Git → open the repo folder in Files.
   - Notes: type a file path in a todo/note, get suggestions (popup like the Nvim
     cmdline), the path shows as a link; enter opens it in Files (or nvim).
6. **Live updates everywhere.** Every part shows changes the moment they happen, without
   pressing refresh: Git reacts to every change live (files edited, staged, committed, pulled,
   branches, also from another program or terminal); Files sees new / changed / deleted files;
   Notes reloads a list or note changed in Nvim or another window; Home refreshes projects,
   recent items and prayer times. Use file watching (fsnotify-style) where possible,
   light polling where not, and keep CPU low.
7. **Ship.** `install.ps1` / `install.sh` with the part picker, release workflow
   (6 builds on a `v*` tag pushed alone), `dotes update`, README. Move the user's own
   settings (TermGit config.txt, TermFiles/TermNotes settings, TermNook config.psd1,
   logo, prayer.json) into `%APPDATA%\dotes\`. Swap TermNook in the PowerShell profile
   for Dotes, remove the old apps (after the user approves), make the old Term repos private.
8. **Design pass (one big batch at the end).** With the user: every part uses the same
   approach and layout for every element. The user doesn't like how the Dotes top bar looks
   and feels stacked above each part's own tabs (Files tabs, Notes Todo lists/Notes/Today,
   Git tabs): redo the top bar and all in-part tabs as ONE tab system, then headers, panels,
   lists, popups, status lines and the hint bar. Muted onedark stays.
   F1 opens the keys / shortcuts page in every part (next to the ~ and ? keys), same page layout everywhere.

**After Dotes:** the private PC-config backup project (was "TermDots"), built for
Dotes' final config layout.

## Rules carried over

- Muted onedark; every action = key + clickable button; hint bar lists every action
  (hideable), `~` help page, which-key popup after the first key of a combo.
- `go test ./...` before every install. Install while running: rename the old exe, then copy.
- No personal data in tracked files (user's paths live in their config).
