package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// A checkout's name is what git says it is, not what the path says.
//
// The path is where a checkout happens to sit and says nothing reliable about
// what it holds: `~/Forge/uptime-com` is `mikluko/workspace-uptime`, and a
// directory renamed on disk keeps the name it was cloned under. The remote is
// the answer, being the thing the checkout was cloned from.

// identify returns `owner/name` for every repository given, and nothing for
// those that have no remote to read it from.
//
// One pass, once, alongside the walk: the answer is on disk and the picker
// asks for it on every turn. Opening a repository is a file read apiece, so
// they are opened in parallel for the same reason the walk is.
func identify(repos []string) map[string]string {
	var mu sync.Mutex
	var wait sync.WaitGroup
	found := make(map[string]string, len(repos))

	for _, dir := range repos {
		wait.Add(1)
		go func() {
			defer wait.Done()
			name := identity(dir)
			if name == "" {
				return
			}
			mu.Lock()
			found[filepath.Clean(dir)] = name
			mu.Unlock()
		}()
	}
	wait.Wait()
	return found
}

// identity returns `owner/name` for the repository at dir, or "".
//
// The config file is read and parsed rather than the repository opened. go-git
// refuses a repository whose `core.repositoryformatversion` carries an
// extension it does not implement, and `worktreeconfig` is one git sets on
// exactly the repositories worktrees have been cut from — which is to say the
// ones most worth naming. Nothing here needs the object store the check is
// there to protect.
//
// `origin` is preferred and any other remote answers where there is none,
// since a checkout with one remote under another name still knows what it is.
func identity(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil {
		return ""
	}
	cfg := config.NewConfig()
	if err := cfg.Unmarshal(raw); err != nil {
		return ""
	}
	if remote, ok := cfg.Remotes[git.DefaultRemoteName]; ok {
		return ownerName(remote)
	}
	for _, name := range sortedKeys(cfg.Remotes) {
		if name := ownerName(cfg.Remotes[name]); name != "" {
			return name
		}
	}
	return ""
}

// sortedKeys orders a map's keys, so a repository with several remotes and no
// `origin` answers the same way twice.
func sortedKeys(remotes map[string]*config.RemoteConfig) []string {
	out := make([]string, 0, len(remotes))
	for name := range remotes {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ownerName reads `owner/name` out of a remote's first URL.
//
// Both forms git writes carry the same two trailing path segments, so the
// path is what is read rather than the scheme: `git@host:owner/name.git` is
// not a URL and would need its own parse.
//
//	https://github.com/uptime-com/api.git  uptime-com/api
//	git@github.com:uptime-com/api.git      uptime-com/api
func ownerName(remote *config.RemoteConfig) string {
	if len(remote.URLs) == 0 {
		return ""
	}
	path := strings.TrimSuffix(strings.TrimSuffix(remote.URLs[0], "/"), ".git")
	parts := strings.Split(strings.ReplaceAll(path, ":", "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	owner, name := parts[len(parts)-2], parts[len(parts)-1]
	if owner == "" || name == "" {
		return ""
	}
	return owner + "/" + name
}
