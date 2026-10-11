package main

import (
	"testing"
	"time"
)

// Recent work counts for more than old work of the same amount.
func TestScoreWeighsRecency(t *testing.T) {
	now := time.Now()
	v := visits{
		"/hour": {Rank: 1, Last: now.Add(-time.Minute).Unix()},
		"/day":  {Rank: 1, Last: now.Add(-2 * time.Hour).Unix()},
		"/week": {Rank: 1, Last: now.Add(-48 * time.Hour).Unix()},
		"/old":  {Rank: 1, Last: now.Add(-30 * 24 * time.Hour).Unix()},
	}
	if !(v.score("/hour", now) > v.score("/day", now) &&
		v.score("/day", now) > v.score("/week", now) &&
		v.score("/week", now) > v.score("/old", now)) {
		t.Error("scores do not fall with age")
	}
	if v.score("/never", now) != 0 {
		t.Error("a place never visited scores")
	}
}

// Past the cap every rank shrinks, and what that leaves below one is
// forgotten, so the record does not grow for ever.
func TestAddScalesDownPastMaxRank(t *testing.T) {
	now := time.Now()
	v := visits{"/big": {Rank: maxRank, Last: now.Unix()}, "/tiny": {Rank: 1, Last: now.Unix()}}
	v.add("/big", now)
	if _, ok := v["/tiny"]; ok {
		t.Error("a place scaled below one is kept")
	}
	if r := v["/big"].Rank; r >= maxRank {
		t.Errorf("rank %v not scaled down", r)
	}
}

func TestRecordVisitsPersists(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()
	recordVisits([]string{"/a", "/a/"}, now)
	recordVisits([]string{"/b"}, now)
	v := loadVisits()
	if v["/a"].Rank != 2 || v["/b"].Rank != 1 {
		t.Errorf("visits %v", v)
	}
}

// A session is credited where it appears and where it moves to, not every
// time it is seen standing still.
func TestCreditMovesOnlyOnAMove(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a, b := t.TempDir(), t.TempDir()
	where := creditMoves(nil, []Session{{Name: "s", Dir: a}})
	where = creditMoves(where, []Session{{Name: "s", Dir: a}})
	creditMoves(where, []Session{{Name: "s", Dir: b}})
	v := loadVisits()
	if v[a].Rank != 1 || v[b].Rank != 1 {
		t.Errorf("visits %v, want one each", v)
	}
}
