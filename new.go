package main

// Target is a place a session can be started, and the name it gets there.
type Target struct {
	Name string
	Dir  string
}

// targets returns every repository below root and every worktree git records
// for one, in repository order.
func targets(root string) []Target {
	var found []Target
	for _, repo := range forgeRepos(root) {
		name := sessionSafe(repoLabel(repo, root))
		found = append(found, Target{Name: name, Dir: repo})
		for _, w := range worktreesOf(repo) {
			found = append(found, Target{Name: sessionSafe(name + "@" + w.Label), Dir: w.Dir})
		}
	}
	return found
}
