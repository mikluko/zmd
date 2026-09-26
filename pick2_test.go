package main

import (
	"path/filepath"
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
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyG     = tea.KeyMsg{Type: tea.KeyCtrlG}
)

func typed(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestPopRestoresCursorAndQuery(t *testing.T) {
	m, _ := stackModel(t)
	press(m, keyG, typed("web"))
	if len(m.top().hits) != 1 {
		t.Fatalf("query web: %d hits, want 1", len(m.top().hits))
	}
	press(m, keyRight)
	if got := m.top().rung.view; got != viewSessions || len(m.stack) != 3 {
		t.Fatalf("after descend: view %v depth %d", got, len(m.stack))
	}
	press(m, keyEsc)
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
