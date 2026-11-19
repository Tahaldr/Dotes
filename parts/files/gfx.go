package files

// Real images with Sixel (Windows Terminal 1.22+, WezTerm, foot, ...). At start Files asks the
// terminal what it can do; if it says "Sixel", images are sent as real pixels, otherwise they're
// drawn with ▀ blocks. DOTES_IMAGES=blocks or =sixel forces one.
//
// The screen drawing (Bubble Tea) doesn't know about images, so the preview fills the image's
// cells with styled blanks, and right after the frame is drawn the picture is written on top.
// To take a picture away, those cells just get a different style: the screen drawing then
// rewrites them, which wipes the pixels. Each new picture flips the style, so the old one is
// always wiped before the new one is drawn.

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/sixel"
	"golang.org/x/image/draw"
)

type imgSpot struct {
	key        string // what + where: when it changes, the picture is redrawn
	img        image.Image
	x, y, w, h int // cells, 0-based
}

type imgSyncMsg struct{}

// detectGfx asks the terminal for its features and its cell size in pixels
func (m *model) detectGfx() tea.Cmd {
	mode := strings.ToLower(os.Getenv("DOTES_IMAGES"))
	if mode == "" {
		mode = cfg.images
	}
	m.gfxForced = mode == "blocks"
	switch mode {
	case "blocks":
		return nil
	case "sixel":
		m.gfx = "sixel"
	}
	if !m.live {
		return nil
	}
	return tea.Sequence(tea.Raw(ansi.RequestPrimaryDeviceAttributes), tea.Raw("\x1b[16t"))
}

// gfxMsg handles the terminal's answers; true = it was one of them
func (m *model) gfxMsg(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case uv.PrimaryDeviceAttributesEvent:
		if m.gfxForced {
			return true
		}
		for _, a := range msg {
			if a == 4 { // 4 = Sixel graphics
				m.gfx = "sixel"
				for k, p := range m.cache { // previews made for blocks: draw them again
					p.rend = nil
					m.cache[k] = p
				}
				m.pv.rend = nil
			}
		}
		return true
	case uv.CellSizeEvent:
		if msg.Width > 0 && msg.Height > 0 {
			m.cellW, m.cellH = msg.Width, msg.Height
			m.imgShown = imgSpot{} // sizes changed: redraw
		}
		return true
	}
	return false
}

func (m *model) imgSyncLater() tea.Cmd {
	if m.gfx != "sixel" || m.dump {
		return nil
	}
	return tea.Tick(25*time.Millisecond, func(time.Time) tea.Msg { return imgSyncMsg{} })
}

// imgSync puts the wanted picture on screen (after View has laid it out), or takes it away
func (m *model) imgSync() tea.Cmd {
	want := m.imgWant
	if m.popupOpen() {
		want = imgSpot{}
	}
	if want.key == m.imgShown.key {
		return nil
	}
	var out strings.Builder
	out.WriteString("\x1b7") // save the cursor
	if want.key != "" {
		if s := m.sixelFor(want); s != "" {
			fmt.Fprintf(&out, "\x1b[%d;%dH", want.y+1+OriginY, want.x+1)
			out.WriteString(s)
		}
	}
	out.WriteString("\x1b8") // and put it back
	m.imgShown = want
	return tea.Raw(out.String())
}

// sixelFor scales the picture to fit its cells (keeping its shape) and encodes it
func (m *model) sixelFor(s imgSpot) string {
	if s.key == m.sixelKey {
		return m.sixelData
	}
	b := s.img.Bounds()
	pw, ph := s.w*m.cellW, s.h*m.cellH
	sc := min(float64(pw)/float64(b.Dx()), float64(ph)/float64(b.Dy()))
	tw, th := max(1, int(float64(b.Dx())*sc)), max(1, int(float64(b.Dy())*sc))
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.CatmullRom.Scale(dst, dst.Bounds(), s.img, b, draw.Src, nil)
	var buf bytes.Buffer
	if err := (&sixel.Encoder{}).Encode(&buf, dst); err != nil {
		return ""
	}
	m.sixelKey, m.sixelData = s.key, ansi.SixelGraphics(0, 1, 0, buf.Bytes())
	return m.sixelData
}

// popupOpen: anything drawn over the preview hides the picture
func (m *model) popupOpen() bool {
	return m.help || m.prompt != nil || m.question != nil || m.showTasks || m.showMarks || m.spot != nil || m.menu != nil ||
		(m.pending != "" && m.hintsOn)
}

// sixelPreview lays out an image preview: blank rows for the picture, then the info lines.
// x, y are where the preview column's text starts.
func (m *model) sixelPreview(w, h, x, y int) []string {
	p := &m.pv
	room := max(1, h-len(p.info))
	b := p.img.Bounds()
	// how many cells the picture needs at this size
	pw, ph := w*m.cellW, room*m.cellH
	sc := min(float64(pw)/float64(b.Dx()), float64(ph)/float64(b.Dy()))
	cw := min(w, max(1, (int(float64(b.Dx())*sc)+m.cellW-1)/m.cellW))
	ch := min(room, max(1, (int(float64(b.Dy())*sc)+m.cellH-1)/m.cellH))
	left := (w - cw) / 2
	m.imgWant = imgSpot{key: fmt.Sprintf("%s|%d|%d|%d|%d|%d|%d", p.key, x+left, y, cw, ch, m.cellW, m.cellH),
		img: p.img, x: x + left, y: y, w: cw, h: ch}
	if m.imgWant.key != m.lastImgKey { // a new picture: new blank style, so the old pixels get wiped
		m.lastImgKey = m.imgWant.key
		m.imgFlip = !m.imgFlip
	}
	st := lipgloss.NewStyle().Italic(true)
	if m.imgFlip {
		st = lipgloss.NewStyle().Faint(true)
	}
	out := make([]string, ch)
	for i := range out {
		out[i] = st.Render(strings.Repeat(" ", w))
	}
	return append(out, p.info...)
}

// sixelTest prints a picture straight to the terminal: if you see it, real images work there
func sixelTest(path string) {
	img, err := loadImage(path)
	if err != nil {
		fmt.Println("can't read the image:", err)
		return
	}
	m := &model{cellW: 10, cellH: 20}
	b := img.Bounds()
	w := min(40, max(1, b.Dx()/10))
	h := max(1, w*b.Dy()*m.cellW/max(1, b.Dx())/m.cellH)
	fmt.Print(m.sixelFor(imgSpot{key: "test", img: img, w: w, h: h}))
	fmt.Println()
	fmt.Println("If you see the picture above, this terminal shows real images (Sixel).")
	fmt.Println("If you see nothing or odd characters, Dotes will draw images with blocks here.")
}
