package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLastSurvivesBeingWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "zmd", "last")

	if got := readLast(path); got != "" {
		t.Errorf("a pane that has attached nothing read %q", got)
	}

	writeLast(path, "src.acme.engine")
	if got := readLast(path); got != "src.acme.engine" {
		t.Errorf("read back %q", got)
	}

	// The newest attach is what escape goes back to, not the first.
	writeLast(path, "src.acme.zmd")
	if got := readLast(path); got != "src.acme.zmd" {
		t.Errorf("read back %q after a second attach", got)
	}
}

// Nothing is reported when the file cannot be written: escape then leaves the
// picker the way it did before.
func TestWriteLastToleratesNowhereToWrite(t *testing.T) {
	writeLast("", "name")
	writeLast(filepath.Join(t.TempDir(), "x"), "")

	file := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeLast(filepath.Join(file, "zmd", "last"), "name")
}

// Attaching a name that has since been killed would create a session rather
// than return to one, in whatever directory the pane happens to stand in.
func TestRunningRequiresTheSessionToStillBeThere(t *testing.T) {
	live := []Session{
		{Name: "src.acme.engine", Dir: "/a"},
		{Name: "supa-7f3a", Dir: "/b"},
	}
	if got := running(live, "src.acme.engine"); got == nil || got.Dir != "/a" {
		t.Errorf("a running session gave %v", got)
	}
	if got := running(live, "killed.since"); got != nil {
		t.Errorf("a session that is gone gave %v, want nothing to go back to", got)
	}
	if got := running(live, ""); got != nil {
		t.Errorf("no recorded name gave %v", got)
	}
	if got := running(nil, "src.acme.engine"); got != nil {
		t.Errorf("nothing running gave %v", got)
	}
}

// One record per pane. Seven panes sharing one is six of them going back to a
// session they were never in.
func TestLastIsKeptPerPane(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if lastFile() == sharedLastFile() {
		t.Fatalf("a pane's record and the shared one are the same file: %s", lastFile())
	}

	live := []Session{{Name: "src.acme.engine", Dir: "/a"}, {Name: "shared.one", Dir: "/b"}}

	// A pane that has held nothing still has somewhere to go, or escape
	// pressed in it closes it.
	writeLast(sharedLastFile(), "shared.one")
	if got := lastHeld(live); got == nil || got.Name != "shared.one" {
		t.Errorf("a pane holding nothing gave %v, want the shared record", got)
	}

	// Its own record wins once it has one.
	writeLast(lastFile(), "src.acme.engine")
	if got := lastHeld(live); got == nil || got.Name != "src.acme.engine" {
		t.Errorf("a pane with a record of its own gave %v", got)
	}

	// A record naming a session that is gone is not one to go back to, and
	// the shared record is read past it.
	writeLast(lastFile(), "killed.since")
	if got := lastHeld(live); got == nil || got.Name != "shared.one" {
		t.Errorf("a record naming a dead session gave %v", got)
	}
}

// Going back to a session another terminal has open would put two clients on
// it, mirroring each other. That is not what leaving the picker asked for, so
// there is nothing to go back to and escape leaves.
func TestLastHeldWillNotGoBackToASessionAnotherTerminalHas(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	live := []Session{
		{Name: "taken", Clients: "1"},
		{Name: "free", Clients: "0"},
	}

	writeLast(lastFile(), "taken")
	writeLast(sharedLastFile(), "free")
	if got := lastHeld(live); got == nil || got.Name != "free" {
		t.Errorf("gave %v, want the record that is not held elsewhere", got)
	}

	writeLast(sharedLastFile(), "taken")
	if got := lastHeld(live); got != nil {
		t.Errorf("gave %v, want nothing to go back to", got)
	}
}

// Nothing clears a pane's record when the pane closes: no code of ours runs
// then, so the next write is what collects it.
func TestPruneDropsTheRecordsOfPanesThatAreGone(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, "last."+strconv.Itoa(os.Getppid()))
	// Pid 1 is there on any machine this runs on; the high one is not.
	alive := filepath.Join(dir, "last.1")
	gone := filepath.Join(dir, "last.999999")
	for _, path := range []string{mine, alive, gone, filepath.Join(dir, "last")} {
		write(t, path, "src.acme.engine\n")
	}

	prune(dir)

	for _, path := range []string{mine, alive, filepath.Join(dir, "last")} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was collected: %v", filepath.Base(path), err)
		}
	}
	if _, err := os.Stat(gone); err == nil {
		t.Error("the record of a pane that is gone was kept")
	}
}
