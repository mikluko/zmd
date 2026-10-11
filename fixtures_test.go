package main

import (
	"os"
	"path/filepath"
	"testing"
)

// repoAt builds a repository: a directory with a real .git directory.
func repoAt(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// worktreeAt builds a linked worktree of repo, the way git records one: a
// .git file in the tree pointing at a record under the repo, and a gitdir
// file in that record pointing back.
func worktreeAt(t *testing.T, repo, id, tree string) string {
	t.Helper()
	record := filepath.Join(repo, ".git", "worktrees", id)
	if err := os.MkdirAll(record, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(tree, ".git"), "gitdir: "+record+"\n")
	write(t, filepath.Join(record, "gitdir"), filepath.Join(tree, ".git")+"\n")
	return tree
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
