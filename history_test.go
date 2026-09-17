package main

import (
	"strings"
	"testing"
)

func filtered(t *testing.T, in string) string {
	t.Helper()
	var out strings.Builder
	if err := sgrOnly(&out, strings.NewReader(in)); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestSGROnlyKeepsColourAndText(t *testing.T) {
	in := "plain \x1b[31mred\x1b[0m and \x1b[1;38;5;208mbright\x1b[m done\n"
	if got := filtered(t, in); got != in {
		t.Errorf("colour did not survive:\n%q\n%q", got, in)
	}
}

// Everything `zmx history --vt` puts in front of the scrollback is addressed
// to a terminal, and the preview's output reaches the one the picker is drawn
// on. The OSC 7 is the worst of them: it names a directory, and a terminal
// sitting inside a session would re-tie that session to it.
func TestSGROnlyDropsWhatIsAddressedToTheTerminal(t *testing.T) {
	for _, c := range []struct{ what, in string }{
		{"alternate screen", "\x1b[?1049h"},
		{"mouse tracking", "\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h"},
		{"bracketed paste", "\x1b[?2004h"},
		{"keyboard protocol", "\x1b[=5;1u"},
		{"cursor position", "\x1b[65;3H"},
		{"charset designation", "\x1b(B"},
		{"osc 7, BEL-terminated", "\x1b]7;file://host/tmp\x07"},
		{"osc 7, ST-terminated", "\x1b]7;file://host/tmp\x1b\\"},
		{"erase in display", "\x1b[2J"},
	} {
		if got := filtered(t, "a"+c.in+"b"); got != "ab" {
			t.Errorf("%s survived as %q", c.what, got)
		}
	}
}

// `ESC [ > 4 ; 2 m` ends in `m` and is not colour: it sets modifyOtherKeys.
// A filter that looks at the final byte alone lets it through.
func TestSGROnlyRejectsSequencesThatOnlyLookLikeColour(t *testing.T) {
	for _, in := range []string{"\x1b[>4;2m", "\x1b[?4m", "\x1b[=1m", "\x1b[<2m"} {
		if got := filtered(t, "a"+in+"b"); got != "ab" {
			t.Errorf("%q survived as %q", in, got)
		}
	}
}

// Truncated input is what a preview killed mid-write leaves behind.
func TestSGROnlyToleratesATruncatedSequence(t *testing.T) {
	for _, in := range []string{"text\x1b", "text\x1b[", "text\x1b[31", "text\x1b]7;file://h"} {
		if got := filtered(t, in); !strings.HasPrefix(got, "text") {
			t.Errorf("%q gave %q", in, got)
		}
	}
}

// The whole preamble at once, as the real thing emits it.
func TestSGROnlyOnWhatZmxActuallyWrites(t *testing.T) {
	in := "\x1b[?1049h\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h\x1b[?2004h" +
		"\x1b[?1004h\x1b[?2031h\x1b[>4;2m\x1b[=5;1u\x1b(B" +
		"\x1b[32m$ go test\x1b[0m\nok\n" +
		"\x1b[65;3H\x1b]7;file://host.local/Users/m/src\x1b\\"

	want := "\x1b[32m$ go test\x1b[0m\nok\n"
	if got := filtered(t, in); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestQuoteSurvivesAPathWithSpacesAndQuotes(t *testing.T) {
	if got := quote("/Applications/My Tools/zmd"); got != `'/Applications/My Tools/zmd'` {
		t.Errorf("got %s", got)
	}
	if got := quote("/tmp/it's"); got != `'/tmp/it'\''s'` {
		t.Errorf("got %s", got)
	}
}
