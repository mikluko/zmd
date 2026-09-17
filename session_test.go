package main

import (
	"strings"
	"testing"
)

// The record zmx 0.8.1 writes: a marker ahead of the first label, two spaces
// or an arrow, tab-separated key=value, a file URI carrying the host, and no
// cmd= on a session started without one.
const listFixture = "" +
	"  name=mikluko.dotfiles\tpid=62474\tclients=1\tcreated=1789551651\tcwd=file://saraksh.local/Users/m/Forge/mikluko/dotfiles\tcmd=/bin/sh\n" +
	"  name=freshone\tpid=63836\tclients=0\tcreated=1789551776\tcwd=file://saraksh.local/Users/m\n"

// The session the terminal is attached to, marked as `zmx list` marks it.
const attachedFixture = "" +
	"→ name=mikluko.here\tpid=1646\tclients=1\tcreated=1789626538\tcwd=file://saraksh.local/Users/m/here\n"

func TestParseSessionsReadsFieldsByKey(t *testing.T) {
	got := parseSessions(strings.NewReader(listFixture))
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2", len(got))
	}
	// Sorted by name, so freshone leads.
	if got[0].Name != "freshone" || got[1].Name != "mikluko.dotfiles" {
		t.Errorf("names %q, %q", got[0].Name, got[1].Name)
	}
	if got[1].Clients != "1" || got[1].PID != "62474" {
		t.Errorf("clients %q pid %q", got[1].Clients, got[1].PID)
	}
	if got[1].Dir != "/Users/m/Forge/mikluko/dotfiles" {
		t.Errorf("dir %q", got[1].Dir)
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
		if s.Name == "mikluko.here" {
			here = s
		}
	}
	if here.Name == "" {
		t.Fatalf("the attached session is missing from %v", got)
	}
	if here.Clients != "1" || here.Dir != "/Users/m/here" {
		t.Errorf("read %+v", here)
	}
}

// A record without cmd= must not shift the columns: this is what breaks a
// reader that takes fields by position.
func TestParseSessionsToleratesMissingCmd(t *testing.T) {
	got := parseSessions(strings.NewReader(listFixture))
	if got[0].Dir != "/Users/m" {
		t.Errorf("dir %q, want /Users/m", got[0].Dir)
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
		{"file://saraksh.local/Users/m/Forge", "/Users/m/Forge"},
		{"file:///Users/m/Forge", "/Users/m/Forge"},
		{"file://host/Users/m/two%20words", "/Users/m/two words"},
		{"/Users/m/plain", "/Users/m/plain"},
		{"", ""},
	} {
		if got := parseCwd(tc.in); got != tc.want {
			t.Errorf("parseCwd(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A slash makes the session's socket path a directory that is not there, so
// zmx creates nothing and reports nothing. Names must not carry one.
func TestSessionSafeRemovesSeparators(t *testing.T) {
	if got := sessionSafe("uptime-com/up2-monitoring"); got != "uptime-com.up2-monitoring" {
		t.Errorf("got %q", got)
	}
	if got := sessionSafe("mikluko/octant@918-arrival"); strings.Contains(got, "/") {
		t.Errorf("got %q, still carries a separator", got)
	}
}
