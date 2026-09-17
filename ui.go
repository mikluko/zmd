package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// One list, holding the sessions there are and the repositories a session can
// be started in. Typing is the navigation: a query narrows both kinds at
// once, and the kind of the row it is pressed on is what decides what a key
// does. The kind travels in a field of its own so the preview can branch on
// it without reading the row back out of what is displayed.
type kind string

const (
	kindSession kind = "session"
	kindRepo    kind = "repo"
)

// A session with a client attached is filled, one with none is hollow. A
// repository carries one circle per session in it, so a place with two
// sessions is legible before its rows are read.
const (
	attached = "●"
	detached = "○"
)

// repoMark separates the two kinds at a glance, since both stand in one list.
const repoMark = "[repo] "

// header is what the keys do.
const header = "⏎ attach   ⌥⏎ label   ⌃n name it   ⇥ mark   ⌦ kill"

// row is one line of the list.
type row struct {
	kind    kind
	session string   // the session to attach to, empty where one has to be made
	dir     string   // where the row stands
	several bool     // more sessions here than one keystroke can choose between
	cells   []string // what the line shows
}

// circles renders the sessions in one directory.
func circles(found []Session) string {
	var b strings.Builder
	for _, s := range found {
		if s.Clients != "0" && s.Clients != "" {
			b.WriteString(attached)
			continue
		}
		b.WriteString(detached)
	}
	return b.String()
}

// byDir groups sessions by the directory they run in, each group by name.
//
// Grouping by directory rather than by name is what makes a session this tool
// never named, supacode's among them, count against the directory it sits in.
func byDir(live []Session) map[string][]Session {
	found := make(map[string][]Session)
	for _, s := range live {
		if s.Dir == "" {
			continue
		}
		key := filepath.Clean(s.Dir)
		found[key] = append(found[key], s)
	}
	for key := range found {
		sort.Slice(found[key], func(i, j int) bool { return found[key][i].Name < found[key][j].Name })
	}
	return found
}

// sessionRows returns one row per running session, in name order.
//
// The path is the last cell, and align leaves the last cell ragged, so a
// session row ends with the directory it runs in. narrow depends on that.
func sessionRows(live []Session) []row {
	sorted := append([]Session(nil), live...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	rows := make([]row, len(sorted))
	for i, s := range sorted {
		rows[i] = row{
			kind:    kindSession,
			session: s.Name,
			dir:     s.Dir,
			cells:   []string{circles([]Session{s}), s.Name, shorten(s.Dir)},
		}
	}
	return rows
}

// repoRows returns one row per repository, marked with what runs in it.
//
// The name cell holds the basename, so the path cell holds the parent: the
// two together are the path, written once. A query still reaches either,
// since fzf searches the line rather than the cell.
func repoRows(dirs []string, live []Session) []row {
	here := byDir(live)

	rows := make([]row, len(dirs))
	for i, dir := range dirs {
		in := here[filepath.Clean(dir)]
		r := row{kind: kindRepo, dir: dir, several: len(in) > 1}
		if len(in) == 1 {
			r.session = in[0].Name
		}
		r.cells = []string{circles(in), repoMark + filepath.Base(dir), shorten(filepath.Dir(dir))}
		rows[i] = r
	}
	return rows
}

// list is the whole picker: every session, then every repository.
//
// Sessions lead because the cursor starts on the row closest to where the
// pane stands and the closest is decided by path length alone: with the
// session first, a directory that has one answers with the session rather
// than with the offer to start a second.
func list(live []Session, repos []string) []row {
	return append(sessionRows(live), repoRows(repos, live)...)
}

// lines renders rows for fzf: the index, the kind, what the preview reads,
// and what is shown. Only the last is searched, and the index is what comes
// back.
func lines(rows []row) []string {
	cells := make([][]string, len(rows))
	for i, r := range rows {
		cells[i] = r.cells
	}
	display := align(cells)

	out := make([]string, len(rows))
	for i, r := range rows {
		target := r.session
		if r.kind == kindRepo {
			target = r.dir
		}
		out[i] = strconv.Itoa(i) + "\t" + string(r.kind) + "\t" + target + "\t" + display[i]
	}
	return out
}

// narrow is the query that leaves only the sessions running in dir.
//
// A session row ends with the path it runs in and fzf's `$` anchors a term to
// the end of the line, so the repository's own row and the worktrees below it
// both fall out. Answering an ambiguity with a query rather than with a
// second list is what keeps the picker one list.
func narrow(dir string) string {
	return shorten(dir) + "$"
}

// named returns the session name a typed query stands for, or "" where the
// query names nothing.
func named(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	return sessionSafe(query)
}

// at returns the row whose directory holds dir most closely, or -1.
//
// The longest directory that is a prefix wins, so a worktree is answered with
// itself rather than with the repository above it.
func at(rows []row, dir string) int {
	if dir == "" {
		return -1
	}
	dir = filepath.Clean(dir)
	best, length := -1, 0
	for i, r := range rows {
		if r.dir == "" {
			continue
		}
		base := filepath.Clean(r.dir)
		if dir != base && !strings.HasPrefix(dir, base+string(filepath.Separator)) {
			continue
		}
		if len(base) > length {
			best, length = i, len(base)
		}
	}
	return best
}

// winsize is what TIOCGWINSZ fills in.
type winsize struct{ rows, cols, xpixel, ypixel uint16 }

// termSize returns the terminal's columns and rows, or zeroes where there is
// no terminal to ask.
func termSize() (cols, rows int) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return 0, 0
	}
	defer tty.Close()

	var ws winsize
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		tty.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	); errno != 0 {
		return 0, 0
	}
	return int(ws.cols), int(ws.rows)
}

