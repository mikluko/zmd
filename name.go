package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A session's name is an address and says nothing about the work. What a
// session is for is metadata kept against this name, which is why the name
// only has to be unique and to say where it came from.
//
// zmx names each session's unix socket after it, flat in one directory, so a
// name carrying a separator creates nothing and reports nothing. These carry
// none.
//
//	zmd.ghostty.6f2a91c4
//	└┬┘ └──┬──┘ └───┬──┘
//	 │     │        └─ four bytes of randomness, hex
//	 │     └────────── the terminal it was made in, from TERM_PROGRAM
//	 └──────────────── what made it, so a session zmd started is told from
//	                   one it merely found
const namespace = "zmd"

// sessionName returns a name no running session has.
//
// Four bytes is four billion names against the tens a machine holds, so the
// retry below almost never runs; it is there because almost never is not
// never.
func sessionName(live []Session) string {
	taken := make(map[string]bool, len(live))
	for _, s := range live {
		taken[s.Name] = true
	}
	prefix := namespace + "." + terminal() + "."
	for range 100 {
		if name := prefix + shortID(); !taken[name] {
			return name
		}
	}
	// Every name the shape can hold is running, which it cannot be; falling
	// back to a counter keeps this total rather than looping forever.
	for n := 0; ; n++ {
		if name := prefix + strconv.Itoa(n); !taken[name] {
			return name
		}
	}
}

// terminal names the terminal the session is being made in, which is what
// TERM_PROGRAM is for. Anything it cannot hold in a name is dropped, and an
// empty answer stands for a terminal that did not say.
func terminal() string {
	var b strings.Builder
	for _, r := range strings.ToLower(os.Getenv("TERM_PROGRAM")) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "term"
	}
	return b.String()
}

// shortID is four bytes of randomness, written as hex.
//
// crypto/rand rather than math/rand so nothing has to seed it: a picker run
// twice in the same second would otherwise name both sessions alike.
func shortID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read does not fail on any platform this runs on, and a name
		// that repeats is caught by the caller either way.
		return "00000000"
	}
	return hex.EncodeToString(b[:])
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
