// Command zmx-nav selects and starts zmx sessions.
//
// zmx persists terminal sessions and deliberately provides no windows, tabs
// or splits. What it also provides no opinion about is which session you
// want, which is the gap this fills: three lists — the filesystem, the
// repositories in it, and the sessions there are — with tab between them.
package main

import (
	"errors"
	"fmt"
	"os"
)

// version is stamped by the release build; `devel` means built from source.
var version = "devel"

const usage = `zmx-nav selects and starts zmx sessions.

Usage:
  zmx-nav            Pick a folder, a repository or a session, and attach
  zmx-nav version    Print the version

In the picker:
  tab            between folders, repositories and sessions
  enter          in folders, go into the directory; elsewhere, attach
  alt-enter      start a session for the directory, under a label where one
                 is already running there
  left           in folders, the parent directory
  ctrl-f         in folders, everything below here rather than its children
  delete         kill the session, after asking; ctrl-delete without asking

A pane opens on the list it was last left on in the directory it stands in.
`

func main() {
	if err := main_(os.Args[1:]); err != nil {
		if errors.Is(err, errCancelled) {
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "zmx-nav: "+err.Error())
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
