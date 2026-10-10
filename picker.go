package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// line is one row of the list as drawn, and where the query landed in it.
type line struct {
	row int // index into the pane's rows
	pos []int
}

// pane is one view with everything the picker holds of it, so switching away
// and back lands where it was left.
type pane struct {
	mode   mode
	root   string   // the directory the filesystem view stands in
	dirs   []string // the directories below root, walked on standing there
	rows   []row
	text   []string // each row's aligned cells, plain, which the query is matched against
	lines  []line
	marked map[string]bool // by rowKey, so a mark survives a refilter
	cursor int
	offset int
	query  string
}

// previewMsg carries a preview, tagged with the cursor move it was fetched for
// so a slower fetch cannot overwrite a newer one.
type previewMsg struct {
	seq  int
	text string
}

// liveMsg is what is running, read again on the refresh tick.
type liveMsg struct {
	live []Session
	err  error
}

// refresh is how often the picker reads what is running, so a session started
// or killed elsewhere shows while the picker is up rather than at its next run.
const refresh = time.Second

// pollSessions reads what is running once refresh has passed.
func pollSessions() tea.Cmd {
	return tea.Tick(refresh, func(time.Time) tea.Msg {
		live, err := sessions()
		return liveMsg{live: live, err: err}
	})
}

// killedMsg reports a kill and what is running after it.
type killedMsg struct {
	live  []Session
	err   error
	stale bool // what is running could not be read, and live says nothing
}

// outcome is what the picker was left with: a session to attach, and where to
// start it when it is new. Empty means leave.
type outcome struct {
	name  string
	dir   string
	place string // the directory chosen, credited as a visit
	take  bool   // detach the other terminal's client first
}

type pickModel struct {
	live     []Session
	held     *Session // what escape leaves for, nil where there is nothing to go back to
	home     string   // where the picker stands on opening
	seen     visits
	attached map[string]int64

	panes [2]*pane
	mode  mode
	query textinput.Model
	match *matcher

	width, height int
	seq           int
	preview       viewport.Model
	steady        steady // what is running, held back until two readings agree
	status        string // an error or a notice, shown until the next key

	asking string // an inline yes-or-no question, empty when none is open
	onYes  func() (tea.Model, tea.Cmd)

	result outcome
}

func newPickModel(live []Session, home string, seen visits, attached map[string]int64, open mode) *pickModel {
	q := textinput.New()
	styles := textinput.DefaultDarkStyles()
	styles.Focused.Prompt = promptStyle
	styles.Focused.Text = inputStyle
	q.SetStyles(styles)
	q.Focus()
	m := &pickModel{
		live: live, held: lastHeld(live),
		home: home, seen: seen, attached: attached,
		query: q, match: newMatcher(), preview: viewport.New(),
	}
	m.panes[modeFiles] = &pane{mode: modeFiles, marked: map[string]bool{}}
	m.panes[modeSessions] = &pane{mode: modeSessions, marked: map[string]bool{}}
	m.stand(m.panes[modeFiles], home)
	m.build(m.panes[modeSessions])
	m.show(open)
	return m
}

func (m *pickModel) top() *pane { return m.panes[m.mode] }

// show makes mo the view on screen, with the query it was left with.
func (m *pickModel) show(mo mode) {
	m.top().query = m.query.Value()
	m.mode = mo
	m.query.SetValue(m.top().query)
	m.query.CursorEnd()
	m.sizeQuery()
}

// where is what the prompt says, in at most width cells: a directory deep
// enough to fill the line keeps its end, which is the part that differs.
func (m *pickModel) where(width int) string {
	if m.mode == modeSessions {
		return "sessions"
	}
	return keepTail(shorten(m.panes[modeFiles].root), max(width, 8))
}

// rowKey is what identifies a row across rebuilds: the session it stands for,
// or what it is and the directory it is about.
func rowKey(r row) string {
	switch r.kind {
	case kindSession:
		return "s:" + r.session
	case kindOffer:
		return "o:" + r.dir
	case kindUp:
		return "u:" + r.dir
	default:
		return "d:" + r.dir
	}
}

