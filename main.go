// Command zmd selects and starts zmx sessions.
//
// zmx persists terminal sessions and deliberately provides no windows, tabs
// or splits. What it also provides no opinion about is which session you
// want, which is the gap this fills: one list of places, being the directories
// a session can be started in and the directories sessions are already running
// in, narrowed by typing.
package main

import (
	"errors"
	"fmt"
	"os"
)

// version is stamped by the release build; `devel` means built from source.
var version = "devel"

const usage = `zmd selects and starts zmx sessions.

Usage:
  zmd            Pick a session or a repository, and attach
  zmd version    Print the version

The picker is a stack of rungs: the sessions running, the repositories one can
be started in, and a repository's checkouts. Right pushes, left pops, and
enter is right's alias. It opens on the sessions, with the cursor on the one
this pane last held.

In the picker:
  enter, right   descend: attach the session, list a repository's worktrees,
                 or enter a checkout and start a session where it has none.
                 A session another terminal has open is taken, after asking
  left           back a rung; backspace does the same. With a query typed
                 these correct it instead
  ctrl-g         from the sessions, the repositories
  alt-enter      start another session where the cursor stands, whatever is
                 already running there
  tab            mark, for a kill that takes several at once
  delete         kill what the marked rows stand for, after asking;
                 ctrl-delete without asking
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
		case "history":
			// What the picker's preview runs on a session row. Not in the
			// usage, being a part of the picker rather than a thing to reach
			// for: it is `zmx history` with what only a terminal should read
			// taken out.
			if len(argv) != 2 {
				return fmt.Errorf("history takes one session name")
			}
			return history(argv[1], os.Stdout)
		default:
			fmt.Fprint(os.Stderr, usage)
			return fmt.Errorf("unknown argument %q", argv[0])
		}
	}
	return run()
}
