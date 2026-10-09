package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// pickItem is a row of the list with what the matcher reads.
type pickItem struct {
	row  row
	text string // the aligned cells, plain, which the query is matched against
}

// hit is one item that survived the query, and where the query landed in it.
type hit struct {
	item  int
	score int
	pos   []int
}

// level is one rung of the stack with everything the view of it holds, so a
// pop lands back where the push left.
type level struct {
	rung   rung
	items  []pickItem
	hits   []hit
	cursor int
	offset int
	query  string
	marked map[string]bool // by rowKey, so a mark survives a refilter
}

// rowKey is what identifies a row across rebuilds of its rung: the session it
// stands for, or else its directory.
func rowKey(r row) string {
	if r.session != "" {
		return "s:" + r.session
	}
	return "d:" + r.dir
}

// previewMsg carries a preview, tagged with the cursor move it was fetched for
// so a slower fetch cannot overwrite a newer one.
type previewMsg struct {
	seq  int
	text string
}

// killedMsg reports a kill and what is running after it.
type killedMsg struct {
	live []Session
	err  error
}

// outcome is what the picker was left with: a session to attach, and where to
// start it when it is new. Empty means leave.
type outcome struct {
	name string
	dir  string
	take bool // detach the other terminal's client first
}

type pickModel struct {
	live  []Session
	held  *Session // what escape leaves for, nil where there is nothing to go back to
	repos []string
	ident map[string]string

	stack []*level
	query textinput.Model
	match *matcher

	width, height int
	seq           int
	preview       viewport.Model
	status        string // an error or a notice, shown until the next key

	asking string // an inline yes-or-no question, empty when none is open
	onYes  func() (tea.Model, tea.Cmd)

	result outcome
}

func newPickModel(live []Session, repos []string, ident map[string]string) *pickModel {
	q := textinput.New()
	q.PromptStyle = promptStyle
	q.TextStyle = inputStyle
	q.Focus()
	return &pickModel{
		live: live, held: lastHeld(live), repos: repos, ident: ident,
		query: q, match: newMatcher(), preview: viewport.New(0, 0),
	}
}

func (m *pickModel) top() *level { return m.stack[len(m.stack)-1] }

// push puts a rung on the stack, keeping the query typed on the one below.
func (m *pickModel) push(r rung) {
	if len(m.stack) > 0 {
		m.top().query = m.query.Value()
	}
	l := &level{rung: r, marked: map[string]bool{}}
	m.stack = append(m.stack, l)
	m.rebuild(l)
	m.query.SetValue("")
	m.query.Prompt = r.prompt() + "> "
}

// pop drops the top rung.
func (m *pickModel) pop() {
	m.truncate(len(m.stack) - 1)
}

// truncate cuts the stack to n rungs and restores the new top as it was left.
func (m *pickModel) truncate(n int) {
	m.stack = m.stack[:n]
	l := m.top()
	m.query.SetValue(l.query)
	m.query.CursorEnd()
	m.query.Prompt = l.rung.prompt() + "> "
}

// toRepos jumps to the repositories: back to them where they are already on
// the stack, pushed above the ground floor where they are not.
func (m *pickModel) toRepos() {
	for i, l := range m.stack {
		if l.rung.view == viewRepos {
			m.top().query = m.query.Value()
			m.truncate(i + 1)
			return
		}
	}
	m.top().query = m.query.Value()
	m.truncate(1)
	m.push(rung{view: viewRepos})
}

// rebuild reads l's rows afresh from what is running, keeping the cursor on
// the row it stood on, or at the same height where that row is gone.
func (m *pickModel) rebuild(l *level) {
	var keep string
	if l.cursor < len(l.hits) {
		keep = rowKey(l.items[l.hits[l.cursor].item].row)
	}
	list := rows(l.rung, m.live, m.repos, m.ident)
	cells := make([][]string, len(list))
	for i, r := range list {
		cells[i] = r.cells
	}
	display := align(cells)
	l.items = make([]pickItem, len(list))
	present := map[string]bool{}
	for i, r := range list {
		l.items[i] = pickItem{row: r, text: display[i]}
		present[rowKey(r)] = true
	}
	for k := range l.marked {
		if !present[k] {
			delete(l.marked, k)
		}
	}
	cursor := l.cursor
	m.refilter(l, l.query)
	l.cursor = min(cursor, max(len(l.hits)-1, 0))
	for i, h := range l.hits {
		if keep != "" && rowKey(l.items[h.item].row) == keep {
			l.cursor = i
		}
	}
}

