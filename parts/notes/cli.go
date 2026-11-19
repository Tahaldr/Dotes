package notes

// Commands for the shell, without opening the app:
//
//	dotes n add "buy milk" [-l LIST] [-d DUE] [-p PRIORITY]
//	dotes n today
//	dotes n lists

import (
	"fmt"
	"os"
	"strings"
)

func cliFail(msg string) {
	fmt.Fprintln(os.Stderr, "dotes: "+msg)
	os.Exit(1)
}

// findList: the todo list whose title is name (or starts with / contains it, any case)
func findList(name string) (docInfo, bool) {
	docs := listDocs()
	low := strings.ToLower(strings.TrimSpace(name))
	for _, pass := range []func(string) bool{
		func(t string) bool { return t == low },
		func(t string) bool { return strings.HasPrefix(t, low) },
		func(t string) bool { return strings.Contains(t, low) },
	} {
		for _, d := range docs {
			if d.kind == "todos" && pass(strings.ToLower(d.title)) {
				return d, true
			}
		}
	}
	return docInfo{}, false
}

func cliAdd(args []string) {
	var words []string
	list, due, prio := cfg.inbox, "", ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() string {
			if i+1 >= len(args) {
				cliFail(a + " needs a value")
			}
			i++
			return args[i]
		}
		switch a {
		case "-l", "--list":
			list = next()
		case "-d", "--due":
			due = next()
		case "-p", "--priority":
			prio = strings.ToLower(next())
		default:
			words = append(words, a)
		}
	}
	text := strings.TrimSpace(strings.Join(words, " "))
	if text == "" {
		cliFail(`what should the todo say?  dotes n add "buy milk" [-l list] [-d fri] [-p high]`)
	}
	if prio != "" && prio != "low" && prio != "medium" && prio != "high" {
		cliFail("priority is low, medium or high")
	}
	if due != "" {
		d, err := parseDueInput(due, today())
		if err != nil {
			cliFail(err.Error())
		}
		due = d
	}
	info, ok := findList(list)
	path := info.path
	if !ok { // a new list with that name
		p, err := createDoc("todos", list)
		if err != nil {
			cliFail("couldn't create " + list + ": " + err.Error())
		}
		path, info.title = p, list
	}
	d, err := loadDoc(path)
	if err != nil {
		cliFail(err.Error())
	}
	if prio != "" {
		text += " @priority(" + prio + ")"
	}
	if due != "" {
		text += " @due(" + due + ")"
	}
	d.appendTodos([]line{{s: "- [ ] " + text}})
	if err := d.save(); err != nil {
		cliFail("couldn't save: " + err.Error())
	}
	extra := ""
	if due != "" {
		l, _ := dueLabel(due, false, today())
		extra = " (due " + l + ")"
	}
	fmt.Printf("Added to %s%s.\n", info.title, extra)
}

// cliToday prints what the Today tab shows
func cliToday() {
	m := &model{}
	m.docs = listDocs()
	m.buildAgenda()
	if len(m.agenda) == 0 {
		fmt.Println("Nothing late or due this week.")
		return
	}
	group := -1
	for _, it := range m.agenda {
		if it.group != group {
			if group != -1 {
				fmt.Println()
			}
			group = it.group
			fmt.Println(agendaGroups[group])
		}
		l, _ := dueLabel(it.due, false, today())
		p := ""
		if it.prio != "" {
			p = "  ! " + it.prio
		}
		fmt.Printf("  [ ] %s  (%s, %s)%s\n", it.text, it.list, l, p)
	}
}

func cliLists() {
	for _, d := range listDocs() {
		if d.kind == "todos" {
			fmt.Printf("%-30s %d/%d done\n", d.title, d.done, d.total)
		}
	}
	for _, d := range listDocs() {
		if d.kind == "notes" {
			fmt.Printf("%-30s note, edited %s\n", d.title, ago(d.mod))
		}
	}
}
