package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// The three lists. Folders are the filesystem, walked a level at a time;
// repositories are the directories below home holding a `.git`; sessions are
// what is running. tab moves between them and is the whole of the navigation.
type mode int

const (
	modeFolders mode = iota
	modeRepos
	modeSessions
)

func (m mode) String() string {
	switch m {
	case modeRepos:
		return "repos"
	case modeSessions:
		return "sessions"
	}
	return "folders"
}

func modeNamed(name string) (mode, bool) {
	for _, m := range []mode{modeFolders, modeRepos, modeSessions} {
		if m.String() == name {
			return m, true
		}
	}
	return 0, false
}

// next returns the view one step along, wrapping at both ends.
func (m mode) next(forward bool) mode {
	if forward {
		return mode((int(m) + 1) % 3)
	}
	return mode((int(m) + 2) % 3)
}

// A session with a client attached is filled, one with none is hollow. The
// column is one circle per session, in the order the session list shows them,
// so the second circle and the second row are the same session.
const (
	attached = "●"
	detached = "○"
)

// row is one line of any of the lists.
type row struct {
	session string   // the session to attach to, empty where one has to be made
	dir     string   // what the row stands for, empty for a session row
	several bool     // more sessions here than one keystroke can choose between
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
// never named, supacode's among them, count against the directory it sits in.
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

// dirRows returns one row per directory, marked with what runs in it.
func dirRows(dirs []string, live []Session, label func(string) string) []row {
	here := byDir(live)

	rows := make([]row, len(dirs))
	for i, dir := range dirs {
		in := here[filepath.Clean(dir)]
		r := row{dir: dir, several: len(in) > 1}
		if len(in) == 1 {
			r.session = in[0].Name
		}
		r.cells = []string{circles(in), label(dir)}
		rows[i] = r
	}
	return rows
}

// sessionRows returns one row per running session, or per session in dir
// where dir is given.
func sessionRows(live []Session, dir string) []row {
	sorted := append([]Session(nil), live...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var rows []row
	for _, s := range sorted {
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
// itself rather than with the repository above it.
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

// args returns the picker's arguments for one view.
//
// The query sits at the top, where the list grows away from it, and the only
// keys the picker hands back are the ones the caller acts on.
func args(m mode, prompt, header string, cursor int) []string {
	out := []string{
		"--delimiter=\t",
		"--with-nth=3..",
		"--height=100%",
		"--layout=reverse",
		"--no-sort",
		"--prompt=" + prompt,
		"--expect=" + strings.Join([]string{
			keyMode, keyModeBack, keyNew, keyKill, keyKillHard, keyUp, keyFind,
		}, ","),
	}

	preview := "lsd -la --color=always {2}"
	if m == modeSessions {
		preview = "zmx history {2}"
	}
	if layout := previewLayout(termSize()); layout != "" {
		out = append(out, "--preview="+preview, "--preview-window="+layout)
	}
	out = append(out, "--header="+header)

	// The cursor starts where the pane stands, so a new pane is one keystroke
	// from where it is. --sync is what makes it land: an action bound to
	// start otherwise runs before the list it moves through has arrived.
	if cursor > 0 {
		out = append(out, "--sync", "--bind=start:pos("+strconv.Itoa(cursor+1)+")")
	}
	return out
}

// view is where the walk of the lists has got to.
type view struct {
	mode   mode
	dir    string // folders: the directory listed. sessions: the one filtered to.
	search bool   // folders: listing everything below dir rather than its children
}

// navigate walks the lists until something is chosen or the picker is left,
// and returns the session to attach to and where to create it.
func navigate(start string) (name, dir string, err error) {
	v := opening(start)
	for {
		live, err := sessions()
		if err != nil {
			return "", "", err
		}

		rows, prompt, header := list(v, live)
		// fzf given nothing to show exits at once, which would take the
		// picker with it: an empty list is a line saying so, and tab still
		// leads out of it.
		if len(rows) == 0 {
			rows = []row{{cells: []string{"", empty(v)}}}
		}
		cursor := -1
		if v.mode != modeSessions {
			cursor = at(rows, start)
		}

		key, index, err := present(lines(rows), args(v.mode, prompt, header, cursor))
		if errors.Is(err, errCancelled) {
			return "", "", errCancelled
		}
		if err != nil {
			return "", "", err
		}
		var chosen row
		if index < len(rows) {
			chosen = rows[index]
		}

		switch key {
		case keyMode, keyModeBack:
			v = view{mode: v.mode.next(key == keyMode), dir: v.dir}
			if v.mode == modeFolders && v.dir == "" {
				v.dir = filepath.Dir(start)
			}
			remember(start, v.mode)
			continue

		case keyUp:
			if v.mode == modeFolders {
				v = view{mode: modeFolders, dir: filepath.Dir(v.dir)}
			}
			continue

		case keyFind:
			if v.mode == modeFolders {
				v.search = !v.search
			}
			continue

		case keyKill, keyKillHard:
			// A directory holding several is the same ambiguity enter meets,
			// and killing every one of them is not what one keystroke means.
			if chosen.several {
				v = view{mode: modeSessions, dir: chosen.dir}
				continue
			}
			if chosen.session == "" {
				continue
			}
			if key == keyKill {
				ok, err := confirm("kill " + chosen.session + "?")
				if err != nil && !errors.Is(err, errCancelled) {
					return "", "", err
				}
				if !ok {
					continue
				}
			}
			if err := killSession(chosen.session); err != nil {
				return "", "", err
			}
			continue

		case keyNew:
			if chosen.dir == "" {
				continue
			}
			if chosen.session == "" && !chosen.several {
				return sessionName(chosen.dir), chosen.dir, nil
			}
			label, err := promptText("label> ", "another session in "+shorten(chosen.dir))
			if errors.Is(err, errCancelled) {
				continue
			}
			if err != nil {
				return "", "", err
			}
			return sessionName(chosen.dir) + "+" + label, chosen.dir, nil
		}

		// Enter. In the folders list it walks: a directory is a place to go
		// into, and alt-enter is what opens a session there.
		if v.mode == modeFolders {
			if chosen.dir != "" {
				v = view{mode: modeFolders, dir: chosen.dir}
			}
			continue
		}
		if chosen.several {
			v = view{mode: modeSessions, dir: chosen.dir}
			continue
		}
		if chosen.session != "" {
			return chosen.session, "", nil
		}
		if chosen.dir != "" {
			return sessionName(chosen.dir), chosen.dir, nil
		}
		// The line saying the list is empty stands for nothing to attach to.
		continue
	}
}

// empty is what a list with nothing in it says.
func empty(v view) string {
	switch v.mode {
	case modeRepos:
		return "no repositories below home"
	case modeSessions:
		if v.dir != "" {
			return "no sessions in " + shorten(v.dir)
		}
		return "no sessions running"
	}
	return "no folders in " + shorten(v.dir)
}

// opening returns the view a pane standing in start opens on: the one it was
// left on there, and otherwise the one that matches where it stands.
func opening(start string) view {
	if m, ok := rememberedView(start); ok {
		if m == modeFolders {
			return view{mode: modeFolders, dir: filepath.Dir(start)}
		}
		return view{mode: m}
	}
	if isRepo(start) {
		return view{mode: modeRepos}
	}
	return view{mode: modeFolders, dir: filepath.Dir(start)}
}

// isRepo reports whether dir holds a `.git`, which a repository and a linked
// worktree both do.
func isRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// list renders the view: its rows, its prompt and what its keys do.
func list(v view, live []Session) ([]row, string, string) {
	switch v.mode {
	case modeRepos:
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		rows := dirRows(repositories(home), live, shorten)
		return rows, "zmx repos> ", "⏎ attach or start   ⌥⏎ label   ⌦ kill   tab folders"

	case modeSessions:
		prompt := "zmx sessions> "
		if v.dir != "" {
			prompt = "zmx " + filepath.Base(v.dir) + "> "
		}
		return sessionRows(live, v.dir), prompt, "⏎ attach   ⌦ kill   tab folders"

	default:
		dirs, header := folders(v.dir), "⏎ enter   ⌥⏎ start here   ← up   ⌃f search   tab repos"
		label := filepath.Base
		if v.search {
			dirs = under(v.dir)
			header = "⏎ enter   ⌥⏎ start here   ⌃f children only   tab repos"
			label = func(dir string) string {
				rel, err := filepath.Rel(v.dir, dir)
				if err != nil {
					return dir
				}
				return rel
			}
		}
		return dirRows(dirs, live, label), "zmx " + shorten(v.dir) + "> ", header
	}
}

// confirm asks a yes or no question, answered with the same arrows as
// everything else. The cursor starts on no, and escape is no.
func confirm(question string) (bool, error) {
	_, index, err := present(
		[]string{"0\t\tno", "1\t\tyes"},
		[]string{
			"--delimiter=\t",
			"--with-nth=3..",
			"--height=100%",
			"--layout=reverse",
			"--no-sort",
			"--no-info",
			"--prompt=" + question + " ",
			"--header=↑↓ choose   ⏎ confirm",
		},
	)
	if err != nil {
		return false, err
	}
	return index == 1, nil
}

// killSession ends a session and every client attached to it.
func killSession(name string) error {
	out, err := exec.Command("zmx", "kill", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("zmx kill %s: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

// run presents the picker and attaches to what it answers.
func run() error {
	start, err := os.Getwd()
	if err != nil {
		return err
	}
	name, dir, err := navigate(start)
	if err != nil {
		return err
	}
	return attach(name, dir)
}
