package main

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
)

// align pads every column to its widest cell, leaving the last ragged.
func align(rows [][]string) []string {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	for _, cells := range rows {
		fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
	w.Flush()
	out := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for i := range out {
		out[i] = strings.TrimRight(out[i], " ")
	}
	return out
}
