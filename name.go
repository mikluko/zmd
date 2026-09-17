package main

import (
	"os"
	"path/filepath"
	"strings"
)

// sessionName is the name a session started in dir gets: the path, dotted,
// relative to home where it is below it and to the root where it is not.
//
// zmx names each session's unix socket after it, so a name carrying a
// separator creates nothing and reports nothing. The path is what a name has
// to be built from now, and all of it: two directories sharing a basename
// would otherwise share the one session zmx keys by that name.
//
//	~/Forge/mikluko/notes  Forge.mikluko.notes
//	~/archive/notes        archive.notes
//	/etc/nginx             etc.nginx
func sessionName(dir string) string {
	path := filepath.Clean(dir)
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
			path = rel
		}
	}
	path = strings.Trim(path, string(filepath.Separator))
	if path == "" || path == "." {
		return "home"
	}
	return sessionSafe(strings.ReplaceAll(path, string(filepath.Separator), "."))
}

// sessionSafe replaces what zmx cannot hold in a name.
func sessionSafe(name string) string {
	return strings.ReplaceAll(name, "/", ".")
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
