package ui

// State describes a UI state with target dimensions.
type State int

const (
	StateCollapsed    State = iota // 190 x 36 pill
	StateExpanded                  // 420 x 140 widget
	StateThinLine                  // height 6 minimal line
	StateEdgeBlade                 // vertical edge variant
	StateVolumeFlyout              // transient volume flyout
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
	Width    *Spring
	Height   *Spring
	Radius   *Spring
	Scale    *Spring
	Glow     *Spring
	Target   State
	DockMode string
}

// NewLayout creates a layout starting in the collapsed state.
func NewLayout() *Layout {
	c := DefaultSizes[StateCollapsed]
	return &Layout{
		Width:    NewSpring(c.W),
		Height:   NewSpring(c.H),
		Radius:   NewSpring(18),
		Scale:    NewSpring(1.0),
		Glow:     NewSpring(0.0),
		Target:   StateCollapsed,
		DockMode: "top_island",
	}
}

// GoTo animates to a new state, retargeting springs (velocity preserved).
func (l *Layout) GoTo(s State) {
	sz := l.sizeFor(s)
	l.Width.SetTarget(sz.W)
	l.Height.SetTarget(sz.H)
	l.Radius.SetTarget(l.radiusFor(s))
	l.Target = s
}

// SetDockMode changes the collapsed geometry while preserving spring velocity.
func (l *Layout) SetDockMode(mode string) bool {
	switch mode {
	case "top_island", "top_notch", "thin_line", "edge_left", "edge_right":
	default:
		return false
	}
	l.DockMode = mode
	l.GoTo(l.Target)
	return true
}

func (l *Layout) sizeFor(s State) Size {
	if s == StateCollapsed {
		switch l.DockMode {
		case "top_notch":
			return Size{220, 36}
		case "thin_line":
			return Size{640, 6}
		case "edge_left", "edge_right":
			return DefaultSizes[StateEdgeBlade]
		}
	}
	return DefaultSizes[s]
}

func (l *Layout) radiusFor(s State) float64 {
	if s == StateCollapsed {
		switch l.DockMode {
		case "thin_line":
			return 3
		case "top_notch":
			return 10
		}
	}
	return 18
}

// Position returns the layout origin in the fixed canvas for the current dock mode.
func (l *Layout) Position(canvasWidth, canvasHeight int) (float64, float64) {
	sz := l.SizeNow()
	switch l.DockMode {
	case "thin_line", "top_island", "top_notch":
		return (float64(canvasWidth) - sz.W) / 2, 0
	case "edge_left":
		return 0, (float64(canvasHeight) - sz.H) / 2
	case "edge_right":
		return float64(canvasWidth) - sz.W, (float64(canvasHeight) - sz.H) / 2
	default:
		return (float64(canvasWidth) - sz.W) / 2, (float64(canvasHeight) - sz.H) / 2
	}
}

// SizeNow returns the current (interpolated) size.
func (l *Layout) SizeNow() Size {
	return Size{W: l.Width.Value(), H: l.Height.Value()}
}

// AtRest reports whether all springs are at rest.
func (l *Layout) AtRest() bool {
	return l.Width.AtRest() && l.Height.AtRest() && l.Radius.AtRest() && l.Scale.AtRest() && l.Glow.AtRest()
}

// Step advances all springs by dt; returns true if still animating.
func (l *Layout) Step(dt float64) bool {
	a := l.Width.Step(dt)
	b := l.Height.Step(dt)
	c := l.Radius.Step(dt)
	d := l.Scale.Step(dt)
	e := l.Glow.Step(dt)
	return a || b || c || d || e
}
