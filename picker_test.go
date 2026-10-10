package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// sceneModel is a picker over scene with home at its root, opened on the
// given view, with titles on two of the engine's sessions.
func sceneModel(t *testing.T, open mode) (*pickModel, string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root, live := scene(t)
	for i := range live {
		switch live[i].Name {
		case "src.acme.engine":
			live[i].Title = "retry-bug"
		case "src.acme.engine+review":
			live[i].Title = "review-pr"
		}
	}
	engine, tree, api, _ := sceneDirs(root)
	m := newPickModel(live, []string{engine, tree, api}, root, visits{}, map[string]int64{}, open)
	return m, root
}

func press(m *pickModel, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		m.Update(k)
	}
}

var (
	keyRight = tea.KeyPressMsg{Code: tea.KeyRight}
	keyLeft  = tea.KeyPressMsg{Code: tea.KeyLeft}
	keyBack  = tea.KeyPressMsg{Code: tea.KeyBackspace}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyMark  = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyTab   = tea.KeyPressMsg{Code: tea.KeyTab}
	keyDel   = tea.KeyPressMsg{Code: tea.KeyDelete}
)

func typed(s string) []tea.KeyPressMsg {
	var out []tea.KeyPressMsg
	for _, r := range s {
		out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return out
}

// labels is what the visible lines say first.
func labels(m *pickModel) []string {
	p := m.top()
	var out []string
	for _, ln := range p.lines {
		out = append(out, p.rows[ln.row].cells[0])
	}
	return out
}

func seekLabel(t *testing.T, m *pickModel, label string) {
	t.Helper()
	for i, l := range labels(m) {
		if l == label {
			m.top().cursor = i
			return
		}
	}
	t.Fatalf("no line %q in %q", label, labels(m))
}

// Entering a directory stands in it; `..` stands in the one above, with the
// cursor back on the directory just left.
func TestEnterStandsInADirectoryAndDotDotLeavesIt(t *testing.T) {
	m, root := sceneModel(t, modeBrowse)
	seekLabel(t, m, "src")
	press(m, keyEnter)
	if got := m.panes[modeBrowse].root; got != filepath.Join(root, "src") {
		t.Fatalf("standing in %q, want src", got)
	}
	if !strings.HasSuffix(m.query.Prompt, "src> ") {
		t.Errorf("prompt %q does not say where", m.query.Prompt)
	}
	seekLabel(t, m, "..")
	press(m, keyEnter)
	if got := m.panes[modeBrowse].root; got != root {
		t.Fatalf("after ..: standing in %q, want %q", got, root)
	}
	if r := m.current(); r == nil || r.cells[0] != "src" {
		t.Errorf("cursor on %+v, want src", r)
	}
}

// Standing in a directory shows its sessions to attach, and the offer starts
// one there.
func TestEnterAttachesASessionAndStartsAnOffer(t *testing.T) {
	m, root := sceneModel(t, modeBrowse)
	engine, _, _, _ := sceneDirs(root)
	m.stand(m.top(), engine)
	seekLabel(t, m, "review-pr")
	if _, cmd := m.Update(keyEnter); cmd == nil || m.result.name != "src.acme.engine+review" {
		t.Errorf("enter on a session: result %+v", m.result)
	}

	m, _ = sceneModel(t, modeBrowse)
	m.stand(m.top(), engine)
	seekLabel(t, m, newSession)
	m.Update(keyEnter)
	if m.result.dir != engine || !strings.HasPrefix(m.result.name, "zmd.") {
		t.Errorf("enter on the offer: result %+v", m.result)
	}
}

// A query narrows what is below to its matches, and the way up and the offer
// are not among them.
func TestAQueryMatchesWhatIsBelow(t *testing.T) {
	m, _ := sceneModel(t, modeBrowse)
	press(m, typed("acme/api")...)
	got := labels(m)
	if len(got) == 0 || got[0] != "src/acme/api" {
		t.Errorf("lines %q, want src/acme/api first", got)
	}
	for _, l := range got {
		if l == newSession || l == ".." {
			t.Errorf("%q survives a query", l)
		}
	}
}

// The arrows edit the query and nothing else.
func TestArrowsOnlyEditTheQuery(t *testing.T) {
	m, _ := sceneModel(t, modeBrowse)
	press(m, typed("acm")...)
	lines := len(m.top().lines)
	press(m, keyLeft, keyRight)
	if m.query.Value() != "acm" || len(m.top().lines) != lines {
		t.Errorf("after arrows: query %q, %d lines", m.query.Value(), len(m.top().lines))
	}
	press(m, keyBack, keyBack, keyBack, keyBack)
	if m.query.Value() != "" {
		t.Errorf("backspace leaves %q", m.query.Value())
	}
}

// Tab cycles checkouts, sessions and browse, each keeping its own query, and
// the view last switched to is remembered.
func TestTabCyclesTheViewsAndRemembersIt(t *testing.T) {
	m, _ := sceneModel(t, modeCheckouts)
	press(m, typed("eng")...)
	for _, want := range []mode{modeSessions, modeBrowse, modeCheckouts} {
		press(m, keyTab)
		if m.mode != want {
			t.Fatalf("after tab: %v, want %v", m.mode, want)
		}
	}
	if m.query.Value() != "eng" {
		t.Errorf("checkouts query %q, want eng back", m.query.Value())
	}
	press(m, keyTab)
	if loadMode() != modeSessions {
		t.Error("the view switched to is not remembered")
	}
	if got := len(m.top().rows); got != len(m.live) {
		t.Errorf("the sessions view lists %d, want every one of %d", got, len(m.live))
	}
}

// Entering a checkout with no session starts one there.
func TestEnterOnAnIdleCheckoutStartsASessionThere(t *testing.T) {
	m, root := sceneModel(t, modeCheckouts)
	_, _, api, _ := sceneDirs(root)
	var rest []Session
	for _, s := range m.live {
		if s.Dir != api {
			rest = append(rest, s)
		}
	}
	m.relive(rest)
	rel := shorten(api)
	seekLabel(t, m, rel)
	m.Update(keyEnter)
	if m.result.dir != api || !strings.HasPrefix(m.result.name, "zmd.") {
		t.Errorf("enter on an idle checkout: result %+v", m.result)
	}
}

// Entering a checkout holding sessions narrows the sessions view to them, the
// offer of another last; tab goes back to the checkouts and widens it again.
func TestEnterOnABusyCheckoutListsItsSessions(t *testing.T) {
	m, root := sceneModel(t, modeCheckouts)
	engine, _, _, _ := sceneDirs(root)
	rel := shorten(engine)
	seekLabel(t, m, rel)
	m.Update(keyEnter)
	if m.mode != modeSessions || m.result.name != "" {
		t.Fatalf("after enter: %v, result %+v", m.mode, m.result)
	}
	got := labels(m)
	if len(got) != 3 || got[2] != newSession {
		t.Fatalf("lines %q, want engine's two sessions and the offer", got)
	}
	seekLabel(t, m, newSession)
	m.Update(keyEnter)
	if m.result.dir != engine {
		t.Errorf("the offer starts in %q, want %q", m.result.dir, engine)
	}

	m, root = sceneModel(t, modeCheckouts)
	engine, _, _, _ = sceneDirs(root)
	seekLabel(t, m, shorten(engine))
	m.Update(keyEnter)
	press(m, keyTab)
	if m.mode != modeCheckouts || m.panes[modeSessions].scope != "" {
		t.Errorf("tab from the narrowed sessions: %v, scope %q", m.mode, m.panes[modeSessions].scope)
	}
}

// Marking a directory marks every session in it or below it for a kill.
func TestKillTargetsPreferMarks(t *testing.T) {
	m, _ := sceneModel(t, modeBrowse)
	seekLabel(t, m, "src/acme/engine")
	press(m, keyMark)
	if got := m.killTargets(); len(got) != 3 {
		t.Errorf("targets %v, want engine's three", got)
	}
}

// A kill asks by what the row shows, its title, not by a name it hides.
func TestAKillAsksByTitle(t *testing.T) {
	m, _ := sceneModel(t, modeSessions)
	seekLabel(t, m, "review-pr")
	press(m, keyDel)
	if m.asking != "kill review-pr?" {
		t.Errorf("asked %q", m.asking)
	}
}

// Escape leaves, from wherever the cursor is.
func TestEscLeavesWithoutAttaching(t *testing.T) {
	m, _ := sceneModel(t, modeBrowse)
	press(m, typed("acm")...)
	_, cmd := m.Update(keyEsc)
	if cmd == nil || m.result.name != "" {
		t.Errorf("esc with nothing held: cmd %v result %+v", cmd, m.result)
	}
}

// Any answer but yes is no, escape included, and the picker stays up.
func TestADeclinedKillKeepsThePicker(t *testing.T) {
	m, _ := sceneModel(t, modeSessions)
	press(m, keyDel)
	if m.asking == "" {
		t.Fatal("delete did not ask")
	}
	_, cmd := m.Update(keyEsc)
	if cmd != nil || m.asking != "" {
		t.Errorf("after escape: cmd %v asking %q", cmd, m.asking)
	}
}

// A kill rebuilds every view and keeps the cursor where it was.
func TestKillKeepsCursorOnSurvivor(t *testing.T) {
	m, _ := sceneModel(t, modeSessions)
	seekLabel(t, m, "review-pr")
	var survivors []Session
	for _, s := range m.live {
		if s.Name != "src.acme.engine" {
			survivors = append(survivors, s)
		}
	}
	m.Update(killedMsg{live: survivors})
	if r := m.current(); r == nil || r.session != "src.acme.engine+review" {
		t.Errorf("cursor after kill on %+v", r)
	}
}

// A session started elsewhere shows once two readings agree, and the cursor
// stays on the row it was on.
func TestARefreshShowsSessionsStartedElsewhere(t *testing.T) {
	m, _ := sceneModel(t, modeSessions)
	seekLabel(t, m, "review-pr")
	live := append(append([]Session(nil), m.live...), Session{Name: "fresh", Clients: "0", Dir: "/", Created: 1})
	m.Update(liveMsg{live: live})
	m.Update(liveMsg{live: live})
	if r := m.current(); r == nil || r.session != "src.acme.engine+review" {
		t.Errorf("cursor after refresh on %+v", r)
	}
	if got := len(m.top().rows); got != len(live) {
		t.Errorf("%d sessions listed after one started, want %d", got, len(live))
	}
}

// The sessions view opens on the session the pane last held.
func TestOpeningLandsOnTheHeldSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root, live := scene(t)
	recordLast("src.acme.engine+review")
	m := newPickModel(live, nil, root, visits{}, map[string]int64{}, modeSessions)
	m.opening()
	if r := m.current(); r == nil || r.session != "src.acme.engine+review" {
		t.Errorf("opened on %+v", r)
	}
}

