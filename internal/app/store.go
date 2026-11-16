package app

import (
	"sync"
	"time"
)

// MediaState is the media snapshot.
type MediaState struct {
	Available         bool
	SourceApp         string
	TrackIdentity     string
	Title             string
	Artist            string
	Album             string
	Status            int32
	Position          time.Duration
	Duration          time.Duration
	PositionUpdatedAt time.Time
	Thumbnail         uintptr
	SessionIndex      int
	SessionCount      int
}

// State is the immutable-ish application snapshot.
type State struct {
	Media    MediaState
	Expanded bool
	Volume   float64
	Muted    bool
	Health   map[string]string // subsystem -> ok/degraded/unavailable
}

// Event is applied by the single writer.
type Event interface {
	apply(*State)
}

type mediaUpdateEvent struct{ m MediaState }

func (e mediaUpdateEvent) apply(s *State) { s.Media = e.m }

type expandEvent struct{ v bool }

func (e expandEvent) apply(s *State) { s.Expanded = e.v }

type volumeEvent struct {
	v     float64
	muted bool
}

func (e volumeEvent) apply(s *State) { s.Volume = e.v; s.Muted = e.muted }

type healthEvent struct{ name, status string }

func (e healthEvent) apply(s *State) {
	if s.Health == nil {
		s.Health = map[string]string{}
	}
	s.Health[e.name] = e.status
}

// Public event constructors.
func MediaUpdate(m MediaState) Event     { return mediaUpdateEvent{m: m} }
func Expand(v bool) Event                { return expandEvent{v: v} }
func Volume(v float64, muted bool) Event { return volumeEvent{v: v, muted: muted} }
func Health(name, status string) Event   { return healthEvent{name: name, status: status} }

// Store is a single-writer state store.
type Store struct {
	events    chan Event
	state     State
	mu        sync.RWMutex
	subs      []chan State
	done      chan struct{}
	stopped   chan struct{}
	closeOnce sync.Once
}

func NewStore() *Store {
	s := &Store{
		events:  make(chan Event, 64),
		state:   State{Health: map[string]string{}},
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *Store) run() {
	defer close(s.stopped)
	for {
		select {
		case ev := <-s.events:
			if ev == nil {
				continue
			}
			s.mu.Lock()
			ev.apply(&s.state)
			snap := cloneState(s.state)
			s.mu.Unlock()
			s.broadcast(snap)
		case <-s.done:
			return
		}
	}
}

func cloneState(state State) State {
	if state.Health != nil {
		health := make(map[string]string, len(state.Health))
		for name, status := range state.Health {
			health[name] = status
		}
		state.Health = health
	}
	return state
}

func (s *Store) broadcast(snap State) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.subs {
		select {
		case ch <- cloneState(snap):
		default:
		}
	}
}

// Send enqueues an event (non-blocking; drops if full).
func (s *Store) Send(ev Event) {
	if ev == nil {
		return
	}
	select {
	case <-s.done:
		return
	default:
	}
	select {
	case s.events <- ev:
	case <-s.done:
	default:
	}
}

// State returns the current snapshot.
func (s *Store) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
}

// Subscribe returns a channel receiving state snapshots.
func (s *Store) Subscribe(buf int) <-chan State {
	ch := make(chan State, buf)
	s.mu.Lock()
	select {
	case <-s.done:
		close(ch)
		s.mu.Unlock()
		return ch
	default:
	}
	s.subs = append(s.subs, ch)
	s.mu.Unlock()
	return ch
}

// Close stops the store.
func (s *Store) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		<-s.stopped
		s.mu.Lock()
		for _, ch := range s.subs {
			close(ch)
		}
		s.subs = nil
		s.mu.Unlock()
	})
}
