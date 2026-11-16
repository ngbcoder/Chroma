package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestStoreSendAndState(t *testing.T) {
	s := NewStore()
	defer s.Close()
	s.Send(Expand(true))
	// allow processing
	time.Sleep(50 * time.Millisecond)
	if !s.State().Expanded {
		t.Fatal("expected expanded")
	}
}

func TestStoreMediaUpdate(t *testing.T) {
	s := NewStore()
	defer s.Close()
	s.Send(MediaUpdate(MediaState{Available: true, Title: "T"}))
	time.Sleep(50 * time.Millisecond)
	if s.State().Media.Title != "T" {
		t.Fatalf("title %s", s.State().Media.Title)
	}
}

func TestStoreSnapshotsDoNotExposeMutableHealthMap(t *testing.T) {
	s := NewStore()
	defer s.Close()
	s.Send(Health("media", "ok"))
	time.Sleep(20 * time.Millisecond)

	snapshot := s.State()
	snapshot.Health["media"] = "corrupted"
	if got := s.State().Health["media"]; got != "ok" {
		t.Fatalf("snapshot mutation changed store state: %q", got)
	}
}

func TestStoreCloseIsIdempotentAndClosesSubscriptions(t *testing.T) {
	s := NewStore()
	sub := s.Subscribe(1)
	s.Close()
	s.Close()
	if _, ok := <-sub; ok {
		t.Fatal("expected subscription to close")
	}
}

func TestLifecycleRecoversFromPanic(t *testing.T) {
	l := NewLifecycle()
	defer l.Shutdown()

	var attempts int32
	started := make(chan struct{})
	l.Go(func(ctx context.Context) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			close(started)
			panic("boom")
		}
		select {
		case <-ctx.Done():
		case <-time.After(25 * time.Millisecond):
		}
	})

	<-started
	time.Sleep(150 * time.Millisecond)
	if got := atomic.LoadInt32(&attempts); got < 2 {
		t.Fatalf("expected retry after panic, got %d attempts", got)
	}
}
