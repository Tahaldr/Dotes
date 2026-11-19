package git

// Demo mode (dotes demo): GitHub answers come from here, so screenshots show a made-up
// account (demo-dev) instead of yours, and nothing goes over the network.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Tahaldr/dotes/internal/app"
	"github.com/cli/go-gh/v2/pkg/api"
)

const demoLogin = "demo-dev"

func restClient() (*api.RESTClient, error) {
	if app.Demo {
		return api.NewRESTClient(api.ClientOptions{Host: "github.com", AuthToken: "demo", Transport: demoTransport{}})
	}
	return api.DefaultRESTClient()
}

func gqlClient() (*api.GraphQLClient, error) {
	if app.Demo {
		return api.NewGraphQLClient(api.ClientOptions{Host: "github.com", AuthToken: "demo", Transport: demoTransport{}})
	}
	return api.DefaultGraphQLClient()
}

type demoTransport struct{}

func (demoTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var body any
	if strings.HasSuffix(r.URL.Path, "/graphql") {
		var q struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&q)
		}
		body = map[string]any{"data": demoGraphQL(q.Query, q.Variables)}
	} else {
		body = demoREST(strings.TrimPrefix(r.URL.Path, "/api/v3"), r.URL.Path)
	}
	b, _ := json.Marshal(body)
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(bytes.NewReader(b)), Request: r}, nil
}

func demoAgo(d time.Duration) string { return time.Now().Add(-d).UTC().Format(time.RFC3339) }

type demoRepo struct {
	nwo, desc, lang, color string
	private                bool
	stars                  int
	pushed                 time.Duration
}

var demoRepos = []demoRepo{
	{"demo-dev/shop-api", "REST API for the shop: orders, payments, stock", "Go", "#00ADD8", false, 48, 20 * time.Minute},
	{"demo-dev/portfolio", "My website, built with Astro", "TypeScript", "#3178c6", false, 12, 3 * time.Hour},
	{"acme/mobile-app", "The Acme app for iOS and Android", "Dart", "#00B4AB", true, 0, 26 * time.Hour},
	{"demo-dev/dotfiles", "Nvim, terminal and shell settings", "Lua", "#000080", false, 31, 2 * 24 * time.Hour},
	{"demo-dev/weather-cli", "The weather in your terminal", "Rust", "#dea584", false, 210, 5 * 24 * time.Hour},
	{"demo-dev/notes-sync", "Sync Markdown notes between PCs", "Python", "#3572A5", true, 0, 9 * 24 * time.Hour},
	{"demo-dev/blog", "Posts about terminals and Go", "MDX", "#fcb32c", false, 7, 21 * 24 * time.Hour},
}