// A session's scrollback opens at its end and a directory's history at its
// start.
func TestPreviewOpensWhereItIsRead(t *testing.T) {
	m, _ := sceneModel(t, modeSessions)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 22})
	long := strings.Repeat("line\n", 100)
	seekLabel(t, m, "retry-bug")
	m.Update(previewMsg{seq: m.seq, text: long})
	if !m.preview.AtBottom() {
		t.Errorf("a session previews from offset %d, want its end", m.preview.YOffset())
	}
	press(m, keyTab)
	seekLabel(t, m, "src")
	m.Update(previewMsg{seq: m.seq, text: long})
	if !m.preview.AtTop() {
		t.Errorf("a directory previews from offset %d, want its start", m.preview.YOffset())
	}
}

// Scrolling past an end is not banked: the first key back moves the preview.
func TestScrollPreviewStopsAtTheEnds(t *testing.T) {
	m, _ := sceneModel(t, modeSessions)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 22})
	seekLabel(t, m, "retry-bug")
	m.Update(previewMsg{seq: m.seq, text: strings.Repeat("line\n", 100)})
	end := m.preview.YOffset()
	press(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt})
	if m.preview.YOffset() != end {
		t.Errorf("scrolled past the end: %d, want %d", m.preview.YOffset(), end)
	}
	press(m, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt}, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt})
	if m.preview.YOffset() != end-2 {
		t.Errorf("two lines up: %d, want %d", m.preview.YOffset(), end-2)
	}
}

