package main

import (
	"os"
	"path/filepath"
	"slices"
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

// One cell, whatever stands in the directory: filled where a client is
// attached, ringed where there is more than one session.
func TestCircleSaysAttachedAndPlural(t *testing.T) {
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
		{"two, no client", []Session{none, none}, severalO},
		{"two, one attached", []Session{none, one}, several},
		{"three, one attached", []Session{none, one, none}, several},
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

	// Bright is the repository and dim is the worktree cut from it, which is
	// the whole of what tells them apart.
	if strings.Contains(status(repo, nil), dim) {
		t.Error("a repository was dimmed")
	}
	if !strings.Contains(status(tree, nil), dim) {
		t.Error("a worktree was not dimmed")
	}
	if strings.Contains(status(plain, nil), checkout) {
		t.Error("a directory git knows nothing about was marked as a checkout")
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

// has reports whether any row stands in dir.
func has(rows []row, dir string) bool {
	for _, r := range rows {
		if r.dir != "" && filepath.Clean(r.dir) == filepath.Clean(dir) {
			return true
		}
	}
	return false
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

// A place is its directory under whatever contains it, and a worktree is
// contained by the repository it was cut from rather than by `worktrees`.
func TestPlaceNameIsTheThingUnderItsContainer(t *testing.T) {
	root := t.TempDir()
	repo := repoAt(t, filepath.Join(root, "gateway"))
	tree := worktreeAt(t, repo, "fix-retry",
		filepath.Join(repo, ".claude", "worktrees", "fix-retry"))

	if got, want := placeName(repo, nil), filepath.Base(root)+"/gateway"; got != want {
		t.Errorf("a repository named %q, want %q", got, want)
	}
	if got, want := placeName(tree, nil), "gateway/fix-retry"; got != want {
		t.Errorf("a worktree named %q, want %q", got, want)
	}
	if got := placeName("/etc/nginx", nil); got != "etc/nginx" {
		t.Errorf("a directory outside home named %q", got)
	}
	if got := placeName("/", nil); got != "/" {
		t.Errorf("the root named %q", got)
	}
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

// A checkout goes by what git says it is. The path is where it happens to
// sit: a directory called `uptime-com` can hold `acme/workspace`.
func TestPlaceNameReadsTheRepositoryRatherThanThePath(t *testing.T) {
	root := t.TempDir()
	repo := repoAt(t, filepath.Join(root, "some-directory"))
	tree := worktreeAt(t, repo, "w",
		filepath.Join(repo, ".claude", "worktrees", "api-rebase-517"))
	ident := map[string]string{repo: "acme/api"}

	if got := placeName(repo, ident); got != "acme/api" {
		t.Errorf("a checkout named %q, want what its remote says", got)
	}
	if got := placeName(tree, ident); got != "acme/api/rebase-517" {
		t.Errorf("a worktree named %q", got)
	}
	// Nothing known about it: the path is all there is to go on.
	if got, want := placeName(repo, nil), filepath.Base(root)+"/some-directory"; got != want {
		t.Errorf("a checkout with no remote named %q, want %q", got, want)
	}
}

// A worktree's basename carries its repository's name as a prefix, which is
// how the tools that cut them name them, and repeating it says nothing.
func TestTrimRepoTakesThePrefixOffAWorktree(t *testing.T) {
	for _, c := range []struct{ base, ident, want string }{
		{"api-rebase-517", "acme/api", "rebase-517"},
		{"api-rebase-517", "api", "rebase-517"},
		{"something-else", "acme/api", "something-else"},
		{"api-", "acme/api", "api-"},
		{"api", "acme/api", "api"},
	} {
		if got := trimRepo(c.base, c.ident); got != c.want {
			t.Errorf("trimRepo(%q, %q) = %q, want %q", c.base, c.ident, got, c.want)
		}
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

// Enter does something on every row, so it is advertised on every row. A key
// that would do nothing is not: nothing to kill where nothing is running, and
// no take where nobody else holds it.
func TestHintsOfferOnlyWhatTheKeyWouldDo(t *testing.T) {
	for _, c := range []struct {
		state    string
		want     []string
		wantNone []string
	}{
		{"r", []string{"⏎ start", "⌥⏎ new"}, []string{"⌦ kill", "take", "⇥ mark"}},
		{"s", []string{"⏎ attach", "⌦ kill", "⇥ mark"}, []string{"take"}},
		{"sh", []string{"⏎ take", "⌦ kill"}, []string{"⏎ attach"}},
		{"m", []string{"⏎ in", "⌦ kill"}, []string{"⌃x"}},
		{"mh", []string{"⏎ in", "⌦ kill"}, []string{"⌃x"}},
	} {
		got := hints(c.state)
		for _, want := range c.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s: %q does not offer %q", c.state, got, want)
			}
		}
		for _, none := range c.wantNone {
			if strings.Contains(got, none) {
				t.Errorf("%s: %q offers %q, which would do nothing", c.state, got, none)
			}
		}
	}
}

// The letters the hint line branches on.
func TestRowStateSaysWhatIsRunningAndWhoHasIt(t *testing.T) {
	for _, c := range []struct {
		what string
		r    row
		want string
	}{
		{"a place with nothing running", row{}, "r"},
		{"a session nobody else is in", row{session: "a"}, "s"},
		{"a session another terminal has", row{session: "a", held: true}, "sh"},
		{"a checkout holding two", row{count: 2}, "m"},
		{"a checkout holding one another terminal has",
			row{count: 1, found: []Session{{Clients: "1"}}}, "mh"},
	} {
		if got := c.r.state(); got != c.want {
			t.Errorf("%s: state = %q, want %q", c.what, got, c.want)
		}
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

// The reason to read a list of 114 rather than type into it is to find what
// is running, so what is running leads.
func TestRepoRowsLeadWithWhatIsAlive(t *testing.T) {
	root := t.TempDir()
	quiet := repoAt(t, filepath.Join(root, "aaa"))
	busy := repoAt(t, filepath.Join(root, "zzz"))
	live := []Session{{Name: "one", Dir: busy}}

	got := repoRows(live, []string{quiet, busy}, nil)
	if len(got) != 2 || got[0].dir != busy {
		t.Fatalf("led with %q, want the one holding a session", got[0].dir)
	}
	if got[0].count != 1 || got[1].count != 0 {
		t.Errorf("counts %d / %d", got[0].count, got[1].count)
	}
	if cells := []rune(visible(got[0].status)); cells[1] != '1' {
		t.Errorf("a repository holding one rendered %q", visible(got[0].status))
	}
}

// How much is alive, in the one cell it gets.
func TestTallyCountsWhatIsBelow(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{{0, " "}, {-1, " "}, {1, "1"}, {9, "9"}, {10, "+"}, {40, "+"}} {
		if got := tally(c.n); got != c.want {
			t.Errorf("tally(%d) = %q, want %q", c.n, got, c.want)
		}
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

// A rung narrowed to one place ends with the offer of another session there.
// Without it a pane standing somewhere new has no way to say so: every other
// row belongs to a session that is already running.
func TestANarrowedSessionsRungOffersOneMore(t *testing.T) {
	live := []Session{
		{Name: "here", Clients: "0", Dir: "/a"},
		{Name: "elsewhere", Clients: "0", Dir: "/b"},
	}

	narrowed := liveRows("/a", live, nil)
	if len(narrowed) != 2 {
		t.Fatalf("narrowed to /a gave %d rows, want the session and the offer", len(narrowed))
	}
	if has(narrowed, "/b") {
		t.Error("narrowing to /a kept a session standing in /b")
	}
	last := narrowed[len(narrowed)-1]
	if last.cells[0] != newSession || last.dir != "/a" || last.session != "" {
		t.Errorf("the last row is %q / %q / %q", last.cells[0], last.dir, last.session)
	}
	// Enter on it starts one rather than attaching, which is what `r` says.
	if got := last.state(); got != "r" {
		t.Errorf("the offer reads as %q, want r", got)
	}

	// Every session there is, with nowhere in particular: no offer, because
	// there is no place for it to mean.
	all := liveRows("", live, nil)
	if len(all) != 2 {
		t.Fatalf("unnarrowed gave %d rows, want one per session", len(all))
	}
	for _, r := range all {
		if r.cells[0] == newSession {
			t.Error("the unnarrowed rung offered a session with no place to start it in")
		}
	}
}

// A session row says what it is, what git calls where it is, and where that
// is. The middle earns its column only where it differs from the path:
// `mikluko/workspace-uptime` stands at `~/Forge/uptime-com`, which the name
// does not give up.
func TestWhereCellsDoNotSayThePathTwice(t *testing.T) {
	root := t.TempDir()
	repo := repoAt(t, filepath.Join(root, "checkout"))
	ident := map[string]string{repo: "acme/api"}

	got := whereCells("harbor-41", repo, ident)
	if got[1] != "acme/api" || got[2] != shorten(repo) {
		t.Errorf("a checkout gave %q / %q", got[1], got[2])
	}

	// Home names itself, so the middle would only repeat the path.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	got = whereCells(newSession, home, nil)
	if got[1] != "" {
		t.Errorf("home gave %q in the middle, want it left blank", got[1])
	}
	if got[2] != "~" {
		t.Errorf("home gave %q as the path", got[2])
	}
}

// Narrowing reaches downwards: a directory holds the work under it as much as
// the work in it, and a worktree is below the repository it was cut from. A
// rung that stopped at an exact match would disagree with the count on the
// repos rung, which already reaches down.
func TestANarrowedSessionsRungReachesDownwards(t *testing.T) {
	live := []Session{
		{Name: "at-the-root", Clients: "0", Dir: "/a"},
		{Name: "in-a-worktree", Clients: "0", Dir: "/a/.claude/worktrees/a-fix"},
		{Name: "deeper-still", Clients: "0", Dir: "/a/b/c"},
		{Name: "a-prefix-not-a-path", Clients: "0", Dir: "/ab"},
		{Name: "elsewhere", Clients: "0", Dir: "/b"},
	}

	got := liveRows("/a", live, nil)
	var names []string
	for _, r := range got {
		if r.session != "" {
			names = append(names, r.session)
		}
	}
	slices.Sort(names)
	if want := "at-the-root,deeper-still,in-a-worktree"; strings.Join(names, ",") != want {
		t.Errorf("narrowed to /a gave %q, want %q", strings.Join(names, ","), want)
	}
	// The offer is for the place itself, not for anywhere below it.
	last := got[len(got)-1]
	if last.cells[0] != newSession || last.dir != "/a" {
		t.Errorf("the offer is %q in %q", last.cells[0], last.dir)
	}
}
