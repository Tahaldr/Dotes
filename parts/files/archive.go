package files

// e extracts the archive under the cursor into a folder next to it; C packs the selection
// into an archive (zip by default, or 7z / tar.gz… when 7-Zip is installed).

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// archiveStem: "photos.tar.gz" → "photos"
func archiveStem(name string) string {
	low := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tar.xz", ".tar.bz2", ".tar.zst"} {
		if strings.HasSuffix(low, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func (m *model) askExtract() tea.Cmd {
	c := m.current()
	if c == nil || c.isDir || !archiveExt[strings.ToLower(filepath.Ext(c.name))] {
		m.setNote("e extracts zip, 7z, rar, tar and gz archives", true)
		return nil
	}
	dest := freeName(m.cwd, archiveStem(c.name))
	m.pendingHover = filepath.Base(dest)
	return m.startTask(&task{kind: taskExtract, items: []item{{c.path, dest}}, undoOK: true})
}

func (m *model) askCompress() {
	t := m.targets()
	if len(t) == 0 {
		return
	}
	name := filepath.Base(m.cwd) + ".zip"
	if len(t) == 1 {
		name = archiveStem(filepath.Base(t[0])) + ".zip"
	}
	m.input("Compress "+nameList(t), "ends with .zip, or .7z / .tar.gz with 7-Zip", name, len([]rune(archiveStem(name))), func(m *model, s string) tea.Cmd {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		dest := filepath.Join(m.cwd, s)
		if _, err := os.Lstat(dest); err == nil {
			m.setNote(s+" already exists", true)
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(s), ".zip") {
			if _, err := exec.LookPath("7z"); err != nil {
				m.setNote("only .zip works without 7-Zip (scoop install 7zip)", true)
				return nil
			}
		}
		m.clearSel()
		m.pendingHover = s
		return m.startTask(&task{kind: taskCompress, items: itemsOf(t), archive: dest, undoOK: true})
	})
}

func extract(t *task, src, dest string) error {
	if strings.EqualFold(filepath.Ext(src), ".zip") || strings.EqualFold(filepath.Ext(src), ".jar") {
		return unzip(t, src, dest)
	}
	t.total.Store(1)
	if err := run7z(t, "x", src, "-o"+dest, "-y"); err != nil {
		return err
	}
	t.done.Store(1)
	return nil
}

func unzip(t *task, src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("can't open %s: %v", filepath.Base(src), err)
	}
	defer r.Close()
	var total int64
	for _, f := range r.File {
		total += int64(f.UncompressedSize64)
	}
	t.total.Store(total)
	root := filepath.Clean(dest) + string(filepath.Separator)
	for _, f := range r.File {
		if t.cancel.Load() {
			return errCanceled
		}
		p := filepath.Join(dest, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(p+string(filepath.Separator), root) { // "../../evil": stay inside
			return fmt.Errorf("%s has a file that points outside the folder: stopped", filepath.Base(src))
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(p, 0o755)
			continue
		}
		t.setCurrent(f.Name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(p)
		if err != nil {
			in.Close()
			return fmt.Errorf("can't write %s: %v", f.Name, errText(err))
		}
		_, err = io.Copy(out, &progressReader{in, t})
		in.Close()
		out.Close()
		if err != nil {
			return err
		}
		os.Chtimes(p, f.Modified, f.Modified)
	}
	return nil
}

func compress(t *task, items []item, dest string) error {
	if !strings.HasSuffix(strings.ToLower(dest), ".zip") {
		t.total.Store(1)
		args := []string{"a", dest}
		for _, it := range items {
			args = append(args, it.src)
		}
		if err := run7z(t, args...); err != nil {
			return err
		}
		t.done.Store(1)
		return nil
	}
	var total int64
	for _, it := range items {
		total += treeSize(it.src)
	}
	t.total.Store(total)
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("can't create %s: %v", filepath.Base(dest), errText(err))
	}
	zw := zip.NewWriter(out)
	for _, it := range items {
		base := filepath.Dir(it.src)
		err := filepath.WalkDir(it.src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if t.cancel.Load() {
				return errCanceled
			}
			if p == dest {
				return nil // don't pack the archive into itself
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !d.IsDir() && !info.Mode().IsRegular() {
				return nil // links and devices stay out
			}
			rel, _ := filepath.Rel(base, p)
			h, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			h.Name = filepath.ToSlash(rel)
			if d.IsDir() {
				h.Name += "/"
				_, err = zw.CreateHeader(h)
				return err
			}
			h.Method = zip.Deflate
			w, err := zw.CreateHeader(h)
			if err != nil {
				return err
			}
			t.setCurrent(p)
			in, err := os.Open(p)
			if err != nil {
				return fmt.Errorf("can't read %s: %v", d.Name(), errText(err))
			}
			_, err = io.Copy(w, &progressReader{in, t})
			in.Close()
			return err
		})
		if err != nil {
			zw.Close()
			out.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

type progressReader struct {
	r io.Reader
	t *task
}

func (p *progressReader) Read(b []byte) (int, error) {
	if p.t.cancel.Load() {
		return 0, errCanceled
	}
	n, err := p.r.Read(b)
	p.t.done.Add(int64(n))
	return n, err
}

// run7z runs 7-Zip and stops it when the task is canceled
func run7z(t *task, args ...string) error {
	if _, err := exec.LookPath("7z"); err != nil {
		return fmt.Errorf("this needs 7-Zip: scoop install 7zip")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for !t.ended.Load() {
			if t.cancel.Load() {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	out, err := exec.CommandContext(ctx, "7z", append(args, "-bso0", "-bsp0")...).CombinedOutput()
	if t.cancel.Load() {
		return errCanceled
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if i := strings.LastIndex(msg, "ERROR:"); i >= 0 {
			msg = msg[i:]
		}
		return fmt.Errorf("7-Zip: %s", trunc(strings.ReplaceAll(msg, "\n", " "), 120))
	}
	return nil
}
