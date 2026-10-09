package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// visit is how much a place has been worked in, and when it last was.
type visit struct {
	Rank float64 `json:"rank"`
	Last int64   `json:"last"`
}

// visits is every place worked in, keyed by its directory.
type visits map[string]visit

// maxRank is the total past which every rank is scaled down, so that places
// worked in long ago fall away rather than holding their rank for ever.
const maxRank = 10000

// visitsFile is where the visits are kept.
func visitsFile() string {
	dir := stateDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "visits.json")
}

// loadVisits reads the visits, or none where there is no record or it does
// not read: what it ranks is an order, and losing it loses nothing else.
func loadVisits() visits {
	out := visits{}
	path := visitsFile()
	if path == "" {
		return out
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(content, &out)
	return out
}

// recordVisits credits a visit to each of dirs, under a lock on the record,
// since the daemon and every picker write it.
func recordVisits(dirs []string, now time.Time) {
	path := visitsFile()
	if path == "" || len(dirs) == 0 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()

	v := loadVisits()
	for _, dir := range dirs {
		v.add(dir, now)
	}
	content, err := json.Marshal(v)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, content, 0o600) != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// add credits one visit to dir, scaling every rank down once their total
// passes maxRank and forgetting what that leaves below one.
func (v visits) add(dir string, now time.Time) {
	dir = filepath.Clean(dir)
	e := v[dir]
	e.Rank++
	e.Last = now.Unix()
	v[dir] = e

	total := 0.0
	for _, e := range v {
		total += e.Rank
	}
	if total <= maxRank {
		return
	}
	for k, e := range v {
		e.Rank *= 0.9
		if e.Rank < 1 {
			delete(v, k)
			continue
		}
		v[k] = e
	}
}

// score is dir's rank weighted by how lately it was visited, which is
// zoxide's frecency: four times within the hour, twice within the day, half
// within the week, a quarter after that.
func (v visits) score(dir string, now time.Time) float64 {
	e, ok := v[filepath.Clean(dir)]
	if !ok {
		return 0
	}
	age := now.Sub(time.Unix(e.Last, 0))
	switch {
	case age < time.Hour:
		return e.Rank * 4
	case age < 24*time.Hour:
		return e.Rank * 2
	case age < 7*24*time.Hour:
		return e.Rank / 2
	default:
		return e.Rank / 4
	}
}

// placeOf is the place a working directory counts as a visit to: the
// repository it stands in, or the directory itself outside any.
func placeOf(dir string) string {
	if root := repoRoot(dir); root != "" {
		return root
	}
	return filepath.Clean(dir)
}
