package ui

import (
	"testing"
	"time"
)

func TestDwellExpandAfterDelay(t *testing.T) {
	c := NewDwellController()
	c.ExpandDelay = 10 * time.Millisecond
	c.CollapseDelay = 20 * time.Millisecond
	now := time.Now()
	c.Update(true, now)
	if c.Expanded() {
		t.Fatal("should not expand immediately")
	}
	c.Update(true, now.Add(15*time.Millisecond))
	if !c.Expanded() {
		t.Fatal("should expand after delay")
	}
}

func TestDwellCollapseAfterLeave(t *testing.T) {
	c := NewDwellController()
	c.ExpandDelay = 1 * time.Millisecond
	c.CollapseDelay = 10 * time.Millisecond
	now := time.Now()
	c.Update(true, now)
	c.Update(true, now.Add(5*time.Millisecond))
	if !c.Expanded() {
		t.Fatal("expected expanded")
	}
	leave := now.Add(6 * time.Millisecond)
	c.Update(false, leave)
	if !c.Expanded() {
		// collapse not yet
	}
	c.Update(false, leave.Add(15*time.Millisecond))
	if c.Expanded() {
		t.Fatal("should collapse after leave delay")
	}
}
