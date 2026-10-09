package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stackModel is a picker over two repositories, one with a worktree, and a
// session in each place, opened on the ground floor.
func stackModel(t *testing.T) (*pickModel, string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	api := repoAt(t, filepath.Join(root, "api"))
	tree := worktreeAt(t, api, "fix", filepath.Join(root, "api-fix"))
	web := repoAt(t, filepath.Join(root, "web"))
	live := []Session{
		{Name: "a", Dir: api, Clients: "0"},
		{Name: "b", Dir: tree, Clients: "0"},
		{Name: "c", Dir: web, Clients: "0"},
	}
	m := newPickModel(live, []string{api, tree, web}, map[string]string{})
	m.push(rung{view: viewSessions})
	return m, api
}

func press(m *pickModel, keys ...tea.KeyMsg) {
	for _, k := range keys {
		m.Update(k)
	}
}

var (
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyRight = tea.KeyMsg{Type: tea.KeyRight}
	keyLeft  = tea.KeyMsg{Type: tea.KeyLeft}
	keyBack  = tea.KeyMsg{Type: tea.KeyBackspace}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyG     = tea.KeyMsg{Type: tea.KeyCtrlG}
	keyTab   = tea.KeyMsg{Type: tea.KeyTab}
	keyDel   = tea.KeyMsg{Type: tea.KeyDelete}
)

func typed(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestPopRestoresCursorAndQuery(t *testing.T) {
	m, _ := stackModel(t)
	press(m, keyG, typed("web"))
	if len(m.top().hits) != 1 {
		t.Fatalf("query web: %d hits, want 1", len(m.top().hits))
	}
	press(m, keyEnter)
	if got := m.top().rung.view; got != viewSessions || len(m.stack) != 3 {
		t.Fatalf("after descend: view %v depth %d", got, len(m.stack))
	}
	press(m, keyLeft)
	if got := m.query.Value(); got != "web" {
		t.Errorf("query after pop = %q, want web", got)
	}
	if got := filepath.Base(m.current().dir); got != "web" {
		t.Errorf("cursor after pop on %q, want web", got)
	}
}

func TestReposJumpDoesNotGrowStack(t *testing.T) {
	m, _ := stackModel(t)
	press(m, keyG)
	for range 3 {
		press(m, keyRight, keyG) // into a checkout's sessions, then back to repos
	}
	if len(m.stack) != 2 {
		t.Errorf("depth = %d, want 2", len(m.stack))
	}
}

func TestKillKeepsCursorOnSurvivor(t *testing.T) {
	m, _ := stackModel(t)
	first := m.current().session
	press(m, keyDown)
	on := m.current().session
	var survivors []Session
	for _, s := range m.live {
		if s.Name != first {
			survivors = append(survivors, s)
		}
	}
	m.Update(killedMsg{live: survivors})
	if got := m.current().session; got != on {
		t.Errorf("cursor on %q after kill, want %q", got, on)
	}
}

func TestKillTargetsPreferMarks(t *testing.T) {
	m, _ := stackModel(t)
	press(m, tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyTab})
	got := m.killTargets()
	if len(got) != 2 {
		t.Errorf("targets = %v, want the two marked", got)
	}
}

func TestEscOnGroundLeavesWithoutAttaching(t *testing.T) {
	m, _ := stackModel(t)
	_, cmd := m.Update(keyEsc)
	if cmd == nil || m.result.name != "" {
		t.Errorf("esc with nothing held: cmd %v result %+v", cmd, m.result)
	}
}

// A picker that cannot correct what was typed is worse than one that cannot
// walk, so the walking keys edit the query while there is one.
func TestWalkingKeysCorrectATypedQuery(t *testing.T) {
	m, _ := stackModel(t)
	press(m, keyG, typed("web"), keyLeft, keyRight)
	if len(m.stack) != 2 || m.query.Value() != "web" {
		t.Fatalf("left and right under a query: depth %d query %q", len(m.stack), m.query.Value())
	}
	press(m, keyBack, keyBack, keyBack)
	if len(m.stack) != 2 || m.query.Value() != "" {
		t.Fatalf("backspace under a query: depth %d query %q", len(m.stack), m.query.Value())
	}
	press(m, keyBack)
	if len(m.stack) != 1 {
		t.Errorf("backspace with nothing typed: depth %d, want 1", len(m.stack))
	}
}

// Escape leaves rather than popping: the arrow is what walks the stack.
func TestEscLeavesFromAnyRung(t *testing.T) {
	m, _ := stackModel(t)
	press(m, keyG)
	_, cmd := m.Update(keyEsc)
	if cmd == nil || len(m.stack) != 2 {
		t.Errorf("esc on the repos rung: cmd %v depth %d", cmd, len(m.stack))
	}
}

// Enter acts on the row under the cursor, so marks made for a kill do not
// outlive it.
func TestEnterClearsMarks(t *testing.T) {
	m, _ := stackModel(t)
	press(m, keyG, keyTab, keyEnter)
	if n := len(m.stack[1].marked); n != 0 {
		t.Errorf("%d marks survive enter", n)
	}
}

// Any answer but yes is no, escape included, and the picker stays up.
func TestADeclinedKillKeepsThePicker(t *testing.T) {
	m, _ := stackModel(t)
	press(m, keyDel)
	if m.asking == "" {
		t.Fatal("delete did not ask")
	}
	_, cmd := m.Update(keyEsc)
	if cmd != nil || m.asking != "" || len(m.live) != 3 {
		t.Errorf("after escape: cmd %v asking %q live %d", cmd, m.asking, len(m.live))
	}
}

// A session's scrollback opens at its end and a place's history at its start.
func TestPreviewOpensWhereItIsRead(t *testing.T) {
	m, _ := stackModel(t)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 22})
	long := strings.Repeat("line\n", 100)
	m.Update(previewMsg{seq: m.seq, text: long})
	if !m.preview.AtBottom() {
		t.Errorf("a session previews from offset %d, want its end", m.preview.YOffset)
	}
	press(m, keyG)
	m.Update(previewMsg{seq: m.seq, text: long})
	if !m.preview.AtTop() {
		t.Errorf("a place previews from offset %d, want its start", m.preview.YOffset)
	}
}

// Scrolling past an end is not banked: the first key back moves the preview.
func TestScrollPreviewStopsAtTheEnds(t *testing.T) {
	m, _ := stackModel(t)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 22})
	m.Update(previewMsg{seq: m.seq, text: strings.Repeat("line\n", 100)})
	end := m.preview.YOffset
	press(m, tea.KeyMsg{Type: tea.KeyDown, Alt: true})
	if m.preview.YOffset != end {
		t.Errorf("scrolled past the end of a session's scrollback: %d, want %d", m.preview.YOffset, end)
	}
	press(m, tea.KeyMsg{Type: tea.KeyUp, Alt: true}, tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	if m.preview.YOffset != end-2 {
		t.Errorf("two lines up: %d, want %d", m.preview.YOffset, end-2)
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
	m, _ := stackModel(t)
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
		if got := strings.Contains(m.View(), "preview-marker"); got != c.shown {
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