func demoGraphQL(q string, vars map[string]any) any {
	switch {
	case strings.Contains(q, "contributionsCollection"):
		var weeks []any
		total := 0
		start := time.Now().AddDate(0, 0, -7*52)
		for w := 0; w < 53; w++ {
			var days []any
			for d := 0; d < 7; d++ {
				day := start.AddDate(0, 0, w*7+d)
				if day.After(time.Now()) {
					break
				}
				n := (w*7 + d*3 + w*w) % 11 // a made-up but lively year
				if d == 0 || d == 6 {
					n /= 3
				}
				if n < 3 {
					n = 0
				}
				total += n
				days = append(days, map[string]any{"date": day.Format("2006-01-02"), "contributionCount": n})
			}
			weeks = append(weeks, map[string]any{"contributionDays": days})
		}
		return map[string]any{"viewer": map[string]any{"contributionsCollection": map[string]any{
			"contributionCalendar": map[string]any{"totalContributions": total, "weeks": weeks}}}}
	case strings.Contains(q, "repositories(first: 100"):
		var nodes []any
		for i, r := range demoRepos {
			nodes = append(nodes, map[string]any{"id": fmt.Sprint("R", i), "nameWithOwner": r.nwo, "name": r.nwo[strings.Index(r.nwo, "/")+1:],
				"description": r.desc, "isPrivate": r.private, "stargazerCount": r.stars, "forkCount": r.stars / 6,
				"pushedAt": demoAgo(r.pushed), "url": "https://github.com/" + r.nwo,
				"defaultBranchRef": map[string]any{"name": "main", "target": map[string]any{"oid": "0000000000000000000000000000000000000000"}},
				"primaryLanguage":  map[string]any{"name": r.lang, "color": r.color}})
		}
		return map[string]any{"viewer": map[string]any{"login": demoLogin, "repositories": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false}, "nodes": nodes}}}
	case strings.Contains(q, "type: ISSUE"):
		s, _ := vars["q"].(string)
		pr := func(n int, title, repo, head string, add, del int, check, review string, age time.Duration) any {
			return map[string]any{"id": fmt.Sprint("PR", n), "number": n, "title": title, "state": "OPEN", "url": "https://github.com/" + repo + "/pull/" + fmt.Sprint(n),
				"createdAt": demoAgo(age), "updatedAt": demoAgo(age / 3), "author": map[string]any{"login": demoLogin},
				"repository": map[string]any{"nameWithOwner": repo}, "headRefName": head, "baseRefName": "main",
				"additions": add, "deletions": del, "changedFiles": 4, "mergeable": "MERGEABLE", "reviewDecision": review,
				"comments": map[string]any{"totalCount": 2},
				"labels":   map[string]any{"nodes": []any{map[string]any{"name": "feature", "color": "a2eeef"}}},
				"last":     map[string]any{"nodes": []any{map[string]any{"commit": map[string]any{"statusCheckRollup": map[string]any{"state": check}}}}}}
		}
		var nodes []any
		switch {
		case strings.Contains(s, "review-requested"):
			nodes = []any{pr(41, "Offline mode for the cart", "acme/mobile-app", "offline-cart", 312, 40, "SUCCESS", "REVIEW_REQUIRED", 5*time.Hour)}
		case strings.Contains(s, "is:pr"):
			nodes = []any{pr(87, "Refunds: partial amounts", "demo-dev/shop-api", "partial-refunds", 184, 22, "FAILURE", "CHANGES_REQUESTED", 26*time.Hour),
				pr(12, "Dark mode", "demo-dev/portfolio", "dark-mode", 96, 11, "SUCCESS", "APPROVED", 3*24*time.Hour)}
		case strings.Contains(s, "is:issue"):
			nodes = []any{
				map[string]any{"id": "I1", "number": 93, "title": "Stock goes below zero on double click", "state": "OPEN", "createdAt": demoAgo(4 * time.Hour), "updatedAt": demoAgo(time.Hour),
					"author": map[string]any{"login": "sara-k"}, "repository": map[string]any{"nameWithOwner": "demo-dev/shop-api"}, "comments": map[string]any{"totalCount": 3},
					"labels": map[string]any{"nodes": []any{map[string]any{"name": "bug", "color": "d73a4a"}}}},
				map[string]any{"id": "I2", "number": 18, "title": "Show the wind direction", "state": "OPEN", "createdAt": demoAgo(50 * time.Hour), "updatedAt": demoAgo(20 * time.Hour),
					"author": map[string]any{"login": "lee"}, "repository": map[string]any{"nameWithOwner": "demo-dev/weather-cli"}, "comments": map[string]any{"totalCount": 1},
					"labels": map[string]any{"nodes": []any{map[string]any{"name": "idea", "color": "0e8a16"}}}},
			}
		}
		return map[string]any{"search": map[string]any{"nodes": nodes}}
	}
	return map[string]any{}
}

const demoReadme = "# shop-api\n\nThe REST API behind the shop: orders, payments and stock.\n\n" +
	"## Run it\n\n```sh\ngo run ./cmd/api\n```\n\n## Endpoints\n\n| Method | Path | What |\n|---|---|---|\n" +
	"| GET | /orders | your orders |\n| POST | /orders | place one |\n| POST | /refunds | money back |\n"

