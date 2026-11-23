package files

// Rich previews: images (drawn with ▀ half blocks, two pixels per cell), a frame of a video
// (ffmpeg), audio details (ffprobe), archive contents, Markdown like render-markdown.nvim,
// and CSV as a table.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

var (
	imageExt   = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true}
	videoExt   = map[string]bool{".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".webm": true, ".m4v": true, ".wmv": true, ".flv": true}
	audioExt   = map[string]bool{".mp3": true, ".flac": true, ".wav": true, ".ogg": true, ".m4a": true, ".opus": true, ".aac": true, ".wma": true}
	archiveExt = map[string]bool{".zip": true, ".7z": true, ".rar": true, ".tar": true, ".gz": true, ".tgz": true, ".xz": true, ".bz2": true, ".zst": true, ".jar": true}
)

// richPreview fills p for files that aren't plain text; false = not one of those
func richPreview(p *preview, e entry, ext string) bool {
	switch {
	case isSheetExt(ext):
		sheetPreview(p, e)
		return true
	case imageExt[ext]:
		img, err := loadImage(e.path)
		if err != nil {
			p.lines = infoCard(e, binaryExt[ext])
			return true
		}
		b := img.Bounds()
		p.kind, p.img = "image", shrink(img, 1200)
		p.info = []string{"", " " + fg(cTxt).Bold(true).Render(e.name),
			" " + fg(cMut).Render(fmt.Sprintf("%s · %d × %d · %s", binaryExt[ext], b.Dx(), b.Dy(), humanSize(e.size)))}
		return true
	case videoExt[ext]:
		facts := probe(e.path, "stream=width,height,codec_name:format=duration")
		info := []string{"", " " + fg(cTxt).Bold(true).Render(e.name)}
		var parts []string
		if d := facts["duration"]; d != "" {
			parts = append(parts, duration(d))
		}
		if facts["width"] != "" {
			parts = append(parts, facts["width"]+" × "+facts["height"])
		}
		if facts["codec_name"] != "" {
			parts = append(parts, facts["codec_name"])
		}
		parts = append(parts, humanSize(e.size))
		info = append(info, " "+fg(cMut).Render(strings.Join(parts, " · ")))
		if img := videoFrame(e.path, facts["duration"]); img != nil {
			p.kind, p.img, p.info = "image", img, info
			return true
		}
		p.lines = infoCard(e, binaryExt[ext])
		if len(parts) > 1 { // more than the size we already show
			p.lines = append(p.lines, info[2:]...)
		}
		return true
	case audioExt[ext]:
		facts := probe(e.path, "format=duration,bit_rate:format_tags=title,artist,album")
		card := infoCard(e, "Audio")
		extra := [][2]string{{"title", facts["TAG:title"]}, {"artist", facts["TAG:artist"]}, {"album", facts["TAG:album"]}}
		if d := facts["duration"]; d != "" {
			extra = append(extra, [2]string{"length", duration(d)})
		}
		if br, err := strconv.Atoi(facts["bit_rate"]); err == nil {
			extra = append(extra, [2]string{"bitrate", fmt.Sprintf("%d kbps", br/1000)})
		}
		var rows []string
		for _, x := range extra {
			if x[1] != "" {
				rows = append(rows, "  "+fg(cMut).Render(pad(x[0], 10))+fg(cTxt).Render(x[1]))
			}
		}
		// details go above "enter opens it…"
		p.lines = append(append(card[:len(card)-2:len(card)-2], rows...), card[len(card)-2:]...)
		return true
	case archiveExt[ext]:
		p.lines = archiveList(e)
		return true
	}
	return false
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// shrink makes a big image small (previews never need more than a few hundred pixels)
func shrink(img image.Image, maxW int) image.Image {
	b := img.Bounds()
	if b.Dx() <= maxW && b.Dy() <= maxW {
		return img
	}
	s := float64(maxW) / float64(max(b.Dx(), b.Dy()))
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*s)), max(1, int(float64(b.Dy())*s))))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// imageLines draws img in w columns × h rows: each cell is ▀ with the top pixel as the
// text color and the bottom pixel as the background, so a cell shows two square pixels
func imageLines(img image.Image, w, h int) []string {
	b := img.Bounds()
	if w < 2 || h < 1 || b.Dx() == 0 || b.Dy() == 0 {
		return nil
	}
	s := min(float64(w)/float64(b.Dx()), float64(2*h)/float64(b.Dy()))
	tw, th := max(1, int(float64(b.Dx())*s)), max(2, int(float64(b.Dy())*s))
	th += th % 2
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	bgR, bgG, bgB := 0x28, 0x2c, 0x34 // transparent parts show the onedark background
	px := func(x, y int) (int, int, int) {
		c := dst.RGBAAt(x, y)
		a := int(c.A)
		return (int(c.R)*255 + bgR*(255-a)) / 255, (int(c.G)*255 + bgG*(255-a)) / 255, (int(c.B)*255 + bgB*(255-a)) / 255
	}
	// premultiplied RGBA: undo it before blending
	for i := 0; i < len(dst.Pix); i += 4 {
		if a := dst.Pix[i+3]; a > 0 && a < 255 {
			for k := 0; k < 3; k++ {
				dst.Pix[i+k] = uint8(min(255, int(dst.Pix[i+k])*255/int(a)))
			}
		}
	}
	left := strings.Repeat(" ", max(0, (w-tw)/2))
	var out []string
	for y := 0; y < th; y += 2 {
		var sb strings.Builder
		sb.WriteString(left)
		for x := 0; x < tw; x++ {
			r1, g1, b1 := px(x, y)
			r2, g2, b2 := px(x, y+1)
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", r1, g1, b1, r2, g2, b2)
		}
		sb.WriteString("\x1b[0m")
		out = append(out, sb.String())
	}
	return out
}