// The thumb runs from the top of the column to its bottom as the view does
// through the lines, and there is none where every line fits.
func TestScrollbar(t *testing.T) {
	thumb := func(bar []string) (first, n int) {
		first = -1
		for i, c := range bar {
			if strings.Contains(c, "│") {
				if first < 0 {
					first = i
				}
				n++
			}
		}
		return first, n
	}
	for _, c := range []struct {
		total, height, offset int
		first, n              int
	}{
		{5, 10, 0, -1, 0},
		{10, 10, 0, -1, 0},
		{100, 10, 0, 0, 1},
		{100, 10, 90, 9, 1},
		{20, 10, 5, 2, 5},
	} {
		first, n := thumb(scrollbar(c.total, c.height, c.offset))
		if first != c.first || n != c.n {
			t.Errorf("scrollbar(%d, %d, %d): thumb at %d, %d long; want %d, %d long",
				c.total, c.height, c.offset, first, n, c.first, c.n)
		}
	}
}

// The split is decided on every draw from the terminal as it is then, so a
// preview a small terminal had no room for comes back when it grows.
func TestTheSplitFollowsAResize(t *testing.T) {
	m, _ := sceneModel(t, modeBrowse)
	m.setPreview("preview-marker")
	for _, c := range []struct {
		cols, rows int
		shown      bool
	}{
		{70, 20, false},
		{200, 50, true},
		{100, 40, true},
		{70, 20, false},
	} {
		m.Update(tea.WindowSizeMsg{Width: c.cols, Height: c.rows})
		if got := strings.Contains(m.View().Content, "preview-marker"); got != c.shown {
			t.Errorf("%dx%d: preview shown %v, want %v", c.cols, c.rows, got, c.shown)
		}
	}
}

