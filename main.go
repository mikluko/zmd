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
  zmd            Pick a checkout to start a session in, or a session, and attach
  zmd daemon     Hold the walk of checkouts warm, and learn which directories
                 are worked in from where sessions cd to
  zmd version    Print the version

The picker has three views, which tab cycles through: the checkouts below ~,
where enter starts a session; every session, where enter attaches; and
browse, which stands in a directory and lists what is below it.

In the picker:
  enter          start a session in a checkout with none, or list the
                 sessions of one that has them; attach a session, taking it
                 after asking where another terminal has it open; in browse,
                 stand in a directory, or the one above on ..
  tab            cycle through checkouts, sessions and browse
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
