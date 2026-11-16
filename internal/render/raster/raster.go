package raster

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/ngbcoder/Chroma/internal/render"
)

// Raster is a pure-Go CPU backend drawing into a premul BGRA buffer.
type Raster struct {
	W, H int
	Buf  []byte // premul BGRA, top-down, len = W*H*4
}

func New() *Raster { return &Raster{} }

func (r *Raster) Begin(w, h int) {
	r.W = w
	r.H = h
	r.Buf = make([]byte, w*h*4)
}

func (r *Raster) Clear() {
	for i := range r.Buf {
		r.Buf[i] = 0
	}
}

func (r *Raster) idx(x, y int) int { return (y*r.W + x) * 4 }

func blendPixel(buf []byte, i int, src render.Color) {
	// src is straight alpha; convert to premul and alpha-blend over dst.
	sa := float64(src.A) / 255.0
	sr := float64(src.R) * sa
	sg := float64(src.G) * sa
	sb := float64(src.B) * sa

	da := float64(buf[i+3]) / 255.0
	dr := float64(buf[i+2])
	dg := float64(buf[i+1])
	db := float64(buf[i+0])

	// blend (src over dst)
	oa := sa + da*(1-sa)
	var or, og, ob float64
	if oa > 0 {
		or = (sr + dr*(1-sa)) 
		og = (sg + dg*(1-sa))
		ob = (sb + db*(1-sa))
	}
	buf[i+0] = uint8(math.Round(ob))
	buf[i+1] = uint8(math.Round(og))
	buf[i+2] = uint8(math.Round(or))
	buf[i+3] = uint8(math.Round(oa * 255))
}

func (r *Raster) FillRoundedRect(x, y, w, h, rad float64, c render.Color) {
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := int(math.Ceil(x + w))
	y1 := int(math.Ceil(y + h))
	for py := y0; py < y1 && py < r.H; py++ {
		if py < 0 {
			continue
		}
		for px := x0; px < x1 && px < r.W; px++ {
			if px < 0 {
				continue
			}
			// distance to rounded rect
			// clamp point to inner rect
			cx := math.Max(x+rad, math.Min(float64(px)+0.5, x+w-rad))
			cy := math.Max(y+rad, math.Min(float64(py)+0.5, y+h-rad))
			dx := float64(px)+0.5 - cx
			dy := float64(py)+0.5 - cy
			dist := math.Hypot(dx, dy)
			if dist <= rad+0.5 {
				// anti-alias edge
				alpha := 1.0
				if dist > rad {
					alpha = rad + 0.5 - dist
					if alpha < 0 {
						alpha = 0
					}
				}
				cc := c
				cc.A = uint8(math.Round(float64(c.A) * alpha))
				if cc.A > 0 {
					blendPixel(r.Buf, r.idx(px, py), cc)
				}
			}
		}
	}
}

func (r *Raster) DrawImage(img image.Image, x, y float64) {
	b := img.Bounds()
	for iy := b.Min.Y; iy < b.Max.Y; iy++ {
		for ix := b.Min.X; ix < b.Max.X; ix++ {
			px := int(x) + (ix - b.Min.X)
			py := int(y) + (iy - b.Min.Y)
			if px < 0 || py < 0 || px >= r.W || py >= r.H {
				continue
			}
			cr, cg, cb, ca := img.At(ix, iy).RGBA()
			col := render.Color{
				R: uint8(cr >> 8),
				G: uint8(cg >> 8),
				B: uint8(cb >> 8),
				A: uint8(ca >> 8),
			}
			blendPixel(r.Buf, r.idx(px, py), col)
		}
	}
}

func (r *Raster) DrawText(text string, x, y float64, s render.TextStyle) {
	face := basicfont.Face7x13
	d := font.Drawer{
		Dst:  &imgWrapper{r: r},
		Src:  image.NewUniform(color.RGBA{R: s.Color.R, G: s.Color.G, B: s.Color.B, A: s.Color.A}),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(int(x)), Y: fixed.I(int(y))},
	}
	d.DrawString(text)
}

// imgWrapper adapts Raster buffer to image.Image/draw.Image for font.Drawer.
type imgWrapper struct {
	r *Raster
}

func (w *imgWrapper) ColorModel() color.Model { return color.RGBAModel }
func (w *imgWrapper) Bounds() image.Rectangle {
	return image.Rect(0, 0, w.r.W, w.r.H)
}
func (w *imgWrapper) At(x, y int) color.Color {
	if x < 0 || y < 0 || x >= w.r.W || y >= w.r.H {
		return color.RGBA{}
	}
	i := w.r.idx(x, y)
	return color.RGBA{B: w.r.Buf[i+0], G: w.r.Buf[i+1], R: w.r.Buf[i+2], A: w.r.Buf[i+3]}
}
func (w *imgWrapper) Set(x, y int, c color.Color) {
	if x < 0 || y < 0 || x >= w.r.W || y >= w.r.H {
		return
	}
	cr, cg, cb, ca := c.RGBA()
	// font drawer gives alpha; blend as src-over with coverage already in alpha
	col := render.Color{R: uint8(cr >> 8), G: uint8(cg >> 8), B: uint8(cb >> 8), A: uint8(ca >> 8)}
	blendPixel(w.r.Buf, w.r.idx(x, y), col)
}

func (r *Raster) Blur(x, y, w, h, radius float64) { /* no-op for v1 */ }
func (r *Raster) Glow(x, y, w, h, radius float64, c render.Color) {
	// simple: expand rect and fade
	r.FillRoundedRect(x-radius, y-radius, w+2*radius, h+2*radius, radius*2, c)
}

func (r *Raster) Present(dst any) error { return nil }

// Expose buffer as image for tests
func (r *Raster) Image() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.W, r.H))
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			i := r.idx(x, y)
			img.SetRGBA(x, y, color.RGBA{R: r.Buf[i+2], G: r.Buf[i+1], B: r.Buf[i+0], A: r.Buf[i+3]})
		}
	}
	return img
}
