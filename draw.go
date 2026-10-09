package main

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

func (m *pickModel) View() string {
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
	bar := scrollbar(v.TotalLineCount(), v.Height, v.YOffset)
	out := make([]string, v.Height)
	for i := range out {
		var row string
		if i < len(rows) {
			row = rows[i]
		}
		out[i] = fit(row, v.Width) + bar[i]
	}
	return out
}

// scrollbar is the column beside height rows showing where among total lines
// the first of them, offset, stands: a thumb and no track, sized and placed as
// fzf places its own, and blank where every line fits.
func scrollbar(total, height, offset int) []string {
	out := make([]string, height)
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
	var keys string
	if r := m.current(); r != nil && r.dir != "" {
		keys = hints(r.state())
	}
	if len(m.stack) > 1 {
		keys += "  ← back"
	}
	if m.top().rung.view != viewRepos {
		keys += "  ⌃g repos"
	}
	if m.held != nil {
		keys += "  ⎋ leave"
	}
	return headerStyle.Render(strings.TrimSpace(keys))
}

// info is the count of rows the query left of those on the rung, the marked
// ones after it, and a rule to the edge.
func (m *pickModel) info(width int) string {
	l := m.top()
	count := strconv.Itoa(len(l.hits)) + "/" + strconv.Itoa(len(l.items))
	if n := len(l.marked); n > 0 {
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
	rows := max(height-len(head), 1)
	body := m.rowsView(width-1, rows)
	bar := scrollbar(len(m.top().hits), rows, m.top().offset)
	for i := range body {
		body[i] = fit(body[i], width-1) + bar[i]
	}
	return append(head, body...)
}

// rowsView is the rows of the top rung that fit in height lines, kept around
// the cursor.
func (m *pickModel) rowsView(width, height int) []string {
	var lines []string
	l := m.top()
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+height {
		l.offset = l.cursor - height + 1
	}
	if len(l.items) == 0 {
		lines = append(lines, "  "+headerStyle.Render(empty(l.rung)))
	}
	for i := l.offset; i < len(l.hits) && len(lines) < height; i++ {
		h := l.hits[i]
		it := l.items[h.item]
		on := i == l.cursor
		marker := " "
		if l.marked[rowKey(it.row)] {
			marker = markerStyle.Render("┃")
		}
		status := it.row.status
		if status == "" {
			status = strings.Repeat(" ", statusWidth)
		}
		text := ansi.Truncate(marker+status+highlight(it.text, h.pos, on), width-1, "…")
		if on {
			lines = append(lines, pointerStyle.Render("▌")+underCursor(text, width-1))
		} else {
			lines = append(lines, gutterStyle.Render("▌")+text)
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
