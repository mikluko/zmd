package main

import (
	"bytes"
	"context"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// pickItem is a row of the list with what the matcher reads and what is drawn.
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

// previewMsg carries a session's scrollback, tagged with the cursor move it
// was fetched for so a slower fetch cannot overwrite a newer one.
type previewMsg struct {
	seq  int
	text string
}

type pickModel struct {
	items   []pickItem
	hits    []hit
	match   *matcher
	query   textinput.Model
	cursor  int
	offset  int
	width   int
	height  int
	seq     int
	preview string
	asking  string // an inline yes-or-no question, empty when none is open
	chosen  *row
	take    bool
}

func newPickModel(list []row, start int) pickModel {
	cells := make([][]string, len(list))
	for i, r := range list {
		cells[i] = r.cells
	}
	display := align(cells)
	items := make([]pickItem, len(list))
	for i, r := range list {
		items[i] = pickItem{row: r, text: display[i]}
	}
	q := textinput.New()
	q.Prompt = "sessions> "
	q.Focus()
	m := pickModel{items: items, match: newMatcher(), query: q}
	m.refilter()
	if start >= 0 {
		m.cursor = start
	}
	return m
}

// refilter recomputes the hits for the current query, best first and in list
// order among equals.
func (m *pickModel) refilter() {
	query := m.query.Value()
	m.hits = m.hits[:0]
	for i, it := range m.items {
		if score, pos, ok := m.match.match(it.text, query); ok {
			m.hits = append(m.hits, hit{item: i, score: score, pos: pos})
		}
	}
	if query != "" {
		sort.SliceStable(m.hits, func(i, j int) bool { return m.hits[i].score > m.hits[j].score })
	}
	m.cursor, m.offset = 0, 0
}

func (m pickModel) current() *pickItem {
	if m.cursor < 0 || m.cursor >= len(m.hits) {
		return nil
	}
	return &m.items[m.hits[m.cursor].item]
}

// fetchPreview supersedes whatever preview is in flight with one for the row
// under the cursor.
func (m *pickModel) fetchPreview() tea.Cmd {
	m.seq++
	return loadPreview(m.current(), m.seq)
}

// loadPreview reads the scrollback of it off the UI goroutine.
func loadPreview(it *pickItem, seq int) tea.Cmd {
	if it == nil || it.row.session == "" {
		return func() tea.Msg { return previewMsg{seq: seq} }
	}
	name := it.row.session
	return func() tea.Msg {
		var buf bytes.Buffer
		history(name, &buf)
		return previewMsg{seq: seq, text: buf.String()}
	}
}

func (m pickModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, loadPreview(m.current(), m.seq))
}

func (m pickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case previewMsg:
		if msg.seq == m.seq {
			m.preview = msg.text
		}
		return m, nil

	case tea.KeyMsg:
		if m.asking != "" {
			return m.answer(msg)
		}
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "ctrl+p":
			return m.move(-1)
		case "down", "ctrl+n":
			return m.move(1)
		case "enter":
			it := m.current()
			if it == nil {
				return m, nil
			}
			if it.row.held {
				m.asking = takes(it.row.cells[0])
				return m, nil
			}
			m.chosen = &it.row
			return m, tea.Quit
		}
		before := m.query.Value()
		var cmd tea.Cmd
		m.query, cmd = m.query.Update(msg)
		if m.query.Value() != before {
			m.refilter()
			return m, tea.Batch(cmd, m.fetchPreview())
		}
		return m, cmd
	}
	return m, nil
}

// answer resolves the open question without leaving the list: y takes, and
// anything else is no.
func (m pickModel) answer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.asking = ""
	if msg.String() == "y" {
		it := m.current()
		m.chosen, m.take = &it.row, true
		return m, tea.Quit
	}
	return m, nil
}

func (m pickModel) move(delta int) (tea.Model, tea.Cmd) {
	next := m.cursor + delta
	if next < 0 || next >= len(m.hits) {
		return m, nil
	}
	m.cursor = next
	return m, m.fetchPreview()
}

var (
	matchStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	gutterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	hintStyle   = lipgloss.NewStyle().Faint(true)
	askStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	borderStyle = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderLeft(true)
)

func (m pickModel) View() string {
	if m.width == 0 {
		return ""
	}
	listWidth, previewWidth := m.width, 0
	if m.width >= 120 {
		previewWidth = m.width * 55 / 100
		listWidth = m.width - previewWidth
	}
	body := m.height - 2

	var footer string
	if m.asking != "" {
		footer = askStyle.Render(m.asking + " y/N")
	} else {
		footer = hintStyle.Render("⏎ attach  ⎋ leave")
	}

	list := m.listView(listWidth, body)
	top := list
	if previewWidth > 0 {
		top = lipgloss.JoinHorizontal(lipgloss.Top, list, borderStyle.Height(body).Render(previewView(m.preview, previewWidth-1, body)))
	}
	return m.query.View() + "\n" + top + "\n" + footer
}

func (m *pickModel) listView(width, height int) string {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+height {
		m.offset = m.cursor - height + 1
	}
	lines := make([]string, 0, height)
	for i := m.offset; i < len(m.hits) && len(lines) < height; i++ {
		h := m.hits[i]
		it := m.items[h.item]
		gutter := " "
		if i == m.cursor {
			gutter = gutterStyle.Render("▌")
		}
		line := ansi.Truncate(it.row.status+highlight(it.text, h.pos), width-1, "…")
		if i == m.cursor {
			line = underCursor(line, width-1)
		}
		line = gutter + line
		lines = append(lines, line)
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

// previewView is the end of the scrollback, cut to the box it is drawn in.
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

// pick2 is the prototype picker: the sessions rung only, drawn by this
// process rather than by fzf.
func pick2() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	_, ident := walked(context.Background(), dialDeck(), home)
	live, err := sessions()
	if err != nil {
		return err
	}
	list := liveRows("", live, ident)
	start := -1
	if held := lastHeld(live); held != nil {
		start = named(list, held.Name)
	}

	final, err := tea.NewProgram(newPickModel(list, start), tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	m := final.(pickModel)
	if m.chosen == nil {
		return errCancelled
	}
	if m.take {
		if err := detachClients(m.chosen.session); err != nil {
			return err
		}
	}
	recordLast(m.chosen.session)
	return attach(m.chosen.session, "")
}
