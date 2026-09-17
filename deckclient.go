package main

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	deckv1 "github.com/mikluko/zmd/internal/zmd/v1"
	"github.com/mikluko/zmd/internal/zmd/v1/deckv1connect"
)

// dialDeck returns a client for the daemon, or nil where there is none.
//
// Nothing here waits: a daemon that is not listening is the ordinary case on
// a machine where none was started, and the picker's answer to that is to do
// the work itself.
func dialDeck() deckv1connect.DeckServiceClient {
	path := socketPath()
	if path == "" {
		return nil
	}
	if conn, err := net.DialTimeout("unix", path, 100*time.Millisecond); err != nil {
		return nil
	} else {
		conn.Close()
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "unix", path)
			},
		},
	}
	// The host in the URL goes nowhere: the transport above dials the socket
	// whatever it says. It has to parse, so it says something.
	return deckv1connect.NewDeckServiceClient(client, "http://deck")
}

// walked returns the checkouts and what git says each is, from the daemon
// where one is listening and from this process where none is.
//
// The fallback is the whole of why the daemon is optional, and it is the same
// two calls the daemon itself makes.
func walked(ctx context.Context, client deckv1connect.DeckServiceClient, home string) ([]string, map[string]string) {
	if client != nil {
		answer, err := client.Places(ctx, connect.NewRequest(&deckv1.PlacesRequest{}))
		if err == nil {
			places := answer.Msg.GetPlaces()
			repos := make([]string, 0, len(places))
			ident := make(map[string]string, len(places))
			for _, p := range places {
				repos = append(repos, p.GetDir())
				if p.GetName() != "" {
					ident[filepath.Clean(p.GetDir())] = p.GetName()
				}
			}
			return repos, ident
		}
	}
	repos := repositories(home)
	return repos, identify(repos)
}