// refilter recomputes l's hits for query, best first and in list order among
// equals, with the cursor back at the top.
func (m *pickModel) refilter(l *level, query string) {
	l.query = query
	l.hits = l.hits[:0]
	for i, it := range l.items {
		if score, pos, ok := m.match.match(it.text, query); ok {
			l.hits = append(l.hits, hit{item: i, score: score, pos: pos})
		}
	}
	if query != "" {
		sort.SliceStable(l.hits, func(i, j int) bool { return l.hits[i].score > l.hits[j].score })
	}
	l.cursor, l.offset = 0, 0
}

func (m *pickModel) current() *row {
	l := m.top()
	if l.cursor < 0 || l.cursor >= len(l.hits) {
		return nil
	}
	return &l.items[l.hits[l.cursor].item].row
}

// fetchPreview supersedes whatever preview is in flight with one for the row
// under the cursor.
func (m *pickModel) fetchPreview() tea.Cmd {
	m.seq++
	return loadPreview(m.current(), m.seq)
}

// setPreview shows text, at its end where the row follows and at its start
// where not.
func (m *pickModel) setPreview(text string) {
	m.preview.SetContent(strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n"))
	if m.follows() {
		m.preview.GotoBottom()
	} else {
		m.preview.GotoTop()
	}
}

// sizePreview fits the preview to its box, keeping one held at its end there.
func (m *pickModel) sizePreview() {
	_, w, h := m.previewBox()
	bottom := m.preview.AtBottom()
	m.preview.Width, m.preview.Height = w, h
	if bottom && m.follows() {
		m.preview.GotoBottom()
	}
}

// follows reports whether the preview anchors at its end: a session's
// scrollback is read from where it stopped, a place's history from its newest.
func (m *pickModel) follows() bool {
	r := m.current()
	return r != nil && r.session != ""
}

// loadPreview reads what r previews as off the UI goroutine: a session's
// scrollback, or the git history of a place git tracks. A read that fails
// previews as its error.
func loadPreview(r *row, seq int) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		var err error
		switch {
		case r == nil:
		case r.session != "":
			err = history(r.session, &buf)
		case r.dir != "" && repoRoot(r.dir) != "":
			cmd := exec.Command("git", "-C", r.dir, "log", "--color=always", "--graph", "-50",
				"--format=%C(auto)%h %as%d %s")
			cmd.Stdout = &buf
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if cmd.Run() != nil {
				err = fmt.Errorf("git log: %s", strings.TrimSpace(stderr.String()))
			}
		}
		if err != nil {
			return previewMsg{seq: seq, text: errStyle.Render(err.Error())}
		}
		return previewMsg{seq: seq, text: buf.String()}
	}
}

func (m *pickModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.fetchPreview())
}

func (m *pickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.sizePreview()
		return m, nil

	case previewMsg:
		if msg.seq == m.seq {
			m.setPreview(msg.text)
		}
		return m, nil

	case killedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		m.live = msg.live
		m.held = lastHeld(m.live)
		for _, l := range m.stack {
			m.rebuild(l)
		}
		return m, m.fetchPreview()

	case tea.KeyMsg:
		m.status = ""
		if m.asking != "" {
			m.asking = ""
			if msg.String() == "y" {
				return m.onYes()
			}
			return m, nil
		}
		return m.key(msg)
	}
	return m, nil
}

func (m *pickModel) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	l := m.top()
	typed := m.query.Value() != ""
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.held != nil {
			m.result = outcome{name: m.held.Name}
		}
		return m, tea.Quit
	case "left", "backspace":
		if typed {
			break
		}
		if len(m.stack) > 1 {
			m.pop()
			return m, m.fetchPreview()
		}
		return m, nil
	case "right":
		if typed {
			break
		}
		return m.descend()
	case "alt+up":
		m.preview.ScrollUp(1)
		return m, nil
	case "alt+down":
		m.preview.ScrollDown(1)
		return m, nil
	case "alt+pgup":
		m.preview.PageUp()
		return m, nil
	case "alt+pgdown":
		m.preview.PageDown()
		return m, nil
	case "up", "ctrl+p":
		return m.move(-1)
	case "down", "ctrl+n":
		return m.move(1)
	case "ctrl+g":
		m.toRepos()
		return m, m.fetchPreview()
	case "tab":
		if r := m.current(); r != nil && r.dir != "" {
			k := rowKey(*r)
			l.marked[k] = !l.marked[k]
			if !l.marked[k] {
				delete(l.marked, k)
			}
		}
		return m.move(1)
	case "enter":
		l.marked = map[string]bool{}
		return m.descend()
	case "alt+enter":
		if r := m.current(); r != nil && r.dir != "" {
			m.result = outcome{name: sessionName(m.live), dir: r.dir}
			return m, tea.Quit
		}
		return m, nil
	case "delete", "alt+delete":
		names := m.killTargets()
		if len(names) == 0 {
			return m, nil
		}
		kill := func() (tea.Model, tea.Cmd) {
			l.marked = map[string]bool{}
			return m, func() tea.Msg {
				err := killSessions(names)
				live, _ := sessions()
				return killedMsg{live: live, err: err}
			}
		}
		if msg.String() == "alt+delete" {
			return kill()
		}
		m.asking, m.onYes = question(names), kill
		return m, nil
	}

	before := m.query.Value()
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	if m.query.Value() != before {
		m.refilter(l, m.query.Value())
		return m, tea.Batch(cmd, m.fetchPreview())
	}
	return m, cmd
}

