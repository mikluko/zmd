package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// The picker lists places, in one of two views: the repositories below ~, or
// the directories outside any repository that sessions stand in or have been
// worked in. A place expands in place into what it holds: its sessions, the
// worktrees with none, and the offer of another.
//
// The two views do not overlap. A session counts against the repository it
// stands in, its worktrees included, and only a session in no repository the
// walk found stands in a directory of its own.
type mode int

const (
	modeRepos mode = iota
	modeDirs
)

// String is the mode as the prompt says it and the record keeps it.
func (m mode) String() string {
	if m == modeDirs {
		return "dirs"
	}
	return "repos"
}

// place is a row of the list and the rows it expands into.
type place struct {
	row  row
	kids []row
}

// newSession is the row that stands for starting one rather than joining one.
const newSession = "<new session>"

// owners maps each session to the repository root it counts against, among
// those walked, or to "" where it stands in none of them.
func owners(live []Session, roots map[string]bool) map[string]string {
	out := make(map[string]string, len(live))
	for _, s := range live {
		if s.Dir == "" {
			continue
		}
		if root := repoRoot(s.Dir); roots[root] {
			out[s.Name] = root
		} else {
			out[s.Name] = ""
		}
	}
	return out
}

// repoPlaces is one place per repository that is not a worktree.
//
// Expanded, a repository holds every session standing in it or in any
// worktree cut from it, then each of those worktrees no session stands in,
// since entering one is how a session is started there, then the offer of a
// session at the repository itself.
func repoPlaces(live []Session, repos []string, ident map[string]string, seen visits, now time.Time) []place {
	roots := map[string]bool{}
	for _, dir := range repos {
		key := filepath.Clean(dir)
		if repoRoot(key) == key {
			roots[key] = true
		}
	}
	owner := owners(live, roots)
	trees := worktreesOf(repos)

	sorted := sortedSessions(live)
	var out []place
	for root := range roots {
		var found []Session
		busy := map[string]bool{}
		for _, s := range sorted {
			if owner[s.Name] == root {
				found = append(found, s)
				busy[filepath.Clean(s.Dir)] = true
			}
		}
		kids := sessionRows(found, root, ident)
		for _, tree := range trees[root] {
			if !busy[tree] {
				kids = append(kids, row{
					dir:    tree,
					status: status(tree, nil),
					cells:  []string{placeName(tree, ident), ""},
				})
			}
		}
		kids = append(kids, offerRow(root))
		out = append(out, place{
			row: row{
				dir:    root,
				found:  found,
				count:  len(found),
				trees:  len(trees[root]),
				status: mark(root) + tally(len(found)) + " ",
				cells:  []string{placeName(root, ident), shorten(filepath.Dir(root))},
			},
			kids: kids,
		})
	}
	return ordered(out, seen, now)
}

// dirPlaces is one place per directory outside the walked repositories that a
// session stands in, that has been worked in, or that the pane stands in.
//
// Expanded, a directory holds the sessions standing exactly there, then the
// offer of another. A directory that has gone from disk since it was worked
// in is not offered.
func dirPlaces(live []Session, repos []string, start string, seen visits, now time.Time) []place {
	roots := map[string]bool{}
	for _, dir := range repos {
		key := filepath.Clean(dir)
		if repoRoot(key) == key {
			roots[key] = true
		}
	}
	owner := owners(live, roots)

	here := map[string][]Session{}
	for _, s := range sortedSessions(live) {
		if s.Dir != "" && owner[s.Name] == "" {
			key := filepath.Clean(s.Dir)
			here[key] = append(here[key], s)
		}
	}
	dirs := map[string]bool{}
	for dir := range here {
		dirs[dir] = true
	}
	for dir := range seen {
		if !roots[dir] && repoRoot(dir) == "" && isDir(dir) {
			dirs[dir] = true
		}
	}
	if start != "" && repoRoot(start) == "" {
		dirs[filepath.Clean(start)] = true
	}

	var out []place
	for dir := range dirs {
		found := here[dir]
		out = append(out, place{
			row: row{
				dir:    dir,
				found:  found,
				count:  len(found),
				status: " " + tally(len(found)) + " ",
				cells:  []string{shorten(dir)},
			},
			kids: append(sessionRows(found, dir, nil), offerRow(dir)),
		})
	}
	return ordered(out, seen, now)
}

// sessionRows is a row per session, saying where it stands where that is not
// the place itself: a worktree by its name, a directory below by its path.
func sessionRows(found []Session, at string, ident map[string]string) []row {
	out := make([]row, 0, len(found))
	for _, s := range found {
		where := ""
		if dir := filepath.Clean(s.Dir); dir != at {
			where = placeName(dir, ident)
		}
		out = append(out, row{
			dir:     s.Dir,
			found:   []Session{s},
			session: s.Name,
			held:    s.heldElsewhere(),
			status:  status(s.Dir, []Session{s}),
			cells:   []string{sessionLabel(s), where},
		})
	}
	return out
}

// offerRow is the offer of a new session in dir.
func offerRow(dir string) row {
	return row{dir: dir, status: "   ", cells: []string{newSession, ""}}
}

// ordered puts the places holding sessions first, then the rest, each by how
// much and how lately they have been worked in, then by name.
func ordered(places []place, seen visits, now time.Time) []place {
	score := make(map[string]float64, len(places))
	for _, p := range places {
		score[p.row.dir] = seen.score(p.row.dir, now)
	}
	sort.SliceStable(places, func(i, j int) bool {
		a, b := places[i].row, places[j].row
		if (a.count > 0) != (b.count > 0) {
			return a.count > 0
		}
		if score[a.dir] != score[b.dir] {
			return score[a.dir] > score[b.dir]
		}
		return a.cells[0] < b.cells[0]
	})
	return places
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

// worktreesOf groups the walked checkouts by the repository they were cut
// from, in the order the walk found them.
func worktreesOf(repos []string) map[string][]string {
	out := make(map[string][]string)
	for _, dir := range repos {
		key := filepath.Clean(dir)
		root := repoRoot(key)
		if root == "" || root == key {
			continue
		}
		out[root] = append(out[root], key)
	}
	return out
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
