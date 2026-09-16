package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// The two lists. Directories are where a session can be, sessions are where
// one is, and tab is the whole of the navigation between them.
type mode int

const (
	modeDir mode = iota
	modeSession
)

// A session with a client attached is filled, one with none is hollow. The
// column is one circle per session, in the order the session list shows them,
// so the second circle and the second row are the same session.
const (
	attached = "●"
	detached = "○"
)

// row is one line of either list: what to attach to, or what to create.
type row struct {
	session string   // the session to attach to, empty where one has to be made
	name    string   // the name to make it under
	dir     string   // where to make it
	several bool     // more sessions here than enter can choose between
	cells   []string // what the line shows
}

// circles renders the sessions in one directory.
func circles(found []Session) string {
	var b strings.Builder
	for _, s := range found {
		if s.Clients != "0" && s.Clients != "" {
			b.WriteString(attached)
			continue
		}
		b.WriteString(detached)
	}
	return b.String()
}

// byDir groups sessions by the directory they run in, each group by name.
//
// Grouping by directory rather than by name is what makes a session this tool
// never named, supacode's among them, count against the repository it sits in.
func byDir(live []Session) map[string][]Session {
	found := make(map[string][]Session)
	for _, s := range live {
		if s.Dir == "" {
			continue
		}
		key := filepath.Clean(s.Dir)
		found[key] = append(found[key], s)
	}
	for key := range found {
		sort.Slice(found[key], func(i, j int) bool { return found[key][i].Name < found[key][j].Name })
	}
	return found
}

// dirRows returns one row per place a session can be started.
func dirRows(found []Target, live []Session) []row {
	here := byDir(live)

	rows := make([]row, len(found))
	for i, t := range found {
		in := here[filepath.Clean(t.Dir)]
		r := row{name: t.Name, dir: t.Dir, several: len(in) > 1}
		if len(in) == 1 {
			r.session = in[0].Name
		}
		r.cells = []string{circles(in), t.Name, shorten(t.Dir)}
		rows[i] = r
	}
	return rows
}

// sessionRows returns one row per running session, or per session in dir
// where dir is given.
func sessionRows(live []Session, dir string) []row {
	var rows []row
	for _, s := range sortedByName(live) {
		if dir != "" && filepath.Clean(s.Dir) != filepath.Clean(dir) {
			continue
		}
		rows = append(rows, row{
			session: s.Name,
			cells:   []string{circles([]Session{s}), s.Name, shorten(s.Dir)},
		})
	}
	return rows
}

func sortedByName(live []Session) []Session {
	out := append([]Session(nil), live...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// lines renders rows for fzf: the index, what the preview reads, and what is
// shown. Only the last is searched, and the index is what comes back.
func lines(rows []row) []string {
	cells := make([][]string, len(rows))
	for i, r := range rows {
		cells[i] = r.cells
	}
	display := align(cells)

	out := make([]string, len(rows))
	for i, r := range rows {
		preview := r.dir
		if preview == "" {
			preview = r.session
		}
		out[i] = strconv.Itoa(i) + "\t" + preview + "\t" + display[i]
	}
	return out
}

// at returns the row whose directory holds dir most closely, or -1.
//
// The longest directory that is a prefix wins, so a worktree is answered with
// itself rather than with the repository it belongs to.
func at(rows []row, dir string) int {
	if dir == "" {
		return -1
	}
	dir = filepath.Clean(dir)
	best, length := -1, 0
	for i, r := range rows {
		if r.dir == "" {
			continue
		}
		base := filepath.Clean(r.dir)
		if dir != base && !strings.HasPrefix(dir, base+string(filepath.Separator)) {
			continue
		}
		if len(base) > length {
			best, length = i, len(base)
		}
	}
	return best
}

// winsize is what TIOCGWINSZ fills in.
type winsize struct{ rows, cols, xpixel, ypixel uint16 }

// termSize returns the terminal's columns and rows, or zeroes where there is
// no terminal to ask.
func termSize() (cols, rows int) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return 0, 0
	}
	defer tty.Close()

	var ws winsize
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		tty.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	); errno != 0 {
		return 0, 0
	}
	return int(ws.cols), int(ws.rows)
}

