package ui

// Spring is a 1D damped harmonic oscillator with unit mass.
//
//	a = -k*(x - target) - c*v
//
// Defaults: stiffness k = 280, damping c = 22 (damping ratio ~0.66, slight overshoot).
type Spring struct {
	x      float64
	v      float64
	target float64

	K float64 // stiffness
	C float64 // damping

	// accumulator for fixed internal substep (1/240 s).
	acc float64

	// rest thresholds
	posEps float64
	velEps float64
}

const substep = 1.0 / 240.0

// NewSpring creates a spring at initial value x0 with default stiffness/damping.
func NewSpring(x0 float64) *Spring {
	s := &Spring{
		x:      x0,
		target: x0,
		K:      280,
		C:      22,
		posEps: 0.05,
		velEps: 0.05,
	}
	return s
}

// SetParams overrides stiffness/damping (e.g. snappiness setting).
func (s *Spring) SetParams(k, c float64) {
	s.K = k
	s.C = c
}

// Value returns the current position.
func (s *Spring) Value() float64 { return s.x }

// Velocity returns the current velocity.
func (s *Spring) Velocity() float64 { return s.v }

// Target returns the target position.
func (s *Spring) Target() float64 { return s.target }

// SetTarget sets a new target, preserving velocity (retargeting mid-flight).
func (s *Spring) SetTarget(t float64) {
	s.target = t
}

// Reset sets both position and target, zeroing velocity.
func (s *Spring) Reset(x float64) {
	s.x = x
	s.v = 0
	s.target = x
	s.acc = 0
}

// Step advances the spring by dt seconds (real frame delta). Uses an
// accumulator with a fixed 1/240 s internal substep so behavior is identical
// at 60/120/144 Hz. Returns true if the spring is still moving (not at rest).
func (s *Spring) Step(dt float64) bool {
	if dt < 0 {
		dt = 0
	}
	s.acc += dt
	// guard against huge deltas (e.g. after sleep) by clamping
	if s.acc > 0.25 {
		s.acc = 0.25
	}
	for s.acc >= substep {
		s.substep(substep)
		s.acc -= substep
	}
	if s.AtRest() {
		s.x = s.target
		s.v = 0
		s.acc = 0
		return false
	}
	return true
}

func (s *Spring) substep(h float64) {
	// semi-implicit Euler
	a := -s.K*(s.x-s.target) - s.C*s.v
	s.v += a * h
	s.x += s.v * h
}

// AtRest reports whether distance to target and velocity are both below epsilon.
func (s *Spring) AtRest() bool {
	d := s.x - s.target
	if d < 0 {
		d = -d
	}
	vv := s.v
	if vv < 0 {
		vv = -vv
	}
	return d < s.posEps && vv < s.velEps
}

// Snap forces the spring to its target and zeroes velocity.
func (s *Spring) Snap() {
	s.x = s.target
	s.v = 0
	s.acc = 0
}
