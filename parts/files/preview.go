package files

// The right column: a folder's contents, a file's text with code colors, or an info card.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Tahaldr/dotes/internal/md"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

const (
	previewBytes = 256 << 10 // read at most this much of a file
	previewLines = 1000
)

type preview struct {
	key     string
	dir     bool
	entries []entry // dir preview
	hiddenN int
	lines   []string // file preview (already colored)

	// rich previews, drawn for the column's size when shown
	kind  string      // "image", "md", "csv" (or "" = lines as they are)
	img   image.Image // image / video frame, already made small
	src   string      // markdown or csv text
	info  []string    // lines under an image
	rendW int
	rendH int
	rend  []string
}

// previewKey changes when the file changes, so the cache never shows old content
func previewKey(e entry, showHidden bool) string {
	return fmt.Sprintf("%s|%d|%d|%v", e.path, e.mod.UnixNano(), e.size, showHidden)
}

// never read these to guess: they're binary
var binaryExt = map[string]string{
	".png": "PNG image", ".jpg": "JPEG image", ".jpeg": "JPEG image", ".gif": "GIF image", ".webp": "WebP image",
	".bmp": "Bitmap image", ".ico": "Icon", ".mp4": "MP4 video", ".mkv": "MKV video", ".avi": "AVI video",
	".mov": "QuickTime video", ".webm": "WebM video", ".mp3": "MP3 audio", ".flac": "FLAC audio", ".wav": "WAV audio",
	".ogg": "Ogg audio", ".m4a": "M4A audio", ".opus": "Opus audio", ".pdf": "PDF document", ".doc": "Word document",
	".docx": "Word document", ".xls": "Excel sheet", ".xlsx": "Excel sheet", ".ppt": "PowerPoint", ".pptx": "PowerPoint",
	".zip": "Zip archive", ".7z": "7-Zip archive", ".rar": "RAR archive", ".tar": "Tar archive", ".gz": "Gzip archive",
	".xz": "XZ archive", ".exe": "Program", ".msi": "Installer", ".dll": "Library", ".ttf": "Font", ".otf": "Font",
	".woff": "Font", ".woff2": "Font", ".db": "Database", ".sqlite": "Database",
}

func makePreview(e entry, showHidden bool) preview {
	p := preview{key: previewKey(e, showHidden)}
	if e.isDir {
		p.dir = true
		list, n, err := readDir(e.path, showHidden)
		if err != nil {
			p.dir = false
			p.lines = []string{fg(cRed).Render("can't open: ") + fg(cMut).Render(errText(err))}
			return p
		}
		p.entries, p.hiddenN = list, n
		return p
	}
	ext := strings.ToLower(filepath.Ext(e.name))
	if richPreview(&p, e, ext) {
		return p
	}
	if kind, ok := binaryExt[ext]; ok {
		p.lines = infoCard(e, kind)
		return p
	}
	data, err := readHead(e.path)
	if err != nil {
		p.lines = []string{fg(cRed).Render("can't read: ") + fg(cMut).Render(errText(err))}
		return p
	}
	text, ok := asText(data)
	if !ok {
		p.lines = infoCard(e, "Binary file")
		return p
	}
	if text == "" {
		p.lines = []string{fg(cMut).Render("(empty file)")}
		return p
	}
	switch {
	case ext == ".md" || ext == ".markdown" || ext == ".mdx":
		p.kind, p.src = "md", text
		return p
	case (ext == ".csv" || ext == ".tsv") && e.size <= previewBytes:
		p.kind, p.src = "csv", text
		return p
	case (ext == ".json" || ext == ".jsonc" || ext == ".geojson") && e.size <= previewBytes:
		var buf bytes.Buffer
		if json.Indent(&buf, []byte(text), "", "  ") == nil {
			text = buf.String()
		}
	}
	p.lines = highlight(e.name, text)
	if e.size > previewBytes {
		p.lines = append(p.lines, "", fg(cMut).Render(fmt.Sprintf("… only the first %s shown", humanSize(previewBytes))))
	}
	return p
}

func errText(err error) string {
	if os.IsPermission(err) {
		return "access denied"
	}
	if pe, ok := err.(*os.PathError); ok {
		return pe.Err.Error()
	}
	return err.Error()
}

