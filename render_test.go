package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// scene builds a root holding one repository, one of its worktrees and a
// second repository, and returns sessions sitting in those plus one outside
// any repository.
func scene(t *testing.T) (root string, found []Session) {
	t.Helper()
	root = t.TempDir()
	repo := repoAt(t, filepath.Join(root, "mikluko", "octant"))
	tree := worktreeAt(t, repo, "octant-918-arrival",
		filepath.Join(root, "mikluko", "octant", "tmp", "worktrees", "octant-918-arrival"))
	other := repoAt(t, filepath.Join(root, "uptime-com", "api"))
	loose := t.TempDir()

	return root, []Session{
		{Name: "mikluko.octant", Clients: "1", Dir: repo},
		{Name: "mikluko.octant+review", Clients: "0", Dir: repo},
		{Name: "mikluko.octant@918-arrival", Clients: "0", Dir: tree},
		{Name: "supa-7f3a", Clients: "2", Dir: other},
		{Name: "scratch", Clients: "0", Dir: loose},
	}
}

// find returns the row for a target name.
func find(t *testing.T, rows []row, name string) row {
	t.Helper()
	for _, r := range rows {
		if r.name == name {
			return r
		}
	}
	t.Fatalf("no row for %q", name)
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
// tool never named still counts against the repository it sits in.
func TestDirRowsCountSessionsByDirectory(t *testing.T) {
	root, live := scene(t)
	rows := dirRows(targets(root), live)

	octant := find(t, rows, "mikluko.octant")
	if octant.cells[0] != attached+detached {
		t.Errorf("two sessions in the repository rendered %q", octant.cells[0])
	}
	if !octant.several {
		t.Error("two sessions is more than enter can choose between")
	}
	if octant.session != "" {
		t.Errorf("a directory with two sessions named one to attach: %q", octant.session)
	}

	api := find(t, rows, "uptime-com.api")
	if api.cells[0] != attached {
		t.Errorf("supacode's session rendered %q, want one filled circle", api.cells[0])
	}
	if api.session != "supa-7f3a" {
		t.Errorf("enter would attach %q, want the session that is there", api.session)
	}

	tree := find(t, rows, "mikluko.octant@918-arrival")
	if tree.cells[0] != detached {
		t.Errorf("the worktree rendered %q, want one hollow circle", tree.cells[0])
	}
}

// A repository with nothing running is where a session gets created, under
// the name the target carries.
func TestDirRowsLeaveEmptyDirectoriesToBeCreated(t *testing.T) {
	root, _ := scene(t)
	rows := dirRows(targets(root), nil)

	for _, r := range rows {
		if r.cells[0] != "" {
			t.Errorf("%s rendered %q with no sessions anywhere", r.name, r.cells[0])
		}
		if r.session != "" || r.several {
			t.Errorf("%s would attach %q rather than create", r.name, r.session)
		}
		if r.dir == "" {
			t.Errorf("%s has nowhere to be created", r.name)
		}
	}
}

func TestSessionRowsFilterToOneDirectory(t *testing.T) {
	root, live := scene(t)
	repo := filepath.Join(root, "mikluko", "octant")

	all := sessionRows(live, "")
	if len(all) != len(live) {
		t.Errorf("listed %d of %d sessions", len(all), len(live))
	}

	in := sessionRows(live, repo)
	if len(in) != 2 {
		t.Fatalf("the repository holds 2 sessions, listed %d", len(in))
	}
	for _, r := range in {
		if !strings.HasPrefix(r.session, "mikluko.octant") {
			t.Errorf("%q is not a session in the repository", r.session)
		}
		if r.dir != "" {
			t.Errorf("%q carries a directory; it is attached where it runs", r.session)
		}
	}
}

// The cursor starts on the directory the pane was opened in, and a worktree
// answers with itself rather than with the repository above it.
func TestAtTakesTheClosestDirectory(t *testing.T) {
	root, live := scene(t)
	rows := dirRows(targets(root), live)
	repo := filepath.Join(root, "mikluko", "octant")
	tree := filepath.Join(repo, "tmp", "worktrees", "octant-918-arrival")

	if got := at(rows, repo); got < 0 || rows[got].name != "mikluko.octant" {
		t.Errorf("the repository selected row %d", got)
	}
	if got := at(rows, filepath.Join(repo, "internal", "deep")); got < 0 || rows[got].name != "mikluko.octant" {
		t.Errorf("a subdirectory selected row %d, want the repository", got)
	}
	if got := at(rows, tree); got < 0 || rows[got].name != "mikluko.octant@918-arrival" {
		t.Errorf("the worktree selected row %d, want the worktree", got)
	}
	if got := at(rows, os.TempDir()); got != -1 {
		t.Errorf("a directory under no target selected row %d, want none", got)
	}
	if got := at(rows, ""); got != -1 {
		t.Errorf("no directory selected row %d, want none", got)
	}
}

// fzf is told to search and show the display only, so the index it hands back
// has to lead every line and the preview's argument follow it.
func TestLinesLeadWithTheIndex(t *testing.T) {
	root, live := scene(t)
	rows := dirRows(targets(root), live)

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

func TestTargetsListsRepositoriesAndTheirWorktrees(t *testing.T) {
	root, _ := scene(t)
	var names []string
	for _, tgt := range targets(root) {
		names = append(names, tgt.Name)
	}
	want := []string{"mikluko.octant", "mikluko.octant@918-arrival", "uptime-com.api"}
	if strings.Join(names, " ") != strings.Join(want, " ") {
		t.Errorf("targets are %q, want %q", names, want)
	}
}

// zmx names each session's socket after it, so a name carrying a separator
// creates nothing and says nothing.
func TestTargetsNamesCarryNoSeparator(t *testing.T) {
	root, _ := scene(t)
	for _, tgt := range targets(root) {
		if strings.Contains(tgt.Name, "/") {
			t.Errorf("%q carries a separator", tgt.Name)
		}
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
