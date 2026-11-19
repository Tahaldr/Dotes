package main

// dotes shell pwsh|bash|zsh [--start]: the shell function the installer puts in your profile.
// It lets Dotes move your shell to the folder you quit Files in (q), close the tab (x on
// Home), and with --start open Home in every new terminal tab.
//
//	PowerShell ($PROFILE):  dotes.exe shell pwsh --start | Out-String | Invoke-Expression
//	bash / zsh (~/.bashrc): eval "$(command dotes shell bash --start)"

import (
	"fmt"
	"os"
)

const pwshInit = `# Dotes: q leaves you in the folder Files (or a picked project) was in; x on Home closes the tab
function dotes {
    $tmp = [IO.Path]::GetTempFileName()
    dotes.exe --cwd-file $tmp @args
    $code = $LASTEXITCODE
    $dir = Get-Content -LiteralPath $tmp -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $tmp -ErrorAction SilentlyContinue
    if ($dir -and $dir -ne $PWD.Path -and (Test-Path -LiteralPath $dir)) { Set-Location -LiteralPath $dir }
    if ($code -eq 3) { [Environment]::Exit(0) }   # x on Home: close this terminal tab
}
`

const pwshStart = `# Home in every new tab (not inside VS Code, Nvim's terminal or Herdr panes)
if (-not $env:DOTES_STARTED -and $env:TERM_PROGRAM -ne 'vscode' -and -not $env:NVIM -and -not $env:HERDR_ENV -and -not $env:HERDR_SOCKET_PATH) {
    $env:DOTES_STARTED = 1; dotes
}
`

const shInit = `# Dotes: q leaves you in the folder Files (or a picked project) was in; x on Home closes the tab
dotes() {
  local tmp code dir
  tmp="$(mktemp)"
  command dotes --cwd-file "$tmp" "$@"; code=$?
  dir="$(cat "$tmp" 2>/dev/null)"; rm -f "$tmp"
  if [ -n "$dir" ] && [ "$dir" != "$PWD" ] && [ -d "$dir" ]; then cd "$dir" || true; fi
  if [ "$code" -eq 3 ]; then exit 0; fi
  return $code
}
`

const shStart = `# Home in every new tab (not inside VS Code or Nvim's terminal)
if [ -z "$DOTES_STARTED" ] && [ "$TERM_PROGRAM" != vscode ] && [ -z "$NVIM" ] && [ -t 1 ]; then
  export DOTES_STARTED=1; dotes
fi
`

func shellInit(args []string) {
	start := false
	sh := ""
	for _, a := range args {
		if a == "--start" {
			start = true
		} else {
			sh = a
		}
	}
	switch sh {
	case "pwsh", "powershell":
		fmt.Print(pwshInit)
		if start {
			fmt.Print(pwshStart)
		}
	case "bash", "zsh", "sh":
		fmt.Print(shInit)
		if start {
			fmt.Print(shStart)
		}
	default:
		fmt.Fprintln(os.Stderr, `dotes shell pwsh|bash|zsh [--start]  prints the shell function for your profile:
  PowerShell ($PROFILE):    dotes.exe shell pwsh --start | Out-String | Invoke-Expression
  bash / zsh (~/.bashrc):   eval "$(command dotes shell bash --start)"
--start also opens Home in every new terminal tab.`)
		os.Exit(1)
	}
}