// A preview that cannot be read says why, rather than looking like an empty
// session.
func TestAFailedPreviewShowsItsError(t *testing.T) {
	fakeZmx(t, "session not found", 1)
	msg := loadPreview(&row{session: "gone", dir: "/"}, 1)().(previewMsg)
	if !strings.Contains(msg.text, "session not found") {
		t.Errorf("preview of a failed read = %q", msg.text)
	}
}

// A directory no repository holds previews as what is in it, directories
// first.
func TestADirectoryPreviewsAsItsListing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "z"), 0o700); err != nil {
		t.Fatal(err)
	}
	msg := loadPreview(&row{dir: dir}, 1)().(previewMsg)
	lines := strings.Split(strings.TrimSpace(ansi.Strip(msg.text)), "\n")
	if len(lines) != 2 || lines[0] != "z/" || lines[1] != "a.txt" {
		t.Errorf("listing %q", lines)
	}
}

// No line of the screen is wider than the terminal, whatever the layout: a
// line past the frame wraps and pushes everything under it down.
func TestNoLineOverflowsTheTerminal(t *testing.T) {
	m, _ := sceneModel(t, modeBrowse)
	m.setPreview(strings.Repeat("x", 500) + "\n")
	for _, size := range [][2]int{{40, 10}, {79, 30}, {100, 40}, {200, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for i, l := range strings.Split(m.View().Content, "\n") {
			if w := ansi.StringWidth(l); w > size[0] {
				t.Errorf("%dx%d: line %d is %d wide", size[0], size[1], i, w)
			}
		}
	}
}

// Attaching from inside a session with no client would end that session, so
// it is refused; one with a client switches it, and no session is no risk.
func TestEndsHost(t *testing.T) {
	live := []Session{{Name: "driven", Clients: "0"}, {Name: "watched", Clients: "1"}}
	for host, want := range map[string]bool{"driven": true, "watched": false, "": false, "gone": false} {
		if got := endsHost(live, host); got != want {
			t.Errorf("endsHost(%q) = %v, want %v", host, got, want)
		}
	}
}

// A long path keeps the end that tells it from its siblings.
func TestKeepTailCutsFromTheLeft(t *testing.T) {
	got := keepTail("/very/long/shared/prefix/freshdir2", 12)
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "/freshdir2") || ansi.StringWidth(got) != 12 {
		t.Errorf("keepTail = %q", got)
	}
	if got := keepTail("/short", 12); got != "/short" {
		t.Errorf("a path that fits was cut: %q", got)
	}
}