// previewLayout returns the --preview-window for a terminal this size, or ""
// where a preview would leave too little of the list to read.
//
// The split follows the shape of the space rather than a preference: a wide
// terminal has room beside the list, a tall narrow one only below it. The
// alternative fzf carries is what answers a resize made while it is up.
func previewLayout(cols, rows int) string {
	switch {
	case cols < 80 || rows < 15:
		return ""
	case cols >= 120:
		return "right,55%,border-left,<100(up,50%,border-bottom)"
	default:
		return "up,50%,border-bottom"
	}
}

// args returns the picker's arguments for one mode.
//
// The query sits at the top, where the list grows away from it, and the only
// keys the picker hands back are the two the caller acts on.
func args(m mode, prompt string, cursor int) []string {
	out := []string{
		"--delimiter=\t",
		"--with-nth=3..",
		"--height=100%",
		"--layout=reverse",
		"--no-sort",
		"--prompt=" + prompt,
		"--expect=" + strings.Join([]string{keyMode, keyModeBack, keyNew}, ","),
	}

	preview := "zmx history {2}"
	header := "⏎ attach   tab directories"
	if m == modeDir {
		preview = "lsd -la --color=always {2}"
		header = "⏎ attach or start   ⌥⏎ start another   tab sessions"
	}
	if layout := previewLayout(termSize()); layout != "" {
		out = append(out, "--preview="+preview, "--preview-window="+layout)
	}
	out = append(out, "--header="+header)

	// The cursor starts on the directory this pane was opened in, so a new
	// pane is one keystroke from where it stands.
	//
	// --sync is what makes it land: an action bound to start runs as soon as
	// the picker opens, which is before the list it moves through has
	// arrived, and moving to the second line of nothing does nothing.
	if cursor > 0 {
		out = append(out, "--sync", "--bind=start:pos("+strconv.Itoa(cursor+1)+")")
	}
	return out
}

// navigate walks the two lists until something is chosen or the picker is
// left, and returns the session to attach to and where to create it.
func navigate(root string) (name, dir string, err error) {
	m, filter := modeDir, ""
	for {
		live, err := sessions()
		if err != nil {
			return "", "", err
		}

		var rows []row
		var prompt string
		switch m {
		case modeDir:
			found := targets(root)
			if len(found) == 0 {
				return "", "", fmt.Errorf("no repositories below %s", shorten(root))
			}
			rows, prompt = dirRows(found, live), "zmx dir> "
		default:
			rows = sessionRows(live, filter)
			prompt = "zmx session> "
			if filter != "" {
				prompt = "zmx " + filepath.Base(filter) + "> "
			}
			if len(rows) == 0 {
				m, filter = modeDir, ""
				continue
			}
		}

		cursor := -1
		if m == modeDir {
			if here, err := os.Getwd(); err == nil {
				cursor = at(rows, here)
			}
		}

		key, index, err := present(lines(rows), args(m, prompt, cursor))
		if errors.Is(err, errCancelled) {
			return "", "", errCancelled
		}
		if err != nil {
			return "", "", err
		}
		chosen := rows[index]

		switch key {
		case keyMode, keyModeBack:
			if m == modeDir {
				m = modeSession
			} else {
				m = modeDir
			}
			filter = ""
			continue

		case keyNew:
			if m != modeDir {
				continue
			}
			label, err := promptText("label> ", "a second session in "+chosen.name)
			if errors.Is(err, errCancelled) {
				continue
			}
			if err != nil {
				return "", "", err
			}
			return sessionSafe(chosen.name + "+" + label), chosen.dir, nil
		}

		// Enter. A directory holding more than one session cannot be
		// answered with a guess, so it opens the list that can.
		if chosen.several {
			m, filter = modeSession, chosen.dir
			continue
		}
		if chosen.session != "" {
			return chosen.session, "", nil
		}
		return chosen.name, chosen.dir, nil
	}
}

// run presents the picker and attaches to what it answers.
func run(root string) error {
	name, dir, err := navigate(root)
	if err != nil {
		return err
	}
	return attach(name, dir)
}
