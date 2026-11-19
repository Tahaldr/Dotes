package notes

// A todo list is a plain Markdown file, the same one the Nvim popup (checkmate.nvim) uses:
//
//	# Groceries
//
//	- [ ] milk @priority(high)
//	  - [x] the cheap one
//	- [x] bread
//
//	## Archive
//	- [x] eggs
//
// Every line is kept as it is; only the lines you change are rewritten, so the file still
// opens fine in Nvim. Sub-todos are indented 2 spaces deeper than their parent.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type line struct {
	s    string
	fold bool // a todo whose sub-todos are hidden
}

type todoDoc struct {
	path       string
	lines      []line
	crlf, eol  bool // the file's line endings, kept when saving
	mod        time.Time
	undo, redo [][]line
}

type todo struct {
	ind, bullet string
	done        bool
	text        string
}

var (
	todoRe     = regexp.MustCompile(`^(\s*)([-*+]) \[([ xX])\](.*)$`)
	headingRe  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*$`)
	priorityRe = regexp.MustCompile(`\s*@priority\(([^)]*)\)`)
)

var priorities = []string{"", "low", "medium", "high"}

func parseTodo(s string) (todo, bool) {
	m := todoRe.FindStringSubmatch(s)
	if m == nil {
		return todo{}, false
	}
	return todo{m[1], m[2], m[3] != " ", strings.TrimPrefix(m[4], " ")}, true
}

func (t todo) String() string {
	box := "[ ]"
	if t.done {
		box = "[x]"
	}
	return t.ind + t.bullet + " " + box + " " + t.text
}

func indentOf(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 2
		default:
			return n
		}
	}
	return n
}

func blank(s string) bool     { return strings.TrimSpace(s) == "" }
func isHeading(s string) bool { return headingRe.MatchString(s) }
func isArchive(s string) bool {
	m := headingRe.FindStringSubmatch(s)
	return m != nil && len(m[1]) > 1 && strings.EqualFold(m[2], "archive")
}

// splitLines: the file's lines, and whether it used \r\n and ended with a newline
func splitLines(text string) (lines []string, crlf, eol bool) {
	text = strings.TrimPrefix(text, "\uFEFF")
	crlf = strings.Contains(text, "\r\n")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	eol = strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil, crlf, eol
	}
	return strings.Split(text, "\n"), crlf, eol
}

func loadDoc(path string) (*todoDoc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ls, crlf, eol := splitLines(string(b))
	d := &todoDoc{path: path, crlf: crlf, eol: eol || len(ls) == 0}
	for _, s := range ls {
		d.lines = append(d.lines, line{s: s})
	}
	if fi, err := os.Stat(path); err == nil {
		d.mod = fi.ModTime()
	}
	return d, nil
}

// save writes the file safely: a temporary file first, then it takes the old one's place
func (d *todoDoc) save() error {
	nl := "\n"
	if d.crlf {
		nl = "\r\n"
	}
	var b strings.Builder
	for i, l := range d.lines {
		if i > 0 {
			b.WriteString(nl)
		}
		b.WriteString(l.s)
	}
	if d.eol {
		b.WriteString(nl)
	}
	if err := writeAtomic(d.path, []byte(b.String())); err != nil {
		return err
	}
	if fi, err := os.Stat(d.path); err == nil {
		d.mod = fi.ModTime()
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dotes-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// ---- undo ----

func copyLines(ls []line) []line { return append([]line(nil), ls...) }

// remember is called before every change
func (d *todoDoc) remember() {
	d.undo = append(d.undo, copyLines(d.lines))
	if len(d.undo) > 200 {
		d.undo = d.undo[1:]
	}
	d.redo = nil
}

// forget drops the last remember (a change that ended up doing nothing)
func (d *todoDoc) forget() {
	if n := len(d.undo); n > 0 {
		d.undo = d.undo[:n-1]
	}
}

func (d *todoDoc) undoLast() bool {
	n := len(d.undo)
	if n == 0 {
		return false
	}
	d.redo = append(d.redo, copyLines(d.lines))
	d.lines, d.undo = d.undo[n-1], d.undo[:n-1]
	return true
}

func (d *todoDoc) redoLast() bool {
	n := len(d.redo)
	if n == 0 {
		return false
	}
	d.undo = append(d.undo, copyLines(d.lines))
	d.lines, d.redo = d.redo[n-1], d.redo[:n-1]
	return true
}

// ---- structure ----

func (d *todoDoc) isTodo(i int) bool {
	if i < 0 || i >= len(d.lines) {
		return false
	}
	_, ok := parseTodo(d.lines[i].s)
	return ok
}

func (d *todoDoc) todoAt(i int) todo { t, _ := parseTodo(d.lines[i].s); return t }

func (d *todoDoc) setTodo(i int, t todo) { d.lines[i].s = t.String() }

func (d *todoDoc) ind(i int) int { return indentOf(d.lines[i].s) }

// archiveStart: the line of the "## Archive" heading (or the end)
func (d *todoDoc) archiveStart() int {
	for i, l := range d.lines {
		if isArchive(l.s) {
			return i
		}
	}
	return len(d.lines)
}

func (d *todoDoc) inArchive(i int) bool { return i >= d.archiveStart() }

// subtreeEnd: the last line of the todo at i and everything indented deeper below it
func (d *todoDoc) subtreeEnd(i int) int {
	base, last := d.ind(i), i
	for j := i + 1; j < len(d.lines); j++ {
		s := d.lines[j].s
		if blank(s) {
			continue
		}
		if indentOf(s) <= base || isHeading(s) {
			break
		}
		last = j
	}
	return last
}

func (d *todoDoc) hasSubs(i int) bool { return d.isTodo(i) && d.subtreeEnd(i) > i }

// parentOf: the todo that i is a sub-todo of, or -1
func (d *todoDoc) parentOf(i int) int {
	in := d.ind(i)
	for j := i - 1; j >= 0; j-- {
		s := d.lines[j].s
		if blank(s) {
			continue
		}
		if isHeading(s) {
			return -1
		}
		if indentOf(s) < in {
			if d.isTodo(j) {
				return j
			}
			return -1
		}
	}
	return -1
}

// children: the todos directly under p
func (d *todoDoc) children(p int) []int {
	var out []int
	for j := p + 1; j <= d.subtreeEnd(p); j++ {
		if d.isTodo(j) && d.parentOf(j) == p {
			out = append(out, j)
		}
	}
	return out
}

// subCount: done / all todos below i
func (d *todoDoc) subCount(i int) (done, all int) {
	for j := i + 1; j <= d.subtreeEnd(i); j++ {
		if t, ok := parseTodo(d.lines[j].s); ok {
			all++
			if t.done {
				done++
			}
		}
	}
	return
}

// progress counts the todos above the archive
func progress(lines []string) (done, all int) {
	for _, s := range lines {
		if isArchive(s) {
			break
		}
		if t, ok := parseTodo(s); ok {
			all++
			if t.done {
				done++
			}
		}
	}
	return
}

func (d *todoDoc) texts() []string {
	out := make([]string, len(d.lines))
	for i, l := range d.lines {
		out[i] = l.s
	}
	return out
}

// rows: the lines shown, in order (no blank lines, no "# title", nothing inside a fold)
func (d *todoDoc) rows() []int {
	var out []int
	hide := -1
	for i, l := range d.lines {
		if i <= hide || blank(l.s) {
			continue
		}
		if m := headingRe.FindStringSubmatch(l.s); m != nil && len(m[1]) == 1 && len(out) == 0 {
			continue // the title is in the header
		}
		out = append(out, i)
		if l.fold && d.hasSubs(i) {
			hide = d.subtreeEnd(i)
		}
	}
	return out
}

// ---- changes (the caller calls remember() first and saves after) ----

// toggle ticks / unticks i with all its sub-todos; a parent is ticked when all its
// sub-todos are, and unticked when one isn't (like checkmate's smart toggle)
func (d *todoDoc) toggle(i int) {
	if !d.isTodo(i) {
		return
	}
	done := !d.todoAt(i).done
	for j := i; j <= d.subtreeEnd(i); j++ {
		if t, ok := parseTodo(d.lines[j].s); ok {
			t.done = done
			d.setTodo(j, t)
		}
	}
	for p := d.parentOf(i); p >= 0; p = d.parentOf(p) {
		all := true
		for _, c := range d.children(p) {
			if !d.todoAt(c).done {
				all = false
			}
		}
		t := d.todoAt(p)
		if t.done == all {
			break
		}
		t.done = all
		d.setTodo(p, t)
	}
}

func (d *todoDoc) insert(at int, ls ...line) {
	at = max(0, min(at, len(d.lines)))
	d.lines = append(d.lines[:at], append(copyLines(ls), d.lines[at:]...)...)
}

func (d *todoDoc) remove(from, to int) []line {
	out := copyLines(d.lines[from : to+1])
	d.lines = append(d.lines[:from], d.lines[to+1:]...)
	return out
}

// mainEnd: where a new todo goes when there's none to put it next to (end of the list,
// above the archive); a blank line keeps it away from a heading right above
func (d *todoDoc) mainEnd() int {
	a := d.archiveStart()
	for a > 0 && blank(d.lines[a-1].s) {
		a--
	}
	if a > 0 && isHeading(d.lines[a-1].s) {
		if a >= len(d.lines) || !blank(d.lines[a].s) {
			d.insert(a, line{s: ""})
		}
		a++ // below the blank line
	}
	if a < len(d.lines) && isArchive(d.lines[a].s) {
		d.insert(a, line{s: ""}) // keep the archive heading apart
	}
	return a
}

func newTodoLine(ind int) line { return line{s: strings.Repeat(" ", ind) + "- [ ] "} }

// addBelow adds an empty todo after i (after its sub-todos); sub = one level deeper.
// It returns the new line's index.
func (d *todoDoc) addBelow(i int, sub bool) int {
	if !d.isTodo(i) {
		at := d.mainEnd()
		d.insert(at, newTodoLine(0))
		return at
	}
	in, end := d.ind(i), d.subtreeEnd(i)
	if sub {
		if end > i {
			for j := i + 1; j <= end; j++ {
				if !blank(d.lines[j].s) {
					in = d.ind(j)
					break
				}
			}
		} else {
			in += 2
		}
		d.lines[i].fold = false
	}
	d.insert(end+1, newTodoLine(in))
	return end + 1
}

func (d *todoDoc) addAbove(i int) int {
	if !d.isTodo(i) {
		return d.addBelow(i, false)
	}
	d.insert(i, newTodoLine(d.ind(i)))
	return i
}

func (d *todoDoc) cut(i int) []line { return d.remove(i, d.subtreeEnd(i)) }

func (d *todoDoc) copyTree(i int) []line { return copyLines(d.lines[i : d.subtreeEnd(i)+1]) }

// paste puts copied todos below i (above when above is set), indented like i; returns where
func (d *todoDoc) paste(i int, reg []line, above bool) int {
	if len(reg) == 0 {
		return i
	}
	target, at := 0, 0
	if d.isTodo(i) {
		target = d.ind(i)
		at = d.subtreeEnd(i) + 1
		if above {
			at = i
		}
	} else {
		at = d.mainEnd()
	}
	base := indentOf(reg[0].s)
	var ls []line
	for _, l := range reg {
		ls = append(ls, line{s: reindent(l.s, base, target), fold: l.fold})
	}
	d.insert(at, ls...)
	return at
}

// reindent moves s from being indented under base to under target
func reindent(s string, base, target int) string {
	if blank(s) {
		return s
	}
	in := indentOf(s)
	return strings.Repeat(" ", max(0, in-base+target)) + strings.TrimLeft(s, " \t")
}

// shift indents (dir 1) or outdents (dir -1) i with its sub-todos; false when it can't
func (d *todoDoc) shift(i, dir int) bool {
	if !d.isTodo(i) || (dir < 0 && d.ind(i) == 0) {
		return false
	}
	end := d.subtreeEnd(i) // found before anything moves
	for j := i; j <= end; j++ {
		if s := d.lines[j].s; !blank(s) {
			d.lines[j].s = reindent(s, 0, 2*dir)
		}
	}
	return true
}

// nextSibling / prevSibling: the todo at the same level next to i, or -1
func (d *todoDoc) nextSibling(i int) int {
	j := d.subtreeEnd(i) + 1
	for j < len(d.lines) && blank(d.lines[j].s) {
		j++
	}
	if j < len(d.lines) && d.isTodo(j) && d.ind(j) == d.ind(i) && d.inArchive(j) == d.inArchive(i) {
		return j
	}
	return -1
}

func (d *todoDoc) prevSibling(i int) int {
	in := d.ind(i)
	for j := i - 1; j >= 0; j-- {
		s := d.lines[j].s
		if blank(s) || (indentOf(s) > in && !isHeading(s)) {
			continue
		}
		if d.isTodo(j) && indentOf(s) == in {
			return j
		}
		return -1
	}
	return -1
}

// moveDown swaps i (with its sub-todos) with the next todo at its level; returns i's new line
func (d *todoDoc) moveDown(i int) int {
	n := d.nextSibling(i)
	if n < 0 {
		return i
	}
	endA, endB := d.subtreeEnd(i), d.subtreeEnd(n)
	a := copyLines(d.lines[i : endA+1])
	mid := copyLines(d.lines[endA+1 : n])
	b := copyLines(d.lines[n : endB+1])
	out := append(append(append(copyLines(d.lines[:i]), b...), mid...), a...)
	d.lines = append(out, d.lines[endB+1:]...)
	return i + len(b) + len(mid)
}

func (d *todoDoc) moveUp(i int) int {
	p := d.prevSibling(i)
	if p < 0 {
		return i
	}
	d.moveDown(p)
	return p
}

// cyclePriority: none → low → medium → high → none (checkmate's @priority tag)
func (d *todoDoc) cyclePriority(i int) string {
	t := d.todoAt(i)
	cur := ""
	if m := priorityRe.FindStringSubmatch(t.text); m != nil {
		cur = strings.ToLower(strings.TrimSpace(m[1]))
	}
	next := ""
	for k, p := range priorities {
		if p == cur {
			next = priorities[(k+1)%len(priorities)]
		}
	}
	t.text = strings.TrimRight(priorityRe.ReplaceAllString(t.text, ""), " ")
	if next != "" {
		t.text += " @priority(" + next + ")"
	}
	d.setTodo(i, t)
	return next
}

var (
	dueRe  = regexp.MustCompile(`\s*@due\(([^)]*)\)`)
	tagsRe = regexp.MustCompile(`\s*@(priority|due)\([^)]*\)`) // the tags Notes draws itself
)

// splitTags: the todo's text without its @priority / @due tags, and their values
func splitTags(text string) (plain, prio, due string) {
	if m := priorityRe.FindStringSubmatch(text); m != nil {
		prio = strings.ToLower(strings.TrimSpace(m[1]))
	}
	if m := dueRe.FindStringSubmatch(text); m != nil {
		due = strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(tagsRe.ReplaceAllString(text, "")), prio, due
}

// takeTags: the text without the tags, and the tags as they were written (" @priority(high) @due(…)")
func takeTags(text string) (plain, tags string) {
	for _, t := range tagsRe.FindAllString(text, -1) {
		tags += " " + strings.TrimSpace(t)
	}
	return strings.TrimSpace(tagsRe.ReplaceAllString(text, "")), tags
}

// setDue writes @due(date) on i ("" removes it)
func (d *todoDoc) setDue(i int, date string) {
	t := d.todoAt(i)
	t.text = strings.TrimRight(dueRe.ReplaceAllString(t.text, ""), " ")
	if date != "" {
		t.text += " @due(" + date + ")"
	}
	d.setTodo(i, t)
}

func prioRank(s string) int {
	t, ok := parseTodo(s)
	if !ok {
		return 4
	}
	switch _, p, _ := splitTags(t.text); p {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	}
	return 3
}

// sortRun sorts sibling todos (ls starts at the first one) by priority, high first, keeping
// each one's sub-todos with it (and sorting those too); equal ones keep their order
func sortRun(ls []line) []line {
	if len(ls) == 0 {
		return ls
	}
	base := indentOf(ls[0].s)
	var blocks [][]line
	for _, l := range ls {
		if len(blocks) == 0 || (!blank(l.s) && indentOf(l.s) <= base) {
			blocks = append(blocks, []line{l})
		} else {
			blocks[len(blocks)-1] = append(blocks[len(blocks)-1], l)
		}
	}
	for i, b := range blocks {
		if len(b) > 1 {
			kids := b[1:]
			k := 0
			for k < len(kids) && blank(kids[k].s) {
				k++
			}
			blocks[i] = append(append([]line{b[0]}, kids[:k]...), sortRun(kids[k:])...)
		}
	}
	sort.SliceStable(blocks, func(a, b int) bool { return prioRank(blocks[a][0].s) < prioRank(blocks[b][0].s) })
	var out []line
	for _, b := range blocks {
		out = append(out, b...)
	}
	return out
}

// sortByPriority sorts every run of todos above the archive (each part between headings)
func (d *todoDoc) sortByPriority() bool {
	before := strings.Join(d.texts(), "\n")
	a := d.archiveStart()
	for i := 0; i < a; {
		if !d.isTodo(i) || d.parentOf(i) >= 0 {
			i++
			continue
		}
		end := i // the run goes on until a heading (or something less indented)
		for j := i + 1; j < a && !isHeading(d.lines[j].s); j++ {
			if s := d.lines[j].s; !blank(s) {
				if indentOf(s) < d.ind(i) {
					break
				}
				end = j
			}
		}
		sorted := sortRun(copyLines(d.lines[i : end+1]))
		copy(d.lines[i:end+1], sorted)
		i = end + 1
	}
	return strings.Join(d.texts(), "\n") != before
}

// takeOut removes i with its sub-todos and returns them indented from 0 (for another list)
func (d *todoDoc) takeOut(i int) []line {
	base := d.ind(i)
	ls := d.cut(i)
	for k := range ls {
		ls[k].s = reindent(ls[k].s, base, 0)
		ls[k].fold = false
	}
	return ls
}

// appendTodos puts todos at the end of the list (above the archive)
func (d *todoDoc) appendTodos(ls []line) {
	d.insert(d.mainEnd(), ls...)
}

// archiveDone moves finished todos (with all their sub-todos finished) under "## Archive",
// newest on top like checkmate does; returns how many moved
func (d *todoDoc) archiveDone() int {
	a := d.archiveStart()
	var moved []line
	n := 0
	for i := 0; i < a; {
		if d.isTodo(i) && d.parentOf(i) < 0 {
			end := d.subtreeEnd(i)
			all := true
			for j := i; j <= end; j++ {
				if t, ok := parseTodo(d.lines[j].s); ok && !t.done {
					all = false
				}
			}
			if all {
				moved = append(moved, d.remove(i, end)...)
				a -= end - i + 1
				n++
				continue
			}
		}
		i++
	}
	if n == 0 {
		return 0
	}
	// no double blank lines where todos left
	for i := 1; i < a && i < len(d.lines); {
		if blank(d.lines[i].s) && blank(d.lines[i-1].s) {
			d.remove(i, i)
			a--
			continue
		}
		i++
	}
	if a >= len(d.lines) {
		for len(d.lines) > 0 && blank(d.lines[len(d.lines)-1].s) {
			d.lines = d.lines[:len(d.lines)-1]
		}
		d.lines = append(d.lines, line{s: ""}, line{s: "## Archive"})
		a = len(d.lines) - 1
	}
	for i := range moved {
		moved[i].fold = false
	}
	d.insert(a+1, moved...)
	return n
}
