package main

import (
	"bufio"
	"io"
	"net/url"
	"os/exec"
	"sort"
	"strings"
)

// Session is one running zmx session, as `zmx list` reports it.
type Session struct {
	Name    string
	PID     string
	Clients string
	Dir     string
}

// sessions returns every running session, ordered by name.
//
// A zmx that is absent, or that reports no sessions, yields none rather than
// an error: a picker with nothing to show is not a failure.
func sessions() ([]Session, error) {
	out, err := exec.Command("zmx", "list").Output()
	if err != nil {
		// `zmx list` exits non-zero with no sessions on some paths, and its
		// stdout is empty in exactly that case, so an empty read is the
		// answer rather than the error.
		if len(out) == 0 {
			return nil, nil
		}
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
			switch key {
			case "name":
				s.Name = value
			case "pid":
				s.PID = value
			case "clients":
				s.Clients = value
			case "cwd":
				s.Dir = parseCwd(value)
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
// (`file://saraksh.local/Users/...`), so the authority is dropped and the
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
