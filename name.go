package main

import (
	"crypto/rand"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A session's name is a handle and says nothing: what a session is for is
// metadata, and metadata is what the list reads it by. Two sessions in one
// directory are told apart by their labels or, failing that, by these names,
// which is the only place one is ever shown.
//
// zmx names each session's unix socket after it, flat in one directory, so a
// name carrying a separator creates nothing and reports nothing. These carry
// none.
var words = []string{
	"amber", "anchor", "arbor", "ash", "aspen", "basin", "beacon", "birch",
	"bramble", "brass", "briar", "cairn", "cedar", "cinder", "clover", "cobalt",
	"copper", "cove", "crest", "dune", "ember", "fathom", "fen", "fjord",
	"flint", "garnet", "gable", "harbor", "hazel", "heath", "hollow", "indigo",
	"ivory", "juniper", "kelp", "lantern", "larch", "ledge", "linen", "lumen",
	"maple", "marsh", "mesa", "moor", "onyx", "opal", "orchard", "pebble",
	"pine", "quarry", "quill", "reef", "ridge", "rill", "rowan", "sable",
	"sage", "shale", "slate", "sorrel", "spruce", "thicket", "tundra", "vale",
	"willow", "zephyr",
}

// sessionName returns a name no running session has.
//
// A word and two digits is short enough to type and to say out loud, which is
// what a handle is for. Sixty-six words against a hundred numbers is thousands
// of names for the tens a machine holds, so the retry below almost never runs;
// it is there because almost never is not never.
func sessionName(live []Session) string {
	taken := make(map[string]bool, len(live))
	for _, s := range live {
		taken[s.Name] = true
	}
	for range 100 {
		name := words[roll(len(words))] + "-" + pad(roll(100))
		if !taken[name] {
			return name
		}
	}
	// Every name the shape can hold is running, which it cannot be; falling
	// back to a counter keeps this total rather than looping forever.
	for n := 0; ; n++ {
		name := "session-" + strconv.Itoa(n)
		if !taken[name] {
			return name
		}
	}
}

// roll returns a number below n.
//
// crypto/rand rather than math/rand so nothing has to seed it: a picker run
// twice in the same second would otherwise name both sessions alike.
func roll(n int) int {
	i, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(i.Int64())
}

// pad writes n as two digits.
func pad(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// placeName is what a directory goes by in the list.
//
// A checkout goes by what git says it is, `owner/name` off its remote, since
// the path is where it happens to sit and says nothing reliable: a directory
// called `uptime-com` can hold `acme/workspace`, and one renamed on disk
// keeps the name it was cloned under.
//
// A worktree goes by its repository and what is left of its own basename once
// the repository's name is taken off the front, that prefix being how the
// tools that cut worktrees name them.
//
// Anything git knows nothing about goes by its directory under its parent,
// which is all there is to go on.
//
//	a checkout       acme/api
//	its worktree     acme/api/rebase-517
//	neither          Documents/notes
func placeName(dir string, ident map[string]string) string {
	path := filepath.Clean(dir)
	if path == string(filepath.Separator) {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil && path == home {
		return "~"
	}

	if name, ok := ident[path]; ok {
		return name
	}

	container := filepath.Dir(path)
	if root := repoRoot(path); root != "" && root != path {
		container = root
		if name, ok := ident[root]; ok {
			return name + "/" + trimRepo(filepath.Base(path), name)
		}
	}
	if container == string(filepath.Separator) {
		return filepath.Base(path)
	}
	return filepath.Base(container) + "/" + filepath.Base(path)
}

// trimRepo takes the repository's own name off the front of a worktree's
// basename, leaving what distinguishes it. A basename that does not carry the
// prefix is left whole.
func trimRepo(base, ident string) string {
	repo := ident
	if _, name, ok := strings.Cut(ident, "/"); ok {
		repo = name
	}
	if rest, ok := strings.CutPrefix(base, repo+"-"); ok && rest != "" {
		return rest
	}
	return base
}

// shorten writes a path as a reader says it.
func shorten(path string) string {
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + rel
	}
	return path
}
