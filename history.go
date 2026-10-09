package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// A session's scrollback is previewed in colour, which `zmx history` gives
// only in its `--vt` form. That form is built to restore a client's terminal
// rather than to be read: alongside the colour it replays the alternate
// screen, four mouse-tracking modes, the keyboard protocol, a cursor
// position, and an OSC 7 naming the session's directory. Every one of those
// is addressed to a terminal, and the preview's output reaches the one the
// picker is drawn on, where OSC 7 would re-tie whatever session that terminal
// is inside.
//
// So the colour is kept and the rest is dropped, which is what this filter
// does.

// sgrOnly copies r to w, keeping the colour and dropping every other escape
// sequence.
//
// Only `ESC [ <digits and semicolons> m` is colour. The parameter bytes have
// to be checked rather than the final byte alone: `ESC [ > 4 ; 2 m` ends in
// `m` and sets modifyOtherKeys.
func sgrOnly(w io.Writer, r io.Reader) error {
	in := bufio.NewReader(r)
	out := bufio.NewWriter(w)
	defer out.Flush()

	for {
		b, err := in.ReadByte()
		if err != nil {
			return nil
		}
		if b != 0x1b {
			if err := out.WriteByte(b); err != nil {
				return err
			}
			continue
		}

		kind, err := in.ReadByte()
		if err != nil {
			return nil
		}
		switch kind {
		case '[':
			seq, ok := readCSI(in)
			if ok {
				if _, err := out.WriteString("\x1b[" + seq); err != nil {
					return err
				}
			}
		case ']':
			skipString(in)
		case 'P', 'X', '^', '_':
			skipString(in)
		case '(', ')', '*', '+', '#', ' ':
			// A two-byte sequence: the byte that selects what it designates
			// belongs to it and must not reach the output as text.
			in.ReadByte()
		}
	}
}

// readCSI reads the rest of a control sequence and reports whether it is
// colour and should be kept.
func readCSI(in *bufio.Reader) (string, bool) {
	var seq []byte
	plain := true
	for {
		b, err := in.ReadByte()
		if err != nil {
			return "", false
		}
		seq = append(seq, b)
		if b >= 0x40 && b <= 0x7e {
			// The final byte. Anything that reached here outside the digits
			// and semicolons of a colour sequence disqualifies it.
			return string(seq), plain && b == 'm'
		}
		if (b < '0' || b > '9') && b != ';' {
			plain = false
		}
	}
}

// skipString consumes a string sequence, which ends at BEL or at ESC \.
func skipString(in *bufio.Reader) {
	for {
		b, err := in.ReadByte()
		if err != nil {
			return
		}
		if b == 0x07 {
			return
		}
		if b == 0x1b {
			next, err := in.ReadByte()
			if err != nil {
				return
			}
			if next == '\\' {
				return
			}
		}
	}
}

// history writes a session's scrollback to w, in colour and without anything
// addressed to the terminal it is drawn on. What zmx writes to stderr comes
// back in the error rather than reaching that terminal.
func history(name string, w io.Writer) error {
	cmd := exec.Command("zmx", "history", "--vt", name)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := sgrOnly(w, out); err != nil {
		_ = cmd.Wait()
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("zmx history %s: %s", name, strings.TrimSpace(stderr.String()))
	}
	return nil
}
