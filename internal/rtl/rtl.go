// Adapted from an Arabic display fix made for cliamp.
// Package rtl makes Arabic/Hebrew text readable on terminals that draw every
// line strictly left-to-right (Windows Terminal, conhost).
//
// For each run of right-to-left text it:
//  1. shapes Arabic letters into their contextual Presentation Forms
//     (initial/medial/final/isolated + lam-alef ligatures) in logical order,
//     so the terminal cannot re-shape them after reordering;
//  2. reverses the run into visual order, keeping combining marks attached
//     to their base letter, keeping digit/Latin groups inside the run
//     left-to-right, and mirroring brackets.
//
// ANSI escape sequences are passed through untouched; text between escapes
// is processed segment by segment.
package rtl

import (
	"strings"
	"unicode"
)

// Enabled turns the transform on or off globally.
var Enabled = true

// forms holds the presentation forms of a letter:
// isolated, final, initial, medial. 0 = form does not exist
// (the letter only joins to the previous letter).
type forms [4]rune

var arabicForms = map[rune]forms{
	0x0621: {0xFE80, 0, 0, 0},                // hamza
	0x0622: {0xFE81, 0xFE82, 0, 0},           // alef madda
	0x0623: {0xFE83, 0xFE84, 0, 0},           // alef hamza above
	0x0624: {0xFE85, 0xFE86, 0, 0},           // waw hamza
	0x0625: {0xFE87, 0xFE88, 0, 0},           // alef hamza below
	0x0626: {0xFE89, 0xFE8A, 0xFE8B, 0xFE8C}, // yeh hamza
	0x0627: {0xFE8D, 0xFE8E, 0, 0},           // alef
	0x0628: {0xFE8F, 0xFE90, 0xFE91, 0xFE92}, // beh
	0x0629: {0xFE93, 0xFE94, 0, 0},           // teh marbuta
	0x062A: {0xFE95, 0xFE96, 0xFE97, 0xFE98}, // teh
	0x062B: {0xFE99, 0xFE9A, 0xFE9B, 0xFE9C}, // theh
	0x062C: {0xFE9D, 0xFE9E, 0xFE9F, 0xFEA0}, // jeem
	0x062D: {0xFEA1, 0xFEA2, 0xFEA3, 0xFEA4}, // hah
	0x062E: {0xFEA5, 0xFEA6, 0xFEA7, 0xFEA8}, // khah
	0x062F: {0xFEA9, 0xFEAA, 0, 0},           // dal
	0x0630: {0xFEAB, 0xFEAC, 0, 0},           // thal
	0x0631: {0xFEAD, 0xFEAE, 0, 0},           // reh
	0x0632: {0xFEAF, 0xFEB0, 0, 0},           // zain
	0x0633: {0xFEB1, 0xFEB2, 0xFEB3, 0xFEB4}, // seen
	0x0634: {0xFEB5, 0xFEB6, 0xFEB7, 0xFEB8}, // sheen
	0x0635: {0xFEB9, 0xFEBA, 0xFEBB, 0xFEBC}, // sad
	0x0636: {0xFEBD, 0xFEBE, 0xFEBF, 0xFEC0}, // dad
	0x0637: {0xFEC1, 0xFEC2, 0xFEC3, 0xFEC4}, // tah
	0x0638: {0xFEC5, 0xFEC6, 0xFEC7, 0xFEC8}, // zah
	0x0639: {0xFEC9, 0xFECA, 0xFECB, 0xFECC}, // ain
	0x063A: {0xFECD, 0xFECE, 0xFECF, 0xFED0}, // ghain
	0x0641: {0xFED1, 0xFED2, 0xFED3, 0xFED4}, // feh
	0x0642: {0xFED5, 0xFED6, 0xFED7, 0xFED8}, // qaf
	0x0643: {0xFED9, 0xFEDA, 0xFEDB, 0xFEDC}, // kaf
	0x0644: {0xFEDD, 0xFEDE, 0xFEDF, 0xFEE0}, // lam
	0x0645: {0xFEE1, 0xFEE2, 0xFEE3, 0xFEE4}, // meem
	0x0646: {0xFEE5, 0xFEE6, 0xFEE7, 0xFEE8}, // noon
	0x0647: {0xFEE9, 0xFEEA, 0xFEEB, 0xFEEC}, // heh
	0x0648: {0xFEED, 0xFEEE, 0, 0},           // waw
	0x0649: {0xFEEF, 0xFEF0, 0xFBE8, 0xFBE9}, // alef maksura
	0x064A: {0xFEF1, 0xFEF2, 0xFEF3, 0xFEF4}, // yeh
	0x0671: {0xFB50, 0xFB51, 0, 0},           // alef wasla
	0x067E: {0xFB56, 0xFB57, 0xFB58, 0xFB59}, // peh
	0x0686: {0xFB7A, 0xFB7B, 0xFB7C, 0xFB7D}, // tcheh
	0x0698: {0xFB8A, 0xFB8B, 0, 0},           // jeh
	0x06A4: {0xFB6A, 0xFB6B, 0xFB6C, 0xFB6D}, // veh
	0x06A9: {0xFB8E, 0xFB8F, 0xFB90, 0xFB91}, // keheh
	0x06AF: {0xFB92, 0xFB93, 0xFB94, 0xFB95}, // gaf
	0x06CC: {0xFBFC, 0xFBFD, 0xFBFE, 0xFBFF}, // farsi yeh
}

