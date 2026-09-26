package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	repos []string
	ident map[string]string

	stack []*level
	query textinput.Model
	match *matcher

	width, height int
	seq           int
	preview       string
	status        string // an error or a notice, shown until the next key

	asking string // an inline yes-or-no question, empty when none is open
	onYes  func() (tea.Model, tea.Cmd)

	result outcome
}

func newPickModel(live []Session, repos []string, ident map[string]string) *pickModel {
	q := textinput.New()
	q.Focus()
	return &pickModel{live: live, repos: repos, ident: ident, query: q, match: newMatcher()}
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

// loadPreview reads what r previews as off the UI goroutine: a session's
// scrollback, or a place's git history.
func loadPreview(r *row, seq int) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		switch {
		case r == nil:
		case r.session != "":
			history(r.session, &buf)
		case r.dir != "":
			out, _ := exec.Command("git", "-C", r.dir, "log", "--color=always", "--graph", "-50",
				"--format=%C(auto)%h %as%d %s").Output()
			buf.Write(out)
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
		return m, nil

	case previewMsg:
		if msg.seq == m.seq {
			m.preview = msg.text
		}
		return m, nil

	case killedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		m.live = msg.live
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
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "left":
		if len(m.stack) > 1 {
			m.pop()
			return m, m.fetchPreview()
		}
		if msg.String() == "left" {
			return m, nil
		}
		if held := lastHeld(m.live); held != nil {
			m.result = outcome{name: held.Name}
		}
		return m, tea.Quit
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
	case "enter", "right":
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

var (
	matchStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	gutterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	markStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	hintStyle   = lipgloss.NewStyle().Faint(true)
	askStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	sideBorder  = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderLeft(true)
	underBorder = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderTop(true)
)

func (m *pickModel) View() string {
	if m.width == 0 {
		return ""
	}
	body := m.height - 2
	var main string
	switch {
	case m.width >= 120:
		pw := m.width * 55 / 100
		main = lipgloss.JoinHorizontal(lipgloss.Top,
			m.listView(m.width-pw, body),
			sideBorder.Height(body).Render(previewView(m.preview, pw-1, body)))
	case m.width >= 80 && m.height >= 15:
		ph := body / 2
		main = lipgloss.JoinVertical(lipgloss.Left,
			m.listView(m.width, body-ph),
			underBorder.Width(m.width).Render(previewView(m.preview, m.width, ph-1)))
	default:
		main = m.listView(m.width, body)
	}
	return m.query.View() + "\n" + main + "\n" + m.footer()
}

// footer is the open question, an error, or the keys that would do something
// where the cursor stands.
func (m *pickModel) footer() string {
	switch {
	case m.asking != "":
		return askStyle.Render(m.asking + " y/N")
	case m.status != "":
		return errStyle.Render(m.status)
	}
	var keys string
	if r := m.current(); r != nil && r.dir != "" {
		keys = hints(r.state())
	}
	if len(m.stack) > 1 {
		keys += "  ⎋ back"
	} else if lastHeld(m.live) != nil {
		keys += "  ⎋ leave"
	}
	if m.top().rung.view != viewRepos {
		keys += "  ⌃g repos"
	}
	if n := len(m.top().marked); n > 0 {
		keys += "  " + strconv.Itoa(n) + " marked"
	}
	return hintStyle.Render(strings.TrimSpace(keys))
}

func (m *pickModel) listView(width, height int) string {
	l := m.top()
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+height {
		l.offset = l.cursor - height + 1
	}
	lines := make([]string, 0, height)
	if len(l.items) == 0 {
		lines = append(lines, hintStyle.Render("  "+empty(l.rung)))
	}
	for i := l.offset; i < len(l.hits) && len(lines) < height; i++ {
		h := l.hits[i]
		it := l.items[h.item]
		gutter := " "
		switch {
		case i == l.cursor:
			gutter = gutterStyle.Render("▌")
		case l.marked[rowKey(it.row)]:
			gutter = markStyle.Render("▐")
		}
		status := it.row.status
		if status == "" {
			status = strings.Repeat(" ", statusWidth)
		}
		line := ansi.Truncate(status+highlight(it.text, h.pos), width-1, "…")
		if i == l.cursor {
			line = underCursor(line, width-1)
		}
		lines = append(lines, gutter+line)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

// cursorBg is the cursor row's background, as the escape that sets it.
const cursorBg = "\x1b[48;5;238m"

// underCursor paints line's background across width cells.
//
// The line carries its own SGR resets, from the status column's dim and from
// the match highlight, and each one would end a style set only once at the
// start; so it is set again after every reset.
func underCursor(line string, width int) string {
	const style = cursorBg + "\x1b[1m"
	pad := max(width-ansi.StringWidth(line), 0)
	line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+style)
	line = strings.ReplaceAll(line, "\x1b[m", "\x1b[m"+style)
	return style + line + strings.Repeat(" ", pad) + "\x1b[0m"
}

// highlight marks the runes of text at the matched positions.
func highlight(text string, pos []int) string {
	if len(pos) == 0 {
		return text
	}
	at := make(map[int]bool, len(pos))
	for _, p := range pos {
		at[p] = true
	}
	var b strings.Builder
	for i, r := range []rune(text) {
		if at[i] {
			b.WriteString(matchStyle.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// previewView is the end of the text, cut to the box it is drawn in.
func previewView(text string, width, height int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(strings.TrimRight(l, "\r"), width, "")
	}
	return strings.Join(lines, "\n")
}

// pick2 is the prototype picker, drawn by this process rather than by fzf.
func pick2() error {
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

	m := newPickModel(live, repos, ident)
	m.push(rung{view: viewSessions})
	if held := lastHeld(live); held != nil {
		m.top().cursor = max(named(rowsOf(m.top()), held.Name), 0)
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
