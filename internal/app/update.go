// Updates: `dotes update` downloads the newest release for this system and replaces itself;
// inside, Dotes checks at most once a day and shows "⬆ version" in the top bar.
// Settings live in the config folder, so updating never touches them.
package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const repoSlug = "Tahaldr/Dotes"

type ghRelease struct {
	Tag    string `json:"tag_name"`
	URL    string `json:"html_url"`
	Assets []struct {
		Name string `json:"name"`
		API  string `json:"url"` // works for private repos too
	} `json:"assets"`
}

// ghToken: the GitHub CLI's login, if any (needed while a repo is private)
func ghToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func ghGet(url, accept string, timeout time.Duration) (*http.Response, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "dotes/"+Version)
	if t := ghToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	return resp, nil
}
func latestRelease() (*ghRelease, error) {
	resp, err := ghGet("https://api.github.com/repos/"+repoSlug+"/releases/latest", "application/vnd.github+json", 15*time.Second)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r ghRelease
	return &r, json.NewDecoder(resp.Body).Decode(&r)
}

// newer: is version a newer than b? ("v1.1.0" > "1.0.9")
func Newer(a, b string) bool {
	pa, pb := verParts(a), verParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func verParts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

func assetName() string {
	n := "dotes_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		n += ".exe"
	}
	return n
}

func updateFail(err error) {
	fmt.Fprintln(os.Stderr, "dotes update: "+err.Error())
	os.Exit(1)
}

// SelfUpdate: dotes update [--check]
func SelfUpdate(checkOnly bool) {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old") // left by the last update (Windows can't delete a running program)
	}
	fmt.Println("Looking for a newer Dotes…")
	rel, err := latestRelease()
	if err != nil {
		updateFail(fmt.Errorf("couldn't reach GitHub: %w", err))
	}
	saveUpdateCheck(rel.Tag)
	if !Newer(rel.Tag, Version) {
		fmt.Printf("You have the newest Version (%s).\n", Version)
		return
	}
	fmt.Printf("Dotes %s is out (you have %s).\n  What's new: %s\n", strings.TrimPrefix(rel.Tag, "v"), Version, rel.URL)
	if checkOnly {
		fmt.Println("Run  dotes update  to get it.")
		return
	}
	url := ""
	for _, a := range rel.Assets {
		if a.Name == assetName() {
			url = a.API
		}
	}
	if url == "" {
		updateFail(fmt.Errorf("this release has no program for %s/%s yet; see %s", runtime.GOOS, runtime.GOARCH, rel.URL))
	}
	exe, err := os.Executable()
	if err != nil {
		updateFail(err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	fmt.Println("Downloading " + assetName() + "…")
	tmp := exe + ".new"
	if err := download(url, tmp); err != nil {
		os.Remove(tmp)
		updateFail(fmt.Errorf("download failed: %w", err))
	}
	os.Chmod(tmp, 0o755)
	// the running program is moved aside (Windows allows renaming it), the new one takes its place
	os.Remove(exe + ".old")
	if err := os.Rename(exe, exe+".old"); err != nil {
		os.Remove(tmp)
		updateFail(fmt.Errorf("couldn't replace %s: %w", exe, err))
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Rename(exe+".old", exe)
		updateFail(fmt.Errorf("couldn't put the new Version in place: %w", err))
	}
	if runtime.GOOS != "windows" {
		os.Remove(exe + ".old")
	}
	fmt.Printf("Updated to %s. Your settings weren't touched.\n", strings.TrimPrefix(rel.Tag, "v"))
}

func download(url, dest string) error {
	resp, err := ghGet(url, "application/octet-stream", 5*time.Minute)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, resp.Body)
	f.Close()
	if err == nil && n < 1<<20 {
		err = fmt.Errorf("the file is too small (%d bytes)", n)
	}
	return err
}

// ---- the daily check ----

type updateCheck struct {
	At     time.Time `json:"at"`
	Latest string    `json:"latest"`
}

// UpdateMsg says a release was found (Latest may not be newer).
type UpdateMsg struct{ Latest string }

func updateFile() string {
	d, _ := os.UserCacheDir()
	return filepath.Join(d, "dotes", "update.json")
}

func saveUpdateCheck(tag string) {
	b, _ := json.Marshal(updateCheck{At: time.Now(), Latest: tag})
	os.MkdirAll(filepath.Dir(updateFile()), 0o755)
	os.WriteFile(updateFile(), b, 0o644)
}

// NewerSaved: a newer version found by an earlier check ("" = none)
func NewerSaved() string {
	var uc updateCheck
	if b, err := os.ReadFile(updateFile()); err == nil && json.Unmarshal(b, &uc) == nil && Newer(uc.Latest, Version) {
		return uc.Latest
	}
	return ""
}

// CheckUpdate: at most once a day, in the background
func CheckUpdate() tea.Cmd {
	var uc updateCheck
	if b, err := os.ReadFile(updateFile()); err == nil && json.Unmarshal(b, &uc) == nil && time.Since(uc.At) < 24*time.Hour {
		return nil
	}
	return func() tea.Msg {
		rel, err := latestRelease()
		if err != nil {
			return nil
		}
		saveUpdateCheck(rel.Tag)
		return UpdateMsg{rel.Tag}
	}
}
