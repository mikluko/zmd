package main

import (
	"os"
	"path/filepath"
	"sort"
)

// The picker has three views, which tab cycles through. The checkouts view is
// every repository and worktree below ~, and entering one starts a session
// there. The sessions view is every session, wherever it stands. The browse
// view stands in one directory, opening at ~, and lists the sessions standing
// there, the offer of another, and the directories below to treeDepth levels;
// entering a directory stands in it, and `..` stands in the one above.
type mode int

const (
	modeCheckouts mode = iota
	modeSessions
	modeBrowse
	modes
)

// String is the mode as the prompt says it and the record keeps it.
func (m mode) String() string {
	switch m {
	case modeSessions:
		return "sessions"
	case modeBrowse:
		return "browse"
	default:
		return "checkouts"
	}
}

// rowKind is what a row is, which is what entering it does.
type rowKind int

const (
	kindSession  rowKind = iota // attach it
	kindOffer                   // start a session in the directory stood in
	kindDir                     // stand in it
	kindUp                      // stand in the directory above
	kindCheckout                // start a session in it
)

// newSession is the row that stands for starting one rather than joining one.
const newSession = "<new session>"

// fileRows is what the filesystem view shows standing in root: the way up
// where there is one, the sessions standing exactly in root, the offer of
// another, then the directories below.
//
// Directories worked in lead, the latest first, since that is where the next
// session most likely goes; the rest follow by path. Each counts the sessions
// standing in it or below it, so activity deep in a tree shows from its top.
func fileRows(root, home string, live []Session, dirs []string, seen visits) []row {
	root = filepath.Clean(root)
	var out []row
	if root != filepath.Clean(home) && root != string(filepath.Separator) {
		out = append(out, row{kind: kindUp, dir: filepath.Dir(root), status: "   ", cells: []string{".."}})
	}
	var here []Session
	for _, s := range sortedSessions(live) {
		if s.Dir != "" && filepath.Clean(s.Dir) == root {
			here = append(here, s)
		}
	}
	for _, s := range here {
		out = append(out, sessionRow(s, ""))
	}
	out = append(out, row{kind: kindOffer, dir: root, status: "   ", cells: []string{newSession}})

	var below []row
	for _, dir := range dirs {
		var found []Session
		for _, s := range live {
			if under(dir, s.Dir) {
				found = append(found, s)
			}
		}
		below = append(below, row{
			kind:   kindDir,
			dir:    dir,
			found:  found,
			count:  len(found),
			status: status(dir, found),
			cells:  []string{shorten(dir)},
		})
	}
	sort.SliceStable(below, func(i, j int) bool {
		a, b := seen[below[i].dir].Last, seen[below[j].dir].Last
		if a != b {
			return a > b
		}
		return below[i].cells[0] < below[j].cells[0]
	})
	return append(out, below...)
}

// checkoutRows is every repository and worktree the walk found, each counting
// the sessions standing in it or below it that no checkout deeper in owns, so
// a worktree kept inside its repository counts its own.
//
// Checkouts worked in lead, the latest first, since that is where the next
// session most likely goes; the rest follow by path.
func checkoutRows(live []Session, repos []string, seen visits) []row {
	latest := map[string]int64{}
	for dir, v := range seen {
		if c := checkoutOf(dir, repos); c != "" && v.Last > latest[c] {
			latest[c] = v.Last
		}
	}

	out := make([]row, 0, len(repos))
	for _, dir := range repos {
		dir = filepath.Clean(dir)
		found := owned(dir, live, repos)
		out = append(out, row{
			kind:   kindCheckout,
			dir:    dir,
			found:  found,
			count:  len(found),
			status: status(dir, found),
			cells:  []string{shorten(dir)},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := latest[out[i].dir], latest[out[j].dir]
		if a != b {
			return a > b
		}
		return out[i].cells[0] < out[j].cells[0]
	})
	return out
}

// checkoutOf is the checkout dir stands in: the deepest of repos it is in or
// below, or "" where it is in none.
func checkoutOf(dir string, repos []string) string {
	best := ""
	for _, c := range repos {
		if under(c, dir) && len(c) > len(best) {
			best = filepath.Clean(c)
		}
	}
	return best
}

// owned is the sessions standing in checkout and in no checkout deeper in it.
func owned(checkout string, live []Session, repos []string) []Session {
	var out []Session
	for _, s := range live {
		if s.Dir != "" && checkoutOf(s.Dir, repos) == filepath.Clean(checkout) {
			out = append(out, s)
		}
	}
	return out
}

// sessionRows is every session, the one zmd attached latest first, a session
// it never attached ranked by when zmx started it.
//
// What an agent in a session is doing will rank ahead of both once something
// reports it; nothing does yet.
func sessionRows(live []Session, attached map[string]int64) []row {
	sorted := append([]Session(nil), live...)
	when := func(s Session) int64 {
		if t, ok := attached[s.Name]; ok {
			return t
		}
		return s.Created
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		if when(sorted[i]) != when(sorted[j]) {
			return when(sorted[i]) > when(sorted[j])
		}
		return sorted[i].Name < sorted[j].Name
	})
	out := make([]row, 0, len(sorted))
	for _, s := range sorted {
		out = append(out, sessionRow(s, shorten(s.Dir)))
	}
	return out
}

// sessionRow is a row for s, saying where it stands where the view needs to.
func sessionRow(s Session, path string) row {
	return row{
		kind:    kindSession,
		dir:     s.Dir,
		found:   []Session{s},
		session: s.Name,
		held:    s.heldElsewhere(),
		status:  status(s.Dir, []Session{s}),
		cells:   []string{sessionLabel(s), path},
	}
}

// keepDirs is the directories the tree lists however it would otherwise
// leave them out: where sessions stand, and where work has been.
func keepDirs(live []Session, seen visits) []string {
	var out []string
	for _, s := range live {
		if s.Dir != "" {
			out = append(out, s.Dir)
		}
	}
	for dir := range seen {
		out = append(out, dir)
	}
	return out
}

// sortedSessions is live by directory, then by name.
func sortedSessions(live []Session) []Session {
	out := append([]Session(nil), live...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir < out[j].Dir
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
