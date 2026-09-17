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

// errRetry reports a question answered no. The picker goes round again rather
// than ending, the answer being about the keystroke and not about the picker.
var errRetry = errors.New("retry")

// keys the picker reports rather than acting on itself.
//
// Delete kills and backspace does not, because backspace is how a query is
// corrected: a filter typed to find the session to kill would otherwise be
// one keystroke from killing whatever the cursor had been left on.
const (
	keyEnter    = ""
	keyNew      = "alt-enter"
	keyRepos    = "ctrl-g"
	keyKill     = "delete"
	keyKillHard = "ctrl-delete"
)

// popRung is what left writes ahead of the rows, to say the stack moved
// rather than a row being chosen.
const popRung = "pop-rung"

// choice is what one turn of the picker answered.
type choice struct {
	query string // what was typed, whether or not it matched anything
	key   string // the key pressed, empty for enter
	pop   bool   // left was pressed with nothing typed: go back a rung
	rows  []int  // the marked rows, or the one under the cursor
}

// runFzf presents lines and returns what fzf wrote and the code it ended on.
//
// fzf runs in this process rather than as a command: acting on a key is then
// a turn of a loop here rather than a reload fzf runs against this binary.
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
	// returns: the query and the expected key both precede the selection.
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

// present runs the picker and returns what it answered.
//
// Every line leads with its own index, so a choice comes back as rows the
// caller already holds rather than as text it has to read back. The arguments
// must carry --print-query and --expect, which is what puts the query and the
// key ahead of the rows.
func present(lines []string, args []string) (choice, error) {
	code, written, err := runFzf(lines, args)
	if err != nil {
		return choice{}, err
	}
	switch code {
	case fzf.ExitOk:
	case fzf.ExitNoMatch:
		// A query matching nothing is an answer rather than a failure: it
		// names a session that does not exist yet. fzf writes the query and
		// the key either way, and only the rows are missing.
	case fzf.ExitInterrupt:
		return choice{}, errCancelled
	default:
		return choice{}, fmt.Errorf("picker exited %d", code)
	}
	return parseChoice(written)
}

// parseChoice reads fzf's output: the query, then the key, then whatever a
// `print` action queued, then a line per selected row.
//
// The marker is told from a row by having no index to lead with, which is
// what every row carries.
func parseChoice(written []string) (choice, error) {
	if len(written) < 2 {
		return choice{}, errCancelled
	}
	answer := choice{query: written[0], key: written[1]}
	for _, line := range written[2:] {
		if line == popRung {
			answer.pop = true
			continue
		}
		field, _, _ := strings.Cut(line, "\t")
		index, err := strconv.Atoi(field)
		if err != nil {
			return choice{}, fmt.Errorf("unreadable selection %q", line)
		}
		answer.rows = append(answer.rows, index)
	}
	return answer, nil
}

// pick runs a picker that answers with one row and nothing else, and returns
// the index that row leads with.
func pick(lines []string, args []string) (int, error) {
	code, written, err := runFzf(lines, args)
	if err != nil {
		return 0, err
	}
	switch code {
	case fzf.ExitOk:
	case fzf.ExitInterrupt, fzf.ExitNoMatch:
		return 0, errCancelled
	default:
		return 0, fmt.Errorf("picker exited %d", code)
	}
	if len(written) == 0 {
		return 0, errCancelled
	}
	field, _, _ := strings.Cut(written[len(written)-1], "\t")
	index, err := strconv.Atoi(field)
	if err != nil {
		return 0, fmt.Errorf("unreadable selection %q", written[len(written)-1])
	}
	return index, nil
}

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
