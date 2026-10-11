package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeZmx puts a zmx on PATH that writes stderr and exits with code.
func fakeZmx(t *testing.T, stderr string, code int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho '" + stderr + "' >&2\nexit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "zmx"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// An empty socket directory is no sessions, whatever zmx exits with; any other
// failure is reported, since read as none it would show every session gone.
func TestSessionsTellsNoneFromAFailure(t *testing.T) {
	for _, c := range []struct {
		stderr  string
		code    int
		wantErr bool
	}{
		{"no sessions found in /tmp/zmx-501", 0, false},
		{"no sessions found in /tmp/zmx-501", 1, false},
		{"error: ReadOnlyFileSystem", 1, true},
	} {
		fakeZmx(t, c.stderr, c.code)
		live, err := sessions()
		if (err != nil) != c.wantErr || len(live) != 0 {
			t.Errorf("%q exit %d: live %v err %v", c.stderr, c.code, live, err)
		}
		if err != nil && !strings.Contains(err.Error(), "ReadOnlyFileSystem") {
			t.Errorf("the error drops what zmx said: %v", err)
		}
	}
}

// The record zmx 0.8.1 writes: a marker ahead of the first label, two spaces
// or an arrow, tab-separated key=value, a file URI carrying the host, and no
// cmd= on a session started without one.
const listFixture = "" +
	"  name=acme.dotfiles\tpid=62474\tclients=1\tcreated=1789551651\tcwd=file://host.local/Users/m/src/acme/dotfiles\tcmd=/bin/sh\n" +
	"  name=freshone\tpid=63836\tclients=0\tcreated=1789551776\tcwd=file://host.local/Users/m\n"

// The session the terminal is attached to, marked as `zmx list` marks it.
const attachedFixture = "" +
	"→ name=acme.here\tpid=1646\tclients=1\tcreated=1789626538\tcwd=file://host.local/Users/m/here\n"

func TestParseSessionsReadsFieldsByKey(t *testing.T) {
	got := parseSessions(strings.NewReader(listFixture))
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2", len(got))
	}
	// Sorted by name, so acme.dotfiles leads.
	if got[0].Name != "acme.dotfiles" || got[1].Name != "freshone" {
		t.Errorf("names %q, %q", got[0].Name, got[1].Name)
	}
	if got[0].Clients != "1" || got[0].PID != "62474" {
		t.Errorf("clients %q pid %q", got[0].Clients, got[0].PID)
	}
	if got[0].Dir != "/Users/m/src/acme/dotfiles" {
		t.Errorf("dir %q", got[0].Dir)
	}
}

// The marker ahead of the first label is not part of it. Reading it as part
// of the label drops the one session the terminal is certainly interested in.
func TestParseSessionsReadsTheAttachedSession(t *testing.T) {
	got := parseSessions(strings.NewReader(attachedFixture + listFixture))
	if len(got) != 3 {
		t.Fatalf("got %d sessions, want 3", len(got))
	}
	var here Session
	for _, s := range got {
		if s.Name == "acme.here" {
			here = s
		}
	}
	if here.Name == "" {
		t.Fatalf("the attached session is missing from %v", got)
	}
	if here.Clients != "1" || here.Dir != "/Users/m/here" {
		t.Errorf("read %+v", here)
	}
	// The marker is also the answer to whose client that is.
	if !here.Here || here.heldElsewhere() {
		t.Errorf("the marked session read as held elsewhere: %+v", here)
	}
	for _, s := range got {
		if s.Name != "acme.here" && s.Here {
			t.Errorf("%s was marked as this terminal's", s.Name)
		}
	}
}

// A record without cmd= must not shift the columns: this is what breaks a
// reader that takes fields by position.
func TestParseSessionsToleratesMissingCmd(t *testing.T) {
	got := parseSessions(strings.NewReader(listFixture))
	if got[1].Dir != "/Users/m" {
		t.Errorf("dir %q, want /Users/m", got[1].Dir)
	}
}

func TestParseSessionsIgnoresBlankAndNamelessLines(t *testing.T) {
	in := "\n  pid=1\tclients=0\n" + listFixture
	if got := parseSessions(strings.NewReader(in)); len(got) != 2 {
		t.Errorf("got %d sessions, want 2", len(got))
	}
}

func TestParseCwd(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"file://host.local/Users/m/src", "/Users/m/src"},
		{"file:///Users/m/src", "/Users/m/src"},
		{"file://host/Users/m/two%20words", "/Users/m/two words"},
		{"/Users/m/plain", "/Users/m/plain"},
		{"", ""},
	} {
		if got := parseCwd(tc.in); got != tc.want {
			t.Errorf("parseCwd(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The marker `zmx list` writes is the only thing that tells a session this
// terminal holds from one another terminal holds, and both report a client.
func TestHeldElsewhereCountsPastThisTerminal(t *testing.T) {
	for _, c := range []struct {
		what    string
		session Session
		want    bool
	}{
		{"nobody attached", Session{Clients: "0"}, false},
		{"this terminal", Session{Clients: "1", Here: true}, false},
		{"another terminal", Session{Clients: "1"}, true},
		{"this one and another", Session{Clients: "2", Here: true}, true},
		{"two others", Session{Clients: "2"}, true},
		{"a count that does not read", Session{Clients: "?"}, false},
	} {
		if got := c.session.heldElsewhere(); got != c.want {
			t.Errorf("%s: heldElsewhere = %v, want %v", c.what, got, c.want)
		}
	}
}
