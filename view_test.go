package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// placeAt returns the place standing for dir.
func placeAt(t *testing.T, places []place, dir string) place {
	t.Helper()
	for _, p := range places {
		if p.row.dir == filepath.Clean(dir) {
			return p
		}
	}
	t.Fatalf("no place for %q", dir)
	return place{}
}

// sceneDirs are the checkouts and the plain directory scene builds.
func sceneDirs(root string) (engine, tree, api, notes string) {
	engine = filepath.Join(root, "src", "acme", "engine")
	tree = filepath.Join(engine, ".claude", "worktrees", "engine-918")
	api = filepath.Join(root, "src", "acme", "api")
	notes = filepath.Join(root, "Documents", "notes")
	return
}

// A repository holds the sessions in its worktrees as much as its own, and a
// worktree is never a place of its own.
func TestRepoPlacesHoldTheirWorktreesSessions(t *testing.T) {
	root, live := scene(t)
	engine, tree, api, _ := sceneDirs(root)
	places := repoPlaces(live, []string{engine, tree, api}, nil, visits{}, time.Now())
	if len(places) != 2 {
		t.Fatalf("%d places, want engine and api", len(places))
	}
	kids := placeAt(t, places, engine).kids
	var names []string
	for _, k := range kids {
		names = append(names, k.cells[0])
	}
	want := []string{"src.acme.engine", "src.acme.engine+review", "tree", newSession}
	if len(names) != len(want) {
		t.Fatalf("engine holds %q, want %q", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("engine holds %q, want %q", names, want)
			break
		}
	}
	if kids[2].cells[1] == "" {
		t.Error("the session in a worktree does not say which")
	}
	if kids[0].cells[1] != "" {
		t.Errorf("a session at the repository says where it is: %q", kids[0].cells[1])
	}
}

// Entering a worktree no session stands in is how one is started there, so
// such a worktree is offered among what its repository holds.
func TestAWorktreeWithNoSessionIsOffered(t *testing.T) {
	root, live := scene(t)
	engine, tree, api, _ := sceneDirs(root)
	var rest []Session
	for _, s := range live {
		if s.Name != "tree" {
			rest = append(rest, s)
		}
	}
	kids := placeAt(t, repoPlaces(rest, []string{engine, tree, api}, nil, visits{}, time.Now()), engine).kids
	offered := kids[len(kids)-2]
	if offered.dir != tree || offered.session != "" {
		t.Errorf("before the offer: %+v, want the idle worktree", offered)
	}
}

// The directories view holds what no repository does, and only that.
func TestDirPlacesHoldWhatNoRepositoryDoes(t *testing.T) {
	root, live := scene(t)
	engine, tree, api, notes := sceneDirs(root)
	places := dirPlaces(live, []string{engine, tree, api}, "", visits{}, time.Now())
	if len(places) != 2 {
		t.Fatalf("%d places, want notes and the loose directory", len(places))
	}
	kids := placeAt(t, places, notes).kids
	if len(kids) != 2 || kids[0].session != "Documents.notes" || kids[1].cells[0] != newSession {
		t.Errorf("notes holds %+v", kids)
	}
}

// A directory worked in before, or the one the pane stands in, is offered
// whether or not a session stands there now; one gone from disk, or inside a
// repository, is not.
func TestDirPlacesOfferVisitedAndStartingDirs(t *testing.T) {
	root, _ := scene(t)
	engine, _, _, notes := sceneDirs(root)
	start := filepath.Join(root, "Downloads")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	seen := visits{
		notes:                       {Rank: 1, Last: now.Unix()},
		filepath.Join(root, "gone"): {Rank: 5, Last: now.Unix()},
		engine:                      {Rank: 5, Last: now.Unix()},
	}
	places := dirPlaces(nil, []string{engine}, start, seen, now)
	got := map[string]bool{}
	for _, p := range places {
		got[p.row.dir] = true
	}
	if !got[notes] || !got[start] || len(got) != 2 {
		t.Errorf("places %v, want notes and the starting directory", got)
	}
}

// What is running leads, and below it what was worked in most and latest.
func TestPlacesHoldingSessionsLead(t *testing.T) {
	now := time.Now()
	seen := visits{"/b": {Rank: 50, Last: now.Unix()}, "/c": {Rank: 2, Last: now.Unix()}}
	got := ordered([]place{
		{row: row{dir: "/a", cells: []string{"a"}}},
		{row: row{dir: "/b", cells: []string{"b"}}},
		{row: row{dir: "/c", cells: []string{"c"}}},
		{row: row{dir: "/d", count: 1, cells: []string{"d"}}},
	}, seen, now)
	var order string
	for _, p := range got {
		order += p.row.cells[0]
	}
	if order != "dbca" {
		t.Errorf("order %q, want dbca", order)
	}
}

// A repository the walk never reached is a directory like any other, or a
// picker started in one would stand somewhere it cannot show.
func TestAnUnwalkedRepositoryIsADirectory(t *testing.T) {
	root, _ := scene(t)
	engine, _, _, _ := sceneDirs(root)
	outside := repoAt(t, filepath.Join(t.TempDir(), "elsewhere"))
	places := dirPlaces(nil, []string{engine}, filepath.Join(outside, "sub"), visits{}, time.Now())
	placeAt(t, places, outside)
}
