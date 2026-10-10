// Command zmd selects and starts zmx sessions.
//
// zmx persists terminal sessions and deliberately provides no windows, tabs
// or splits. What it also provides no opinion about is which session you
// want, which is the gap this fills: one list of places, being the directories
// a session can be started in and the directories sessions are already running
// in, narrowed by typing.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// version is stamped by the release build; `devel` means built from source.
var version = "devel"

const usage = `zmd selects and starts zmx sessions.

Usage:
  zmd            Pick a session or a repository, and attach
  zmd daemon     Hold the walk warm, and learn directories from where
                 sessions cd to
  zmd version    Print the version

The picker lists places in two views: repositories below ~, and directories
outside any that sessions stand in or have been worked in. Tab switches
between them. A place opens in place into its sessions, its idle worktrees and
the offer of another session. It opens on the session this pane last held.

In the picker:
  enter          open or close a place; attach a session, taking it after
                 asking where another terminal has it open; start one on
                 <new session> or an idle worktree
  tab            switch between repos and dirs
  shift-enter    mark, for a kill that takes several at once
  alt-enter      start another session where the cursor stands, whatever is
                 already running there
  delete         kill what the marked rows stand for, after asking;
                 alt-delete without asking
  pgup/pgdown    move the cursor a screenful
  alt-up/down    scroll the preview; alt-pgup and alt-pgdown by the page
  esc            leave for the session this pane last held
`

func main() {
	if err := main_(os.Args[1:]); err != nil {
		if errors.Is(err, errCancelled) {
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "zmd: "+err.Error())
		os.Exit(1)
	}
}

func main_(argv []string) error {
	if len(argv) > 0 {
		switch argv[0] {
		case "version", "--version", "-v":
			fmt.Println(version)
			return nil
		case "help", "--help", "-h":
			fmt.Print(usage)
			return nil
		case "daemon":
			// Optional, and the picker never waits on it: what it holds is
			// what a picker would otherwise work out for itself, and a
			// picker that cannot reach it does exactly that.
			return serve(context.Background())
		default:
			fmt.Fprint(os.Stderr, usage)
			return fmt.Errorf("unknown argument %q", argv[0])
		}
	}
	return run()
}
