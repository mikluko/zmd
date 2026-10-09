package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
