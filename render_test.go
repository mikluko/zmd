package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// scene builds a tree holding two repositories, a worktree of one of them and
// a directory that is neither, and returns sessions sitting in some of them.
func scene(t *testing.T) (root string, found []Session) {
	t.Helper()
	root = t.TempDir()
	repo := repoAt(t, filepath.Join(root, "Forge", "mikluko", "octant"))
	tree := worktreeAt(t, repo, "octant-918-arrival",
		filepath.Join(root, "Forge", "mikluko", "octant", ".claude", "worktrees", "octant-918-arrival"))
	other := repoAt(t, filepath.Join(root, "Forge", "uptime-com", "api"))
	plain := filepath.Join(root, "Documents", "notes")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	return root, []Session{
		{Name: "Forge.mikluko.octant", Clients: "1", Dir: repo},
		{Name: "Forge.mikluko.octant+review", Clients: "0", Dir: repo},
		{Name: "supa-7f3a", Clients: "2", Dir: other},
		{Name: "Documents.notes", Clients: "0", Dir: plain},
		{Name: "loose", Clients: "0", Dir: t.TempDir()},
		{Name: "tree", Clients: "0", Dir: tree},
	}
}

// find returns the row standing for dir.
func find(t *testing.T, rows []row, dir string) row {
	t.Helper()
	for _, r := range rows {
		if filepath.Clean(r.dir) == filepath.Clean(dir) {
			return r
		}
	}
	t.Fatalf("no row for %q", dir)
	return row{}
}

func TestCirclesFillWithAClient(t *testing.T) {
	got := circles([]Session{
		{Name: "a", Clients: "1"},
		{Name: "b", Clients: "0"},
		{Name: "c", Clients: ""},
	})
	if got != attached+detached+detached {
		t.Errorf("rendered %q, want one filled and two hollow", got)
	}
	if circles(nil) != "" {
		t.Errorf("a directory with no sessions rendered %q", circles(nil))
	}
}

// The marker counts sessions by the directory they run in, so a session this
// tool never named still counts against the directory it sits in.
func TestDirRowsCountSessionsByDirectory(t *testing.T) {
	root, live := scene(t)
	repo := filepath.Join(root, "Forge", "mikluko", "octant")
	api := filepath.Join(root, "Forge", "uptime-com", "api")
	empty := filepath.Join(root, "Documents")

	rows := dirRows([]string{repo, api, empty}, live, filepath.Base)

	if got := find(t, rows, repo); got.cells[0] != attached+detached {
		t.Errorf("two sessions rendered %q", got.cells[0])
	} else if !got.several || got.session != "" {
		t.Errorf("two sessions named %q to attach, want the ambiguity kept", got.session)
	}
	if got := find(t, rows, api); got.session != "supa-7f3a" || got.cells[0] != attached {
		t.Errorf("a session named elsewhere rendered %q / %q", got.cells[0], got.session)
	}
	if got := find(t, rows, empty); got.cells[0] != "" || got.session != "" {
		t.Errorf("a directory with nothing in it rendered %q / %q", got.cells[0], got.session)
	}
}

// A name has to carry the path, since two directories sharing a basename
// would otherwise share the one session zmx keys by that name.
func TestSessionNameIsThePathDotted(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	for _, c := range []struct{ dir, want string }{
		{filepath.Join(home, "Forge", "mikluko", "notes"), "Forge.mikluko.notes"},
		{filepath.Join(home, "archive", "notes"), "archive.notes"},
		{filepath.Join(home, "notes"), "notes"},
		{home, "home"},
		{"/etc/nginx", "etc.nginx"},
	} {
		if got := sessionName(c.dir); got != c.want {
			t.Errorf("%s named %q, want %q", c.dir, got, c.want)
		}
	}
}

func TestSessionNamesCarryNoSeparator(t *testing.T) {
	for _, dir := range []string{"/etc/nginx", "/var/log/a/b", "/"} {
		if strings.Contains(sessionName(dir), "/") {
			t.Errorf("%s named %q, which zmx cannot hold", dir, sessionName(dir))
		}
	}
}

// A repository and a linked worktree are both places to work, and git marks
// both with a `.git`.
func TestRepositoriesFindsReposAndWorktrees(t *testing.T) {
	root, _ := scene(t)
	found := repositories(root)

	want := []string{
		filepath.Join(root, "Forge", "mikluko", "octant"),
		filepath.Join(root, "Forge", "mikluko", "octant", ".claude", "worktrees", "octant-918-arrival"),
		filepath.Join(root, "Forge", "uptime-com", "api"),
	}
	if strings.Join(found, "\n") != strings.Join(want, "\n") {
		t.Errorf("found:\n%s\nwant:\n%s", strings.Join(found, "\n"), strings.Join(want, "\n"))
	}
}

