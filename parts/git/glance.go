package git

// Glance: a few lines about your repos for Home, from what's on this PC (git status) and
// what the Git part saved last time (CI runs). No network, so Home never waits.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// GlanceRepo is a repo on this PC that needs you.
type GlanceRepo struct {
	Name                 string // "Dotes"
	Dir                  string
	Dirty, Ahead, Behind int
}

// GlanceInfo: repos with changes or commits to push / pull, and CI that's failing now.
type GlanceInfo struct {
	Local   int // repos on this PC
	Repos   []GlanceRepo
	Failing []string // "release · TermFiles"
}

// Glance looks at the repos on this PC. It runs git in each one: call it in the background.
func Glance() GlanceInfo {
	c := loadConfig() // its own copy: the Git part may be running at the same time
	var cache reposCache
	loadCache("repos.json", &cache)
	byName := map[string]Repo{}
	for _, r := range cache.Repos {
		byName[strings.ToLower(r.NameWithOwner)] = r
	}
	local := scanLocal(c.lookIn)
	g := GlanceInfo{Local: len(local)}
	for nwo, dir := range local {
		r := byName[nwo]
		li := gitInfo(dir, r.Branch(), r.HeadOid())
		if li.dirty+li.ahead+li.behind > 0 {
			name := r.Name // as GitHub spells it
			if name == "" {
				name = shortRepo(nwo)
			}
			g.Repos = append(g.Repos, GlanceRepo{Name: name, Dir: dir, Dirty: li.dirty, Ahead: li.ahead, Behind: li.behind})
		}
	}
	sort.Slice(g.Repos, func(i, j int) bool {
		a, b := g.Repos[i], g.Repos[j]
		if a.Dirty+a.Ahead != b.Dirty+b.Ahead {
			return a.Dirty+a.Ahead > b.Dirty+b.Ahead
		}
		return a.Name < b.Name
	})
	var runs map[string][]Run
	loadCache("runs.json", &runs)
	var all []Run
	for nwo, rs := range runs {
		for _, r := range rs {
			r.Repo = nwo
			all = append(all, r)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Created.After(all[j].Created) })
	seen := map[string]bool{}
	for _, r := range all {
		k := fmt.Sprintf("%s|%d|%s", r.Repo, r.WorkflowID, r.Branch)
		if seen[k] {
			continue
		}
		seen[k] = true
		if r.failed() && time.Since(r.Created) < 30*24*time.Hour {
			g.Failing = append(g.Failing, r.Name+" · "+shortRepo(r.Repo))
		}
	}
	return g
}
