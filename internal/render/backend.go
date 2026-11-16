package render

import "image"

// Color is a premul? stored as straight RGBA 0-255 for API; backends handle premul.
type Color struct {
	R, G, B, A uint8
}

// TextStyle configures text rendering.
type TextStyle struct {
	Size      float64
	Color     Color
	Bold      bool
	Alignment int // 0 = Left, 1 = Center, 2 = Right
	Width     float64
}

// Backend is the rendering abstraction. All coordinates are in device-independent
// (logical) pixels; backends apply DPI scaling internally if needed.
type Backend interface {
	Begin(w, h int)
	Clear()
	FillRoundedRect(x, y, w, h, r float64, c Color)
	DrawImage(img image.Image, x, y float64)
	DrawText(text string, x, y float64, s TextStyle)
	// Blur/Glow are optional; no-op is acceptable.
	Blur(x, y, w, h, radius float64)
	Glow(x, y, w, h, radius float64, c Color)
	Present(dst any) error
}

// Color helpers
func RGB(r, g, b uint8) Color { return Color{R: r, G: g, B: b, A: 255} }
func RGBA(r, g, b, a uint8) Color {
	return Color{R: r, G: g, B: b, A: a}
}
