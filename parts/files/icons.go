package files

import (
	"image/color"
	"path/filepath"
	"strings"
)

// Nerd Font icons (same folder / repo / file icons as TermNook's picker)
const (
	icFolder   = ""
	icRepo     = ""
	icFile     = ""
	icLink     = ""
	icDrive    = ""
	icHome     = ""
	icBranch   = ""
	icImage    = ""
	icVideo    = ""
	icAudio    = ""
	icArchive  = ""
	icText     = ""
	icConfig   = ""
	icLock     = ""
	icGit      = ""
	icEdit     = ""
	icInfo     = ""
	icWarn     = ""
	icTasks    = ""
	icBookmark = ""
	icSearch   = ""
	icFilter   = ""
	icSort     = ""
	icOpen     = ""
	icPC       = ""
)

type icon struct {
	glyph string
	col   color.Color
}

var extIcons = map[string]icon{
	".go": {"", cCyan}, ".mod": {"", cCyan}, ".sum": {"", cCyan},
	".md": {"", cTxt}, ".markdown": {"", cTxt}, ".mdx": {"", cTxt},
	".js": {"", cYellow}, ".mjs": {"", cYellow}, ".cjs": {"", cYellow},
	".jsx": {"", cCyan}, ".tsx": {"", cAcc}, ".ts": {"", cAcc},
	".json": {"", cYellow}, ".jsonc": {"", cYellow},
	".lua": {"", cAcc}, ".vim": {"", cGreen},
	".py": {"", cYellow}, ".rs": {"", cOrange}, ".rb": {"", cRed},
	".html": {"", cOrange}, ".htm": {"", cOrange}, ".css": {"", cAcc}, ".scss": {"", cPurple},
	".php": {"", cPurple}, ".blade.php": {"", cRed},
	".ps1": {"", cAcc}, ".psm1": {"", cAcc}, ".psd1": {"", cAcc},
	".sh": {"", cGreen}, ".bash": {"", cGreen}, ".zsh": {"", cGreen}, ".bat": {"", cGreen}, ".cmd": {"", cGreen},
	".c": {"", cAcc}, ".h": {"", cPurple}, ".cpp": {"", cAcc}, ".hpp": {"", cPurple},
	".java": {"", cRed}, ".kt": {"", cPurple}, ".swift": {"", cOrange}, ".dart": {"", cCyan},
	".sql": {"", cTxt}, ".db": {"", cTxt}, ".sqlite": {"", cTxt},
	".toml": {icConfig, cHint}, ".yaml": {icConfig, cHint}, ".yml": {icConfig, cHint}, ".ini": {icConfig, cHint},
	".conf": {icConfig, cHint}, ".cfg": {icConfig, cHint}, ".env": {icConfig, cYellow}, ".xml": {"\U000F05C0", cOrange},
	".lock": {icLock, cMut},
	".txt":  {icText, cTxt}, ".log": {icText, cMut},
	".exe": {"", cAcc}, ".msi": {"", cAcc}, ".dll": {"", cMut},
	".zip": {icArchive, cYellow}, ".7z": {icArchive, cYellow}, ".rar": {icArchive, cYellow}, ".tar": {icArchive, cYellow},
	".gz": {icArchive, cYellow}, ".xz": {icArchive, cYellow}, ".bz2": {icArchive, cYellow}, ".zst": {icArchive, cYellow},
	".png": {icImage, cPurple}, ".jpg": {icImage, cPurple}, ".jpeg": {icImage, cPurple}, ".gif": {icImage, cPurple},
	".webp": {icImage, cPurple}, ".bmp": {icImage, cPurple}, ".ico": {icImage, cPurple}, ".svg": {icImage, cOrange},
	".mp4": {icVideo, cRed}, ".mkv": {icVideo, cRed}, ".avi": {icVideo, cRed}, ".mov": {icVideo, cRed}, ".webm": {icVideo, cRed},
	".mp3": {icAudio, cCyan}, ".flac": {icAudio, cCyan}, ".wav": {icAudio, cCyan}, ".ogg": {icAudio, cCyan}, ".m4a": {icAudio, cCyan}, ".opus": {icAudio, cCyan},
	".pdf": {"", cRed}, ".doc": {"", cAcc}, ".docx": {"", cAcc},
	".xls": {"", cGreen}, ".xlsx": {"", cGreen}, ".csv": {"", cGreen},
	".ppt": {"", cOrange}, ".pptx": {"", cOrange},
	".ttf": {"", cTxt}, ".otf": {"", cTxt}, ".woff": {"", cTxt}, ".woff2": {"", cTxt},
}

var nameIcons = map[string]icon{
	".gitignore": {icGit, cOrange}, ".gitattributes": {icGit, cOrange}, ".gitmodules": {icGit, cOrange},
	"license": {"", cYellow}, "license.md": {"", cYellow}, "license.txt": {"", cYellow},
	"dockerfile": {"", cAcc}, "makefile": {"", cOrange},
	"readme.md": {"", cYellow}, "go.mod": {"", cPurple}, "go.sum": {"", cPurple},
	"package.json": {"", cRed},
}

var dirIcons = map[string]string{
	".git": icGit, "node_modules": "", ".github": "", ".config": "",
	"desktop": "", "documents": "", "downloads": "", "music": "",
	"pictures": "", "videos": "",
}

// iconFor picks the icon and its color for a list row
func iconFor(e *entry) icon {
	low := strings.ToLower(e.name)
	switch {
	case e.drive:
		return icon{icDrive, cAcc}
	case e.isDir && e.isRepo:
		return icon{icRepo, cAcc}
	case e.isDir:
		if g, ok := dirIcons[low]; ok {
			return icon{g, cAcc}
		}
		return icon{icFolder, cAcc}
	}
	if ic, ok := nameIcons[low]; ok {
		return ic
	}
	if strings.HasSuffix(low, ".blade.php") {
		return extIcons[".blade.php"]
	}
	if ic, ok := extIcons[filepath.Ext(low)]; ok {
		return ic
	}
	return icon{icFile, cHint}
}
