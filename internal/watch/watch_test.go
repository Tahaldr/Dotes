package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatch(t *testing.T) {
	d := t.TempDir()
	w := New("t")
	if w == nil {
		t.Skip("no watching here")
	}
	defer w.Close()
	w.Skip(func(p string) bool { return filepath.Ext(p) == ".lock" })
	if !w.Set(d) || w.Watching() != 1 {
		t.Fatal("couldn't watch")
	}
	got := make(chan Msg, 1)
	go func() { got <- w.Next()().(Msg) }()
	os.WriteFile(filepath.Join(d, "x.lock"), []byte("1"), 0o644)
	for i := 0; i < 5; i++ { // quick saves come as one change
		os.WriteFile(filepath.Join(d, "a.txt"), []byte{byte(i)}, 0o644)
	}
	select {
	case m := <-got:
		if m.Key != "t" || len(m.Paths) != 1 || filepath.Base(m.Paths[0]) != "a.txt" {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no change seen")
	}
	w.Set() // nothing
	if w.Watching() != 0 {
		t.Fatal("still watching")
	}
}
