// Command zmx-deck selects and starts zmx sessions.
//
// zmx persists terminal sessions and deliberately provides no windows, tabs
// or splits. What it also provides no opinion about is which session you
// want, which is the gap this fills: one list holding the sessions there are
// and the repositories a session can be started in, narrowed by typing.
package main

import (
	"errors"
	"fmt"
	"os"
)

// version is stamped by the release build; `devel` means built from source.
var version = "devel"

const usage = `zmx-deck selects and starts zmx sessions.

Usage:
  zmx-deck            Pick a session or a repository, and attach
  zmx-deck version    Print the version

The picker holds one list: the sessions there are, then the repositories a
session can be started in. Typing narrows both at once.

In the picker:
  enter          attach the session, or start one in the repository; where
                 what was typed matches nothing, start a session under that
                 name here
  alt-enter      start another session where the cursor stands, under a label
                 you type
  ctrl-n         start a session named after what you typed, matched or not
  tab            mark, for a kill that takes several at once
  delete         kill, after asking; ctrl-delete without asking
`

func main() {
	if err := main_(os.Args[1:]); err != nil {
		if errors.Is(err, errCancelled) {
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "zmx-deck: "+err.Error())
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
		default:
			fmt.Fprint(os.Stderr, usage)
			return fmt.Errorf("unknown argument %q", argv[0])
		}
	}
	return run()
}
