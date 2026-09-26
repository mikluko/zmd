package main

import (
	"slices"
	"testing"
)

func TestMatch(t *testing.T) {
	m := newMatcher()
	cases := []struct {
		text, query string
		ok          bool
		pos         []int
	}{
		{"retry-bug", "", true, nil},
		{"retry-bug", "rb", true, []int{0, 6}},
		{"retry-bug", "RB", false, nil},
		{"retry-bug  acme/gateway", "bug gate", true, []int{6, 7, 8, 16, 17, 18, 19}},
		{"retry-bug", "bug zzz", false, nil},
	}
	for _, c := range cases {
		_, pos, ok := m.match(c.text, c.query)
		if ok != c.ok {
			t.Errorf("match(%q, %q) ok = %v, want %v", c.text, c.query, ok, c.ok)
			continue
		}
		slices.Sort(pos)
		if ok && !slices.Equal(pos, c.pos) {
			t.Errorf("match(%q, %q) pos = %v, want %v", c.text, c.query, pos, c.pos)
		}
	}
}

func TestUnderCursorSurvivesResets(t *testing.T) {
	got := underCursor(dim+"x"+undim+"y", 4)
	want := cursorBg + "\x1b[1m" + dim + "x" + "\x1b[0m" + cursorBg + "\x1b[1m" + "y  " + "\x1b[0m"
	if got != want {
		t.Errorf("underCursor = %q, want %q", got, want)
	}
}

func TestMatchRanksTighterHigher(t *testing.T) {
	m := newMatcher()
	tight, _, _ := m.match("gateway", "gate")
	loose, _, _ := m.match("g-a-t-e", "gate")
	if tight <= loose {
		t.Errorf("tight %d should outscore loose %d", tight, loose)
	}
}
