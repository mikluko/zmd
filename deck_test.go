package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The socket goes where a forwarded connection can reach it when told, and
// beside the rest of the state when not.
func TestSocketPathFollowsTheOverride(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	t.Setenv("ZMD_DECK_DIR", "")
	if got, want := socketPath(), "/tmp/state/zmd/deck.sock"; got != want {
		t.Errorf("socketPath = %q, want %q", got, want)
	}

	t.Setenv("ZMD_DECK_DIR", "/tmp/forwarded")
	if got, want := socketPath(), "/tmp/forwarded/deck.sock"; got != want {
		t.Errorf("with the override, socketPath = %q, want %q", got, want)
	}
}

// A picker that cannot reach the daemon does the work itself. That is the
// whole of why the daemon is an optimisation and not a dependency.
func TestWalkedFallsBackToWalkingWhenNoDaemonAnswers(t *testing.T) {
	t.Setenv("ZMD_DECK_DIR", t.TempDir())

	if client := dialDeck(); client != nil {
		t.Fatal("a client was returned with nothing listening")
	}

	root := t.TempDir()
	repo := repoAt(t, filepath.Join(root, "checkout"))
	write(t, filepath.Join(repo, ".git", "config"), "[remote \"origin\"]\n\turl = git@github.com:acme/api.git\n")

	repos, ident := walked(context.Background(), nil, root)
	if len(repos) != 1 || repos[0] != repo {
		t.Fatalf("walked %v, want just the checkout", repos)
	}
	if ident[repo] != "acme/api" {
		t.Errorf("named %q, want what the remote says", ident[repo])
	}
}

// A daemon answering is the same answer, without the walk.
func TestWalkedPrefersTheDaemon(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ZMD_DECK_DIR", dir)

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx) }()
	defer func() {
		stop()
		<-done
	}()

	waitFor(t, filepath.Join(dir, "deck.sock"))

	client := dialDeck()
	if client == nil {
		t.Fatal("no client with the daemon listening")
	}
	home, _ := os.UserHomeDir()
	repos, _ := walked(ctx, client, home)
	direct := repositories(home)
	if len(repos) != len(direct) {
		t.Errorf("the daemon answered %d checkouts, the walk finds %d", len(repos), len(direct))
	}
}

// waitFor blocks until the path exists, so a test does not race the listener.
func waitFor(t *testing.T, path string) {
	t.Helper()
	for range 200 {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
}
