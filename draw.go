package main

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The picker looks the way fzf drew it: fzf's dark 256-colour theme, under the
// `--layout=reverse --border` it was run with.
var (
	borderColor   = lipgloss.Color("59")
	ruleStyle     = lipgloss.NewStyle().Foreground(borderColor)
	frameStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).Padding(0, 1)
	promptStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	inputStyle    = lipgloss.NewStyle().Bold(true)
	infoStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("144"))
	headerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("109"))
	dirStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	matchStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
	curMatchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("151")).Bold(true)
	pointerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("161")).Background(lipgloss.Color("236")).Bold(true)
	gutterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	markerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("168"))
	askStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// currentRow is the cursor row's colours and weight, as the escape that sets
// them.
const currentRow = "\x1b[38;5;254;48;5;236;1m"

// inner is the size inside the frame: a border on every side, and a column of
// padding either side of the content.
func (m *pickModel) inner() (int, int) {
	return m.width - 4, m.height - 2
}

// previewBox is where the preview is drawn in the terminal as it now is, and
// the width and height of what it shows.
//
// Beside the list it takes 55% of the width, a rule, a column of padding and
// its scrollbar among it; above the list, half the height, a rule among it,
// and its scrollbar the last column.
func (m *pickModel) previewBox() (layout, int, int) {
	w, h := m.inner()
	switch lay := previewLayout(m.width, m.height); lay {
	case layoutBeside:
		return lay, w*55/100 - 3, h
	case layoutAbove:
		return lay, w - 1, h/2 - 1
	default:
		return lay, 0, 0
	}
}

func (m *pickModel) View() tea.View {
	v := tea.NewView(m.screen())
	v.AltScreen = true
	return v
}

// screen is everything drawn, inside the frame.
func (m *pickModel) screen() string {
	w, h := m.inner()
	if w < 10 || h < 4 {
		return ""
	}
	lay, pw, ph := m.previewBox()
	var lines []string
	switch lay {
	case layoutBeside:
		lw := w - pw - 3
		list := m.listView(lw, h)
		preview := m.previewView()
		for i := range h {
			lines = append(lines, fit(list[i], lw)+ruleStyle.Render("│")+" "+preview[i])
		}
	case layoutAbove:
		lines = append(lines, m.previewView()...)
		lines = append(lines, ruleStyle.Render(strings.Repeat("─", w)))
		lines = append(lines, m.listView(w, h-ph-1)...)
	default:
		lines = m.listView(w, h)
	}
	return frameStyle.Render(strings.Join(lines, "\n"))
}

// previewView is the preview's rows, each cut to the box with the preview's
// scrollbar after it.
func (m *pickModel) previewView() []string {
	v := &m.preview
	rows := strings.Split(v.View(), "\n")
	bar := scrollbar(v.TotalLineCount(), v.Height(), v.YOffset())
	out := make([]string, v.Height())
	for i := range out {
		var row string
		if i < len(rows) {
			row = rows[i]
		}
		out[i] = fit(row, v.Width()) + bar[i]
	}
	return out
}

// scrollbar is the column beside height rows showing where among total lines
// the first of them, offset, stands: a thumb and no track, sized and placed as
// fzf places its own, and blank where every line fits.
func scrollbar(total, height, offset int) []string {
	out := make([]string, max(height, 0))
	for i := range out {
		out[i] = " "
	}
	if total <= height || height <= 0 {
		return out
	}
	length := max(1, height*height/total)
	start := min(height-length, (height-length)*offset/(total-height))
	for i := start; i < start+length; i++ {
		out[i] = ruleStyle.Render("│")
	}
	return out
}

// fit cuts s to width cells, or pads it out to them.
func fit(s string, width int) string {
	s = ansi.Truncate(s, width, "")
	return s + "\x1b[0m" + strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
}

// header is the open question, an error, or the keys that would do something
// where the cursor stands.
func (m *pickModel) header() string {
	switch {
	case m.asking != "":
		return askStyle.Render(m.asking + " y/N")
	case m.status != "":
		return errStyle.Render(m.status)
	}
	keys := m.hints()
	if m.mode == modeRepos {
		keys = append(keys, "⇥ dirs")
	} else {
		keys = append(keys, "⇥ repos")
	}
	if m.held != nil {
		keys = append(keys, "⎋ leave")
	}
	return headerStyle.Render(strings.Join(keys, "  "))
}

