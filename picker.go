package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// line is one line of the list as drawn: a place's own row, or one of the
// rows it expands into, and where the query landed in it.
type line struct {
	place int // index into the pane's places
	kid   int // index into that place's kids, or -1 for the place itself
	pos   []int
}

// pane is one view with everything the picker holds of it, so switching away
// and back lands where it was left.
type pane struct {
	mode    mode
	places  []place
	text    []string   // each place's aligned cells, plain, which the query is matched against
	kidText [][]string // the same for each place's kids
	lines   []line
	open    map[string]bool // expanded places, by directory
	marked  map[string]bool // by lineKey, so a mark survives a refilter
	cursor  int
	offset  int
	query   string
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

// reposMsg is the repositories, walked or asked for again.
type reposMsg struct {
	repos []string
	ident map[string]string
}

// rewalk is how often the picker asks for the repositories again, so one
// cloned while it is up shows. A daemon answers at once; without one this is
// a walk of ~, which is why it is not every second.
const rewalk = 30 * time.Second

// pollRepos asks for the repositories below home once rewalk has passed.
func pollRepos(home string) tea.Cmd {
	return tea.Tick(rewalk, func(time.Time) tea.Msg {
		repos, ident := walked(context.Background(), dialDeck(), home)
		return reposMsg{repos: repos, ident: ident}
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
	live  []Session
	held  *Session // what escape leaves for, nil where there is nothing to go back to
	repos []string
	ident map[string]string
	home  string // where repositories are walked from, "" for never again
	start string
	seen  visits
	now   time.Time

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

func newPickModel(live []Session, repos []string, ident map[string]string, start string, seen visits, open mode) *pickModel {
	q := textinput.New()
	styles := textinput.DefaultDarkStyles()
	styles.Focused.Prompt = promptStyle
	styles.Focused.Text = inputStyle
	q.SetStyles(styles)
	q.Focus()
	m := &pickModel{
		live: live, held: lastHeld(live), repos: repos, ident: ident,
		start: start, seen: seen, now: time.Now(),
		query: q, match: newMatcher(), preview: viewport.New(),
	}
	for _, mo := range []mode{modeRepos, modeDirs} {
		m.panes[mo] = &pane{mode: mo, open: map[string]bool{}, marked: map[string]bool{}}
		m.build(m.panes[mo])
	}
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
	m.query.Prompt = mo.String() + "> "
	m.sizeQuery()
}

// lineKey is what identifies a line across rebuilds: the place it is, the
// session it stands for, or the directory it offers to start in.
func lineKey(p *pane, ln line) string {
	pl := p.places[ln.place]
	if ln.kid < 0 {
		return "p:" + pl.row.dir
	}
	r := pl.kids[ln.kid]
	if r.session != "" {
		return "s:" + r.session
	}
	return "o:" + r.dir + ":" + r.cells[0]
}

// build reads p's places afresh from what is running, keeping the cursor on
// the line it stood on, or at the same height where that line is gone.
func (m *pickModel) build(p *pane) {
	keep := ""
	if p.cursor < len(p.lines) {
		keep = lineKey(p, p.lines[p.cursor])
	}
	if p.mode == modeDirs {
		p.places = dirPlaces(m.live, m.repos, m.start, m.seen, m.now)
	} else {
		p.places = repoPlaces(m.live, m.repos, m.ident, m.seen, m.now)
	}

	cells := make([][]string, len(p.places))
	var kidCells [][]string
	for i, pl := range p.places {
		cells[i] = pl.row.cells
		for _, k := range pl.kids {
			kidCells = append(kidCells, k.cells)
		}
	}
	p.text = align(cells)
	flat := align(kidCells)
	p.kidText = make([][]string, len(p.places))
	n := 0
	for i, pl := range p.places {
		p.kidText[i] = flat[n : n+len(pl.kids)]
		n += len(pl.kids)
	}

	present := map[string]bool{}
	for i, pl := range p.places {
		present[lineKey(p, line{place: i, kid: -1})] = true
		for k := range pl.kids {
			present[lineKey(p, line{place: i, kid: k})] = true
		}
	}
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

// seek puts p's cursor on the line with key, where there is one.
func (m *pickModel) seek(p *pane, key string) {
	for i, ln := range p.lines {
		if key != "" && lineKey(p, ln) == key {
			p.cursor = i
			return
		}
	}
}

// refilter recomputes p's lines for its query.
//
// With nothing typed every place is listed, and an expanded one is followed by
// what it holds. A query is matched against each place, and against the label
// of each session or worktree it holds: a place matching only itself is listed
// closed, and one holding a match is listed open on its matches alone, so a
// match is never shown without the place it stands in. Places are ranked by
// their best match, and the matches within one by their own.
func (m *pickModel) refilter(p *pane) {
	p.lines = p.lines[:0]
	if p.query == "" {
		for i, pl := range p.places {
			p.lines = append(p.lines, line{place: i, kid: -1})
			if p.open[pl.row.dir] {
				for k := range pl.kids {
					p.lines = append(p.lines, line{place: i, kid: k})
				}
			}
		}
		return
	}

	type scored struct {
		line
		score int
	}
	type group struct {
		best  int
		lines []scored
	}
	var groups []group
	for i, pl := range p.places {
		g := group{best: -1}
		head := scored{line: line{place: i, kid: -1}}
		if s, pos, ok := m.match.match(p.text[i], p.query); ok {
			head.score, head.pos, g.best = s, pos, s
		}
		var kids []scored
		for k, kid := range pl.kids {
			if kid.cells[0] == newSession {
				continue
			}
			if s, pos, ok := m.match.match(kid.cells[0], p.query); ok {
				kids = append(kids, scored{line: line{place: i, kid: k, pos: pos}, score: s})
				g.best = max(g.best, s)
			}
		}
		if g.best < 0 {
			continue
		}
		sort.SliceStable(kids, func(a, b int) bool { return kids[a].score > kids[b].score })
		g.lines = append([]scored{head}, kids...)
		groups = append(groups, g)
	}
	sort.SliceStable(groups, func(a, b int) bool { return groups[a].best > groups[b].best })
	for _, g := range groups {
		for _, s := range g.lines {
			p.lines = append(p.lines, s.line)
		}
	}
}

// current is the line under the cursor, the place it belongs to, and the row
// it draws; nil where the list is empty.
func (m *pickModel) current() (*line, *place, *row) {
	p := m.top()
	if p.cursor < 0 || p.cursor >= len(p.lines) {
		return nil, nil, nil
	}
	ln := &p.lines[p.cursor]
	pl := &p.places[ln.place]
	if ln.kid < 0 {
		return ln, pl, &pl.row
	}
	return ln, pl, &pl.kids[ln.kid]
}

// expanded reports whether the place at line i is followed by what it holds.
func (p *pane) expanded(i int) bool {
	return i+1 < len(p.lines) && p.lines[i+1].kid >= 0 && p.lines[i+1].place == p.lines[i].place
}

// fetchPreview supersedes whatever preview is in flight with one for the row
// under the cursor.
func (m *pickModel) fetchPreview() tea.Cmd {
	m.seq++
	_, _, r := m.current()
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
	_, _, r := m.current()
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
	cmds := []tea.Cmd{textinput.Blink, m.fetchPreview(), pollSessions()}
	if m.home != "" {
		cmds = append(cmds, pollRepos(m.home))
	}
	return tea.Batch(cmds...)
}

// relive rebuilds every view over live, keeping each cursor where it was.
func (m *pickModel) relive(live []Session) {
	m.live = live
	m.held = lastHeld(m.live)
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

	case reposMsg:
		if slices.Equal(msg.repos, m.repos) && maps.Equal(msg.ident, m.ident) {
			return m, pollRepos(m.home)
		}
		m.repos, m.ident = msg.repos, msg.ident
		m.relive(m.live)
		return m, pollRepos(m.home)

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
	case "enter":
		return m.enter()
	case "shift+enter":
		if ln, _, r := m.current(); ln != nil && (ln.kid < 0 || r.session != "") {
			k := lineKey(p, *ln)
			p.marked[k] = !p.marked[k]
			if !p.marked[k] {
				delete(p.marked, k)
			}
		}
		return m.move(1)
	case "alt+enter":
		if _, _, r := m.current(); r != nil && r.dir != "" {
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

// enter opens or closes the place under the cursor, attaches the session, or
// starts one where the row offers to.
//
// Opening a place that a query found clears the query, since what it holds is
// what was being looked for, and leaves the cursor on the place, open.
func (m *pickModel) enter() (tea.Model, tea.Cmd) {
	p := m.top()
	ln, pl, r := m.current()
	if ln == nil {
		return m, nil
	}
	p.marked = map[string]bool{}
	if ln.kid < 0 {
		key := lineKey(p, *ln)
		if p.query != "" {
			p.query = ""
			m.query.SetValue("")
			p.open[pl.row.dir] = true
		} else {
			p.open[pl.row.dir] = !p.open[pl.row.dir]
		}
		m.refilter(p)
		m.seek(p, key)
		return m, m.fetchPreview()
	}
	if r.session == "" {
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

// killTargets is every session the marked lines stand for, or the line under
// the cursor where nothing is marked: a place stands for everything it holds.
func (m *pickModel) killTargets() []Session {
	p := m.top()
	var picked []row
	for i, pl := range p.places {
		if p.marked[lineKey(p, line{place: i, kid: -1})] {
			picked = append(picked, pl.row)
			continue
		}
		for k, kid := range pl.kids {
			if p.marked[lineKey(p, line{place: i, kid: k})] {
				picked = append(picked, kid)
			}
		}
	}
	if len(picked) == 0 {
		if _, _, r := m.current(); r != nil {
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

// loadMode is the view last switched to, the repositories where none was.
func loadMode() mode {
	if readLast(modeFile()) == modeDirs.String() {
		return modeDirs
	}
	return modeRepos
}

func saveMode(mo mode) { writeLast(modeFile(), mo.String()) }

// opening puts the cursor where the picker is opened for: on the session the
// pane last held, its place open around it; or, for a pane that has held
// nothing, on the offer of a session where the pane stands.
func (m *pickModel) opening() {
	p := m.top()
	want := ""
	var dir string
	switch {
	case m.held != nil:
		want = "s:" + m.held.Name
	case m.start != "":
		dir = placeOf(m.start)
	}
	for i, pl := range p.places {
		for k, kid := range pl.kids {
			hit := want != "" && lineKey(p, line{place: i, kid: k}) == want
			if want == "" && dir != "" && pl.row.dir == filepath.Clean(dir) && kid.cells[0] == newSession {
				hit = true
			}
			if hit {
				key := lineKey(p, line{place: i, kid: k})
				p.open[pl.row.dir] = true
				m.refilter(p)
				m.seek(p, key)
				return
			}
		}
	}
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

// unseen is the place of every session the visits do not know yet, so that
// a picker run without the daemon still learns where sessions are.
func unseen(live []Session, seen visits) []string {
	var out []string
	for _, s := range live {
		if s.Dir == "" {
			continue
		}
		if _, ok := seen[placeOf(s.Dir)]; !ok {
			out = append(out, placeOf(s.Dir))
		}
	}
	return out
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
	seen := loadVisits()
	if fresh := unseen(live, seen); len(fresh) > 0 {
		recordVisits(fresh, time.Now())
		seen = loadVisits()
	}

	m := newPickModel(live, repos, ident, start, seen, loadMode())
	m.home = home
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
	if res.place != "" {
		recordVisits([]string{placeOf(res.place)}, time.Now())
	}
	// Recorded before the exec, since there is no returning from it.
	recordLast(res.name)
	return attach(res.name, res.dir)
}
