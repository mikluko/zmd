package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// The picker has two views. In the filesystem view it stands in one directory,
// opening at ~, and lists the sessions standing there, the offer of another,
// and the directories below to treeDepth levels; entering a directory stands
// in it, and `..` stands in the one above. Repositories are directories with a
// mark. The sessions view is every session, whatever directory it stands in.
type mode int

const (
	modeFiles mode = iota
	modeSessions
)

// String is the mode as the record keeps it.
func (m mode) String() string {
	if m == modeSessions {
		return "sessions"
	}
	return "files"
}

// rowKind is what a row is, which is what entering it does.
type rowKind int

const (
	kindSession rowKind = iota // attach it
	kindOffer                  // start a session in the directory stood in
	kindDir                    // stand in it
	kindUp                     // stand in the directory above
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
func fileRows(root, home string, live []Session, dirs []string, ident map[string]string, seen visits) []row {
	root = filepath.Clean(root)
	var out []row
	if root != filepath.Clean(home) && root != string(filepath.Separator) {
		out = append(out, row{kind: kindUp, dir: filepath.Dir(root), status: "   ", cells: []string{"..", ""}})
	}
	var here []Session
	for _, s := range sortedSessions(live) {
		if s.Dir != "" && filepath.Clean(s.Dir) == root {
			here = append(here, s)
		}
	}
	for _, s := range here {
		out = append(out, sessionRow(s, "", ""))
	}
	out = append(out, row{kind: kindOffer, dir: root, status: "   ", cells: []string{newSession, ""}})

	var below []row
	for _, dir := range dirs {
		var found []Session
		for _, s := range live {
			if under(dir, s.Dir) {
				found = append(found, s)
			}
		}
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			rel = dir
		}
		below = append(below, row{
			kind:   kindDir,
			dir:    dir,
			found:  found,
			count:  len(found),
			status: mark(dir) + tally(len(found)) + " ",
			cells:  []string{rel, ident[dir]},
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

// sessionRows is every session, the one zmd attached latest first, a session
// it never attached ranked by when zmx started it.
//
// What an agent in a session is doing will rank ahead of both once something
// reports it; nothing does yet.
func sessionRows(live []Session, attached map[string]int64, ident map[string]string) []row {
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
		name, path := placeName(s.Dir, ident), shorten(s.Dir)
		if name == path {
			name = ""
		}
		out = append(out, sessionRow(s, name, path))
	}
	return out
}

// sessionRow is a row for s, with what git calls where it stands and that
// path where the view needs to say so.
func sessionRow(s Session, name, path string) row {
	return row{
		kind:    kindSession,
		dir:     s.Dir,
		found:   []Session{s},
		session: s.Name,
		held:    s.heldElsewhere(),
		status:  status(s.Dir, []Session{s}),
		cells:   []string{sessionLabel(s), name, path},
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

// tally writes how many sessions are alive below a row in the one cell it
// gets: nothing for none, the count itself up to nine, and `+` past that.
func tally(n int) string {
	switch {
	case n <= 0:
		return " "
	case n > 9:
		return "+"
	default:
		return strconv.Itoa(n)
	}
}
