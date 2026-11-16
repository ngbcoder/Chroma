package app

import (
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