// hints is the keys worth offering for the line under the cursor.
//
// A key that would do nothing is not offered: nothing to take where nobody
// else holds the session, nothing to kill where nothing is running, and
// `⏎ start` rather than `⏎ attach` where entering means making one.
func (m *pickModel) hints() []string {
	ln, pl, r := m.current()
	switch {
	case ln == nil:
		return nil
	case ln.kid < 0:
		keys := []string{"⏎ open"}
		if m.top().query == "" && m.top().expanded(m.top().cursor) {
			keys[0] = "⏎ close"
		}
		keys = append(keys, "⌥⏎ new", "⇧⏎ mark")
		if pl.row.count > 0 {
			keys = append(keys, "⌦ kill")
		}
		return keys
	case r.session == "":
		return []string{"⏎ start"}
	case r.held:
		return []string{"⏎ take", "⌥⏎ new", "⇧⏎ mark", "⌦ kill"}
	default:
		return []string{"⏎ attach", "⌥⏎ new", "⇧⏎ mark", "⌦ kill"}
	}
}

// info is the count of places the query left of those in the view, the
// marked lines after it, and a rule to the edge.
func (m *pickModel) info(width int) string {
	p := m.top()
	shown := 0
	for _, ln := range p.lines {
		if ln.kid < 0 {
			shown++
		}
	}
	count := strconv.Itoa(shown) + "/" + strconv.Itoa(len(p.places))
	if n := len(p.marked); n > 0 {
		count += " (" + strconv.Itoa(n) + ")"
	}
	rule := max(width-len(count)-3, 0)
	return "  " + infoStyle.Render(count) + " " + ruleStyle.Render(strings.Repeat("─", rule))
}

// listView is the query, the info line, the header and the rows under them,
// height lines in all, with the list's scrollbar in the last column beside
// the rows.
func (m *pickModel) listView(width, height int) []string {
	head := []string{m.query.View(), m.info(width), m.header()}
	for i := range head {
		head[i] = fit(head[i], width)
	}
	rows := max(height-len(head), 1)
	body := m.rowsView(width-1, rows)
	bar := scrollbar(len(m.top().lines), rows, m.top().offset)
	for i := range body {
		body[i] = fit(body[i], width-1) + bar[i]
	}
	return append(head, body...)
}

// rowsView is the lines of the view that fit in height rows, kept around the
// cursor. A place is marked open or closed, and what it holds is indented
// under it.
func (m *pickModel) rowsView(width, height int) []string {
	var lines []string
	p := m.top()
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+height {
		p.offset = p.cursor - height + 1
	}
	if len(p.places) == 0 {
		lines = append(lines, "  "+headerStyle.Render(empty(p.mode)))
	}
	for i := p.offset; i < len(p.lines) && len(lines) < height; i++ {
		ln := p.lines[i]
		pl := p.places[ln.place]
		on := i == p.cursor
		marker := " "
		if p.marked[lineKey(p, ln)] {
			marker = markerStyle.Render("┃")
		}
		var r row
		var text, lead string
		if ln.kid < 0 {
			r, text = pl.row, p.text[ln.place]
			lead = "▸ "
			if p.expanded(i) {
				lead = "▾ "
			}
		} else {
			r, text = pl.kids[ln.kid], p.kidText[ln.place][ln.kid]
			lead = "    "
		}
		status := r.status
		if status == "" {
			status = strings.Repeat(" ", statusWidth)
		}
		body := ansi.Truncate(marker+ruleStyle.Render(lead)+status+highlight(text, ln.pos, on), width-1, "…")
		if on {
			lines = append(lines, pointerStyle.Render("▌")+underCursor(body, width-1))
		} else {
			lines = append(lines, gutterStyle.Render("▌")+body)
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

// underCursor paints line in the cursor row's colours across width cells.
//
// The line carries its own SGR resets, from the status column's dim and from
// the match highlight, and each one would end a style set only once at the
// start; so it is set again after every reset.
func underCursor(line string, width int) string {
	pad := max(width-ansi.StringWidth(line), 0)
	line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+currentRow)
	line = strings.ReplaceAll(line, "\x1b[m", "\x1b[m"+currentRow)
	return currentRow + line + strings.Repeat(" ", pad) + "\x1b[0m"
}

// highlight marks the runes of text at the matched positions, in the cursor
// row's match colour where on is set.
func highlight(text string, pos []int, on bool) string {
	if len(pos) == 0 {
		return text
	}
	style := matchStyle
	if on {
		style = curMatchStyle
	}
	at := make(map[int]bool, len(pos))
	for _, p := range pos {
		at[p] = true
	}
	var b strings.Builder
	for i, r := range []rune(text) {
		if at[i] {
			b.WriteString(style.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