// stand makes dir the directory the filesystem view stands in, walking what is
// below it, with the cursor on the directory just left where it is listed.
func (m *pickModel) stand(p *pane, dir string) {
	from := p.root
	p.root = filepath.Clean(dir)
	p.dirs = tree(p.root, treeDepth, keepDirs(m.live, m.seen))
	p.query, p.cursor, p.offset = "", 0, 0
	if m.top() == p {
		m.query.SetValue("")
	}
	m.build(p)
	if from != "" {
		m.seek(p, "d:"+from)
	}
	m.sizeQuery()
}

// build reads p's rows afresh from what is running, keeping the cursor on the
// row it stood on, or at the same height where that row is gone.
func (m *pickModel) build(p *pane) {
	keep := ""
	if r := p.at(p.cursor); r != nil {
		keep = rowKey(*r)
	}
	if p.mode == modeFiles {
		p.rows = fileRows(p.root, m.home, m.live, p.dirs, m.seen)
	} else {
		p.rows = sessionRows(m.live, m.attached)
	}
	present := map[string]bool{}
	for _, r := range p.rows {
		present[rowKey(r)] = true
	}
	p.text = rowText(p.rows)
	for k := range p.marked {
		if !present[k] {
			delete(p.marked, k)
		}
	}
	cursor := p.cursor
	m.refilter(p)
	p.cursor = min(cursor, max(len(p.lines)-1, 0))
	m.seek(p, keep)
}

// rowText is what each row shows, plain. Sessions are aligned against each
// other; a directory is its path alone, since one deep path would otherwise
// push every session's columns aside.
func rowText(rows []row) []string {
	out := make([]string, len(rows))
	var cells [][]string
	var at []int
	for i, r := range rows {
		if r.kind == kindDir {
			out[i] = r.cells[0]
			continue
		}
		cells = append(cells, r.cells)
		at = append(at, i)
	}
	if len(cells) == 0 {
		return out
	}
	for j, line := range align(cells) {
		out[at[j]] = line
	}
	return out
}

// seek puts p's cursor on the row with key, where there is one.
func (m *pickModel) seek(p *pane, key string) {
	for i, ln := range p.lines {
		if key != "" && rowKey(p.rows[ln.row]) == key {
			p.cursor = i
			return
		}
	}
}

// refilter recomputes p's lines for its query: every row with nothing typed;
// otherwise the rows matching it, best first, with the way up and the offer of
// a session left out, since neither is something a query looks for.
func (m *pickModel) refilter(p *pane) {
	p.lines = p.lines[:0]
	type scored struct {
		line
		score int
	}
	var hits []scored
	for i, r := range p.rows {
		if p.query == "" {
			p.lines = append(p.lines, line{row: i})
			continue
		}
		if r.kind == kindUp || r.kind == kindOffer {
			continue
		}
		if s, pos, ok := m.match.match(p.text[i], p.query); ok {
			hits = append(hits, scored{line: line{row: i, pos: pos}, score: s})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	for _, h := range hits {
		p.lines = append(p.lines, h.line)
	}
}

// at is the row drawn at line i, or nil past the end.
func (p *pane) at(i int) *row {
	if i < 0 || i >= len(p.lines) {
		return nil
	}
	return &p.rows[p.lines[i].row]
}

// current is the row under the cursor, or nil where the list is empty.
func (m *pickModel) current() *row {
	p := m.top()
	return p.at(p.cursor)
}

// fetchPreview supersedes whatever preview is in flight with one for the row
// under the cursor.
func (m *pickModel) fetchPreview() tea.Cmd {
	m.seq++
	r := m.current()
	return loadPreview(r, m.seq)
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
	m.preview.SetWidth(w)
	m.preview.SetHeight(h)
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
// scrollback, the git history of a place git tracks, or what any other
// directory holds. A read that fails previews as its error.
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
		case r.dir != "":
			err = listDir(r.dir, &buf)
		}
		if err != nil {
			return previewMsg{seq: seq, text: errStyle.Render(err.Error())}
		}
		return previewMsg{seq: seq, text: buf.String()}
	}
}

