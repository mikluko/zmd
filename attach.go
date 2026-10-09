package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// errCancelled reports a picker the user dismissed. It is not a failure.
var errCancelled = errors.New("cancelled")

// attach replaces this process with a zmx client attached to name.
//
// dir becomes the session's working directory only where the session is being
// created, which is when zmx chdirs to spawn the shell. A session that already
// exists reports its own: zmx reads the directory from the OSC 7 its shell
// emits, so the session follows a `cd` and an attach cannot place it.
func attach(name, dir string) error {
	path, err := exec.LookPath("zmx")
	if err != nil {
		return fmt.Errorf("zmx not found in PATH")
	}
	if dir != "" {
		if err := os.Chdir(dir); err != nil {
			return fmt.Errorf("chdir %s: %w", dir, err)
		}
	}
	// ZMX_SESSION names the session this process is already inside, and zmx
	// reads an attach made with it set as that session switching to the name
	// given. Inside a session that is what should happen; the variable is
	// left alone so it does.
	return syscall.Exec(path, []string{"zmx", "attach", name}, os.Environ())
}
