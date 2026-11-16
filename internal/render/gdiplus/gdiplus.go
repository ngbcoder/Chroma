package gdiplus

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/ngbcoder/Chroma/internal/render"
)

func float32ToUintptr(f float32) uintptr {
	return uintptr(math.Float32bits(f))
}

var (
	gdiplusDll = syscall.NewLazyDLL("gdiplus.dll")
	gdi32     = syscall.NewLazyDLL("gdi32.dll")

	procGdiplusStartup               = gdiplusDll.NewProc("GdiplusStartup")
	procGdiplusShutdown              = gdiplusDll.NewProc("GdiplusShutdown")
	procGdipCreateBitmapFromScan0    = gdiplusDll.NewProc("GdipCreateBitmapFromScan0")
	procGdipGetImageGraphicsContext  = gdiplusDll.NewProc("GdipGetImageGraphicsContext")
	procGdipDeleteGraphics           = gdiplusDll.NewProc("GdipDeleteGraphics")
	procGdipGraphicsClear            = gdiplusDll.NewProc("GdipGraphicsClear")
	procGdipSetSmoothingMode         = gdiplusDll.NewProc("GdipSetSmoothingMode")
	procGdipCreateSolidFill          = gdiplusDll.NewProc("GdipCreateSolidFill")
	procGdipDeleteBrush              = gdiplusDll.NewProc("GdipDeleteBrush")
	procGdipCreatePath               = gdiplusDll.NewProc("GdipCreatePath")
	procGdipDeletePath               = gdiplusDll.NewProc("GdipDeletePath")
	procGdipAddPathArc               = gdiplusDll.NewProc("GdipAddPathArc")
	procGdipClosePathFigure          = gdiplusDll.NewProc("GdipClosePathFigure")
	procGdipFillPath                 = gdiplusDll.NewProc("GdipFillPath")
	procGdipDrawString               = gdiplusDll.NewProc("GdipDrawString")
	procGdipMeasureString            = gdiplusDll.NewProc("GdipMeasureString")
	procGdipDrawImageRect            = gdiplusDll.NewProc("GdipDrawImageRectI")
	procGdipCreateFontFamilyFromName = gdiplusDll.NewProc("GdipCreateFontFamilyFromName")
	procGdipDeleteFontFamily         = gdiplusDll.NewProc("GdipDeleteFontFamily")
	procGdipCreateFont               = gdiplusDll.NewProc("GdipCreateFont")
	procGdipDeleteFont               = gdiplusDll.NewProc("GdipDeleteFont")
	procGdipCreateStringFormat       = gdiplusDll.NewProc("GdipCreateStringFormat")
	procGdipDeleteStringFormat       = gdiplusDll.NewProc("GdipDeleteStringFormat")
	procGdipSetStringFormatFlags     = gdiplusDll.NewProc("GdipSetStringFormatFlags")
	procGdipSetStringFormatTrimming  = gdiplusDll.NewProc("GdipSetStringFormatTrimming")
	procGdipDisposeImage             = gdiplusDll.NewProc("GdipDisposeImage")
	procGdipBitmapLockBits           = gdiplusDll.NewProc("GdipBitmapLockBits")
	procGdipBitmapUnlockBits         = gdiplusDll.NewProc("GdipBitmapUnlockBits")

	procCreatePrivateFontCollection  = gdiplusDll.NewProc("GdipNewPrivateFontCollection")
	procDeletePrivateFontCollection  = gdiplusDll.NewProc("GdipDeletePrivateFontCollection")
	procPrivateAddFontFile           = gdiplusDll.NewProc("GdipPrivateAddFontFile")

	procAddFontResourceExW           = gdi32.NewProc("AddFontResourceExW")
)

var initialized bool
var fontCollection uintptr

type gdiplusStartupInput struct {
	GdiplusVersion           uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCode     int32
}

func InitGDIPlus() error {
	if initialized {
		return nil
	}
	input := gdiplusStartupInput{GdiplusVersion: 1}
	var token uintptr
	r, _, _ := procGdiplusStartup.Call(uintptr(unsafe.Pointer(&token)), uintptr(unsafe.Pointer(&input)), 0)
	if r != 0 {
		return fmt.Errorf("GdiplusStartup failed: 0x%x", r)
	}

	procCreatePrivateFontCollection.Call(uintptr(unsafe.Pointer(&fontCollection)))
	registerFonts()

	initialized = true
	return nil
}

