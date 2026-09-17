package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skipped are directory names a walk does not descend into. They hold no
// repository worth opening a session in, and between them they are most of
// what a home directory costs to read.
var skipped = map[string]bool{
	".git":         true,
	".Trash":       true,
	".cache":       true,
	".cargo":       true,
	".gradle":      true,
	".npm":         true,
	".rustup":      true,
	".venv":        true,
	"Library":      true,
	"Pictures":     true,
	"node_modules": true,
	"target":       true,
	"vendor":       true,
	"venv":         true,
}

// repositories returns every directory below root holding a `.git`, which is
// a repository and a linked worktree alike: git records a worktree with a
// `.git` file where a repository has a directory, and both are places to work.
func repositories(root string) []string {
	var found []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && (skipped[name] || strings.HasPrefix(name, ".") && name != ".claude") {
			return fs.SkipDir
		}
		if marker, err := os.Stat(filepath.Join(path, ".git")); err == nil && marker != nil {
			found = append(found, path)
			// A repository's own subdirectories are not repositories, but the
			// worktrees it keeps under .claude are, so the walk goes on.
		}
		return nil
	})
	sort.Strings(found)
	return found
}

// folders returns the directories directly below dir, in name order.
func folders(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") && e.Name() != ".claude" {
			continue
		}
		found = append(found, filepath.Join(dir, e.Name()))
	}
	return found
}

// under returns every directory below dir, for searching one level's subtree
// rather than walking it a level at a time.
func under(dir string) []string {
	var found []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path == dir {
			return nil
		}
		if skipped[d.Name()] || strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		found = append(found, path)
		return nil
	})
	return found
}
