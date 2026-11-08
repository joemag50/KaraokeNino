package player

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Item is a song in the queue.
type Item struct {
	TrackID int64
	Artist  string
	Title   string
	File    string // absolute path on the disk
	Type    string // AVI, MP3...
	Audio   bool   // no video: the menu stays on screen
	Karaoke bool   // a karaoke video (vs. a music video): "Cantar" on the keypad
	Folder  string // catalog folder, for the cover
	Cover   string

	// Filler marks background music: any guest song replaces it right away.
	Filler bool
}

// Label is "Title — Artist".
func (it Item) Label() string {
	if it.Artist == "" {
		return it.Title
	}
	return it.Title + " — " + it.Artist
}

// Player plays one file at a time. The channel closes when playback ends.
type Player interface {
	Play(it Item) (<-chan struct{}, error)
	Stop() error
}

var (
	ErrQueueFull   = errors.New("la cola está llena")
	ErrArtistInRow = errors.New("ese artista ya está varias veces seguidas")
)

// Options are the guest limits and background music.
type Options struct {
	MaxQueue          int
	MaxPerArtistInRow int
	// Filler returns a background song when the queue is empty (nil = off).
	Filler func() (Item, bool)
	// FillerAfter waits this long in silence before background music.
	FillerAfter time.Duration
}

// Jukebox plays queued songs one after another, like a jukebox playlist.
// Callbacks run on background goroutines.
type Jukebox struct {
	p Player

	OnChange func()
	OnError  func(Item, error)
	// OnStart runs when a song starts (play history, "Siguiente" notice).
	OnStart func(cur Item, next *Item)
	// OnFinished runs when the last guest song ended and nothing is left.
	OnFinished func()

	mu      sync.Mutex
	opts    Options
	queue   []Item
	current *Item
	gen     int // bumps on every start so stale "done" signals are ignored
	idle    *time.Timer
	// sung is set once a guest song played in this session; background
	// music only starts after that (never right after the program opens).
	sung bool
}

func NewJukebox(p Player, opts Options) *Jukebox {
	return &Jukebox{p: p, opts: opts}
}

// SetOptions changes limits/background music (admin menu).
func (j *Jukebox) SetOptions(o Options) {
	j.mu.Lock()
	j.opts = o
	idle := j.current == nil && len(j.queue) == 0 && j.sung
	j.mu.Unlock()
	if idle {
		j.scheduleFiller()
	}
}

// Add queues a song and starts it right away if nothing (or only background
// music) is playing. It returns the position: 0 = playing now, 1 = next...
func (j *Jukebox) Add(it Item) (int, error) {
	j.mu.Lock()
	if j.opts.MaxQueue > 0 && len(j.queue) >= j.opts.MaxQueue {
		j.mu.Unlock()
		return 0, ErrQueueFull
	}
	if n := j.opts.MaxPerArtistInRow; n > 0 && it.Artist != "" && j.artistRunLocked(it.Artist) >= n {
		j.mu.Unlock()
		return 0, ErrArtistInRow
	}
	it.Filler = false
	j.queue = append(j.queue, it)
	pos := len(j.queue)
	if j.current == nil || j.current.Filler {
		j.startNextLocked()
		pos = 0
	}
	j.mu.Unlock()
	j.notify()
	return pos, nil
}

// artistRunLocked counts how many songs in a row by artist end the queue
// (including the current song when the queue is empty behind it).
func (j *Jukebox) artistRunLocked(artist string) int {
	all := j.queue
	if j.current != nil && !j.current.Filler {
		all = append([]Item{*j.current}, j.queue...)
	}
	n := 0
	for i := len(all) - 1; i >= 0 && strings.EqualFold(all[i].Artist, artist); i-- {
		n++
	}
	return n
}

// Restore puts back the queue saved before a restart WITHOUT playing it:
// the rockola stays silent after a restart. The saved songs start when a
// guest adds a song or presses "siguiente".
func (j *Jukebox) Restore(items []Item) {
	if len(items) == 0 {
		return
	}
	j.mu.Lock()
	j.queue = append(j.queue, items...)
	j.mu.Unlock()
	j.notify()
}

