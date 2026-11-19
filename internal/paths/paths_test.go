package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFind(t *testing.T) {
	got := []string{}
	for _, s := range Find(`read ~/Documents/a.md, then (C:\Users\x\b.txt). not a~/b or http://x C:/y/z.`) {
		got = append(got, s.Path)
	}
	want := []string{"~/Documents/a.md", `C:\Users\x\b.txt`, "C:/y/z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSuggest(t *testing.T) {
	d := t.TempDir()
	os.Mkdir(filepath.Join(d, "Coding"), 0o755)
	os.Mkdir(filepath.Join(d, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(d, "code.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(d, "notes.md"), nil, 0o644)
	base := filepath.ToSlash(d) + "/"
	got := Suggest(base+"co", 0)
	want := []string{base + "Coding/", base + "code.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	if n := len(Suggest(base, 0)); n != 3 {
		t.Fatalf("hidden shown: %d", n)
	}
	if got := Suggest(base+".h", 0); len(got) != 1 {
		t.Fatalf("dot: %q", got)
	}
}

func TestWord(t *testing.T) {
	text := []rune("see ~/Doc now")
	if s, w, ok := Word(text, 9); !ok || s != 4 || w != "~/Doc" {
		t.Fatalf("%d %q %v", s, w, ok)
	}
	if _, _, ok := Word(text, 2); ok {
		t.Fatal("not a path")
	}
}
