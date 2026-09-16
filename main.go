// Command zmx-nav selects and starts zmx sessions.
//
// zmx persists terminal sessions and deliberately provides no windows, tabs
// or splits. What it also provides no opinion about is which session you
// want, which is the gap this fills: two lists, one of the directories a
// session can be in and one of the sessions there are, with tab between them.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// version is stamped by the release build; `devel` means built from source.
var version = "devel"

const usage = `zmx-nav selects and starts zmx sessions.

Usage:
  zmx-nav            Pick a directory or a session, and attach
  zmx-nav version    Print the version

Options:
  --root DIR    Where repositories live (default ~/Forge, or $ZMX_NAV_ROOT)

In the picker:
  tab            between the directories and the sessions
  enter          attach to the session here, or start one
  alt-enter      start another session here, under a label you give
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
		}
	}

	fs := flag.NewFlagSet("zmx-nav", flag.ContinueOnError)
	root := fs.String("root", defaultRoot(), "where repositories live")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	return run(*root)
}

// defaultRoot is where repositories are looked for: $ZMX_NAV_ROOT, else
// ~/Forge.
func defaultRoot() string {
	if root := os.Getenv("ZMX_NAV_ROOT"); root != "" {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Forge")
}