// previewLayout returns the --preview-window for a terminal this size, or ""
// where a preview would leave too little of the list to read.
//
// The split follows the shape of the space rather than a preference: a wide
// terminal has room beside the list, a tall narrow one only below it. The
// alternative fzf carries is what answers a resize made while it is up.
func previewLayout(cols, rows int) string {
	switch {
	case cols < 80 || rows < 15:
		return ""
	case cols >= 120:
		return "right,55%,border-left,<100(up,50%,border-bottom)"
	default:
		return "up,50%,border-bottom"
	}
}

// preview branches on the kind, since one list holds both. It runs under
// `sh -c` rather than the login shell, which need not be one that parses it.
const preview = `if test {2} = session; then
	zmx history {3}
elif test {2} = repo; then
	git -C {3} log --color=always --graph -50 --format='%C(auto)%h %as%d %s'
fi`

// follow puts a session's preview at the end of its scrollback and leaves a
// repository's at the head of its history.
const follow = `focus:transform:test {2} = session ` +
	`&& echo 'change-preview-window(follow)' ` +
	`|| echo 'change-preview-window(nofollow)'`

// args returns the picker's arguments for a preview of the given layout, or
// for no preview at all where the layout is empty.
//
// The query sits at the top, where the list grows away from it. Enter clears
// the marks rather than acting on them, so marking is for the keys that take
// several rows and enter stays a thing done to one.
func args(query string, cursor int, layout string) []string {
	out := []string{
		"--delimiter=\t",
		"--with-nth=4..",
		"--height=100%",
		"--layout=reverse",
		"--multi",
		"--tiebreak=index",
		"--with-shell=sh -c",
		"--print-query",
		"--query=" + query,
		"--prompt=zmx> ",
		"--header=" + header,
		"--bind=enter:clear-selection+accept",
		"--expect=" + strings.Join([]string{keyNew, keyNamed, keyKill, keyKillHard}, ","),
	}

	if layout != "" {
		out = append(out,
			"--preview="+preview,
			"--preview-window="+layout,
			"--bind="+follow,
		)
	}

	// The cursor starts where the pane stands, so a new pane is one keystroke
	// from where it is. --sync is what makes it land: an action bound to
	// start otherwise runs before the list it moves through has arrived.
	if cursor > 0 {
		out = append(out, "--sync", "--bind=start:pos("+strconv.Itoa(cursor+1)+")")
	}
	return out
}

