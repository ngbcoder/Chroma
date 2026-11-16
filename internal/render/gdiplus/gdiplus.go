package gdiplus

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/ngbcoder/Chroma/internal/render"
)

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

	procCreatePrivateFontCollection  = gdiplusDll.NewProc("GdiplusCreatePrivateFontCollection")
	procDeletePrivateFontCollection  = gdiplusDll.NewProc("GdiplusDeletePrivateFontCollection")
	procPrivateAddFontFile           = gdiplusDll.NewProc("GdiplusPrivateAddFontFile")

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
	exePath, err := os.Executable()
	if err != nil {
		exePath = "."
	}
	dir := filepath.Dir(exePath)
	for _, f := range []string{"Doto-Regular.ttf", "BBH-Bogle-Regular.ttf"} {
		p := filepath.Join(dir, "assets", "fonts", f)
		if _, err := os.Stat(p); err == nil {
			u, _ := syscall.UTF16PtrFromString(p)
			procAddFontResourceExW.Call(uintptr(unsafe.Pointer(u)), 0x10, 0)
			if fontCollection != 0 {
				procPrivateAddFontFile.Call(fontCollection, uintptr(unsafe.Pointer(u)))
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
	procGdipCreateBitmapFromScan0.Call(uintptr(w), uintptr(h), uintptr(w*4), 0x00262006, 0, uintptr(unsafe.Pointer(&bmp)))
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

	procGdipAddPathArc.Call(path, fx, fy, d, d, 180, 90)
	procGdipAddPathArc.Call(path, fx+fw-d, fy, d, d, 270, 90)
	procGdipAddPathArc.Call(path, fx+fw-d, fy+fh-d, d, d, 0, 90)
	procGdipAddPathArc.Call(path, fx, fy+fh-d, d, d, 90, 90)

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
		r, _, _ := procGdipCreateFontFamilyFromName.Call(uintptr(unsafe.Pointer(famName)), fontCollection, uintptr(unsafe.Pointer(&family)))
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
	var font uintptr
	procGdipCreateFont.Call(family, uintptr(unsafe.Pointer(&size)), 0, 0, uintptr(unsafe.Pointer(&font)))
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
			0, 0,
		)
		if box.Width > 0 {
			x = x + (s.Width-float64(box.Width))/2
		}
	}

	var format uintptr
	procGdipCreateStringFormat.Call(0, 0, uintptr(unsafe.Pointer(&format)))
	defer procGdipDeleteStringFormat.Call(format)
	procGdipSetStringFormatFlags.Call(format, 0x00001000)
	procGdipSetStringFormatTrimming.Call(format, 1)

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
	procGdipBitmapLockBits.Call(b.bitmap, uintptr(unsafe.Pointer(&rect)), 1, 0x00262006, uintptr(unsafe.Pointer(&bd)))
	if bd.Scan0 != 0 {
		src := unsafe.Slice((*byte)(unsafe.Pointer(bd.Scan0)), b.W*b.H*4)
		copy(dest, src)
	}
	procGdipBitmapUnlockBits.Call(b.bitmap, uintptr(unsafe.Pointer(&bd)))
}
