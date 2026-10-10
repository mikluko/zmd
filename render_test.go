package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-git/go-git/v5/config"
)

// scene builds a tree holding two repositories, a worktree of one of them and
// a directory that is neither, and returns sessions sitting in some of them.
func scene(t *testing.T) (root string, found []Session) {
	t.Helper()
	root = t.TempDir()
	repo := repoAt(t, filepath.Join(root, "src", "acme", "engine"))
	tree := worktreeAt(t, repo, "engine-918",
		filepath.Join(root, "src", "acme", "engine", ".claude", "worktrees", "engine-918"))
	other := repoAt(t, filepath.Join(root, "src", "acme", "api"))
	plain := filepath.Join(root, "Documents", "notes")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	return root, []Session{
		{Name: "src.acme.engine", Clients: "1", Dir: repo},
		{Name: "src.acme.engine+review", Clients: "0", Dir: repo},
		{Name: "supa-7f3a", Clients: "2", Dir: other},
		{Name: "Documents.notes", Clients: "0", Dir: plain},
		{Name: "loose", Clients: "0", Dir: t.TempDir()},
		{Name: "tree", Clients: "0", Dir: tree},
	}
}

// One cell, however many stand there: filled where every one has a client
// attached, hollow where none has, half filled where some have, blank where
// there are none.
func TestCircleSaysWhetherAnyIsAttached(t *testing.T) {
	one, none, some := Session{Clients: "1"}, Session{Clients: "0"}, Session{Clients: ""}
	for _, c := range []struct {
		what  string
		found []Session
		want  string
	}{
		{"nothing there", nil, " "},
		{"one, no client", []Session{none}, detached},
		{"one, a client", []Session{one}, attached},
		{"one, no count at all", []Session{some}, detached},
		{"two, no client", []Session{none, none}, detached},
		{"three, one attached", []Session{none, one, none}, mixed},
		{"two, both attached", []Session{one, one}, attached},
	} {
		if got := circle(c.found); got != c.want {
			t.Errorf("%s: rendered %q, want %q", c.what, got, c.want)
		}
	}
}

// Three cells wide whatever it says, or every name below it steps sideways.
// Escapes are not cells, so the count is of what the terminal draws.
func TestStatusIsAlwaysThreeCells(t *testing.T) {
	root := t.TempDir()
	repo := repoAt(t, filepath.Join(root, "engine"))
	tree := worktreeAt(t, repo, "engine-918", filepath.Join(root, "elsewhere", "engine-918"))
	plain := filepath.Join(root, "notes")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ what, dir string }{
		{"a repository", repo},
		{"a worktree", tree},
		{"neither", plain},
	} {
		for _, found := range [][]Session{nil, {{Clients: "1"}}, {{Clients: "0"}, {Clients: "1"}}} {
			got := status(c.dir, found)
			if n := utf8.RuneCountInString(visible(got)); n != statusWidth {
				t.Errorf("%s with %d sessions: %q is %d cells, want %d",
					c.what, len(found), got, n, statusWidth)
			}
		}
	}

}

// visible strips the SGR sequences, leaving what occupies a cell.
func visible(s string) string {
	for {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			return s
		}
		j := strings.IndexByte(s[i:], 'm')
		if j < 0 {
			return s
		}
		s = s[:i] + s[i+j+1:]
	}
}

// A prefix is not a path: `/a/bc` starts with `/a/b` and stands nowhere
// below it.
func TestUnderIsAPathAndNotAPrefix(t *testing.T) {
	for _, c := range []struct {
		base, path string
		want       bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/b/c", true},
		{"/a/b", "/a/bc", false},
		{"/a/b", "/a", false},
		{"", "/a", false},
	} {
		if got := under(c.base, c.path); got != c.want {
			t.Errorf("under(%q, %q) = %v, want %v", c.base, c.path, got, c.want)
		}
	}
}