// The query scrolls within its line rather than running off the end of it,
// so what is typed last and the cursor stay in view.
func TestALongQueryKeepsItsEndInView(t *testing.T) {
	m, _ := sceneModel(t, modeBrowse)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	press(m, typed(strings.Repeat("a", 120)+"TAIL")...)
	if !strings.Contains(m.View().Content, "TAIL") {
		t.Error("the end of a long query is out of view")
	}
}

// A kill whose aftermath could not be read leaves the list as it was rather
// than emptying it.
func TestAKillThatCannotBeReadKeepsTheList(t *testing.T) {
	m, _ := sceneModel(t, modeBrowse)
	before := fingerprint(m.live)
	m.Update(killedMsg{live: nil, stale: true})
	if fingerprint(m.live) != before {
		t.Error("the list was emptied")
	}
}

// One reading showing a client is not believed until the next agrees: reading
// a preview attaches a client for as long as the read takes.
func TestAClientSeenOnceIsNotBelieved(t *testing.T) {
	m, _ := sceneModel(t, modeBrowse)
	live := append([]Session(nil), m.live...)
	for i := range live {
		live[i].Clients = "1"
	}
	m.Update(liveMsg{live: m.live})
	m.Update(liveMsg{live: live})
	if fingerprint(m.live) == fingerprint(live) {
		t.Error("a single reading was believed")
	}
	m.Update(liveMsg{live: live})
	if fingerprint(m.live) != fingerprint(live) {
		t.Error("two agreeing readings were not believed")
	}
}

// PgDown and PgUp move a screenful of lines, and stop at either end rather
// than refusing to move.
func TestPageMovesAScreenful(t *testing.T) {
	m, _ := sceneModel(t, modeBrowse)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 9})
	m.top().cursor = 0
	rows, last := m.listRows(), len(m.top().lines)-1
	press(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if want := min(rows, last); m.top().cursor != want {
		t.Errorf("after PgDown: cursor %d, want %d", m.top().cursor, want)
	}
	for range 5 {
		press(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if m.top().cursor != last {
		t.Errorf("PgDown past the end: cursor %d, want %d", m.top().cursor, last)
	}
	for range 5 {
		press(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	if m.top().cursor != 0 {
		t.Errorf("PgUp past the start: cursor %d, want 0", m.top().cursor)
	}
}

// With nothing running the sessions view is empty rather than a crash.
func TestNothingRunningIsAnEmptySessionsView(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m := newPickModel(nil, nil, t.TempDir(), visits{}, map[string]int64{}, modeSessions)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(m.View().Content, empty(modeSessions)) {
		t.Error("an empty sessions view does not say so")
	}
}
