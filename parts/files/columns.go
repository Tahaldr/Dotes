package files

// Column widths: 1 : 4 : 3 like yazi, until you drag a │ line between two columns with the
// mouse. The widths are kept in files.txt (columns = parent current, as parts of the width);
// = puts them back.

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	minParent  = 8
	minCurrent = 16
	minPreview = 12
)

// columnWidths: parent, current and preview widths for this window (parent 0 = not shown)
func (m *model) columnWidths() (pw, cw, prw int) {
	avail := m.w - 2
	narrow := m.w < 70
	if narrow {
		avail = m.w - 1
	}
	m.avail = avail
	pf, cf := 1.0/8, 1.0/2
	if m.cols[1] > 0 {
		pf, cf = m.cols[0], m.cols[1]
	}
	if narrow { // no parent column: current and preview share the width as they would
		pw = 0
		cw = int(float64(avail) * cf / (1 - pf))
		if m.cols[1] == 0 {
			cw = avail * 4 / 7
		}
	} else {
		pw = max(minParent, int(float64(avail)*pf+0.5))
		cw = int(float64(avail)*cf + 0.5)
	}
	cw = max(minCurrent, min(cw, avail-pw-minPreview))
	return pw, cw, avail - pw - cw
}

// dragStart: a press on a line between two columns starts resizing
func (m *model) dragStart(x, y int) bool {
	if y < m.bodyY || y >= m.bodyY+m.bodyH {
		return false
	}
	switch {
	case m.colX[0][1] > 0 && x == m.colX[0][1]:
		m.drag = 1
	case x == m.colX[1][1]:
		m.drag = 2
	default:
		return false
	}
	return true
}

func (m *model) dragTo(x int) {
	if m.drag == 0 || m.avail <= 0 {
		return
	}
	pw, cw, _ := m.columnWidths()
	if m.drag == 1 {
		npw := max(minParent, min(x, m.avail-cw-minPreview))
		m.cols = [2]float64{float64(npw) / float64(m.avail), float64(cw) / float64(m.avail)}
		return
	}
	start := 0
	if pw > 0 {
		start = pw + 1
	}
	ncw := max(minCurrent, min(x-start, m.avail-pw-minPreview))
	pf := float64(pw) / float64(m.avail)
	if pw == 0 { // narrow: keep the parent's share as it was
		pf = 1.0 / 8
		if m.cols[1] > 0 {
			pf = m.cols[0]
		}
		m.cols = [2]float64{pf, float64(ncw) / float64(m.avail) * (1 - pf)}
		return
	}
	m.cols = [2]float64{pf, float64(ncw) / float64(m.avail)}
}

func (m *model) dragEnd() {
	if m.drag == 0 {
		return
	}
	m.drag = 0
	m.imgShown = imgSpot{} // the picture moved with the preview column
	if !m.dump {
		setConfigValue("columns", fmt.Sprintf("%.3f %.3f", m.cols[0], m.cols[1]))
	}
}

func (m *model) resetColumns() {
	m.cols = [2]float64{}
	m.imgShown = imgSpot{}
	if !m.dump {
		setConfigValue("columns", "")
	}
	m.setNote("columns back to 1 : 4 : 3", false)
}

// parseColumns reads "0.125 0.500" (empty or odd = the usual widths)
func parseColumns(v string) [2]float64 {
	f := strings.Fields(v)
	if len(f) != 2 {
		return [2]float64{}
	}
	p, err1 := strconv.ParseFloat(f[0], 64)
	c, err2 := strconv.ParseFloat(f[1], 64)
	if err1 != nil || err2 != nil || p < 0 || c <= 0 || p+c >= 0.95 {
		return [2]float64{}
	}
	return [2]float64{p, c}
}
