package player

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// fakePlayer "plays" each song for d, or fails for files in bad.
type fakePlayer struct {
	d   time.Duration
	bad map[string]bool

	mu     sync.Mutex
	played []string
	stop   chan struct{}
}

func (f *fakePlayer) Play(it Item) (<-chan struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stop != nil {
		close(f.stop)
	}
	if f.bad[it.File] {
		f.stop = nil
		return nil, errors.New("no existe")
	}
	f.played = append(f.played, it.Title)
	stop := make(chan struct{})
	f.stop = stop
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-time.After(f.d):
		case <-stop:
		}
	}()
	return done, nil
}

func (f *fakePlayer) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stop != nil {
		close(f.stop)
		f.stop = nil
	}
	return nil
}

func newTestJukebox(t *testing.T, opts Options) (*Jukebox, *fakePlayer) {
	t.Helper()
	p := &fakePlayer{d: 50 * time.Millisecond}
	j := NewJukebox(p, opts)
	t.Cleanup(j.Clear)
	return j, p
}

func title(it *Item) string {
	if it == nil {
		return ""
	}
	return it.Title
}

func mustAdd(t *testing.T, j *Jukebox, it Item) int {
	t.Helper()
	pos, err := j.Add(it)
	if err != nil {
		t.Fatal(err)
	}
	return pos
}

func TestJukeboxPlaysInOrder(t *testing.T) {
	j, _ := newTestJukebox(t, Options{})
	if pos := mustAdd(t, j, Item{Title: "a"}); pos != 0 {
		t.Fatalf("first Add pos = %d, want 0", pos)
	}
	if pos := mustAdd(t, j, Item{Title: "b"}); pos != 1 {
		t.Fatalf("second Add pos = %d, want 1", pos)
	}
	cur, q := j.Snapshot()
	if title(cur) != "a" || len(q) != 1 {
		t.Fatalf("playing %q with queue %v", title(cur), q)
	}
	time.Sleep(80 * time.Millisecond)
	if cur, _ := j.Snapshot(); title(cur) != "b" {
		t.Fatalf("after first song, playing %q, want b", title(cur))
	}
	time.Sleep(80 * time.Millisecond)
	if cur, _ := j.Snapshot(); cur != nil {
		t.Fatalf("queue should be finished, playing %q", title(cur))
	}
}

func TestJukeboxNextSkips(t *testing.T) {
	j, _ := newTestJukebox(t, Options{})
	mustAdd(t, j, Item{Title: "a"})
	mustAdd(t, j, Item{Title: "b"})
	mustAdd(t, j, Item{Title: "c"})
	j.Next()
	if cur, q := j.Snapshot(); title(cur) != "b" || len(q) != 1 {
		t.Fatalf("after Next playing %q queue %v", title(cur), q)
	}
	// The skipped song's timer must not advance the queue a second time.
	time.Sleep(30 * time.Millisecond)
	if cur, _ := j.Snapshot(); title(cur) != "b" {
		t.Fatalf("stale done advanced queue to %q", title(cur))
	}
}

func TestJukeboxSkipsBrokenFiles(t *testing.T) {
	j, p := newTestJukebox(t, Options{})
	p.bad = map[string]bool{"/roto.mp4": true}
	errs := make(chan error, 1)
	j.OnError = func(_ Item, err error) { errs <- err }
	mustAdd(t, j, Item{Title: "x", File: "/roto.mp4"})
	if cur, _ := j.Snapshot(); cur != nil {
		t.Fatalf("broken file should not play, got %q", title(cur))
	}
	select {
	case <-errs:
	case <-time.After(time.Second):
		t.Fatal("expected OnError")
	}
	// The next song still plays.
	mustAdd(t, j, Item{Title: "y"})
	if cur, _ := j.Snapshot(); title(cur) != "y" {
		t.Fatalf("playing %q", title(cur))
	}
}

func TestJukeboxLimits(t *testing.T) {
	j, _ := newTestJukebox(t, Options{MaxQueue: 2, MaxPerArtistInRow: 2})
	mustAdd(t, j, Item{Title: "1", Artist: "Juan Gabriel"})
	mustAdd(t, j, Item{Title: "2", Artist: "juan gabriel"})
	if _, err := j.Add(Item{Title: "3", Artist: "Juan Gabriel"}); err != ErrArtistInRow {
		t.Fatalf("err=%v, quería ErrArtistInRow", err)
	}
	mustAdd(t, j, Item{Title: "4", Artist: "Shakira"})
	if _, err := j.Add(Item{Title: "5", Artist: "Maná"}); err != ErrQueueFull {
		t.Fatalf("err=%v, quería ErrQueueFull", err)
	}
}

func TestJukeboxRemoveAndMove(t *testing.T) {
	j, _ := newTestJukebox(t, Options{})
	for _, s := range []string{"a", "b", "c", "d"} {
		mustAdd(t, j, Item{Title: s})
	}
	// "a" plays right away; the queue is [b c d].
	if k := j.Move(2, -1); k != 1 { // [b d c]
		t.Fatalf("Move=%d", k)
	}
	j.Remove(0) // [d c]
	_, q := j.Snapshot()
	if len(q) != 2 || q[0].Title != "d" || q[1].Title != "c" {
		t.Fatalf("cola: %+v", q)
	}
}

func TestBackgroundMusicYieldsToGuests(t *testing.T) {
	var started []string
	var mu sync.Mutex
	j, _ := newTestJukebox(t, Options{
		FillerAfter: 10 * time.Millisecond,
		Filler:      func() (Item, bool) { return Item{Title: "fondo"}, true },
	})
	j.OnStart = func(cur Item, _ *Item) {
		mu.Lock()
		started = append(started, cur.Title)
		mu.Unlock()
	}
	j.Restore(nil) // at startup: silence, no background music yet
	time.Sleep(30 * time.Millisecond)
	if cur, _ := j.Snapshot(); cur != nil {
		t.Fatalf("al abrir no debe sonar nada, suena %+v", cur)
	}
	// After a guest song ends, background music fills the silence.
	mustAdd(t, j, Item{Title: "primera"})
	time.Sleep(100 * time.Millisecond)
	cur, _ := j.Snapshot()
	if cur == nil || !cur.Filler {
		t.Fatalf("esperaba música de fondo, suena %+v", cur)
	}
	// A guest song interrupts background music immediately.
	if pos := mustAdd(t, j, Item{Title: "invitado"}); pos != 0 {
		t.Fatalf("pos=%d", pos)
	}
	if cur, _ := j.Snapshot(); title(cur) != "invitado" || cur.Filler {
		t.Fatalf("suena %+v", cur)
	}
}

func TestRestoreKeepsQueueSilent(t *testing.T) {
	j, p := newTestJukebox(t, Options{})
	j.Restore([]Item{{Title: "a"}, {Title: "b"}})
	cur, q := j.Snapshot()
	if cur != nil || len(q) != 2 {
		t.Fatalf("después de reiniciar no debe sonar: %q %v", title(cur), q)
	}
	// "Siguiente" (or a new song) resumes the saved queue in order.
	j.Next()
	if cur, q := j.Snapshot(); title(cur) != "a" || len(q) != 1 {
		t.Fatalf("%q %v", title(cur), q)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.played) != 1 {
		t.Fatalf("played %v", p.played)
	}
}
