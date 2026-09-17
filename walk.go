package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/charlievieth/fastwalk"
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
//
// The walk is parallel because it is what the picker waits on: a home
// directory is some fifteen thousand directories, and one `stat` per
// directory issued serially is most of a second before anything is drawn.
// Callbacks run on several goroutines, so what they touch is guarded and the
// result is sorted rather than arriving in order.
func repositories(root string) []string {
	var mu sync.Mutex
	var found []string

	fastwalk.Walk(&fastwalk.Config{}, root, func(path string, d fs.DirEntry, err error) error {
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
			mu.Lock()
			found = append(found, path)
			mu.Unlock()
			// A repository's own subdirectories are not repositories, but the
			// worktrees it keeps under .claude are, so the walk goes on.
		}
		return nil
	})
	sort.Strings(found)
	return found
}
