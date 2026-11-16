package ui

import "testing"

func TestSpringRestAtTarget(t *testing.T) {
	s := NewSpring(0)
	s.SetTarget(100)
	// step long enough to settle
	for i := 0; i < 10000; i++ {
		s.Step(1.0 / 60.0)
	}
	if !s.AtRest() {
		t.Fatalf("spring not at rest: x=%v v=%v", s.Value(), s.Velocity())
	}
	if s.Value() != 100 {
		t.Fatalf("expected snap to 100, got %v", s.Value())
	}
}

func TestSpringRetargetPreservesVelocity(t *testing.T) {
	s := NewSpring(0)
	s.SetTarget(100)
	s.Step(0.1)
	v0 := s.Velocity()
	s.SetTarget(200)
	if s.Velocity() != v0 {
		t.Fatalf("retarget changed velocity: %v vs %v", s.Velocity(), v0)
	}
}

func TestSpringRestDetectionThreshold(t *testing.T) {
	s := NewSpring(5)
	if !s.AtRest() {
		t.Fatal("spring at target should be at rest")
	}
}

func TestLayoutTransition(t *testing.T) {
	l := NewLayout()
	if l.SizeNow().W != 190 {
		t.Fatalf("initial width %v", l.SizeNow().W)
	}
	l.GoTo(StateExpanded)
	for i := 0; i < 10000; i++ {
		l.Step(1.0 / 60.0)
		if l.AtRest() {
			break
		}
	}

	if !l.AtRest() {
		t.Fatal("layout not at rest")
	}
	if l.SizeNow().W != 420 || l.SizeNow().H != 140 {
		t.Fatalf("final size %v", l.SizeNow())
	}
}

func TestDockModesSetCollapsedGeometry(t *testing.T) {
	tests := []struct {
		mode string
		size Size
	}{
		{mode: "top_island", size: Size{190, 36}},
		{mode: "top_notch", size: Size{220, 36}},
		{mode: "thin_line", size: Size{640, 6}},
		{mode: "edge_left", size: Size{40, 200}},
		{mode: "edge_right", size: Size{40, 200}},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			l := NewLayout()
			if !l.SetDockMode(tt.mode) {
				t.Fatal("dock mode rejected")
			}
			for i := 0; i < 10000 && !l.AtRest(); i++ {
				l.Step(1.0 / 60.0)
			}
			if got := l.SizeNow(); got != tt.size {
				t.Fatalf("got geometry %+v, want %+v", got, tt.size)
			}
		})
	}
}

func TestDockModePositionAnchorsToEdge(t *testing.T) {
	l := NewLayout()
	if !l.SetDockMode("edge_right") {
		t.Fatal("dock mode rejected")
	}
	for i := 0; i < 10000 && !l.AtRest(); i++ {
		l.Step(1.0 / 60.0)
	}
	x, y := l.Position(640, 260)
	if x != 600 || y != 30 {
		t.Fatalf("edge-right position (%v,%v)", x, y)
	}
	if l.SetDockMode("not-a-mode") {
		t.Fatal("accepted unknown dock mode")
	}
}
