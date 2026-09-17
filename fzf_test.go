package main

import (
	"errors"
	"testing"
)

// fzf writes the query first, then the key, then a line per selected row.
func TestParseChoiceReadsQueryKeyAndRows(t *testing.T) {
	got, err := parseChoice([]string{"oct", "delete", "3\tsession\ta\t●  a", "7\trepo\t/b\t   [repo] b"})
	if err != nil {
		t.Fatal(err)
	}
	if got.query != "oct" || got.key != "delete" {
		t.Errorf("query %q key %q", got.query, got.key)
	}
	if len(got.rows) != 2 || got.rows[0] != 3 || got.rows[1] != 7 {
		t.Errorf("rows %v, want 3 and 7", got.rows)
	}
}

// Enter writes an empty key, and a query matching nothing writes no rows at
// all: the query is then the whole of the answer.
func TestParseChoiceKeepsAQueryThatMatchedNothing(t *testing.T) {
	got, err := parseChoice([]string{"scratch", ""})
	if err != nil {
		t.Fatal(err)
	}
	if got.query != "scratch" || got.key != keyEnter || len(got.rows) != 0 {
		t.Errorf("read %+v", got)
	}
}

// Left accepts, so the marker it queues arrives between the key and the row
// the cursor was left on.
func TestParseChoiceReadsThePopMarkerAheadOfTheRows(t *testing.T) {
	got, err := parseChoice([]string{"", "", popRung, "1\ts\t/b\t b"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.pop {
		t.Error("the pop marker was not read")
	}
	if len(got.rows) != 1 || got.rows[0] != 1 {
		t.Errorf("rows %v, want the row under the cursor", got.rows)
	}
}

func TestParseChoiceOnNothingWrittenIsCancelled(t *testing.T) {
	if _, err := parseChoice(nil); !errors.Is(err, errCancelled) {
		t.Errorf("empty output gave %v, want cancelled", err)
	}
}

func TestParseChoiceRejectsALineWithNoIndex(t *testing.T) {
	if _, err := parseChoice([]string{"", "", "not-an-index\tsession\ta\ta"}); err == nil {
		t.Error("a line leading with no index was read as a row")
	}
}
