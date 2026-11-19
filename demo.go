package main

// dotes demo: Dotes with made-up data, for screenshots. Everything lives in a temp folder
// (dotes-demo): its own settings, a sample notebook, three projects (one with changes to
// commit) and a fake GitHub account (demo-dev). Your own settings, notes and repos are
// never touched.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tahaldr/dotes/internal/app"
	git2 "github.com/Tahaldr/dotes/parts/git"
)

// setupDemo builds the demo folder and points Dotes at it; it returns the folder to start in.
func setupDemo() string {
	root := filepath.Join(os.TempDir(), "dotes-demo")
	os.RemoveAll(root) // fresh every time (it's only ever made by this function)
	cfg := filepath.Join(root, "config")
	nb := filepath.Join(root, "Notebook")
	proj := filepath.Join(root, "projects")
	for _, d := range []string{cfg, filepath.Join(nb, "todos"), filepath.Join(nb, "notes"), proj} {
		os.MkdirAll(d, 0o755)
	}
	os.Setenv("DOTES_CONFIG", cfg)
	os.Setenv("DOTES_DEMO", "1")
	app.Demo = true
	home, _ := os.UserHomeDir()
	tilde := func(p string) string { // ~/... in the notes (no user name in screenshots)
		if r, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(r, "..") {
			return "~/" + filepath.ToSlash(r)
		}
		return filepath.ToSlash(p)
	}

	day := func(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }
	write := func(p, s string) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(strings.TrimLeft(s, "\n")), 0o644)
	}

	// settings
	write(filepath.Join(cfg, "dotes.txt"), "parts = home, files, notes, git\nhints = on\ntabbar = on\n")
	write(filepath.Join(cfg, "home.txt"), `
logo = home-logo.txt
recent = off
prayer = Casablanca | Morocco | 21
app = Nvim | editor | n | f36f | nvim .
app = VS Code | code | v | f0a1e | code .
app = Browser | firefox | b | f0239 | firefox
app = Music | spotify | m | f04c7 | spotify
app = Docker | lazydocker | d | f0868 | lazydocker
project_roots = `+proj+`
project_open_with = Nvim, VS Code, Shell
project_search = `+proj+`
`)
	write(filepath.Join(cfg, "notes.txt"), "notebook = "+nb+"\ninbox = Inbox\n")
	write(filepath.Join(cfg, "git.txt"), "clone_into = "+proj+"\nlook_in = "+proj+"\n")
	write(filepath.Join(cfg, "files-bookmarks.txt"), proj+"\n"+nb+"\n")

	// the notebook
	shop := filepath.Join(proj, "shop-api")
	write(filepath.Join(nb, "todos", "Shop launch.md"), fmt.Sprintf(`
# Shop launch

- [ ] Fix refunds with partial amounts @priority(high) @due(%s)
  - [x] Write the failing test
  - [ ] Round to cents, not dollars
- [ ] Load test the checkout @priority(medium) @due(%s)
- [ ] Read the notes in %s/README.md
- [x] Stripe keys in the vault
- [x] Order emails
- [ ] Launch post for the blog @due(%s)
`, day(-1), day(0), tilde(shop), day(4)))
	write(filepath.Join(nb, "todos", "Groceries.md"), `
# Groceries

- [ ] Milk
- [ ] Bread @due(`+day(1)+`)
- [x] Coffee
- [ ] Olive oil
`)
	write(filepath.Join(nb, "todos", "Inbox.md"), `
# Inbox

- [ ] Call the bank @due(`+day(2)+`)
- [ ] Renew the domain @priority(high) @due(`+day(6)+`)
- [ ] Try the new Nvim theme
`)
	write(filepath.Join(nb, "notes", "Ideas.md"), `
# Ideas

- A **weather** widget for the terminal (see weather-cli)
- Dark mode for the portfolio
- Sync notes between PCs with *git*

> Small tools, used every day.
`)
	write(filepath.Join(nb, "notes", "Meeting - Nov 18.md"), `
# Meeting - Nov 18

**Who:** Sara, Lee, me

## Decided

1. Launch on Friday
2. Refunds first, then the blog post
3. Load test before launch

## Next

- [ ] Sara: payment emails
- [ ] Lee: the mobile cart

Code: `+tilde(shop)+`
`)

	// projects: git repos with a GitHub remote; shop-api has work to commit
	git := func(dir string, args ...string) {
		c := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=demo-dev", "-c", "user.email=demo@example.com",
			"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...)
		c.Run()
	}
	repo := func(name string, files map[string]string) string {
		dir := filepath.Join(proj, name)
		for f, s := range files {
			write(filepath.Join(dir, filepath.FromSlash(f)), s)
		}
		git(dir, "init", "-q")
		git(dir, "remote", "add", "origin", "https://github.com/demo-dev/"+name+".git")
		git(dir, "add", "-A")
		git(dir, "commit", "-q", "-m", "First version")
		return dir
	}
	repo("shop-api", map[string]string{
		"README.md":           "# shop-api\n\nThe REST API behind the shop: orders, payments and stock.\n",
		"go.mod":              "module example.com/shop-api\n\ngo 1.23\n",
		"cmd/api/main.go":     "package main\n\nimport \"example.com/shop-api/orders\"\n\nfunc main() {\n\torders.Serve(\":8080\")\n}\n",
		"orders/orders.go":    "package orders\n\n// Serve starts the orders API.\nfunc Serve(addr string) {}\n",
		"orders/refunds.go":   "package orders\n\n// Refund gives the money back.\nfunc Refund(id string, cents int) error {\n\treturn nil\n}\n",
		"stock/stock.go":      "package stock\n\n// Take removes n from the stock.\nfunc Take(sku string, n int) {}\n",
		".gitignore":          "bin/\n",
		"docs/screenshot.png": "",
	})
	write(filepath.Join(shop, "orders", "refunds.go"), "package orders\n\nimport \"errors\"\n\n// Refund gives the money back: all of it, or part of it (in cents).\nfunc Refund(id string, cents int) error {\n\tif cents <= 0 {\n\t\treturn errors.New(\"nothing to refund\")\n\t}\n\treturn nil\n}\n")
	write(filepath.Join(shop, "orders", "refunds_test.go"), "package orders\n\nimport \"testing\"\n\nfunc TestPartialRefund(t *testing.T) {\n\tif err := Refund(\"o1\", 250); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n")
	write(filepath.Join(shop, "README.md"), "# shop-api\n\nThe REST API behind the shop: orders, payments, refunds and stock.\n")
	repo("portfolio", map[string]string{
		"package.json":          "{\n  \"name\": \"portfolio\",\n  \"scripts\": { \"dev\": \"astro dev\" }\n}\n",
		"README.md":             "# portfolio\n\nMy website.\n",
		"src/pages/index.astro": "---\nconst title = 'Hi, I build small tools'\n---\n<h1>{title}</h1>\n",
	})
	repo("weather-cli", map[string]string{
		"Cargo.toml":  "[package]\nname = \"weather-cli\"\nversion = \"1.4.0\"\n",
		"README.md":   "# weather-cli\n\nThe weather in your terminal.\n",
		"src/main.rs": "fn main() {\n    println!(\"☀ 24°C Casablanca\");\n}\n",
	})
	git2.SeedDemo()
	return shop
}
