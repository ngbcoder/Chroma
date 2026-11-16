package media

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
)

var (
	shcore                 = syscall.NewLazyDLL("shcore.dll")
	procCreateStreamOverRA = shcore.NewProc("CreateStreamOverRandomAccessStream")
	gdiplusDll             = syscall.NewLazyDLL("gdiplus.dll")
	procGdipCreateBmpFromStm = gdiplusDll.NewProc("GdipCreateBitmapFromStream")
	procGdipDisposeImg     = gdiplusDll.NewProc("GdipDisposeImage")
)

func roInit() {
	_ = ole.RoInitialize(1)
}

func slot(ins *ole.IInspectable, n int) uintptr {
	return *(*uintptr)(unsafe.Pointer(uintptr(unsafe.Pointer(ins.RawVTable)) + uintptr(n)*unsafe.Sizeof(uintptr(0))))
}

func syscallN(slotv uintptr, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(slotv, args...)
	return r
}

func getHString(ins *ole.IInspectable, idx int) (string, error) {
	var hstr ole.HString
	hr := syscallN(slot(ins, idx), uintptr(unsafe.Pointer(ins)), uintptr(unsafe.Pointer(&hstr)))
	if int32(hr) < 0 {
		return "", fmt.Errorf("slot %d hr=%d", idx, hr)
	}
	defer ole.DeleteHString(hstr)
	return hstr.String(), nil
}

