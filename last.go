package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// The session escape goes back to, and the directory the picker opens scoped
// to.
//
// A pane holding the picker is a pane with nothing attached, and leaving the
// picker ends the process, which ends the loop that runs it and takes the
// pane with it. Escape attaches what was attached last instead, so the pane
// survives being left.
//
// The loop's shell never chdirs, so the directory the picker is run in is
// where the pane was opened and not where its work has been since. What the
// pane last held is the answer to both questions.

// stateDir is where what outlives a run of the picker is kept.
func stateDir() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "zmd")
}

// lastFile is where the session this pane last attached is recorded.
//
// The pane is its loop's shell, which is the picker's parent and the same
// process on every turn of the loop, so its pid is what tells one pane from
// another. Panes sharing one record is panes going back to a session they
// were never in, and opening scoped to work that is not theirs.
func lastFile() string {
	dir := stateDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "last."+strconv.Itoa(os.Getppid()))
}

// sharedLastFile is where the session any pane last attached is recorded.
//
// A pane that has held nothing yet has no record of its own, and escape
// pressed in one still has to attach something or the pane closes.
func sharedLastFile() string {
	dir := stateDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "last")
}

// lastHeld returns the session the picker goes back to: the one this pane
// last attached, or the one any pane did where this pane has held none.
//
// A name is not enough on its own. Attaching one that has since been killed
// creates a session rather than returning to one, in whatever directory the
// pane happens to stand in, so a name no longer running answers with nothing.
// Neither does one another terminal has open: going back to it would put two
// clients on it, mirroring each other, which is not what leaving the picker
// asked for. Where that leaves nothing to go back to, there is nothing, and
// escape leaves instead.
func lastHeld(live []Session) *Session {
	for _, path := range []string{lastFile(), sharedLastFile()} {
		if s := running(live, readLast(path)); s != nil && !s.heldElsewhere() {
			return s
		}
	}
	return nil
}

// recordLast writes name to this pane's record and to the shared one.
func recordLast(name string) {
	writeLast(lastFile(), name)
	writeLast(sharedLastFile(), name)
	prune(stateDir())
}

// prune drops the records of panes that are gone.
//
// Nothing else clears them: a pane closing runs no code of ours. Signal 0 is
// the ask whether a pid is there at all, which is what a record is named
// after.
func prune(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		pid, ok := strings.CutPrefix(entry.Name(), "last.")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(pid)
		if err != nil || n == os.Getppid() {
			continue
		}
		if syscall.Kill(n, 0) == syscall.ESRCH {
			os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// readLast returns the name recorded at path, or "" where there is none.
func readLast(path string) string {
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// writeLast records name at path.
//
// Failing to write is not worth reporting: escape then leaves the picker the
// way it did before, and nothing else is lost.
func writeLast(path, name string) {
	if path == "" || name == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	os.WriteFile(path, []byte(name+"\n"), 0o600)
}

// running returns the session by that name, or nil where none is running.
func running(live []Session, name string) *Session {
	if name == "" {
		return nil
	}
	for i, s := range live {
		if s.Name == name {
			return &live[i]
		}
	}
	return nil
}