// listDir writes what dir holds, a line per entry, directories first and
// marked with a trailing slash.
func listDir(dir string, w *bytes.Buffer) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].IsDir() && !entries[j].IsDir() })
	for _, e := range entries {
		if e.IsDir() {
			w.WriteString(dirStyle.Render(e.Name()+"/") + "\n")
		} else {
			w.WriteString(e.Name() + "\n")
		}
	}
	return nil
}

func (m *pickModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.fetchPreview(), pollSessions())
}

// relive rebuilds every view over live, keeping each cursor where it was. The
// tree is walked again too, since a session standing somewhere new can bring
// a hidden directory into it.
func (m *pickModel) relive(live []Session) {
	m.live = live
	m.held = lastHeld(m.live)
	files := m.panes[modeFiles]
	files.dirs = tree(files.root, treeDepth, keepDirs(m.live, m.seen))
	for _, p := range m.panes {
		m.build(p)
	}
}

func (m *pickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.sizePreview()
		m.sizeQuery()
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
		if msg.stale {
			return m, nil
		}
		m.relive(msg.live)
		return m, m.fetchPreview()

	case liveMsg:
		if msg.err != nil {
			return m, pollSessions()
		}
		fp := fingerprint(msg.live)
		if !m.steady.settled(fp) || fp == fingerprint(m.live) {
			return m, pollSessions()
		}
		m.relive(msg.live)
		return m, tea.Batch(pollSessions(), m.fetchPreview())

	case tea.KeyPressMsg:
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

func (m *pickModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.top()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.held != nil {
			m.result = outcome{name: m.held.Name}
		}
		return m, tea.Quit
	case "tab":
		m.show(1 - m.mode)
		saveMode(m.mode)
		return m, m.fetchPreview()
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
	case "pgup":
		return m.page(-1)
	case "pgdown":
		return m.page(1)
	case "enter":
		return m.enter()
	case "shift+enter":
		if r := m.current(); r != nil && (r.kind == kindSession || r.kind == kindDir) {
			k := rowKey(*r)
			p.marked[k] = !p.marked[k]
			if !p.marked[k] {
				delete(p.marked, k)
			}
		}
		return m.move(1)
	case "alt+enter":
		if r := m.current(); r != nil && r.kind != kindUp && r.dir != "" {
			m.result = outcome{name: sessionName(m.live), dir: r.dir, place: r.dir}
			return m, tea.Quit
		}
		return m, nil
	case "delete", "alt+delete":
		targets := m.killTargets()
		if len(targets) == 0 {
			return m, nil
		}
		var names, labels []string
		for _, s := range targets {
			names = append(names, s.Name)
			labels = append(labels, sessionLabel(s))
		}
		kill := func() (tea.Model, tea.Cmd) {
			p.marked = map[string]bool{}
			return m, func() tea.Msg {
				err := killSessions(names)
				live, listErr := sessions()
				return killedMsg{live: live, err: errors.Join(err, listErr), stale: listErr != nil}
			}
		}
		if msg.String() == "alt+delete" {
			return kill()
		}
		m.asking, m.onYes = question(labels), kill
		return m, nil
	}

	before := m.query.Value()
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	if m.query.Value() != before {
		p.query = m.query.Value()
		m.refilter(p)
		p.cursor, p.offset = 0, 0
		return m, tea.Batch(cmd, m.fetchPreview())
	}
	return m, cmd
}

// enter stands in the directory under the cursor, or the one above; attaches
// the session, taking it after asking where another terminal has it open; or
// starts one where the row offers to.
func (m *pickModel) enter() (tea.Model, tea.Cmd) {
	p := m.top()
	r := m.current()
	if r == nil {
		return m, nil
	}
	p.marked = map[string]bool{}
	switch r.kind {
	case kindDir, kindUp:
		m.stand(p, r.dir)
		return m, m.fetchPreview()
	case kindOffer:
		m.result = outcome{name: sessionName(m.live), dir: r.dir, place: r.dir}
		return m, tea.Quit
	}
	if r.held {
		name, dir := r.session, r.dir
		m.asking = takes(r.cells[0])
		m.onYes = func() (tea.Model, tea.Cmd) {
			m.result = outcome{name: name, place: dir, take: true}
			return m, tea.Quit
		}
		return m, nil
	}
	m.result = outcome{name: r.session, place: r.dir}
	return m, tea.Quit
}