const (
	tatweel = 0x0640
	zwj     = 0x200D
)

func isRTL(r rune) bool {
	switch {
	case r >= 0x0590 && r <= 0x08FF: // Hebrew, Arabic, Syriac, Thaana, NKo, ...
		return !isMark(r) && !isArabicDigit(r)
	case r >= 0xFB1D && r <= 0xFDFF, r >= 0xFE70 && r <= 0xFEFF:
		return true
	}
	return false
}

func isArabicDigit(r rune) bool {
	return (r >= 0x0660 && r <= 0x0669) || (r >= 0x06F0 && r <= 0x06F9)
}

func isMark(r rune) bool { return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) }

// isLTRStrong is a char that must keep left-to-right order inside an RTL run.
func isLTRStrong(r rune) bool {
	return unicode.IsLetter(r) && !isRTL(r) || unicode.IsDigit(r)
}

// joinsBoth reports whether r can connect to the letter after it.
func joinsNext(r rune) bool {
	if r == tatweel || r == zwj {
		return true
	}
	f, ok := arabicForms[r]
	return ok && f[2] != 0
}

func joinsPrev(r rune) bool {
	if r == tatweel || r == zwj {
		return true
	}
	_, ok := arabicForms[r]
	return ok
}

// shape converts Arabic letters to presentation forms (logical order).
func shape(rs []rune) []rune {
	// neighbour lookup that skips combining marks
	prevBase := func(i int) rune {
		for j := i - 1; j >= 0; j-- {
			if !isMark(rs[j]) {
				return rs[j]
			}
		}
		return 0
	}
	nextBase := func(i int) (rune, int) {
		for j := i + 1; j < len(rs); j++ {
			if !isMark(rs[j]) {
				return rs[j], j
			}
		}
		return 0, -1
	}

	out := make([]rune, 0, len(rs))
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		f, ok := arabicForms[r]
		if !ok {
			out = append(out, r)
			continue
		}
		p := prevBase(i)
		joinPrev := p != 0 && joinsNext(p)

		// Lam-alef is left as two joined letters (lam initial/medial + alef final)
		// instead of the U+FEF5-FEFC ligatures: monospace fonts such as Cascadia
		// draw it that way, and a ligature squeezed into one cell looks stacked.
		n, _ := nextBase(i)
		joinNext := f[2] != 0 && n != 0 && joinsPrev(n)

		var g rune
		switch {
		case joinPrev && joinNext:
			g = f[3]
		case joinPrev:
			g = f[1]
		case joinNext:
			g = f[2]
		}
		// Isolated: use the isolated presentation form, so the terminal cannot
		// join two neighbouring lone letters after reordering. Plain letter only
		// when no isolated form exists.
		if g == 0 {
			g = f[0]
		}
		if g == 0 {
			g = r
		}
		out = append(out, g)
	}
	return out
}

