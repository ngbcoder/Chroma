package ui

// State describes a UI state with target dimensions.
type State int

const (
	StateCollapsed State = iota // 190 x 36 pill
	StateExpanded              // 420 x 140 widget
	StateThinLine              // height 6 minimal line
	StateEdgeBlade             // vertical edge variant
	StateVolumeFlyout          // transient volume flyout
)

// Size holds width/height in device-independent units.
type Size struct {
	W, H float64
}

// DefaultSizes maps each state to its default dimensions.
var DefaultSizes = map[State]Size{
	StateCollapsed:    {190, 36},
	StateExpanded:     {420, 140},
	StateThinLine:     {190, 6},
	StateEdgeBlade:    {40, 200},
	StateVolumeFlyout: {220, 60},
}

// Layout holds spring-driven dimensions for the current transition.
type Layout struct {
	Width  *Spring
	Height *Spring
	Radius *Spring
	// current target state
	Target State
}

// NewLayout creates a layout starting in the collapsed state.
func NewLayout() *Layout {
	c := DefaultSizes[StateCollapsed]
	return &Layout{
		Width:  NewSpring(c.W),
		Height: NewSpring(c.H),
		Radius: NewSpring(18),
		Target: StateCollapsed,
	}
}

// GoTo animates to a new state, retargeting springs (velocity preserved).
func (l *Layout) GoTo(s State) {
	sz := DefaultSizes[s]
	l.Width.SetTarget(sz.W)
	l.Height.SetTarget(sz.H)
	l.Target = s
}

// SizeNow returns the current (interpolated) size.
func (l *Layout) SizeNow() Size {
	return Size{W: l.Width.Value(), H: l.Height.Value()}
}

// AtRest reports whether all springs are at rest.
func (l *Layout) AtRest() bool {
	return l.Width.AtRest() && l.Height.AtRest() && l.Radius.AtRest()
}

// Step advances all springs by dt; returns true if still animating.
func (l *Layout) Step(dt float64) bool {
	a := l.Width.Step(dt)
	b := l.Height.Step(dt)
	c := l.Radius.Step(dt)
	return a || b || c
}