// Next skips the current song.
func (j *Jukebox) Next() {
	j.mu.Lock()
	j.startNextLocked()
	j.mu.Unlock()
	j.notify()
}

// Remove drops queue position i (0 = next song).
func (j *Jukebox) Remove(i int) bool {
	j.mu.Lock()
	if i < 0 || i >= len(j.queue) {
		j.mu.Unlock()
		return false
	}
	j.queue = append(j.queue[:i], j.queue[i+1:]...)
	j.mu.Unlock()
	j.notify()
	return true
}

// Move shifts queue position i by delta (-1 = earlier) and returns the new position.
func (j *Jukebox) Move(i, delta int) int {
	j.mu.Lock()
	k := i + delta
	if i < 0 || i >= len(j.queue) || k < 0 || k >= len(j.queue) {
		j.mu.Unlock()
		return i
	}
	j.queue[i], j.queue[k] = j.queue[k], j.queue[i]
	j.mu.Unlock()
	j.notify()
	return k
}

// Clear stops playback and empties the queue.
func (j *Jukebox) Clear() {
	j.mu.Lock()
	j.queue = nil
	j.current = nil
	j.gen++
	if j.idle != nil {
		j.idle.Stop()
	}
	_ = j.p.Stop()
	j.mu.Unlock()
	j.notify()
	go j.scheduleFiller()
}

// Snapshot returns the current song (nil when idle) and a copy of the queue.
func (j *Jukebox) Snapshot() (*Item, []Item) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var cur *Item
	if j.current != nil {
		c := *j.current
		cur = &c
	}
	return cur, append([]Item(nil), j.queue...)
}

func (j *Jukebox) startNextLocked() {
	j.gen++
	if j.idle != nil {
		j.idle.Stop()
	}
	for len(j.queue) > 0 {
		it := j.queue[0]
		j.queue = j.queue[1:]
		if j.playLocked(it) {
			return
		}
	}
	finished := j.current != nil && !j.current.Filler
	j.current = nil
	_ = j.p.Stop()
	if finished && j.OnFinished != nil {
		go j.OnFinished()
	}
	go j.scheduleFiller()
}

func (j *Jukebox) playLocked(it Item) bool {
	done, err := j.p.Play(it)
	if err != nil {
		j.reportError(it, err)
		return false
	}
	j.current = &it
	if !it.Filler {
		j.sung = true
	}
	go j.waitDone(done, j.gen)
	if j.OnStart != nil {
		var next *Item
		if len(j.queue) > 0 {
			n := j.queue[0]
			next = &n
		}
		go j.OnStart(it, next)
	}
	return true
}

// scheduleFiller starts background music after FillerAfter of silence.
func (j *Jukebox) scheduleFiller() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.opts.Filler == nil || j.current != nil || len(j.queue) > 0 || !j.sung {
		return
	}
	if j.idle != nil {
		j.idle.Stop()
	}
	gen := j.gen
	j.idle = time.AfterFunc(j.opts.FillerAfter, func() {
		j.mu.Lock()
		if gen != j.gen || j.current != nil || len(j.queue) > 0 || j.opts.Filler == nil {
			j.mu.Unlock()
			return
		}
		filler := j.opts.Filler
		j.mu.Unlock()

		it, ok := filler()
		if !ok {
			return
		}
		it.Filler = true
		j.mu.Lock()
		if gen != j.gen || j.current != nil || len(j.queue) > 0 {
			j.mu.Unlock()
			return
		}
		j.gen++
		started := j.playLocked(it)
		j.mu.Unlock()
		if started {
			j.notify()
		} else {
			j.scheduleFiller()
		}
	})
}

func (j *Jukebox) waitDone(done <-chan struct{}, gen int) {
	<-done
	j.mu.Lock()
	if gen != j.gen {
		j.mu.Unlock()
		return
	}
	j.startNextLocked()
	j.mu.Unlock()
	j.notify()
}

func (j *Jukebox) reportError(it Item, err error) {
	if j.OnError != nil {
		go j.OnError(it, fmt.Errorf("%s: %w", it.Title, err))
	}
}

func (j *Jukebox) notify() {
	if j.OnChange != nil {
		j.OnChange()
	}
}
