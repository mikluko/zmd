package main

import (
	"strings"
	"unicode"

	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

// matcher scores text against a query the way fzf's default mode does.
//
// It holds the scratch space fzf's algorithm allocates into, so one matcher
// serves one goroutine.
type matcher struct {
	slab *util.Slab
}

func newMatcher() *matcher {
	algo.Init("default")
	return &matcher{slab: util.MakeSlab(100*1024, 2048)}
}

// match reports whether every space-separated term of query matches text,
// their summed score, and the rune positions they matched at.
//
// A term is case-sensitive only where it carries an upper-case letter, which
// is fzf's smart case. The empty query matches everything with no positions.
func (m *matcher) match(text, query string) (int, []int, bool) {
	chars := util.ToChars([]byte(text))
	score := 0
	var pos []int
	for _, term := range strings.Fields(query) {
		pattern := []rune(term)
		sensitive := false
		for _, r := range pattern {
			if unicode.IsUpper(r) {
				sensitive = true
				break
			}
		}
		if !sensitive {
			pattern = []rune(strings.ToLower(term))
		}
		pattern = algo.NormalizeRunes(pattern)
		res, at := algo.FuzzyMatchV2(sensitive, true, true, &chars, pattern, true, m.slab)
		if res.Start < 0 {
			return 0, nil, false
		}
		score += res.Score
		if at != nil {
			pos = append(pos, *at...)
		}
	}
	return score, pos, true
}
