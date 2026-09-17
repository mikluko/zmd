package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	fzf "github.com/junegunn/fzf/src"
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

// find returns the row of the given kind standing for dir.
func find(t *testing.T, rows []row, k kind, dir string) row {
	t.Helper()
	for _, r := range rows {
		if r.kind == k && filepath.Clean(r.dir) == filepath.Clean(dir) {
			return r
		}
	}
	t.Fatalf("no %s row for %q", k, dir)
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
func TestRepoRowsCountSessionsByDirectory(t *testing.T) {
	root, live := scene(t)
	repo := filepath.Join(root, "Forge", "mikluko", "octant")
	api := filepath.Join(root, "Forge", "uptime-com", "api")
	empty := filepath.Join(root, "Documents")

	rows := repoRows([]string{repo, api, empty}, live)

	if got := find(t, rows, kindRepo, repo); got.cells[0] != attached+detached {
		t.Errorf("two sessions rendered %q", got.cells[0])
	} else if !got.several || got.session != "" {
		t.Errorf("two sessions named %q to attach, want the ambiguity kept", got.session)
	}
	if got := find(t, rows, kindRepo, api); got.session != "supa-7f3a" || got.cells[0] != attached {
		t.Errorf("a session named elsewhere rendered %q / %q", got.cells[0], got.session)
	}
	if got := find(t, rows, kindRepo, empty); got.cells[0] != "" || got.session != "" {
		t.Errorf("a directory with nothing in it rendered %q / %q", got.cells[0], got.session)
	}
}

// The name cell is the basename and the path cell is the parent, so a
// repository is not written twice on its own line.
func TestRepoRowsSplitTheNameFromTheParent(t *testing.T) {
	root, live := scene(t)
	repo := filepath.Join(root, "Forge", "mikluko", "octant")

	got := find(t, repoRows([]string{repo}, live), kindRepo, repo)
	if got.cells[1] != repoMark+"octant" {
		t.Errorf("named %q, want the basename marked as a repository", got.cells[1])
	}
	if got.cells[2] != shorten(filepath.Dir(repo)) {
		t.Errorf("placed at %q, want the parent %q", got.cells[2], shorten(filepath.Dir(repo)))
	}
}

func TestSessionRowsListEveryRunningSession(t *testing.T) {
	_, live := scene(t)
	rows := sessionRows(live)

	if len(rows) != len(live) {
		t.Fatalf("listed %d of %d sessions", len(rows), len(live))
	}
	for i, r := range rows {
		if r.kind != kindSession || r.session == "" || r.dir == "" {
			t.Errorf("row %d is %v / %q / %q", i, r.kind, r.session, r.dir)
		}
		if i > 0 && rows[i-1].session > r.session {
			t.Errorf("row %d (%q) follows %q, which is out of name order", i, r.session, rows[i-1].session)
		}
	}
}

// Sessions lead so the cursor, which takes the closest directory, answers a
// directory that has one with the session rather than with the repository.
func TestListLeadsWithSessions(t *testing.T) {
	root, live := scene(t)
	repo := filepath.Join(root, "Forge", "mikluko", "octant")
	rows := list(live, repositories(root))

	for i, r := range rows[:len(live)] {
		if r.kind != kindSession {
			t.Fatalf("row %d is a %v, want the sessions first", i, r.kind)
		}
	}
	got := at(rows, repo)
	if got < 0 || rows[got].kind != kindSession {
		t.Errorf("the cursor landed on row %d, want a session in the repository", got)
	}
}

// The cursor starts where the pane stands, and the closest directory wins, so
// a worktree answers with itself rather than with the repository above it.
func TestAtTakesTheClosestDirectory(t *testing.T) {
	root, live := scene(t)
	repo := filepath.Join(root, "Forge", "mikluko", "octant")
	tree := filepath.Join(repo, ".claude", "worktrees", "octant-918-arrival")
	rows := repoRows([]string{repo, tree}, live)

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
// leads every line, and the kind and the preview's argument follow it.
func TestLinesCarryTheIndexTheKindAndTheTarget(t *testing.T) {
	root, live := scene(t)
	rows := list(live, repositories(root))

	for i, line := range lines(rows) {
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 {
			t.Fatalf("line %d has %d fields, want 4: %q", i, len(parts), line)
		}
		if parts[0] != strconv.Itoa(i) {
			t.Errorf("line %d leads with %q", i, parts[0])
		}
		if parts[1] != string(rows[i].kind) {
			t.Errorf("line %d is a %q, want %q", i, parts[1], rows[i].kind)
		}
		want := rows[i].session
		if rows[i].kind == kindRepo {
			want = rows[i].dir
		}
		if parts[2] != want {
			t.Errorf("line %d previews %q, want %q", i, parts[2], want)
		}
	}
}

// narrow answers a directory holding several sessions with a query rather
// than with a second list, which holds only because a session's line ends
// with the path it runs in: fzf anchors a `$` term to the end of the line.
func TestNarrowReachesTheSessionsInTheDirectoryAndNothingElse(t *testing.T) {
	root, live := scene(t)
	repo := filepath.Join(root, "Forge", "mikluko", "octant")
	anchored := strings.TrimSuffix(narrow(repo), "$")

	var matched []string
	for _, line := range lines(list(live, repositories(root))) {
		display := strings.SplitN(line, "\t", 4)[3]
		if strings.HasSuffix(display, anchored) {
			matched = append(matched, display)
		}
	}

	if len(matched) != 2 {
		t.Fatalf("the anchor matched %d lines, want the repository's 2 sessions:\n%s",
			len(matched), strings.Join(matched, "\n"))
	}
	for _, line := range matched {
		if !strings.Contains(line, "Forge.mikluko.octant") {
			t.Errorf("matched %q, which is not a session in the repository", line)
		}
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

// A query that matched nothing is the name of a session to start, and a query
// of nothing but whitespace is not a name.
func TestNamedTurnsAQueryIntoASessionName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"scratch", "scratch"},
		{"  scratch  ", "scratch"},
		{"mikluko/octant", "mikluko.octant"},
		{"   ", ""},
		{"", ""},
	} {
		if got := named(c.in); got != c.want {
			t.Errorf("named(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Killing the session this runs inside takes the process down with it, so
// anything ordered after it would never be reached.
func TestKillOrderPutsTheCurrentSessionLast(t *testing.T) {
	got := killOrder([]string{"a", "here", "b"}, "here")
	if strings.Join(got, ",") != "a,b,here" {
		t.Errorf("ordered %q", got)
	}
	if got := killOrder([]string{"a", "b"}, ""); strings.Join(got, ",") != "a,b" {
		t.Errorf("outside a session, ordered %q", got)
	}
	if got := killOrder([]string{"a", "b"}, "elsewhere"); strings.Join(got, ",") != "a,b" {
		t.Errorf("in an unlisted session, ordered %q", got)
	}
}

func TestQuestionCountsWhatAKillTakes(t *testing.T) {
	if got := question([]string{"one"}); got != "kill one?" {
		t.Errorf("one session asked %q", got)
	}
	if got := question([]string{"one", "two"}); got != "kill 2 sessions?" {
		t.Errorf("two sessions asked %q", got)
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

// Every key binding, preview window and expected key is a string fzf parses
// rather than anything the compiler sees, so a typo in one surfaces only when
// the picker is opened. Parsing them here is what opens it.
func TestThePickerArgumentsParse(t *testing.T) {
	for _, c := range []struct {
		what string
		args []string
	}{
		{"opening on a row, beside a preview", args("", 12, previewLayout(200, 50))},
		{"opening on a row, below one", args("", 12, previewLayout(100, 40))},
		{"reopened under a query, with no preview", args(narrow("/tmp/x"), -1, "")},
		{"a question", confirmArgs("kill one?")},
	} {
		if _, err := fzf.ParseOptions(true, c.args); err != nil {
			t.Errorf("%s: %v", c.what, err)
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