var mirror = map[rune]rune{
	'(': ')', ')': '(', '[': ']', ']': '[', '{': '}', '}': '{',
	'<': '>', '>': '<', '«': '»', '»': '«',
}

// reverseRun turns a logical RTL run into visual order.
func reverseRun(rs []rune) []rune {
	// split into clusters: base+marks, or LTR groups (latin words / numbers)
	var clusters [][]rune
	for i := 0; i < len(rs); {
		j := i + 1
		if isLTRStrong(rs[i]) {
			// group LTR chars plus inner '.', ',', ':' , '/' between them
			for j < len(rs) && (isLTRStrong(rs[j]) || isMark(rs[j]) ||
				(strings.ContainsRune(".,:/-'", rs[j]) && j+1 < len(rs) && isLTRStrong(rs[j+1]))) {
				j++
			}
		} else {
			for j < len(rs) && isMark(rs[j]) {
				j++
			}
		}
		clusters = append(clusters, rs[i:j])
		i = j
	}
	out := make([]rune, 0, len(rs))
	for k := len(clusters) - 1; k >= 0; k-- {
		c := clusters[k]
		if len(c) == 1 {
			if m, ok := mirror[c[0]]; ok {
				out = append(out, m)
				continue
			}
		}
		out = append(out, c...)
	}
	return out
}

// Line converts one plain-text line (no escape codes) to visual order.
// Runs start and end at an RTL letter; spaces, punctuation and numbers
// between two RTL letters belong to the run.
func Line(s string) string {
	if !Enabled || !hasRTL(s) {
		return s
	}
	rs := []rune(s)
	out := make([]rune, 0, len(rs))
	for i := 0; i < len(rs); {
		if !isRTL(rs[i]) {
			out = append(out, rs[i])
			i++
			continue
		}
		// extend run while another RTL letter follows before any LTR letter
		last := i
		for j := i + 1; j < len(rs); j++ {
			r := rs[j]
			if isRTL(r) {
				last = j
				continue
			}
			if unicode.IsLetter(r) && !isRTL(r) {
				break // latin word ends the RTL run
			}
		}
		end := last + 1
		// trailing marks belong to the last letter
		for end < len(rs) && isMark(rs[end]) {
			end++
		}
		run := rs[i:end]
		shaped := shape(run)
		out = append(out, reverseRun(shaped)...)
		// lam-alef ligatures merge two cells into one; pad so the line keeps
		// its width and box borders stay aligned.
		for n := cells(run) - cells(shaped); n > 0; n-- {
			out = append(out, ' ')
		}
		i = end
	}
	return string(out)
}

// cells counts terminal columns (combining marks take none).
func cells(rs []rune) int {
	n := 0
	for _, r := range rs {
		if !isMark(r) {
			n++
		}
	}
	return n
}

func hasRTL(s string) bool {
	for _, r := range s {
		if isRTL(r) {
			return true
		}
	}
	return false
}

// Frame applies Line to every line of a rendered frame, skipping ANSI/OSC
// escape sequences so styling is preserved.
func Frame(s string) string {
	if !Enabled || !hasRTL(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	var seg strings.Builder
	flush := func() {
		if seg.Len() > 0 {
			b.WriteString(Line(seg.String()))
			seg.Reset()
		}
	}
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\n' {
			flush()
			b.WriteByte(c)
			i++
			continue
		}
		if c == 0x1b && i+1 < len(s) {
			flush()
			j := escapeEnd(s, i)
			b.WriteString(s[i:j])
			i = j
			continue
		}
		seg.WriteByte(c)
		i++
	}
	flush()
	return b.String()
}

// escapeEnd returns the index just past the escape sequence starting at i.
func escapeEnd(s string, i int) int {
	switch s[i+1] {
	case '[': // CSI: ESC [ params final(0x40-0x7E)
		for j := i + 2; j < len(s); j++ {
			if s[j] >= 0x40 && s[j] <= 0x7E {
				return j + 1
			}
		}
	case ']', 'P', '_', '^': // OSC/DCS/APC/PM: until BEL or ST
		for j := i + 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
		}
	default:
		return i + 2
	}
	return len(s)
}
