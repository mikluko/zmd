package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// sceneModel is a picker over scene, opened on the given view, with titles on
// two of the engine's sessions.
func sceneModel(t *testing.T, open mode) (*pickModel, string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root, live := scene(t)
	engine, tree, api, _ := sceneDirs(root)
	for i := range live {
		switch live[i].Name {
		case "src.acme.engine":
			live[i].Title = "retry-bug"
		case "src.acme.engine+review":
			live[i].Title = "review-pr"
		}
	}
	m := newPickModel(live, []string{engine, tree, api}, map[string]string{}, "", visits{}, open)
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

// labels is what the visible lines say, a place by its name and what it
// holds indented under it.
func labels(m *pickModel) []string {
	p := m.top()
	var out []string
	for _, ln := range p.lines {
		pl := p.places[ln.place]
		if ln.kid < 0 {
			out = append(out, pl.row.cells[0])
		} else {
			out = append(out, "  "+pl.kids[ln.kid].cells[0])
		}
	}
	return out
}

func seekLabel(t *testing.T, m *pickModel, label string) {
	t.Helper()
	for i, l := range labels(m) {
		if strings.TrimSpace(l) == label || strings.HasSuffix(l, label) {
			m.top().cursor = i
			return
		}
	}
	t.Fatalf("no line %q in %q", label, labels(m))
}

func TestEnterOpensAndClosesAPlace(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	before := len(m.top().lines)
	seekLabel(t, m, "engine")
	press(m, keyEnter)
	if got := len(m.top().lines); got != before+4 {
		t.Fatalf("open engine: %d lines, want %d: %q", got, before+4, labels(m))
	}
	if !m.top().expanded(m.top().cursor) {
		t.Error("the cursor left the place it opened")
	}
	press(m, keyEnter)
	if got := len(m.top().lines); got != before {
		t.Errorf("closed engine: %d lines, want %d", got, before)
	}
}

// A session is found by its title, under its place, without its siblings.
func TestAQueryFindsASessionUnderItsPlace(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	press(m, typed("retry-bug")...)
	got := labels(m)
	if len(got) != 2 || !strings.HasSuffix(got[0], "engine") || got[1] != "  retry-bug" {
		t.Errorf("lines %q, want engine then retry-bug alone", got)
	}
}

// A place matching only itself is listed closed, and entering it clears the
// query and leaves it open under the cursor.
func TestEnteringAFoundPlaceClearsTheQuery(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	press(m, typed("api")...)
	got := labels(m)
	if len(got) != 1 {
		t.Fatalf("lines %q, want api alone and closed", got)
	}
	press(m, keyEnter)
	if m.query.Value() != "" || m.top().query != "" {
		t.Errorf("query %q survives entering", m.query.Value())
	}
	if !m.top().expanded(m.top().cursor) || !strings.HasSuffix(labels(m)[m.top().cursor], "api") {
		t.Errorf("cursor on %q, want api open", labels(m)[m.top().cursor])
	}
}

// The arrows edit the query and nothing else.
func TestArrowsOnlyEditTheQuery(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	press(m, typed("eng")...)
	lines := len(m.top().lines)
	press(m, keyLeft, keyRight)
	if m.query.Value() != "eng" || len(m.top().lines) != lines {
		t.Errorf("after arrows: query %q, %d lines", m.query.Value(), len(m.top().lines))
	}
	press(m, keyBack, keyBack, keyBack, keyBack)
	if m.query.Value() != "" {
		t.Errorf("backspace leaves %q", m.query.Value())
	}
}

// Tab flips the view, each keeping its own query, and the view is remembered.
func TestTabSwitchesViewAndRemembersIt(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	press(m, typed("eng")...)
	press(m, keyTab)
	if m.mode != modeDirs || m.query.Value() != "" {
		t.Fatalf("after tab: %v, query %q", m.mode, m.query.Value())
	}
	if loadMode() != modeDirs {
		t.Error("the view switched to is not remembered")
	}
	press(m, keyTab)
	if m.query.Value() != "eng" {
		t.Errorf("repos query %q, want eng back", m.query.Value())
	}
}

// Marking a place marks everything it holds for a kill.
func TestKillTargetsPreferMarks(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	seekLabel(t, m, "engine")
	press(m, keyMark)
	if got := m.killTargets(); len(got) != 3 {
		t.Errorf("targets %v, want engine's three", got)
	}
}

func TestEnterAttachesASessionAndStartsAnOffer(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	seekLabel(t, m, "engine")
	press(m, keyEnter)
	seekLabel(t, m, "review-pr")
	if _, cmd := m.Update(keyEnter); cmd == nil || m.result.name != "src.acme.engine+review" {
		t.Errorf("enter on a session: result %+v", m.result)
	}

	m, _ = sceneModel(t, modeRepos)
	seekLabel(t, m, "engine")
	press(m, keyEnter)
	seekLabel(t, m, newSession)
	m.Update(keyEnter)
	if m.result.dir == "" || !strings.HasPrefix(m.result.name, "zmd.") {
		t.Errorf("enter on the offer: result %+v", m.result)
	}
}

// Escape leaves, from wherever the cursor is.
func TestEscLeavesWithoutAttaching(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	press(m, typed("eng")...)
	_, cmd := m.Update(keyEsc)
	if cmd == nil || m.result.name != "" {
		t.Errorf("esc with nothing held: cmd %v result %+v", cmd, m.result)
	}
}

// Any answer but yes is no, escape included, and the picker stays up.
func TestADeclinedKillKeepsThePicker(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	seekLabel(t, m, "engine")
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
	m, _ := sceneModel(t, modeRepos)
	seekLabel(t, m, "engine")
	press(m, keyEnter)
	seekLabel(t, m, "review-pr")
	var survivors []Session
	for _, s := range m.live {
		if s.Name != "src.acme.engine" {
			survivors = append(survivors, s)
		}
	}
	m.Update(killedMsg{live: survivors})
	if _, _, r := m.current(); r == nil || r.session != "src.acme.engine+review" {
		t.Errorf("cursor after kill on %+v", r)
	}
}

// A pane that held nothing opens on the offer of a session where it stands,
// its place open around it.
func TestOpeningLandsOnTheOfferWhereThePaneStands(t *testing.T) {
	m, root := sceneModel(t, modeRepos)
	engine, _, _, _ := sceneDirs(root)
	m.start = engine
	m.opening()
	_, pl, r := m.current()
	if r == nil || r.cells[0] != newSession || pl.row.dir != engine {
		t.Errorf("opened on %+v", r)
	}
}

// A session's scrollback opens at its end and a place's history at its start.
func TestPreviewOpensWhereItIsRead(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 22})
	long := strings.Repeat("line\n", 100)
	seekLabel(t, m, "engine")
	press(m, keyEnter)
	seekLabel(t, m, "retry-bug")
	m.Update(previewMsg{seq: m.seq, text: long})
	if !m.preview.AtBottom() {
		t.Errorf("a session previews from offset %d, want its end", m.preview.YOffset())
	}
	seekLabel(t, m, "engine")
	m.Update(previewMsg{seq: m.seq, text: long})
	if !m.preview.AtTop() {
		t.Errorf("a place previews from offset %d, want its start", m.preview.YOffset())
	}
}

// Scrolling past an end is not banked: the first key back moves the preview.
func TestScrollPreviewStopsAtTheEnds(t *testing.T) {
	m, _ := sceneModel(t, modeRepos)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 22})
	seekLabel(t, m, "engine")
	press(m, keyEnter)
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
	m, _ := sceneModel(t, modeRepos)
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

// The picker opens on the session the pane last held, its place open around
// it, so its peers are a keystroke away.
func TestOpeningLandsOnTheHeldSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root, live := scene(t)
	engine, tree, api, _ := sceneDirs(root)
	recordLast("src.acme.engine+review")
	m := newPickModel(live, []string{engine, tree, api}, map[string]string{}, "", visits{}, modeRepos)
	m.opening()
	if _, _, r := m.current(); r == nil || r.session != "src.acme.engine+review" {
		t.Errorf("opened on %+v", r)
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