// killTargets is every session the marked rows stand for, or the row under the
// cursor where nothing is marked: a directory stands for every session in it
// or below it.
func (m *pickModel) killTargets() []Session {
	p := m.top()
	var picked []row
	for _, r := range p.rows {
		if p.marked[rowKey(r)] {
			picked = append(picked, r)
		}
	}
	if len(picked) == 0 {
		if r := m.current(); r != nil {
			picked = append(picked, *r)
		}
	}
	seen := map[string]bool{}
	var out []Session
	for _, r := range picked {
		for _, s := range r.found {
			if !seen[s.Name] {
				seen[s.Name] = true
				out = append(out, s)
			}
		}
	}
	return out
}

func (m *pickModel) move(delta int) (tea.Model, tea.Cmd) {
	p := m.top()
	next := p.cursor + delta
	if next < 0 || next >= len(p.lines) {
		return m, nil
	}
	p.cursor = next
	return m, m.fetchPreview()
}

// page moves the cursor a screenful of lines, stopping at either end.
func (m *pickModel) page(delta int) (tea.Model, tea.Cmd) {
	p := m.top()
	next := min(max(p.cursor+delta*m.listRows(), 0), max(len(p.lines)-1, 0))
	if next == p.cursor {
		return m, nil
	}
	p.cursor = next
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

// modeFile is where the view last switched to is kept.
func modeFile() string {
	dir := stateDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "view")
}

// loadMode is the view last switched to, the filesystem where none was.
func loadMode() mode {
	if readLast(modeFile()) == modeSessions.String() {
		return modeSessions
	}
	return modeFiles
}

func saveMode(mo mode) { writeLast(modeFile(), mo.String()) }

// opening puts the sessions view's cursor on the session the pane last held,
// which a detach makes the one just left. The filesystem view always opens at
// the top of ~.
func (m *pickModel) opening() {
	if m.held == nil {
		return
	}
	m.seek(m.panes[modeSessions], "s:"+m.held.Name)
}

// endsHost reports whether attaching from inside host would end it.
//
// zmx 0.8.1 reads an attach made with ZMX_SESSION set as the host's client
// switching to the target, and a host with no client to switch is ended
// instead. A host is clientless where it is driven by `zmx send` rather than
// from a terminal.
func endsHost(live []Session, host string) bool {
	s := running(live, host)
	return s != nil && s.Clients == "0"
}

// unseen is every directory a session stands in that the visits do not know
// yet, so that a picker run without the daemon still learns where sessions are.
func unseen(live []Session, seen visits) []string {
	var out []string
	for _, s := range live {
		if s.Dir == "" {
			continue
		}
		if _, ok := seen[filepath.Clean(s.Dir)]; !ok {
			out = append(out, filepath.Clean(s.Dir))
		}
	}
	return out
}

// run presents the picker and attaches to what it answers.
func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	live, err := sessions()
	if err != nil {
		return err
	}
	seen := loadVisits()
	if fresh := unseen(live, seen); len(fresh) > 0 {
		recordVisits(fresh, time.Now())
		seen = loadVisits()
	}

	m := newPickModel(live, home, seen, loadAttaches(), loadMode())
	m.opening()

	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return err
	}
	res := final.(*pickModel).result
	if res.name == "" {
		return errCancelled
	}
	if live, err := sessions(); err == nil {
		if host := os.Getenv("ZMX_SESSION"); endsHost(live, host) {
			return fmt.Errorf("session %s has no client attached, and attaching from inside it would end it: attach from a terminal", host)
		}
	}
	if res.take {
		if err := detachClients(res.name); err != nil {
			return err
		}
	}
	now := time.Now()
	if res.place != "" {
		recordVisits([]string{filepath.Clean(res.place)}, now)
	}
	recordAttach(res.name, now)
	// Recorded before the exec, since there is no returning from it.
	recordLast(res.name)
	return attach(res.name, res.dir)
}
