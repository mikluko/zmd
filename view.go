package main

import (
	"path/filepath"
	"sort"
	"strconv"
)

// The picker is a stack of rungs, and the keys that move between them are the
// arrows: right pushes, left pops back to wherever it was pushed from.
//
// Sessions are the ground floor and where the picker opens, being what a
// switch is between and what a scan is over: an agent wanting attention and a
// shell left running are both facts about sessions, and looking for either in
// a list of repositories means reading a list twenty times longer and then
// descending to learn which session it was.
//
// The repositories above are how a session is started somewhere new, which is
// the rarer thing and the only thing they are for. Reaching a checkout that
// already holds sessions pushes straight back down to them, filtered.
type view int

const (
	viewSessions  view = iota // every session running, or those in one checkout
	viewRepos                 // one row per checkout that is not a worktree
	viewCheckouts             // one repository's checkouts: itself, then its worktrees
)

// rung is one thing the picker is looking at.
type rung struct {
	view view
	dir  string // the checkout a sessions rung is filtered to, or a repository's root
	name string // what the prompt says
}

// prompt is what the rung calls itself.
func (r rung) prompt() string {
	if r.name != "" {
		return r.name
	}
	switch r.view {
	case viewRepos:
		return "repos"
	default:
		return "sessions"
	}
}

// rows renders whatever the rung is looking at.
func rows(r rung, live []Session, repos []string, ident map[string]string) []row {
	switch r.view {
	case viewRepos:
		return repoRows(live, repos, ident)
	case viewCheckouts:
		return checkoutRows(r.dir, live, repos, ident)
	default:
		return liveRows(r.dir, live, ident)
	}
}

// newSession is the row that stands for starting one rather than joining one.
const newSession = "<new session>"

// liveRows is one row per running session, or per session standing in dir.
//
// The place is the last cell and the session's own handle the first, since
// what tells two sessions apart is what they are for rather than where they
// are: a place holding several holds them at one path.
//
// A rung narrowed to one place ends with the offer of another session there.
// Without it a pane standing somewhere new has no way to say so: every row
// belongs to a session that is already running, and starting one beside the
// cursor starts it wherever the cursor happens to be.
func liveRows(dir string, live []Session, ident map[string]string) []row {
	sorted := append([]Session(nil), live...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Dir != sorted[j].Dir {
			return sorted[i].Dir < sorted[j].Dir
		}
		return sorted[i].Name < sorted[j].Name
	})

	out := make([]row, 0, len(sorted))
	for i := range sorted {
		s := sorted[i]
		if dir != "" && filepath.Clean(s.Dir) != filepath.Clean(dir) {
			continue
		}
		out = append(out, row{
			dir:     s.Dir,
			found:   []Session{s},
			session: s.Name,
			held:    s.heldElsewhere(),
			status:  status(s.Dir, []Session{s}),
			cells:   []string{sessionLabel(s), placeName(s.Dir, ident)},
		})
	}
	if dir == "" {
		return out
	}
	return append(out, row{
		dir:    filepath.Clean(dir),
		status: status(dir, nil),
		cells:  []string{newSession, placeName(dir, ident)},
	})
}

// repoRows is one row per checkout that is not a worktree.
//
// The count is what the cell says rather than how many are attached: at a
// level where no single session can be acted on, how much is alive below is
// the whole of what the mark is read for. A repository cloned twice stands
// twice, the two rows differing only in where they are.
func repoRows(live []Session, repos []string, ident map[string]string) []row {
	here := byDir(live)
	trees := worktreesOf(repos)

	var out []row
	for _, dir := range repos {
		key := filepath.Clean(dir)
		if root := repoRoot(key); root != "" && root != key {
			continue
		}
		under := trees[key]
		alive := len(here[key])
		for _, tree := range under {
			alive += len(here[tree])
		}
		out = append(out, row{
			dir:     key,
			count:   alive,
			trees:   len(under),
			status:  mark(key) + tally(alive) + " ",
			cells:   []string{placeName(key, ident), shorten(filepath.Dir(key))},
			several: alive > 1,
		})
	}
	// What is alive leads, since the list is 114 rows and the reason to read
	// it rather than type into it is to find what is running.
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].count > 0) != (out[j].count > 0) {
			return out[i].count > 0
		}
		return out[i].cells[0] < out[j].cells[0]
	})
	return out
}

// checkoutRows is a repository and the worktrees cut from it, itself first.
//
// The repository leads because it is where the work usually is: its worktrees
// are the exceptions it spun off, and a list that buried it under them would
// put the common answer last.
func checkoutRows(root string, live []Session, repos []string, ident map[string]string) []row {
	here := byDir(live)
	dirs := append([]string{filepath.Clean(root)}, worktreesOf(repos)[filepath.Clean(root)]...)

	out := make([]row, 0, len(dirs))
	for _, dir := range dirs {
		found := here[dir]
		out = append(out, row{
			dir:     dir,
			found:   found,
			count:   len(found),
			several: len(found) > 1,
			status:  status(dir, found),
			cells:   []string{placeName(dir, ident), shorten(filepath.Dir(dir))},
		})
	}
	return out
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
