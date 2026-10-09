package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Session is one running zmx session, as `zmx list` reports it.
type Session struct {
	Name    string
	PID     string
	Clients string
	Dir     string
	Title   string // what a `zmx set <name> title=` says the session is for
	Here    bool   // this terminal is one of the clients
}

// heldElsewhere reports whether a terminal other than this one has the session
// open.
//
// zmx lets any number of clients attach to one session and they mirror each
// other, keystrokes and all, which is worth walking into deliberately rather
// than by pressing enter on a row that looked free.
func (s Session) heldElsewhere() bool {
	n, err := strconv.Atoi(s.Clients)
	if err != nil {
		return false
	}
	if s.Here {
		n--
	}
	return n > 0
}

// sessions returns every running session, ordered by name.
//
// A zmx that is absent, or that reports no sessions, yields none rather than
// an error: a picker with nothing to show is not a failure. A zmx that fails
// is one, carrying what it wrote to stderr, since read as none it would show
// every session as gone.
func sessions() ([]Session, error) {
	out, err := exec.Command("zmx", "list").Output()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		msg := strings.TrimSpace(string(exit.Stderr))
		if strings.HasPrefix(msg, "no sessions found") {
			return nil, nil
		}
		return nil, fmt.Errorf("zmx list: %s", msg)
	case errors.Is(err, exec.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	}
	return parseSessions(strings.NewReader(string(out))), nil
}

// parseSessions reads the tab-separated `key=value` records `zmx list` writes.
//
// Fields are read by key because the record is not positional: 0.8.1 omits
// `cmd=` on a session started without one, and a reader counting fields then
// takes the wrong column for every session in the list.
func parseSessions(r io.Reader) []Session {
	var found []Session
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var s Session
		for _, field := range strings.Split(line, "\t") {
			key, value, ok := strings.Cut(strings.TrimSpace(field), "=")
			if !ok {
				continue
			}
			// `zmx list` writes a marker ahead of the first field's label,
			// `→ ` on the session the terminal is attached to, so the label
			// is the last word of what precedes the `=` rather than all of
			// it. Reading the whole of it drops that session from the list.
			//
			// That there was something to strip is the marker, and the marker
			// is how a session this terminal holds is told from one another
			// terminal holds.
			if i := strings.LastIndexAny(key, " \t"); i >= 0 {
				key = key[i+1:]
				s.Here = true
			}
			switch key {
			case "name":
				s.Name = value
			case "pid":
				s.PID = value
			case "clients":
				s.Clients = value
			case "cwd":
				s.Dir = parseCwd(value)
			case "title":
				// A label, written on the same record as the fields zmx
				// keeps itself, so it costs no second call to read.
				s.Title = value
			}
		}
		if s.Name == "" {
			continue
		}
		if s.Clients == "" {
			s.Clients = "0"
		}
		found = append(found, s)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found
}

// parseCwd resolves a `cwd=` value to a filesystem path.
//
// zmx reports it as a file URI carrying the host the session was created on
// (`file://host.local/Users/...`), so the authority is dropped and the
// path percent-decoded. A value that does not parse is returned as given.
func parseCwd(value string) string {
	if value == "" {
		return ""
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "file" {
		if decoded, err := url.PathUnescape(value); err == nil {
			return decoded
		}
		return value
	}
	return u.Path
}

// fingerprint is what of live a picker draws, as one string: two readings
// with the same fingerprint draw the same list.
func fingerprint(live []Session) string {
	var b strings.Builder
	for _, s := range live {
		b.WriteString(s.Name + "\x00" + s.Clients + "\x00" + s.Title + "\x00" + s.Dir + "\n")
	}
	return b.String()
}
