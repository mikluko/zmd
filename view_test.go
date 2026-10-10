package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sceneDirs are the checkouts and the plain directory scene builds.
func sceneDirs(root string) (engine, tree, api, notes string) {
	engine = filepath.Join(root, "src", "acme", "engine")
	tree = filepath.Join(engine, ".claude", "worktrees", "engine-918")
	api = filepath.Join(root, "src", "acme", "api")
	notes = filepath.Join(root, "Documents", "notes")
	return
}

// labelsOf is what each row says first.
func labelsOf(rows []row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.cells[0])
	}
	return out
}

// The tree goes treeDepth levels down past the walk's skips and leaves hidden
// directories out, save one a session stands in or work has been done in.
func TestTreeListsToDepthAndLeavesHiddenOut(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a/b/c/d/e", ".hidden/x", "kept/.secret", "node_modules/pkg"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := map[string]bool{}
	for _, d := range tree(root, 4, []string{filepath.Join(root, "kept", ".secret")}) {
		rel, _ := filepath.Rel(root, d)
		got[rel] = true
	}
	for _, want := range []string{"a", "a/b/c/d", "kept", "kept/.secret"} {
		if !got[want] {
			t.Errorf("%s is not listed: %v", want, got)
		}
	}
	for _, not := range []string{"a/b/c/d/e", ".hidden", "node_modules"} {
		if got[not] {
			t.Errorf("%s is listed", not)
		}
	}
}

// Standing somewhere lists the sessions there, the offer, then what is below:
// worked-in directories first, latest first, the rest by path. A directory
// counts every session in it or below it.
func TestFileRowsStandingSomewhere(t *testing.T) {
	root, live := scene(t)
	_, _, api, notes := sceneDirs(root)
	now := time.Now()
	seen := visits{
		notes: {Rank: 1, Last: now.Add(-time.Hour).Unix()},
		api:   {Rank: 1, Last: now.Unix()},
	}
	src := filepath.Join(root, "src")
	rows := fileRows(src, root, live, tree(src, treeDepth, nil), nil, seen)
	got := labelsOf(rows)
	if got[0] != ".." || got[1] != newSession || got[2] != "acme/api" {
		t.Fatalf("rows %q, want .., the offer, then acme/api as the latest worked in", got)
	}
	for _, r := range rows {
		if r.cells[0] == "acme" && r.count != 4 {
			t.Errorf("acme counts %d sessions, want the four at or below it", r.count)
		}
	}
}

// At the top there is no way up, and the sessions standing there lead.
func TestFileRowsAtHome(t *testing.T) {
	root, live := scene(t)
	live = append(live, Session{Name: "athome", Dir: root, Clients: "0"})
	got := labelsOf(fileRows(root, root, live, nil, nil, visits{}))
	if len(got) < 2 || got[0] != "athome" || got[1] != newSession {
		t.Errorf("rows %q, want the session at home, then the offer", got)
	}
}

// The latest attached leads; a session zmd never attached ranks by when zmx
// started it.
func TestSessionRowsLatestAttachedFirst(t *testing.T) {
	live := []Session{
		{Name: "old", Created: 100},
		{Name: "young", Created: 300},
		{Name: "attached", Created: 50},
	}
	got := labelsOf(sessionRows(live, map[string]int64{"attached": 500}, nil))
	if strings.Join(got, " ") != "attached young old" {
		t.Errorf("order %q", got)
	}
}

// An application bundle is a directory macOS presents as a file, and holds no
// work to stand in.
func TestTreeLeavesBundlesOut(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Applications", "Tool.app", "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range tree(root, 4, nil) {
		if strings.Contains(d, ".app") {
			t.Errorf("%s is listed", d)
		}
	}
}
