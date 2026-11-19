# Dotes installer for Windows.
#   irm https://raw.githubusercontent.com/Tahaldr/Dotes/master/install.ps1 | iex
# Puts dotes.exe in ~\.local\bin (added to your PATH), asks which parts you want, and offers
# to add the shell function to your PowerShell profile.

$ErrorActionPreference = 'Stop'
$repo = 'Tahaldr/Dotes'
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$asset = "dotes_windows_$arch.exe"
$dir = Join-Path $HOME '.local\bin'
$exe = Join-Path $dir 'dotes.exe'

Write-Host ''
Write-Host '  Dotes' -ForegroundColor Cyan -NoNewline
Write-Host '  one terminal app with the parts you pick: Home, Files, Notes, Git'
Write-Host ''

# 1. the program
New-Item -ItemType Directory -Force $dir | Out-Null
$rel = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest" -Headers @{ 'User-Agent' = 'dotes-installer' }
$url = ($rel.assets | Where-Object name -eq $asset).browser_download_url
if (-not $url) { throw "No $asset in Dotes $($rel.tag_name)" }
Write-Host "  Downloading Dotes $($rel.tag_name)..."
$tmp = "$exe.download"
Invoke-WebRequest $url -OutFile $tmp -UseBasicParsing
if (Test-Path $exe) {   # a running dotes.exe can't be overwritten, but it can be renamed
    Remove-Item "$exe.old" -Force -ErrorAction SilentlyContinue
    Rename-Item $exe "dotes.exe.old"
}
Rename-Item $tmp 'dotes.exe'
Write-Host "  Installed $exe" -ForegroundColor Green

# 2. PATH
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($userPath -split ';') -contains $dir)) {
    [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ';' + $dir).TrimStart(';'), 'User')
    Write-Host "  Added $dir to your PATH"
}
if (-not (($env:Path -split ';') -contains $dir)) { $env:Path += ";$dir" }

# 3. the parts
Write-Host ''
Write-Host '  Pick the parts you want (space ticks, enter saves). Change it later with: dotes setup'
& $exe setup

# 4. the shell function
Write-Host ''
$line = 'dotes.exe shell pwsh | Out-String | Invoke-Expression'
$startLine = 'dotes.exe shell pwsh --start | Out-String | Invoke-Expression'
$has = (Test-Path $PROFILE) -and (Select-String -LiteralPath $PROFILE -SimpleMatch 'dotes.exe shell pwsh' -Quiet)
if ($has) {
    Write-Host '  Your profile already loads Dotes.'
} else {
    Write-Host '  Add Dotes to your PowerShell profile? It lets q in Files leave you in that folder,'
    Write-Host '  and x on Home close the tab.'
    $a = Read-Host '  Add it? [Y/n]'
    if ($a -eq '' -or $a -match '^[yY]') {
        $b = Read-Host '  Also open Dotes Home in every new terminal tab? [y/N]'
        $use = if ($b -match '^[yY]') { $startLine } else { $line }
        if (-not (Test-Path $PROFILE)) { New-Item -ItemType File -Force $PROFILE | Out-Null }
        Add-Content -LiteralPath $PROFILE -Value "`n# Dotes (dotes shell pwsh prints what this loads)`n$use"
        Write-Host "  Added to $PROFILE" -ForegroundColor Green
    }
}

Write-Host ''
Write-Host '  Done. Open a new terminal and type  dotes  (dotes --help lists everything).' -ForegroundColor Cyan
Write-Host ''