func TestFoldersListsChildDirectoriesOnly(t *testing.T) {
	root, _ := scene(t)
	got := folders(filepath.Join(root, "Forge"))

	want := []string{
		filepath.Join(root, "Forge", "mikluko"),
		filepath.Join(root, "Forge", "uptime-com"),
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("listed %q, want %q", got, want)
	}
	if len(folders(filepath.Join(root, "Documents", "notes"))) != 0 {
		t.Error("a directory with no subdirectories listed something")
	}
}

func TestSessionRowsFilterToOneDirectory(t *testing.T) {
	root, live := scene(t)
	repo := filepath.Join(root, "Forge", "mikluko", "octant")

	if all := sessionRows(live, ""); len(all) != len(live) {
		t.Errorf("listed %d of %d sessions", len(all), len(live))
	}
	in := sessionRows(live, repo)
	if len(in) != 2 {
		t.Fatalf("the repository holds 2 sessions, listed %d", len(in))
	}
	for _, r := range in {
		if !strings.HasPrefix(r.session, "Forge.mikluko.octant") {
			t.Errorf("%q is not a session in the repository", r.session)
		}
	}
}

// The cursor starts where the pane stands, and the closest directory wins, so
// a worktree answers with itself rather than with the repository above it.
func TestAtTakesTheClosestDirectory(t *testing.T) {
	root, live := scene(t)
	repo := filepath.Join(root, "Forge", "mikluko", "octant")
	tree := filepath.Join(repo, ".claude", "worktrees", "octant-918-arrival")
	rows := dirRows([]string{repo, tree}, live, filepath.Base)

	if got := at(rows, repo); got < 0 || rows[got].dir != repo {
		t.Errorf("the repository selected row %d", got)
	}
	if got := at(rows, filepath.Join(repo, "internal", "deep")); got < 0 || rows[got].dir != repo {
		t.Errorf("a subdirectory selected row %d, want the repository", got)
	}
	if got := at(rows, tree); got < 0 || rows[got].dir != tree {
		t.Errorf("the worktree selected row %d, want the worktree", got)
	}
	if got := at(rows, os.TempDir()); got != -1 {
		t.Errorf("a directory under no row selected row %d, want none", got)
	}
}

// fzf is told to search and show the display only, so the index it hands back
// has to lead every line and the preview's argument follow it.
func TestLinesLeadWithTheIndex(t *testing.T) {
	root, live := scene(t)
	rows := dirRows(repositories(root), live, shorten)

	for i, line := range lines(rows) {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			t.Fatalf("line %d has %d fields, want 3: %q", i, len(parts), line)
		}
		if parts[0] != strconv.Itoa(i) {
			t.Errorf("line %d leads with %q", i, parts[0])
		}
		if parts[1] != rows[i].dir {
			t.Errorf("line %d previews %q, want %q", i, parts[1], rows[i].dir)
		}
	}
}

// tab reaches every view and comes back, and shift-tab undoes it.
func TestModesCycleBothWays(t *testing.T) {
	seen := map[mode]bool{}
	m := modeFolders
	for range 3 {
		seen[m] = true
		m = m.next(true)
	}
	if m != modeFolders || len(seen) != 3 {
		t.Errorf("forward through the views reached %d of 3 and landed on %v", len(seen), m)
	}
	for m := range seen {
		if back := m.next(true).next(false); back != m {
			t.Errorf("%v forward then back landed on %v", m, back)
		}
		if named, ok := modeNamed(m.String()); !ok || named != m {
			t.Errorf("%v does not survive being written down", m)
		}
	}
}

// The preview follows the shape of the space: beside the list where the
// terminal is wide, below it where it is not, and nowhere at all where what
// is left of the list would be unreadable.
func TestPreviewLayoutFollowsTheShapeOfTheTerminal(t *testing.T) {
	for _, c := range []struct {
		cols, rows int
		want       string
	}{
		{200, 50, "right"},
		{120, 40, "right"},
		{119, 40, "up"},
		{80, 24, "up"},
		{79, 40, ""},
		{200, 14, ""},
	} {
		got := previewLayout(c.cols, c.rows)
		switch {
		case c.want == "" && got != "":
			t.Errorf("%dx%d previews as %q, want none", c.cols, c.rows, got)
		case c.want != "" && !strings.HasPrefix(got, c.want):
			t.Errorf("%dx%d previews as %q, want %s", c.cols, c.rows, got, c.want)
		}
	}
	if !strings.Contains(previewLayout(200, 50), "<100(up") {
		t.Error("the wide layout carries no alternative for a narrowed window")
	}
}

func TestAlignPadsToTheWidestCell(t *testing.T) {
	got := align([][]string{
		{"●", "short", "x"},
		{"", "a-much-longer-name", "y"},
	})
	if len(got) != 2 {
		t.Fatalf("aligned %d rows, want 2", len(got))
	}
	// Counted in runes, as the terminal and tabwriter both count: a circle is
	// one column and three bytes.
	if column(got[0], "x") != column(got[1], "y") {
		t.Errorf("columns not aligned: %q / %q", got[0], got[1])
	}
}

// column returns how many columns into line the given cell starts.
func column(line, cell string) int {
	return utf8.RuneCountInString(line[:strings.Index(line, cell)])
}