func registerFonts() {
	searchPaths := []string{}
	if wd, err := os.Getwd(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(wd, "assets", "fonts"))
	}
	if exePath, err := os.Executable(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(filepath.Dir(exePath), "assets", "fonts"))
	}
	searchPaths = append(searchPaths, filepath.Join(".", "assets", "fonts"))

	for _, f := range []string{"Doto-Regular.ttf", "BBH-Bogle-Regular.ttf"} {
		for _, sp := range searchPaths {
			p := filepath.Join(sp, f)
			if _, err := os.Stat(p); err == nil {
				u, _ := syscall.UTF16PtrFromString(p)
				procAddFontResourceExW.Call(uintptr(unsafe.Pointer(u)), 0x10, 0)
				if fontCollection != 0 {
					procPrivateAddFontFile.Call(fontCollection, uintptr(unsafe.Pointer(u)))
				}
				break
			}
		}
	}
}

type Backend struct {
	W, H     int
	bitmap   uintptr
	graphics uintptr
}

func New() (*Backend, error) {
	if err := InitGDIPlus(); err != nil {
		return nil, err
	}
	return &Backend{}, nil
}

func (b *Backend) Begin(w, h int) {
	b.W = w
	b.H = h
	if b.graphics != 0 {
		procGdipDeleteGraphics.Call(b.graphics)
	}
	if b.bitmap != 0 {
		procGdipDisposeImage.Call(b.bitmap)
	}
	var bmp uintptr
	procGdipCreateBitmapFromScan0.Call(uintptr(w), uintptr(h), uintptr(w*4), 0x0026200a, 0, uintptr(unsafe.Pointer(&bmp)))
	b.bitmap = bmp
	var g uintptr
	procGdipGetImageGraphicsContext.Call(bmp, uintptr(unsafe.Pointer(&g)))
	b.graphics = g
	procGdipSetSmoothingMode.Call(g, 4)
}

func (b *Backend) Clear() {
	procGdipGraphicsClear.Call(b.graphics, 0)
}

func (b *Backend) FillRoundedRect(x, y, w, h, rad float64, c render.Color) {
	var brush uintptr
	argb := uint32(c.A)<<24 | uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
	procGdipCreateSolidFill.Call(uintptr(argb), uintptr(unsafe.Pointer(&brush)))
	defer procGdipDeleteBrush.Call(brush)

	var path uintptr
	procGdipCreatePath.Call(0, uintptr(unsafe.Pointer(&path)))
	defer procGdipDeletePath.Call(path)

	d := float32(rad * 2)
	fx := float32(x)
	fy := float32(y)
	fw := float32(w)
	fh := float32(h)

	procGdipAddPathArc.Call(path, float32ToUintptr(fx), float32ToUintptr(fy), float32ToUintptr(d), float32ToUintptr(d), float32ToUintptr(180), float32ToUintptr(90))
	procGdipAddPathArc.Call(path, float32ToUintptr(fx+fw-d), float32ToUintptr(fy), float32ToUintptr(d), float32ToUintptr(d), float32ToUintptr(270), float32ToUintptr(90))
	procGdipAddPathArc.Call(path, float32ToUintptr(fx+fw-d), float32ToUintptr(fy+fh-d), float32ToUintptr(d), float32ToUintptr(d), float32ToUintptr(0), float32ToUintptr(90))
	procGdipAddPathArc.Call(path, float32ToUintptr(fx), float32ToUintptr(fy+fh-d), float32ToUintptr(d), float32ToUintptr(d), float32ToUintptr(90), float32ToUintptr(90))

	procGdipClosePathFigure.Call(path)
	procGdipFillPath.Call(b.graphics, brush, path)
}

func (b *Backend) DrawImage(img image.Image, x, y float64) {}

