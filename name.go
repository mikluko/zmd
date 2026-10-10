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
