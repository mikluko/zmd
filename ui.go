package main

import (
	"context"
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

// One list of places: the repositories a session can be started in, and the
// directories sessions are already running in. Typing is the navigation.
//
// A session is not a row. It is something a place holds, counted in the
// status column, and named only where a place holds more than one and has to
// be asked which.

// The status column stands ahead of the name and is three cells wide,
// whatever it has to say:
//
//	1  the checkout: nothing, a repository, or a worktree cut from one
//	2  the sessions standing there
//	3  reserved for what the agent in a session is doing
//
// Fixed width is what lets it carry colour. text/tabwriter counts an escape
// byte as a character, so a cell it aligns cannot hold one, and the column is
// written past align rather than through it.
const statusWidth = 3

// A session with a client attached is filled, one with none is hollow, and a
// place holding more than one says so with a ring inside the ring rather than
// with a circle per session: the column is three cells wide whatever stands
// there.
const (
	attached = "●"
	detached = "○"
	several  = "◉"
	severalO = "◎"
)

// checkout marks a directory git tracks. The same mark carries both kinds,
// bright where the directory is the repository and dim where it is a worktree
// cut from one, since a worktree is that repository seen from another branch
// rather than a different sort of thing.
//
// The mark is a Nerd Font glyph, which Berkeley Mono does not carry and the
// terminal's fallback does; so are none of the circles above, which the
// terminal has been drawing from fallback all along.
const (
	checkout = ""
	dim      = "\x1b[2m"
	undim    = "\x1b[0m"
)

// hints is the keys worth advertising for a row in the given state.
//
// A key that would do nothing is not offered: nothing to take where nobody
// else holds it, nothing to kill where nothing is running, and `⏎ start`
// rather than `⏎ attach` where entering means making one.
func hints(state string) string {
	keys := []string{}
	switch state {
	case "s":
		keys = append(keys, "⏎ attach")
	case "sh":
		keys = append(keys, "⏎ take")
	case "m", "mh":
		keys = append(keys, "⏎ in")
	default:
		keys = append(keys, "⏎ start")
	}
	keys = append(keys, "⌥⏎ new")
	if state != "r" {
		keys = append(keys, "⇥ mark", "⌦ kill")
	}
	return strings.Join(keys, "  ")
}

// header is the hint line before fzf has told anyone what the cursor is on.
const header = "⏎ attach  ⌥⏎ new"

// hintBind rewrites the hint line and the preview's follow mode on every move
// of the cursor, both being answers to what the cursor is standing on.
//
// One bind for the two because fzf runs a shell per bind per keystroke, and
// the branch is the same branch. tail is what the turn knows and the row does
// not: whether there is a rung under this one, and anything to leave to.
func hintBind(tail string) string {
	var b strings.Builder
	b.WriteString("focus:transform:case {2} in ")
	for _, state := range []string{"sh", "s", "mh", "m", "r"} {
		window := "nofollow"
		if strings.HasPrefix(state, "s") {
			window = "follow"
		}
		b.WriteString(state + ") echo 'change-preview-window(" + window + ")" +
			"+change-header(" + hints(state) + tail + ")' ;; ")
	}
	b.WriteString("esac")
	return b.String()
}

// row is one line of the list.
type row struct {
	dir     string    // the place the row stands for
	found   []Session // what is running there, by name
	session string    // the one session to attach to, empty where there is not exactly one
	count   int       // sessions alive at or below the row
	trees   int       // worktrees cut from the checkout, where the row is one
	several bool      // more sessions here than one keystroke can choose between
	held    bool      // another terminal has that session open
	status  string    // the fixed cells ahead of the name, which may carry colour
	cells   []string  // what the line shows, aligned against every other row
}

// state is what the row is, in the letters the keys branch on: `r` a place
// with nothing running, `s` a place with one session, `m` a place with
// several, and `h` appended where another terminal has one of them.
//
// It travels in a field of its own because fzf is what reads it. The preview
// and the hint line both branch per row, between turns of the loop, where
// nothing of this program is running to be asked.
func (r row) state() string {
	at := "r"
	switch {
	case r.session != "":
		at = "s"
	case r.count > 0:
		at = "m"
	}
	if r.held {
		return at + "h"
	}
	for _, s := range r.found {
		if s.heldElsewhere() {
			return at + "h"
		}
	}
	return at
}

// circle renders the sessions in one directory in the one cell they get.
//
// How many there are past one is not what the cell answers; whether any of
// them is attached is. A count belongs to the rows themselves, which the
// query reaches.
func circle(found []Session) string {
	if len(found) == 0 {
		return " "
	}
	for _, s := range found {
		if s.Clients != "0" && s.Clients != "" {
			if len(found) > 1 {
				return several
			}
			return attached
		}
	}
	if len(found) > 1 {
		return severalO
	}
	return detached
}

// mark renders what git makes of dir in the one cell it gets.
func mark(dir string) string {
	root := repoRoot(dir)
	switch {
	case root == "":
		return " "
	case root == filepath.Clean(dir):
		return checkout
	default:
		return dim + checkout + undim
	}
}

// status is the three cells that stand ahead of the name.
//
// The third is reserved and blank: nothing sets it yet, and holding the width
// now is what keeps every name in the list where it is when something does.
func status(dir string, found []Session) string {
	return mark(dir) + circle(found) + " "
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

// lines renders rows for fzf: the index, the kind, what the preview reads,
// and what is shown. Only the last is searched, and the index is what comes
// back.
//
// The status is written ahead of what align returns rather than through it,
// since it is the one part of a line that may carry colour and align counts
// in characters.
func lines(rows []row) []string {
	cells := make([][]string, len(rows))
	for i, r := range rows {
		cells[i] = r.cells
	}
	display := align(cells)

	out := make([]string, len(rows))
	for i, r := range rows {
		what, target := r.state(), r.dir
		if len(r.found) == 1 {
			target = r.found[0].Name
		}
		head := r.status
		if head == "" {
			head = strings.Repeat(" ", statusWidth)
		}
		out[i] = strconv.Itoa(i) + "\t" + what + "\t" + target + "\t" + head + display[i]
	}
	return out
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
		if !under(base, dir) {
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

// previewCmd branches on the kind, since one list holds both. It runs under
// `sh -c` rather than the login shell, which need not be one that parses it.
//
// A session's scrollback comes back through this binary rather than from
// `zmx history` directly, which is what keeps the colour and drops what the
// `--vt` form addresses to the terminal.
func previewCmd() string {
	return `case {2} in
s*) ` + quote(executable()) + ` history {3} ;;
*)  git -C {3} log --color=always --graph -50 --format='%C(auto)%h %as%d %s' ;;
esac`
}

// executable is the path to this binary, or the name to look up on PATH where
// the path cannot be had.
func executable() string {
	path, err := os.Executable()
	if err != nil {
		return "zmd"
	}
	return path
}

// quote wraps s for the shell the preview runs in.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// scroll moves the preview rather than the list.
//
// The keys arrive as alt-modified because command never reaches a terminal
// process on macOS: the terminal is bound to send the alt form, which is what
// fzf decodes. Nothing here can bind command itself.
var scroll = []string{
	"alt-up:preview-up",
	"alt-down:preview-down",
	"alt-page-up:preview-page-up",
	"alt-page-down:preview-page-down",
}

// expected is every key the loop acts on. fzf reports only what it is told to
// expect, so a key missing here is a key that does nothing however carefully
// the loop handles it, and however plainly the hint line offers it.
var expected = []string{keyNew, keyRepos, keyKill, keyKillHard}

// walk moves the stack rather than the cursor: right pushes and left pops,
// and enter is right's alias since descending is the one motion.
//
// Each of these keys edits the query first, because a picker that cannot
// correct what was typed is worse than one that cannot walk. With nothing
// typed there is nothing to correct, and the key is free to mean the other
// thing; fzf decides which per keystroke by running the test below. The stack
// is not a thing fzf holds, so a pop goes back to the loop as a printed
// marker and the turn ends.
var walk = []string{
	"backspace:transform:[ -n {q} ] && echo backward-delete-char || echo 'print(" + popRung + ")+accept'",
	"left:transform:[ -n {q} ] && echo backward-char || echo 'print(" + popRung + ")+accept'",
	"right:transform:[ -n {q} ] && echo forward-char || echo accept",
}

// args returns the picker's arguments for a preview of the given layout, or
// for no preview at all where the layout is empty.
//
// The query sits at the top, where the list grows away from it. Enter clears
// the marks rather than acting on them, so marking is for the keys that take
// several rows and enter stays a thing done to one.
func args(here rung, query, tail string, cursor int, layout string) []string {
	out := []string{
		"--delimiter=\t",
		"--with-nth=4..",
		// The status column is dim on a worktree, which is an escape
		// sequence in the line rather than a thing fzf is told.
		"--ansi",
		"--height=100%",
		"--layout=reverse",
		"--multi",
		"--tiebreak=index",
		"--with-shell=sh -c",
		"--print-query",
		"--query=" + query,
		// The prompt is the rung, since where the picker stands is what
		// decides what is in the list at all.
		"--prompt=" + here.prompt() + "> ",
		"--header=" + header + tail,
		"--bind=enter:clear-selection+accept",
		"--expect=" + strings.Join(expected, ","),
	}
	for _, bind := range walk {
		out = append(out, "--bind="+bind)
	}
	out = append(out, "--bind="+hintBind(tail))

	if layout != "" {
		out = append(out,
			"--preview="+previewCmd(),
			"--preview-window="+layout,
			"--bind="+strings.Join(scroll, ","),
		)
	}

	// The cursor starts where the picker is focused, so what it was opened
	// for is one keystroke away. --sync is what makes it land: an action
	// bound to start otherwise runs before the list it moves through has
	// arrived.
	if cursor > 0 {
		out = append(out, "--sync", "--bind=start:pos("+strconv.Itoa(cursor+1)+")")
	}
	return out
}

// navigate walks the stack until something is chosen or the picker is left,
// and returns the session to attach to and where to create it.
func navigate(start string) (name, dir string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	// The walk outlives every turn: a kill changes what is running, which is
	// read again, and not what is on disk. The daemon holds it where one is
	// listening, which is a quarter of a second this process then does not
	// spend before drawing anything.
	deck := dialDeck()
	repos, ident := walked(context.Background(), deck, home)

	// The ground floor is every session there is. Nothing above it is where
	// the picker opens, the sessions being what a switch is between.
	stack := []rung{{view: viewSessions}}
	query := ""
	for {
		live, err := sessions()
		if err != nil {
			return "", "", err
		}
		here := stack[len(stack)-1]
		list := rows(here, live, repos, ident)
		if len(list) == 0 {
			list = []row{{cells: []string{"", empty(here)}}}
		}

		// The cursor is placed only where nothing has been typed: a query
		// narrows the list, and a position counted against the whole of it
		// would land somewhere else. On the ground floor it opens on the
		// session the pane last held, which is the one just left.
		cursor := -1
		if query == "" && here.view == viewSessions {
			if held := lastHeld(live); held != nil {
				cursor = named(list, held.Name)
			}
		}

		answer, err := present(lines(list), args(here, query, tail(stack, live), cursor, previewLayout(termSize())))
		if errors.Is(err, errCancelled) {
			// Escape leaves rather than popping: the arrow is what walks the
			// stack. Where the pane last held a session that is still free,
			// going back to it is what leaving means.
			if held := lastHeld(live); held != nil {
				return held.Name, "", nil
			}
			return "", "", errCancelled
		}
		if err != nil {
			return "", "", err
		}
		query = answer.query

		var picked []row
		for _, i := range answer.rows {
			if i < len(list) {
				picked = append(picked, list[i])
			}
		}
		var first row
		if len(picked) > 0 {
			first = picked[0]
		}

		// Left pops, and the ground floor has nothing under it.
		if answer.pop {
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
				query = ""
			}
			continue
		}

		switch answer.key {
		case keyRepos:
			if here.view == viewSessions {
				stack = append(stack, rung{view: viewRepos})
				query = ""
			}
			continue

		case keyNew:
			// A session beside the one the cursor stands on, or in the place
			// it stands on. Named rather than asked about: a name is a handle
			// and says nothing, so there is nothing for a prompt to ask.
			if first.dir == "" {
				continue
			}
			return sessionName(live), first.dir, nil

		case keyKill, keyKillHard:
			var names []string
			for _, r := range picked {
				names = append(names, killable(r, live)...)
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

		// Enter and right both descend, being the same motion.
		if len(picked) == 0 || first.dir == "" {
			continue
		}
		switch here.view {
		case viewSessions:
			name, dir, err := enter(first, live)
			if errors.Is(err, errRetry) {
				continue
			}
			return name, dir, err

		case viewRepos:
			if first.trees > 0 {
				stack = append(stack, rung{view: viewCheckouts, dir: first.dir, name: first.cells[0]})
				query = ""
				continue
			}
			fallthrough

		default:
			// A checkout holding sessions hands straight back down to them,
			// filtered to itself: switching between them is the ground
			// floor's work and it does it better than another rung would.
			if first.count == 0 {
				return sessionName(live), first.dir, nil
			}
			stack = append(stack, rung{view: viewSessions, dir: first.dir, name: first.cells[0]})
			query = ""
			continue
		}
	}
}

// tail is what the turn knows and a row does not: whether there is a rung
// under this one, and anything to leave to.
func tail(stack []rung, live []Session) string {
	out := ""
	if len(stack) > 1 {
		out += "  ← back"
	}
	if stack[len(stack)-1].view == viewSessions {
		out += "  ⌃g repos"
	}
	if lastHeld(live) != nil {
		out += "  ⎋ leave"
	}
	return out
}

// enter attaches the session a row stands for, taking it from the terminal
// that has it open after asking.
//
// Taking drops that terminal's client out from under whoever is at it, which
// is not a thing to do without saying so.
func enter(r row, live []Session) (string, string, error) {
	if r.session == "" {
		return sessionName(live), r.dir, nil
	}
	if r.held {
		ok, err := confirm(takes(r.cells[0]))
		if err != nil && !errors.Is(err, errCancelled) {
			return "", "", err
		}
		if !ok {
			return "", "", errRetry
		}
		if err := detachClients(r.session); err != nil {
			return "", "", err
		}
	}
	return r.session, "", nil
}

// killable is every session a kill aimed at the row would take.
func killable(r row, live []Session) []string {
	if r.session != "" {
		return []string{r.session}
	}
	var names []string
	for _, s := range live {
		if r.dir != "" && under(r.dir, s.Dir) {
			names = append(names, s.Name)
		}
	}
	return names
}

// named is the row standing for the given session, or -1.
func named(list []row, name string) int {
	for i, r := range list {
		if r.session == name {
			return i
		}
	}
	return -1
}

// empty is the line a rung with nothing to show puts in place of a list.
func empty(r rung) string {
	switch r.view {
	case viewRepos:
		return "no repositories below ~"
	case viewCheckouts:
		return "no checkouts here"
	default:
		return "nothing running; ⌃g reaches the repositories"
	}
}

// takes is what the selector asks before a session changes hands.
func takes(label string) string {
	return "take " + label + " from the other terminal?"
}

// sessionLabel is what a session goes by: what its labels say it is for, or
// its own name where they say nothing.
//
// The name is a handle and carries no meaning, so it is the fallback and not
// the answer. `zmx list` writes the labels on the same record it writes the
// name on, so both arrive in the one call the picker already makes.
func sessionLabel(s Session) string {
	if s.Title != "" {
		return s.Title
	}
	return s.Name
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

// detachClients drops every client attached to the named session.
//
// zmx 0.8.1 takes no session name here: `zmx detach` reads the one it is
// addressed to out of ZMX_SESSION, so the name is passed that way. It drops
// the calling terminal's own client along with the target's, which is why
// this is only reached on the way to an attach that takes that client back.
func detachClients(name string) error {
	cmd := exec.Command("zmx", "detach")
	cmd.Env = append(os.Environ(), "ZMX_SESSION="+name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("zmx detach %s: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
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
	// Recorded before the exec, since there is no returning from it. This is
	// what escape goes back to next time, and what the picker opens scoped to.
	recordLast(name)
	return attach(name, dir)
}