// navigate presents the list until something is chosen or the picker is left,
// and returns the session to attach to and where to create it.
func navigate(start string) (name, dir string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	// The repositories are walked once and the walk outlives every turn: a
	// kill changes what is running, which is read again, and not what is on
	// disk.
	repos := repositories(home)

	query := ""
	for {
		live, err := sessions()
		if err != nil {
			return "", "", err
		}

		rows := list(live, repos)
		// An empty list is a line saying so, which leaves the picker
		// something to show and the query somewhere to sit.
		if len(rows) == 0 {
			rows = []row{{cells: []string{"", "nothing running, and no repositories below " + shorten(home)}}}
		}
		// The cursor is placed only on the turn that opens the picker: a
		// query narrows the list, and a position counted against the whole of
		// it would land somewhere else.
		cursor := -1
		if query == "" {
			cursor = at(rows, start)
		}

		answer, err := present(lines(rows), args(query, cursor, previewLayout(termSize())))
		if err != nil {
			return "", "", err
		}
		query = answer.query

		var picked []row
		for _, i := range answer.rows {
			if i < len(rows) {
				picked = append(picked, rows[i])
			}
		}
		var first row
		if len(picked) > 0 {
			first = picked[0]
		}

		switch answer.key {
		case keyNamed:
			// Named from the query, whatever the cursor stands on.
			if name := named(query); name != "" {
				return name, start, nil
			}
			continue

		case keyNew:
			if first.dir == "" {
				continue
			}
			if first.kind == kindRepo && first.session == "" && !first.several {
				return sessionName(first.dir), first.dir, nil
			}
			label, err := promptText("label> ", "another session in "+shorten(first.dir))
			if errors.Is(err, errCancelled) {
				continue
			}
			if err != nil {
				return "", "", err
			}
			return sessionName(first.dir) + "+" + label, first.dir, nil

		case keyKill, keyKillHard:
			// A repository holding several is the same ambiguity enter meets,
			// and killing every one of them is not what one keystroke means.
			if len(picked) == 1 && first.several {
				query = narrow(first.dir)
				continue
			}
			var names []string
			for _, r := range picked {
				if r.session != "" {
					names = append(names, r.session)
				}
			}
			if len(names) == 0 {
				continue
			}
			if answer.key == keyKill {
				ok, err := confirm(question(names))
				if err != nil && !errors.Is(err, errCancelled) {
					return "", "", err
				}
				if !ok {
					continue
				}
			}
			if err := killSessions(names); err != nil {
				return "", "", err
			}
			continue
		}

		// Enter.
		if len(picked) == 0 {
			// Nothing matched what was typed, so what was typed is the name
			// of a session to start where the pane stands.
			if name := named(query); name != "" {
				return name, start, nil
			}
			continue
		}
		if first.several {
			query = narrow(first.dir)
			continue
		}
		if first.session != "" {
			return first.session, "", nil
		}
		if first.dir != "" {
			return sessionName(first.dir), first.dir, nil
		}
		// The line saying the list is empty stands for nothing to attach to.
		continue
	}
}

// confirmArgs returns the arguments for the picker that asks q.
func confirmArgs(q string) []string {
	return []string{
		"--delimiter=\t",
		"--with-nth=3..",
		"--height=100%",
		"--layout=reverse",
		"--no-sort",
		"--no-info",
		"--prompt=" + q + " ",
		"--header=↑↓ choose   ⏎ confirm",
	}
}

// confirm asks a yes or no question, answered with the same arrows as
// everything else. The cursor starts on no, and escape is no.
func confirm(q string) (bool, error) {
	index, err := pick([]string{"0\t\tno", "1\t\tyes"}, confirmArgs(q))
	if err != nil {
		return false, err
	}
	return index == 1, nil
}

// question is what a kill asks before it happens.
func question(names []string) string {
	if len(names) == 1 {
		return "kill " + names[0] + "?"
	}
	return "kill " + strconv.Itoa(len(names)) + " sessions?"
}

// killOrder puts the session this is running inside last.
//
// Killing it takes this process down with it, so anything ordered after it
// would never be reached.
func killOrder(names []string, current string) []string {
	ordered := make([]string, 0, len(names))
	found := false
	for _, name := range names {
		if current != "" && name == current {
			found = true
			continue
		}
		ordered = append(ordered, name)
	}
	if found {
		ordered = append(ordered, current)
	}
	return ordered
}

// killSessions ends the named sessions and every client attached to them.
func killSessions(names []string) error {
	for _, name := range killOrder(names, os.Getenv("ZMX_SESSION")) {
		out, err := exec.Command("zmx", "kill", name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("zmx kill %s: %s", name, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// run presents the picker and attaches to what it answers.
func run() error {
	start, err := os.Getwd()
	if err != nil {
		return err
	}
	name, dir, err := navigate(start)
	if err != nil {
		return err
	}
	return attach(name, dir)
}
