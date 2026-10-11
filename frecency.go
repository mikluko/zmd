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

// loadVisits reads the visits.
func loadVisits() visits {
	return load[visits](visitsFile())
}

// recordVisits credits a visit to each of dirs.
func recordVisits(dirs []string, now time.Time) {
	if len(dirs) == 0 {
		return
	}
	update(visitsFile(), func(v visits) {
		for _, dir := range dirs {
			v.add(dir, now)
		}
	})
}

// attachesFile is where the time zmd last attached each session is kept.
func attachesFile() string {
	dir := stateDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "attaches.json")
}

// loadAttaches is when zmd last attached each session, by name, in Unix
// seconds; none where there is no record.
func loadAttaches() map[string]int64 {
	return load[map[string]int64](attachesFile())
}

// recordAttach notes that zmd attached name now.
func recordAttach(name string, now time.Time) {
	update(attachesFile(), func(a map[string]int64) {
		a[name] = now.Unix()
	})
}

// load reads the record at path, or an empty one where there is none or it
// does not read: what these records hold is an order, and losing it loses
// nothing else.
func load[M ~map[K]V, K comparable, V any](path string) M {
	out := M{}
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

// update applies change to the record at path under a lock on it, since the
// daemon and every picker write these records.
func update[M ~map[K]V, K comparable, V any](path string, change func(M)) {
	if path == "" {
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

	m := load[M](path)
	change(m)
	content, err := json.Marshal(m)
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