func demoREST(p, full string) any {
	p = strings.TrimPrefix(p, "/")
	switch {
	case strings.HasPrefix(p, "users/") && strings.HasSuffix(p, "/events"):
		ev := func(t, repo string, age time.Duration, payload map[string]any) any {
			return map[string]any{"type": t, "created_at": demoAgo(age), "repo": map[string]any{"name": repo}, "payload": payload}
		}
		commit := func(msg string) any { return map[string]any{"message": msg} }
		return []any{
			ev("PushEvent", "demo-dev/shop-api", 20*time.Minute, map[string]any{"ref": "refs/heads/main", "size": 2, "commits": []any{commit("Orders: pay later"), commit("Fix the stock count")}}),
			ev("PullRequestEvent", "demo-dev/shop-api", 26*time.Hour, map[string]any{"action": "opened", "pull_request": map[string]any{"number": 87, "title": "Refunds: partial amounts"}}),
			ev("PushEvent", "demo-dev/portfolio", 3*time.Hour, map[string]any{"ref": "refs/heads/dark-mode", "size": 1, "commits": []any{commit("Dark mode colors")}}),
			ev("IssuesEvent", "demo-dev/weather-cli", 2*24*time.Hour, map[string]any{"action": "closed", "issue": map[string]any{"number": 15, "title": "Crash with no internet"}}),
			ev("ReleaseEvent", "demo-dev/weather-cli", 5*24*time.Hour, map[string]any{"action": "published", "release": map[string]any{"tag_name": "v1.4.0"}}),
			ev("CreateEvent", "demo-dev/blog", 21*24*time.Hour, map[string]any{"ref_type": "repository"}),
		}
	case strings.HasPrefix(p, "notifications"):
		n := func(id, title, typ, repo, reason string, age time.Duration) any {
			return map[string]any{"id": id, "unread": true, "reason": reason, "updated_at": demoAgo(age),
				"subject":    map[string]any{"title": title, "type": typ, "url": "https://api.github.com/repos/" + repo + "/pulls/41"},
				"repository": map[string]any{"full_name": repo, "html_url": "https://github.com/" + repo}}
		}
		return []any{
			n("1", "Offline mode for the cart", "PullRequest", "acme/mobile-app", "review_requested", 5*time.Hour),
			n("2", "Stock goes below zero on double click", "Issue", "demo-dev/shop-api", "mention", 4*time.Hour),
		}
	case strings.Contains(p, "/actions/runs") && !strings.Contains(p, "/jobs"):
		nwo := strings.Join(strings.SplitN(strings.TrimPrefix(p, "repos/"), "/", 3)[:2], "/")
		run := func(id int, name, title, concl string, age time.Duration) any {
			return map[string]any{"id": id, "name": name, "display_title": title, "run_number": id % 100, "run_attempt": 1, "event": "push",
				"status": "completed", "conclusion": concl, "head_branch": "main", "head_sha": "abc1234",
				"html_url": "https://github.com/" + nwo + "/actions", "created_at": demoAgo(age), "updated_at": demoAgo(age - time.Minute),
				"run_started_at": demoAgo(age), "workflow_id": id / 100, "actor": map[string]any{"login": demoLogin}}
		}
		if nwo != "demo-dev/shop-api" {
			return map[string]any{"workflow_runs": []any{run(301, "test", "Update deps", "success", 3*time.Hour)}}
		}
		return map[string]any{"workflow_runs": []any{
			run(512, "test", "Orders: pay later", "failure", 20*time.Minute),
			run(611, "deploy", "Fix the stock count", "success", 2*time.Hour),
			run(511, "test", "Fix the stock count", "success", 2*time.Hour),
		}}
	case strings.HasSuffix(p, "/readme"):
		return map[string]any{"content": base64.StdEncoding.EncodeToString([]byte(demoReadme)), "encoding": "base64"}
	case strings.Contains(p, "/git/trees/"):
		var tree []any
		for _, f := range []string{"cmd", "cmd/api", "cmd/api/main.go", "orders", "orders/orders.go", "orders/refunds.go", "stock", "stock/stock.go", ".gitignore", "go.mod", "README.md"} {
			typ := "blob"
			if !strings.Contains(f, ".") {
				typ = "tree"
			}
			tree = append(tree, map[string]any{"path": f, "type": typ, "sha": "0", "size": 1200})
		}
		return map[string]any{"tree": tree}
	case strings.HasSuffix(p, "/releases"), strings.HasSuffix(p, "/labels"), strings.HasSuffix(p, "/files"):
		return []any{}
	}
	return map[string]any{}
}

// SeedDemo saves the demo account's repos and CI runs, as the Git part would after a visit,
// so Home's Git panel has something to show before Git is opened.
func SeedDemo() {
	var rr struct {
		Viewer struct {
			Login        string
			Repositories struct{ Nodes []Repo }
		}
	}
	b, _ := json.Marshal(demoGraphQL("repositories(first: 100", nil))
	json.Unmarshal(b, &rr)
	saveCache("repos.json", reposCache{Login: rr.Viewer.Login, Repos: rr.Viewer.Repositories.Nodes, At: time.Now()})
	runs := map[string][]Run{}
	for _, r := range demoRepos[:2] {
		var out struct {
			Runs []Run `json:"workflow_runs"`
		}
		b, _ := json.Marshal(demoREST("repos/"+r.nwo+"/actions/runs", ""))
		json.Unmarshal(b, &out)
		runs[r.nwo] = out.Runs
	}
	saveCache("runs.json", runs)
}