// probe asks ffprobe about a media file: key=value pairs
func probe(path, entries string) map[string]string {
	out := map[string]string{}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", entries,
		"-of", "default=nw=1", path).Output()
	if err != nil {
		b, _ = exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", entries, "-of", "default=nw=1", path).Output()
	}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok && v != "N/A" && out[k] == "" {
			out[k] = v
		}
	}
	return out
}

func duration(secs string) string {
	f, err := strconv.ParseFloat(secs, 64)
	if err != nil {
		return secs
	}
	t := int(f)
	if t >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", t/3600, t/60%60, t%60)
	}
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

// videoFrame grabs a frame a tenth of the way in (or at 0 for very short clips)
func videoFrame(path, dur string) image.Image {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil
	}
	at := 0.0
	if f, err := strconv.ParseFloat(dur, 64); err == nil {
		at = f / 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-ss", fmt.Sprintf("%.2f", at), "-i", path,
		"-frames:v", "1", "-vf", "scale=480:-2", "-f", "image2pipe", "-vcodec", "png", "-").Output()
	if err != nil || len(b) == 0 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	return img
}

// ---------------------------------------------------------------------------
// archives
// ---------------------------------------------------------------------------

type arcItem struct {
	name  string
	size  int64
	isDir bool
}

func archiveList(e entry) []string {
	var items []arcItem
	var err error
	if strings.EqualFold(filepath.Ext(e.name), ".zip") || strings.EqualFold(filepath.Ext(e.name), ".jar") {
		items, err = zipItems(e.path)
	} else {
		items, err = sevenZipItems(e.path)
	}
	head := []string{"", "  " + fg(cYellow).Render(icArchive) + "  " + fg(cTxt).Bold(true).Render(e.name)}
	if err != nil {
		return append(head, "", "  "+fg(cMut).Render("can't list it: "+err.Error()))
	}
	var total int64
	files := 0
	for _, it := range items {
		if !it.isDir {
			files++
			total += it.size
		}
	}
	head = append(head, "  "+fg(cMut).Render(fmt.Sprintf("%s · %s inside · %s packed", plural(files, "file"), humanSize(total), humanSize(e.size))), "")
	sort.SliceStable(items, func(i, j int) bool { return natLess(items[i].name, items[j].name) })
	for i, it := range items {
		if i >= 500 {
			head = append(head, "  "+fg(cMut).Render(fmt.Sprintf("… %d more", len(items)-500)))
			break
		}
		name := strings.TrimSuffix(filepath.ToSlash(it.name), "/")
		depth := strings.Count(name, "/")
		base := name[strings.LastIndex(name, "/")+1:]
		ent := entry{name: base, isDir: it.isDir}
		ic := iconFor(&ent)
		row := "  " + strings.Repeat("  ", depth) + fg(ic.col).Render(ic.glyph) + " " + fg(cTxt).Render(base)
		if !it.isDir {
			row += "  " + fg(cMut).Render(shortSize(it.size))
		}
		head = append(head, row)
	}
	return head
}

func zipItems(path string) ([]arcItem, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var out []arcItem
	for _, f := range r.File {
		out = append(out, arcItem{f.Name, int64(f.UncompressedSize64), strings.HasSuffix(f.Name, "/")})
	}
	return out, nil
}

// sevenZipItems lists 7z / rar / tar / gz with 7-Zip
func sevenZipItems(path string) ([]arcItem, error) {
	if _, err := exec.LookPath("7z"); err != nil {
		return nil, fmt.Errorf("install 7-Zip (scoop install 7zip) to look inside")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "7z", "l", "-slt", "-ba", path).Output()
	if err != nil {
		return nil, fmt.Errorf("7-Zip couldn't read it")
	}
	var out []arcItem
	var cur *arcItem
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		k, v, ok := strings.Cut(l, " = ")
		if !ok {
			continue
		}
		switch k {
		case "Path":
			out = append(out, arcItem{name: v})
			cur = &out[len(out)-1]
		case "Size":
			if cur != nil {
				cur.size, _ = strconv.ParseInt(v, 10, 64)
			}
		case "Folder":
			if cur != nil && v == "+" {
				cur.isDir = true
			}
		case "Attributes":
			if cur != nil && strings.HasPrefix(v, "D") {
				cur.isDir = true
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// CSV: turned into a Markdown table and drawn by the Markdown renderer
// ---------------------------------------------------------------------------

func csvAsMarkdown(text string, tab bool) string {
	r := csv.NewReader(strings.NewReader(text))
	if tab {
		r.Comma = '\t'
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var b strings.Builder
	rows := 0
	for rows < 200 {
		rec, err := r.Read()
		if err != nil {
			break
		}
		for i := range rec {
			rec[i] = strings.ReplaceAll(strings.TrimSpace(rec[i]), "|", `\|`)
		}
		b.WriteString("| " + strings.Join(rec, " | ") + " |\n")
		if rows == 0 {
			b.WriteString(strings.Repeat("|---", len(rec)) + "|\n")
		}
		rows++
	}
	return b.String()
}

// richLines draws a rich preview for a w × h column (cached until the size changes)
func (p *preview) richLines(w, h int) []string {
	if p.rend != nil && p.rendW == w && p.rendH == h {
		return p.rend
	}
	switch p.kind {
	case "image":
		room := max(1, h-len(p.info))
		p.rend = append(imageLines(p.img, w, room), p.info...)
	case "md":
		p.rend = renderMarkdown(p.src, w)
	case "csv":
		p.rend = renderMarkdown(csvAsMarkdown(p.src, strings.Contains(strings.SplitN(p.src, "\n", 2)[0], "\t")), w)
	}
	p.rendW, p.rendH = w, h
	return p.rend
}
