package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	fzf "github.com/junegunn/fzf/src"
)

// errCancelled reports a picker the user dismissed. It is not a failure.
var errCancelled = errors.New("cancelled")

// keys the picker reports rather than acting on itself.
const (
	keyEnter    = ""
	keyMode     = "tab"
	keyNew      = "alt-enter"
	keyModeBack = "shift-tab"
)

// runFzf presents lines and returns what fzf wrote and the code it ended on.
//
// fzf runs in this process rather than as a command: a mode is then a loop
// here rather than a reload fzf runs against this binary, and each mode is
// free to show different columns, which one picker cannot.
func runFzf(lines []string, args []string) (int, []string, error) {
	opts, err := fzf.ParseOptions(true, args)
	if err != nil {
		return 0, nil, err
	}

	in := make(chan string, len(lines)+1)
	for _, line := range lines {
		in <- line
	}
	close(in)
	opts.Input = in

	// Buffered past anything fzf writes, since nothing drains it until Run
	// returns: --expect adds a line of its own before the selection.
	out := make(chan string, len(lines)+8)
	opts.Output = out

	code, err := fzf.Run(opts)
	close(out)
	if err != nil {
		return code, nil, err
	}

	var written []string
	for line := range out {
		written = append(written, line)
	}
	return code, written, nil
}

// present runs one picker and returns the key pressed, empty for enter, and
// the index of the line it was pressed on.
//
// Every line leads with its own index, so a choice comes back as a row the
// caller already holds rather than as text it has to read back.
func present(lines []string, args []string) (key string, index int, err error) {
	code, written, err := runFzf(lines, args)
	if err != nil {
		return "", 0, err
	}
	switch code {
	case fzf.ExitOk:
	case fzf.ExitInterrupt, fzf.ExitNoMatch:
		return "", 0, errCancelled
	default:
		return "", 0, fmt.Errorf("picker exited %d", code)
	}
	if len(written) < 2 {
		return "", 0, errCancelled
	}

	key = written[0]
	field, _, _ := strings.Cut(written[len(written)-1], "\t")
	index, err = strconv.Atoi(field)
	if err != nil {
		return "", 0, fmt.Errorf("unreadable selection %q", written[len(written)-1])
	}
	return key, index, nil
}

// promptText reads a line typed into an empty picker.
//
// Enter on a query matching nothing is how fzf reports typed text, so the
// no-match exit is the answer here rather than a failure. Escape is not, and
// neither is an empty line.
func promptText(prompt, header string) (string, error) {
	_, written, err := runFzf(nil, []string{
		"--height=100%",
		"--layout=reverse",
		"--no-info",
		"--print-query",
		"--prompt=" + prompt,
		"--header=" + header,
	})
	if err != nil {
		return "", err
	}
	if len(written) == 0 {
		return "", errCancelled
	}
	text := strings.TrimSpace(written[0])
	if text == "" {
		return "", errCancelled
	}
	return text, nil
}

// attach replaces this process with a zmx client attached to name.
//
// dir becomes the session's working directory, but only where the session is
// being created: zmx records it once, when the daemon starts.
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