func readHead(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, previewBytes))
}

// asText decodes UTF-8 / UTF-16 text; ok is false for binary data
func asText(b []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		b = b[3:]
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}), bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		le := b[0] == 0xFF
		b = b[2:]
		u := make([]uint16, len(b)/2)
		for i := range u {
			if le {
				u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
			} else {
				u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
			}
		}
		return string(utf16.Decode(u)), true
	}
	head := b
	if len(head) > 8000 {
		head = head[:8000]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return "", false
	}
	// mostly valid UTF-8 = text (a few odd bytes, like an old Windows-1252 file, are fine)
	bad, n := 0, 0
	for i := 0; i < len(head); n++ {
		r, size := utf8.DecodeRune(head[i:])
		if r == utf8.RuneError && size == 1 && len(head)-i > 3 {
			bad++
		}
		i += size
	}
	if n > 0 && bad*10 > n {
		return "", false
	}
	return strings.ToValidUTF8(string(b), "�"), true
}

// isTextFile: open in the editor (true) or in the default Windows app (false)
func isTextFile(e entry) bool {
	if _, ok := binaryExt[strings.ToLower(filepath.Ext(e.name))]; ok {
		return false
	}
	data, err := readHead(e.path)
	if err != nil {
		return false
	}
	_, ok := asText(data)
	return ok
}

// infoCard: what a file is, when it can't be shown as text
func infoCard(e entry, kind string) []string {
	ic := iconFor(&e)
	k := fg(cMut)
	v := fg(cTxt)
	out := []string{
		"",
		"  " + fg(ic.col).Render(ic.glyph) + "  " + fg(cTxt).Bold(true).Render(e.name),
		"",
		"  " + k.Render("kind      ") + v.Render(kind),
		"  " + k.Render("size      ") + v.Render(humanSize(e.size)),
		"  " + k.Render("modified  ") + v.Render(e.mod.Format("2006-01-02 15:04")),
	}
	if strings.HasSuffix(kind, "image") {
		if f, err := os.Open(e.path); err == nil {
			if c, _, err := image.DecodeConfig(f); err == nil {
				out = append(out, "  "+k.Render("pixels    ")+v.Render(fmt.Sprintf("%d × %d", c.Width, c.Height)))
			}
			f.Close()
		}
	}
	return append(out, "", "  "+fg(cMut).Render("enter opens it in its default app"))
}

var codeStyle = styles.Get("onedark")

// highlight colors source code: one string per line, with line numbers
func highlight(name, text string) []string {
	text = strings.TrimRight(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\t", "    "), "\n")
	if n := strings.Count(text, "\n"); n >= previewLines {
		text = text[:nthIndex(text, '\n', previewLines)]
	}
	lexer := lexers.Match(name)
	if lexer == nil {
		lexer = lexers.Analyse(text)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	base := lipgloss.NewStyle().Foreground(cTxt)
	var lines []string
	if it, err := lexer.Tokenise(nil, text); err != nil {
		for _, l := range strings.Split(text, "\n") {
			lines = append(lines, base.Render(l))
		}
	} else {
		cur := ""
		for _, tok := range it.Tokens() {
			st := base
			if e := codeStyle.Get(tok.Type); e.Colour.IsSet() {
				st = st.Foreground(lipgloss.Color(e.Colour.String()))
			}
			for i, p := range strings.Split(tok.Value, "\n") {
				if i > 0 {
					lines = append(lines, cur)
					cur = ""
				}
				if p != "" {
					cur += st.Render(p)
				}
			}
		}
		lines = append(lines, cur)
	}
	digits := len(fmt.Sprint(len(lines)))
	num := fg(cDim)
	for i := range lines {
		lines[i] = num.Render(fmt.Sprintf("%*d ", digits, i+1)) + lines[i]
	}
	return lines
}

func nthIndex(s string, c byte, n int) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			n--
			if n == 0 {
				return i
			}
		}
	}
	return len(s)
}

// colorCode splits code into colored lines (bg = background color, or nil)
func colorCode(lexer chroma.Lexer, text string, bg color.Color) []string {
	return md.ColorCode(lexer, text, bg)
}

// renderMarkdown draws markdown the shared Dotes way
func renderMarkdown(s string, w int) []string { return md.Render(s, w) }