// A worktree and the repository it was cut from answer with the same root,
// which is what lets a scope standing in one reach the other.
func TestRepoRootAnswersWithTheRepositoryAWorktreeWasCutFrom(t *testing.T) {
	root := t.TempDir()
	repo := repoAt(t, filepath.Join(root, "engine"))
	tree := worktreeAt(t, repo, "engine-918", filepath.Join(root, "elsewhere", "engine-918"))
	plain := filepath.Join(root, "notes")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ what, dir, want string }{
		{"a repository", repo, repo},
		{"a directory inside one", filepath.Join(repo, "internal"), repo},
		{"a worktree", tree, repo},
		{"a directory that is neither", plain, ""},
	} {
		if got := repoRoot(c.dir); got != c.want {
			t.Errorf("%s: repoRoot = %q, want %q", c.what, got, c.want)
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
	// A plugin cache under ~/.claude is a repository nobody works in; the
	// walk enters .claude only for the worktrees kept there.
	repoAt(t, filepath.Join(root, ".claude", "plugins", "cache", "plugin"))
	found := repositories(root)

	// Sorted, so acme leads acme and a worktree follows the repository
	// it sits under.
	want := []string{
		filepath.Join(root, "src", "acme", "api"),
		filepath.Join(root, "src", "acme", "engine"),
		filepath.Join(root, "src", "acme", "engine", ".claude", "worktrees", "engine-918"),
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
		want       layout
	}{
		{200, 50, layoutBeside},
		{120, 40, layoutBeside},
		{119, 40, layoutAbove},
		{80, 24, layoutAbove},
		{79, 40, layoutNone},
		{200, 14, layoutNone},
	} {
		if got := previewLayout(c.cols, c.rows); got != c.want {
			t.Errorf("%dx%d previews as %d, want %d", c.cols, c.rows, got, c.want)
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

// A name is an address. It has to be unique among what is running, say where
// it came from, and carry none of the separator zmx cannot hold in a socket
// name.
func TestSessionNameIsAFreeAddress(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	var live []Session
	seen := map[string]bool{}
	for range 200 {
		name := sessionName(live)
		if seen[name] {
			t.Fatalf("%q was handed out twice", name)
		}
		if strings.ContainsAny(name, "/ \t") {
			t.Fatalf("%q carries what zmx cannot hold in a name", name)
		}
		if !strings.HasPrefix(name, "zmd.ghostty.") {
			t.Fatalf("%q does not say what made it or where", name)
		}
		seen[name] = true
		live = append(live, Session{Name: name})
	}
}

// The label says what a session is for and the name says nothing, so the name
// is the fallback rather than the answer.
func TestSessionLabelPrefersWhatTheLabelsSay(t *testing.T) {
	if got := sessionLabel(Session{Name: "harbor-41", Title: "retry-bug"}); got != "retry-bug" {
		t.Errorf("a labelled session read as %q", got)
	}
	if got := sessionLabel(Session{Name: "harbor-41"}); got != "harbor-41" {
		t.Errorf("an unlabelled session read as %q, want its name", got)
	}
}

// Both forms git writes carry the same two trailing segments.
func TestOwnerNameReadsBothRemoteForms(t *testing.T) {
	for _, c := range []struct{ url, want string }{
		{"https://github.com/acme/api.git", "acme/api"},
		{"git@github.com:acme/api.git", "acme/api"},
		{"git@github.com:acme/api", "acme/api"},
		{"https://gitlab.com/group/sub/api.git", "sub/api"},
		{"/srv/mirrors/api.git", "mirrors/api"},
		{"api.git", ""},
		{"", ""},
	} {
		got := ownerName(&config.RemoteConfig{URLs: []string{c.url}})
		if got != c.want {
			t.Errorf("ownerName(%q) = %q, want %q", c.url, got, c.want)
		}
	}
	if got := ownerName(&config.RemoteConfig{}); got != "" {
		t.Errorf("a remote with no URL gave %q", got)
	}
}

// go-git refuses a repository whose format version carries an extension it
// does not implement, and git sets `worktreeconfig` on exactly the ones
// worktrees have been cut from. The config file is read instead.
func TestIdentityReadsARepositoryGoGitWouldRefuseToOpen(t *testing.T) {
	dir := repoAt(t, filepath.Join(t.TempDir(), "checkout"))
	write(t, filepath.Join(dir, ".git", "config"), `[core]
	repositoryformatversion = 1
[extensions]
	worktreeconfig = true
[remote "origin"]
	url = git@github.com:acme/api.git
`)
	if got := identity(dir); got != "acme/api" {
		t.Errorf("identity = %q, want %q", got, "acme/api")
	}
}

// A checkout with one remote under another name still knows what it is.
func TestIdentityFallsPastOrigin(t *testing.T) {
	dir := repoAt(t, filepath.Join(t.TempDir(), "checkout"))
	write(t, filepath.Join(dir, ".git", "config"), `[remote "upstream"]
	url = https://github.com/acme/api.git
`)
	if got := identity(dir); got != "acme/api" {
		t.Errorf("identity = %q, want %q", got, "acme/api")
	}
	if got := identity(filepath.Join(t.TempDir(), "nothing")); got != "" {
		t.Errorf("a directory with no config gave %q", got)
	}
}

// The middle segment is the terminal the session was made in, read rather
// than assumed, so the name stays true where the deck runs somewhere else.
func TestTerminalNamesWhatTheSessionWasMadeIn(t *testing.T) {
	for _, c := range []struct{ env, want string }{
		{"ghostty", "ghostty"},
		{"Apple_Terminal", "appleterminal"},
		{"iTerm.app", "itermapp"},
		{"", "term"},
		{"!!!", "term"},
	} {
		t.Setenv("TERM_PROGRAM", c.env)
		if got := terminal(); got != c.want {
			t.Errorf("TERM_PROGRAM=%q gave %q, want %q", c.env, got, c.want)
		}
	}
}

// Four bytes, hex, and never the same twice in a row by accident.
func TestShortIDIsEightHexCharacters(t *testing.T) {
	seen := map[string]bool{}
	for range 500 {
		id := shortID()
		if len(id) != 8 {
			t.Fatalf("%q is %d characters, want 8", id, len(id))
		}
		if strings.Trim(id, "0123456789abcdef") != "" {
			t.Fatalf("%q is not hex", id)
		}
		seen[id] = true
	}
	if len(seen) < 450 {
		t.Errorf("500 draws gave %d distinct ids", len(seen))
	}
}
