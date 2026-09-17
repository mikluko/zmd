package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// The view a pane opens on is the one it was left on, remembered against the
// directory the pane stands in: panes in different projects reopen
// differently, and a pane opened where none has been before falls back to the
// view that matches where it stands.

// stateFile is where the remembered views live.
func stateFile() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "zmx-nav", "views")
}

// rememberedView returns the view last left in dir.
func rememberedView(dir string) (mode, bool) {
	path := stateFile()
	if path == "" || dir == "" {
		return 0, false
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer file.Close()

	want := filepath.Clean(dir)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		name, where, ok := strings.Cut(scanner.Text(), "\t")
		if !ok || where != want {
			continue
		}
		if m, ok := modeNamed(name); ok {
			return m, true
		}
	}
	return 0, false
}

// remember records that dir was last left on m.
//
// The file is rewritten rather than appended to, so a directory holds one
// line however many times it is visited. Failing to write it is not worth
// reporting: the next pane opens on the fallback and nothing else is lost.
func remember(dir string, m mode) {
	path := stateFile()
	if path == "" || dir == "" {
		return
	}
	dir = filepath.Clean(dir)

	kept := []string{m.String() + "\t" + dir}
	if file, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if _, where, ok := strings.Cut(line, "\t"); ok && where != dir {
				kept = append(kept, line)
			}
		}
		file.Close()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o600)
}