func (b *Backend) DrawText(text string, x, y float64, s render.TextStyle) {
	u, _ := syscall.UTF16PtrFromString(text)

	candidates := []string{"Doto", "BBH Bogle", "Segoe UI", "Arial"}
	if s.Bold {
		candidates = []string{"BBH Bogle", "Doto", "Segoe UI", "Arial"}
	}

	var family uintptr
	for _, name := range candidates {
		famName, _ := syscall.UTF16PtrFromString(name)
		if fontCollection != 0 {
			r, _, _ := procGdipCreateFontFamilyFromName.Call(uintptr(unsafe.Pointer(famName)), fontCollection, uintptr(unsafe.Pointer(&family)))
			if r == 0 && family != 0 {
				break
			}
		}
		r, _, _ := procGdipCreateFontFamilyFromName.Call(uintptr(unsafe.Pointer(famName)), 0, uintptr(unsafe.Pointer(&family)))
		if r == 0 && family != 0 {
			break
		}
	}
	if family == 0 {
		famName, _ := syscall.UTF16PtrFromString("Arial")
		procGdipCreateFontFamilyFromName.Call(uintptr(unsafe.Pointer(famName)), 0, uintptr(unsafe.Pointer(&family)))
	}
	if family == 0 {
		return
	}
	defer procGdipDeleteFontFamily.Call(family)

	size := float32(s.Size)
	if size == 0 {
		size = 12
	}
	style := 0
	if s.Bold {
		style = 1
	}
	unit := 3 // UnitPoint
	var font uintptr
	procGdipCreateFont.Call(family, float32ToUintptr(size), uintptr(style), uintptr(unit), uintptr(unsafe.Pointer(&font)))
	if font == 0 {
		return
	}
	defer procGdipDeleteFont.Call(font)

	var brush uintptr
	argb := uint32(s.Color.A)<<24 | uint32(s.Color.R)<<16 | uint32(s.Color.G)<<8 | uint32(s.Color.B)
	procGdipCreateSolidFill.Call(uintptr(argb), uintptr(unsafe.Pointer(&brush)))
	defer procGdipDeleteBrush.Call(brush)

	// If center alignment requested and width specified, measure text width and center x
	if s.Alignment == 1 && s.Width > 0 {
		var box struct{ X, Y, Width, Height float32 }
		layoutRect := struct{ X, Y, Width, Height float32 }{0, 0, 10000, 10000}
		procGdipMeasureString.Call(
			b.graphics,
			uintptr(unsafe.Pointer(u)),
			uintptr(len(text)),
			font,
			uintptr(unsafe.Pointer(&layoutRect)),
			0, // string format
			uintptr(unsafe.Pointer(&box)),
			0,
			0,
		)
		if box.Width > 0 {
			x = x + (s.Width-float64(box.Width))/2
		}
	}

	var format uintptr
	procGdipCreateStringFormat.Call(0, 0, uintptr(unsafe.Pointer(&format)))
	defer procGdipDeleteStringFormat.Call(format)
	procGdipSetStringFormatFlags.Call(format, 0x00001000) // StringFormatFlagsNoWrap
	procGdipSetStringFormatTrimming.Call(format, 3)        // StringTrimmingEllipsisCharacter

	w := s.Width
	if w <= 0 {
		w = float64(b.W) - x
	}
	layout := struct{ X, Y, Width, Height float32 }{
		X: float32(x), Y: float32(y), Width: float32(w), Height: float32(s.Size * 2.5),
	}
	procGdipDrawString.Call(b.graphics, uintptr(unsafe.Pointer(u)), uintptr(len(text)), font, uintptr(unsafe.Pointer(&layout)), format, brush)
}

func (b *Backend) Blur(x, y, w, h, radius float64) {}
func (b *Backend) Glow(x, y, w, h, radius float64, c render.Color) {
	b.FillRoundedRect(x-radius, y-radius, w+2*radius, h+2*radius, radius*2, c)
}

func (b *Backend) DrawBitmap(bmp uintptr, x, y, w, h float64) {
	if bmp == 0 {
		return
	}
	procGdipDrawImageRect.Call(
		b.graphics,
		bmp,
		uintptr(int(x)),
		uintptr(int(y)),
		uintptr(int(w)),
		uintptr(int(h)),
	)
}

func (b *Backend) Present(dst any) error {
	return nil
}

func (b *Backend) CopyTo(dest []byte) {
	if b.bitmap == 0 {
		return
	}
	type bitmapData struct {
		Width, Height int32
		Stride        int32
		PixelFormat   int32
		Scan0         uintptr
		Reserved      uintptr
	}
	var bd bitmapData
	rect := struct{ X, Y, Width, Height int32 }{0, 0, int32(b.W), int32(b.H)}
	procGdipBitmapLockBits.Call(b.bitmap, uintptr(unsafe.Pointer(&rect)), 1, 0x0026200a, uintptr(unsafe.Pointer(&bd)))
	if bd.Scan0 != 0 {
		src := unsafe.Slice((*byte)(unsafe.Pointer(bd.Scan0)), b.W*b.H*4)
		copy(dest, src)
	}
	procGdipBitmapUnlockBits.Call(b.bitmap, uintptr(unsafe.Pointer(&bd)))
}
