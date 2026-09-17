package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	deckv1 "github.com/mikluko/zmd/internal/zmd/v1"
	"github.com/mikluko/zmd/internal/zmd/v1/deckv1connect"
)

// The daemon holds what a picker would otherwise work out for itself, and
// tells an open picker when what it shows has moved.
//
// Walking the home directory costs a quarter of a second, and a picker paid
// it on every detach before drawing a single row. Nothing about the walk is
// per-picker: it is the same answer for every pane, and it changes when the
// filesystem does rather than when a picker opens.
//
// A picker that cannot reach the daemon does the work itself. That is what
// keeps this an optimisation rather than a dependency, and what lets the
// daemon be restarted under a running picker.

// tick is how often the daemon re-walks. The walk answers a question about
// the filesystem, which changes on the scale of a clone rather than a
// keystroke.
const tick = 30 * time.Second

// sessionTick is how often the daemon re-reads what is running. `zmx list`
// costs two milliseconds, so this is as often as anything needs.
const sessionTick = time.Second

// socketPath is where the daemon listens.
//
// ZMD_DECK_DIR names it instead where the socket has to be somewhere a
// forwarded connection can reach, an SSH RemoteForward being the case that
// needs it.
func socketPath() string {
	if dir := os.Getenv("ZMD_DECK_DIR"); dir != "" {
		return filepath.Join(dir, "deck.sock")
	}
	dir := stateDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "deck.sock")
}

// deck is the daemon's state: the walk, and who is waiting to hear it moved.
type deck struct {
	home string

	mu     sync.RWMutex
	places []*deckv1.Place

	watchers sync.Map // chan deckv1.WatchResponse_Reason, keyed by itself
}

// Places answers with the walk as it stands. It never walks on demand: a
// picker asking is a picker waiting, and the answer is already known.
func (d *deck) Places(_ context.Context, _ *connect.Request[deckv1.PlacesRequest]) (*connect.Response[deckv1.PlacesResponse], error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return connect.NewResponse(&deckv1.PlacesResponse{Places: d.places}), nil
}

// Watch streams until the picker goes away.
func (d *deck) Watch(ctx context.Context, _ *connect.Request[deckv1.WatchRequest], stream *connect.ServerStream[deckv1.WatchResponse]) error {
	events := make(chan deckv1.WatchResponse_Reason, 4)
	d.watchers.Store(events, struct{}{})
	defer d.watchers.Delete(events)

	for {
		select {
		case <-ctx.Done():
			return nil
		case reason := <-events:
			if err := stream.Send(&deckv1.WatchResponse{Reason: reason}); err != nil {
				return err
			}
		}
	}
}

// announce tells every watcher what moved, dropping the news for any whose
// buffer is full: a picker behind by one reload is already going to reload.
func (d *deck) announce(reason deckv1.WatchResponse_Reason) {
	d.watchers.Range(func(key, _ any) bool {
		select {
		case key.(chan deckv1.WatchResponse_Reason) <- reason:
		default:
		}
		return true
	})
}

// walk re-reads the filesystem and announces a change only where the answer
// differs, so a picker is not woken thirty seconds at a time for nothing.
func (d *deck) walk() {
	repos := repositories(d.home)
	ident := identify(repos)

	next := make([]*deckv1.Place, 0, len(repos))
	for _, dir := range repos {
		next = append(next, &deckv1.Place{Dir: dir, Name: ident[filepath.Clean(dir)]})
	}

	d.mu.Lock()
	same := slices.EqualFunc(d.places, next, func(a, b *deckv1.Place) bool {
		return a.GetDir() == b.GetDir() && a.GetName() == b.GetName()
	})
	d.places = next
	d.mu.Unlock()

	if !same {
		d.announce(deckv1.WatchResponse_REASON_PLACES)
	}
}

// watchSessions announces a change in what is running.
//
// The names and their clients are what a picker draws, so a change in either
// is a reason to redraw and nothing else is.
func (d *deck) watchSessions(ctx context.Context) {
	var was string
	for {
		live, err := sessions()
		if err == nil {
			now := ""
			for _, s := range live {
				now += s.Name + ":" + s.Clients + ":" + s.Title + "\n"
			}
			if now != was {
				if was != "" {
					d.announce(deckv1.WatchResponse_REASON_SESSIONS)
				}
				was = now
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(sessionTick):
		}
	}
}

// serve runs the daemon until the context ends or the socket is taken.
//
// The socket is unlinked before it is bound: a daemon killed rather than
// stopped leaves the file behind, and refusing to start because of it would
// need a human where none is wanted.
func serve(ctx context.Context) error {
	path := socketPath()
	if path == "" {
		return errors.New("nowhere to put the socket")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if conn, err := net.Dial("unix", path); err == nil {
		conn.Close()
		return errors.New("a daemon is already listening at " + path)
	}
	_ = os.Remove(path)

	listener, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(path)

	// A daemon killed rather than stopped would leave the socket behind, and
	// the next one has to guess whether a live daemon is behind it. Catching
	// the signal is what makes the unlink below run.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	d := &deck{home: home}
	d.walk()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(tick):
				d.walk()
			}
		}
	}()
	go d.watchSessions(ctx)

	mux := http.NewServeMux()
	mux.Handle(deckv1connect.NewDeckServiceHandler(d))
	server := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		server.Close()
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
