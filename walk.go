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

// under reports whether path is base or stands below it.
func under(base, path string) bool {
	if base == "" || path == "" {
		return false
	}
	base, path = filepath.Clean(base), filepath.Clean(path)
	return path == base || strings.HasPrefix(path, base+string(filepath.Separator))
}

// repoRoot returns the repository dir belongs to, or "" where it belongs to
// none.
//
// A linked worktree and the repository it was cut from answer with the same
// path, which is what lets a scope standing in one reach the other: git
// records the repository in the `.git` file it writes in the worktree, where
// a repository has a `.git` directory instead.
func repoRoot(dir string) string {
	at := filepath.Clean(dir)
	for {
		marker := filepath.Join(at, ".git")
		if info, err := os.Stat(marker); err == nil {
			if info.IsDir() {
				return at
			}
			return gitdirOwner(marker)
		}
		parent := filepath.Dir(at)
		if parent == at {
			return ""
		}
		at = parent
	}
}

// gitdirOwner reads the repository out of a linked worktree's `.git` file,
// which git writes as `gitdir: <repo>/.git/worktrees/<id>`.
//
// A form that does not carry a worktree record belongs to no repository this
// can name: a submodule's `gitdir:` points into `.git/modules` instead.
func gitdirOwner(marker string) string {
	body, err := os.ReadFile(marker)
	if err != nil {
		return ""
	}
	path, ok := strings.CutPrefix(strings.TrimSpace(string(body)), "gitdir:")
	if !ok {
		return ""
	}
	sep := string(filepath.Separator)
	record := sep + ".git" + sep + "worktrees" + sep
	i := strings.Index(strings.TrimSpace(path), record)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(path)[:i]
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

// bundles are the extensions of directories macOS presents as single files:
// applications, libraries and packages, which hold no work to stand in.
var bundles = map[string]bool{
	".app":           true,
	".appex":         true,
	".bundle":        true,
	".framework":     true,
	".kext":          true,
	".localized":     true,
	".lproj":         true,
	".photoslibrary": true,
	".plugin":        true,
	".xcassets":      true,
}

// treeDepth is how many levels below the directory the picker stands in it
// lists.
const treeDepth = 4

// tree is every directory below root, to depth levels, past the walk's skips
// and leaving out hidden ones: they are most of a home directory and almost
// none of the work in it. A directory in keep is listed wherever it is within
// depth, hidden or not, since keep is where sessions stand and work has been.
func tree(root string, depth int, keep []string) []string {
	root = filepath.Clean(root)
	levels := func(path string) int {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return 0
		}
		return strings.Count(rel, string(filepath.Separator)) + 1
	}

	var mu sync.Mutex
	seen := map[string]bool{}
	fastwalk.Walk(&fastwalk.Config{}, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == root {
			return nil
		}
		name := d.Name()
		if skipped[name] || bundles[filepath.Ext(name)] || strings.HasPrefix(name, ".") || levels(path) > depth {
			return fs.SkipDir
		}
		mu.Lock()
		seen[filepath.Clean(path)] = true
		mu.Unlock()
		if levels(path) == depth {
			return fs.SkipDir
		}
		return nil
	})
	for _, dir := range keep {
		dir = filepath.Clean(dir)
		if n := levels(dir); n > 0 && n <= depth && isDir(dir) {
			seen[dir] = true
		}
	}
	out := make([]string, 0, len(seen))
	for dir := range seen {
		out = append(out, dir)
	}
	sort.Strings(out)
	return out
}