// killTargets is every session the marked rows stand for, or the row under
// the cursor where nothing is marked.
func (m *pickModel) killTargets() []string {
	l := m.top()
	var picked []row
	for _, it := range l.items {
		if l.marked[rowKey(it.row)] {
			picked = append(picked, it.row)
		}
	}
	if len(picked) == 0 {
		if r := m.current(); r != nil {
			picked = append(picked, *r)
		}
	}
	seen := map[string]bool{}
	var names []string
	for _, r := range picked {
		for _, name := range killable(r, m.live) {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// descend is enter and right: attach, take, start, or push the rung below.
func (m *pickModel) descend() (tea.Model, tea.Cmd) {
	r := m.current()
	if r == nil || r.dir == "" {
		return m, nil
	}
	here := m.top().rung
	switch {
	case here.view == viewSessions && r.session != "":
		if r.held {
			name := r.session
			m.asking = takes(r.cells[0])
			m.onYes = func() (tea.Model, tea.Cmd) {
				m.result = outcome{name: name, take: true}
				return m, tea.Quit
			}
			return m, nil
		}
		m.result = outcome{name: r.session}
		return m, tea.Quit
	case here.view == viewRepos && r.trees > 0:
		m.push(rung{view: viewCheckouts, dir: r.dir, name: r.cells[0]})
		return m, m.fetchPreview()
	case here.view != viewSessions && r.count > 0:
		m.push(rung{view: viewSessions, dir: r.dir, name: r.cells[0]})
		return m, m.fetchPreview()
	default:
		m.result = outcome{name: sessionName(m.live), dir: r.dir}
		return m, tea.Quit
	}
}

func (m *pickModel) move(delta int) (tea.Model, tea.Cmd) {
	l := m.top()
	next := l.cursor + delta
	if next < 0 || next >= len(l.hits) {
		return m, nil
	}
	l.cursor = next
	return m, m.fetchPreview()
}

// layout is where the preview stands against the list.
type layout int

const (
	layoutNone layout = iota
	layoutBeside
	layoutAbove
)

// previewLayout returns where the preview goes in a terminal this size.
//
// The split follows the shape of the space rather than a preference: a wide
// terminal has room beside the list, a narrower one only above it, and one too
// small for both keeps the list.
func previewLayout(cols, rows int) layout {
	switch {
	case cols < 80 || rows < 15:
		return layoutNone
	case cols >= 120:
		return layoutBeside
	default:
		return layoutAbove
	}
}

// run presents the picker and attaches to what it answers.
func run() error {
	start, err := os.Getwd()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	repos, ident := walked(context.Background(), dialDeck(), home)
	live, err := sessions()
	if err != nil {
		return err
	}

	// The ground floor is every session there is, with the cursor on what the
	// pane last held, so its peers are around it. A pane that has held
	// nothing has only where it stands to go on, so it opens one rung in,
	// narrowed to that, on the offer of a session there.
	m := newPickModel(live, repos, ident)
	m.push(rung{view: viewSessions})
	if m.held != nil {
		m.top().cursor = max(named(rowsOf(m.top()), m.held.Name), 0)
	} else if at := filepath.Clean(start); under(home, at) {
		m.push(rung{view: viewSessions, dir: at, name: placeName(at, ident)})
		m.top().cursor = len(m.top().hits) - 1
	}

	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	res := final.(*pickModel).result
	if res.name == "" {
		return errCancelled
	}
	if res.take {
		if err := detachClients(res.name); err != nil {
			return err
		}
	}
	// Recorded before the exec, since there is no returning from it.
	recordLast(res.name)
	return attach(res.name, res.dir)
}

// rowsOf is l's rows in list order.
func rowsOf(l *level) []row {
	out := make([]row, len(l.items))
	for i, it := range l.items {
		out[i] = it.row
	}
	return out
}