func FetchThumbnail(props *ole.IInspectable) uintptr {
	if props == nil {
		return 0
	}
	var thumbRef *ole.IInspectable
	// props slot 15 = get_Thumbnail
	hr := syscallN(slot(props, 15), uintptr(unsafe.Pointer(props)), uintptr(unsafe.Pointer(&thumbRef)))
	if int32(hr) < 0 || thumbRef == nil {
		return 0
	}
	defer thumbRef.Release()

	// thumbRef slot 6 = OpenReadAsync -> IAsyncOperation<IRandomAccessStreamWithContentType>
	var asyncOp *ole.IInspectable
	hr = syscallN(slot(thumbRef, 6), uintptr(unsafe.Pointer(thumbRef)), uintptr(unsafe.Pointer(&asyncOp)))
	if int32(hr) < 0 || asyncOp == nil {
		return 0
	}
	defer asyncOp.Release()

	// GetResults slot 8
	var raStream *ole.IInspectable
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		hr = syscallN(slot(asyncOp, 8), uintptr(unsafe.Pointer(asyncOp)), uintptr(unsafe.Pointer(&raStream)))
		if int32(hr) >= 0 {
			break
		}
		if hr != 0x8000000E {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if raStream == nil {
		return 0
	}
	defer raStream.Release()

	iidIStream := ole.NewGUID("{0000000c-0000-0000-c000-000000000046}")
	var iStream *ole.IUnknown
	hRes, _, _ := procCreateStreamOverRA.Call(
		uintptr(unsafe.Pointer(raStream)),
		uintptr(unsafe.Pointer(iidIStream)),
		uintptr(unsafe.Pointer(&iStream)),
	)
	if hRes != 0 || iStream == nil {
		return 0
	}
	defer iStream.Release()

	var bitmap uintptr
	hRes, _, _ = procGdipCreateBmpFromStm.Call(
		uintptr(unsafe.Pointer(iStream)),
		uintptr(unsafe.Pointer(&bitmap)),
	)
	if hRes != 0 || bitmap == 0 {
		return 0
	}

	return bitmap
}

func DisposeThumbnail(bmp uintptr) {
	if bmp != 0 {
		procGdipDisposeImg.Call(bmp)
	}
}

func FetchCurrent() (SessionInfo, error) {
	roInit()

	ins, err := ole.RoGetActivationFactory(
		"Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager",
		ole.NewGUID("{2050c4ee-11a0-57de-aed7-c97c70338245}"),
	)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("RoGetActivationFactory: %w", err)
	}
	defer ins.Release()

	var asyncOp *ole.IInspectable
	hr := syscallN(slot(ins, 6), uintptr(unsafe.Pointer(ins)), uintptr(unsafe.Pointer(&asyncOp)))
	if int32(hr) < 0 || asyncOp == nil {
		return SessionInfo{}, fmt.Errorf("RequestAsync hr=%d", hr)
	}
	defer asyncOp.Release()

	var mgr *ole.IInspectable
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		hr = syscallN(slot(asyncOp, 8), uintptr(unsafe.Pointer(asyncOp)), uintptr(unsafe.Pointer(&mgr)))
		if int32(hr) >= 0 {
			break
		}
		if hr != 0x8000000E {
			return SessionInfo{}, fmt.Errorf("GetResults(manager) hr=%d", hr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if mgr == nil {
		return SessionInfo{}, fmt.Errorf("GetResults(manager) timeout/nil")
	}
	defer mgr.Release()

	var session *ole.IInspectable
	hr = syscallN(slot(mgr, 6), uintptr(unsafe.Pointer(mgr)), uintptr(unsafe.Pointer(&session)))
	if int32(hr) < 0 {
		return SessionInfo{}, fmt.Errorf("GetCurrentSession hr=%d", hr)
	}
	if session == nil {
		return SessionInfo{}, fmt.Errorf("no current session")
	}
	defer session.Release()

	info := SessionInfo{}
	info.SourceApp, _ = getHString(session, 6)

	var asyncOp2 *ole.IInspectable
	hr = syscallN(slot(session, 7), uintptr(unsafe.Pointer(session)), uintptr(unsafe.Pointer(&asyncOp2)))
	if int32(hr) >= 0 && asyncOp2 != nil {
		defer asyncOp2.Release()
		var props *ole.IInspectable
		deadline2 := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline2) {
			hr2 := syscallN(slot(asyncOp2, 8), uintptr(unsafe.Pointer(asyncOp2)), uintptr(unsafe.Pointer(&props)))
			if int32(hr2) >= 0 {
				break
			}
			if hr2 != 0x8000000E {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if props != nil {
			defer props.Release()
			info.Title, _ = getHString(props, 6)
			info.Artist, _ = getHString(props, 9)
			info.Album, _ = getHString(props, 10)
			info.Thumbnail = FetchThumbnail(props)
		}
	}

	var timeline *ole.IInspectable
	hr = syscallN(slot(session, 8), uintptr(unsafe.Pointer(session)), uintptr(unsafe.Pointer(&timeline)))
	if int32(hr) >= 0 && timeline != nil {
		defer timeline.Release()
		var end, pos int64
		syscallN(slot(timeline, 7), uintptr(unsafe.Pointer(timeline)), uintptr(unsafe.Pointer(&end)))
		syscallN(slot(timeline, 10), uintptr(unsafe.Pointer(timeline)), uintptr(unsafe.Pointer(&pos)))
		info.Duration = time.Duration(end) * 100
		info.Position = time.Duration(pos) * 100
	}

	var playback *ole.IInspectable
	hr = syscallN(slot(session, 9), uintptr(unsafe.Pointer(session)), uintptr(unsafe.Pointer(&playback)))
	if int32(hr) >= 0 && playback != nil {
		defer playback.Release()
		var status int32
		syscallN(slot(playback, 7), uintptr(unsafe.Pointer(playback)), uintptr(unsafe.Pointer(&status)))
		info.Status = status
	}

	return info, nil
}

type SessionInfo struct {
	SourceApp string
	Title     string
	Artist    string
	Album     string
	Status    int32
	Position  time.Duration
	Duration  time.Duration
	Thumbnail uintptr
}

func FetchCurrentTitle() (string, error) {
	info, err := FetchCurrent()
	if err != nil {
		return "", err
	}
	return info.Title, nil
}

func control(session *ole.IInspectable, slotIdx int) {
	if session == nil {
		return
	}
	var op *ole.IInspectable
	hr := syscallN(slot(session, slotIdx), uintptr(unsafe.Pointer(session)), uintptr(unsafe.Pointer(&op)))
	if int32(hr) >= 0 && op != nil {
		op.Release()
	}
}

func Play(session *ole.IInspectable)  { control(session, 10) }
func Pause(session *ole.IInspectable) { control(session, 11) }
func Next(session *ole.IInspectable)  { control(session, 16) }
func Prev(session *ole.IInspectable)  { control(session, 17) }
func Toggle(session *ole.IInspectable) { control(session, 20) }

func FetchSession() (*ole.IInspectable, error) {
	roInit()
	ins, err := ole.RoGetActivationFactory(
		"Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager",
		ole.NewGUID("{2050c4ee-11a0-57de-aed7-c97c70338245}"),
	)
	if err != nil {
		return nil, err
	}
	defer ins.Release()
	var asyncOp *ole.IInspectable
	hr := syscallN(slot(ins, 6), uintptr(unsafe.Pointer(ins)), uintptr(unsafe.Pointer(&asyncOp)))
	if int32(hr) < 0 || asyncOp == nil {
		return nil, fmt.Errorf("RequestAsync hr=%d", hr)
	}
	defer asyncOp.Release()
	var mgr *ole.IInspectable
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		hr = syscallN(slot(asyncOp, 8), uintptr(unsafe.Pointer(asyncOp)), uintptr(unsafe.Pointer(&mgr)))
		if int32(hr) >= 0 {
			break
		}
		if hr != 0x8000000E {
			return nil, fmt.Errorf("GetResults hr=%d", hr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if mgr == nil {
		return nil, fmt.Errorf("no manager")
	}
	defer mgr.Release()
	var session *ole.IInspectable
	hr = syscallN(slot(mgr, 6), uintptr(unsafe.Pointer(mgr)), uintptr(unsafe.Pointer(&session)))
	if int32(hr) < 0 || session == nil {
		return nil, fmt.Errorf("GetCurrentSession hr=%d", hr)
	}
	return session, nil
}
