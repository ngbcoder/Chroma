package media

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
)

var (
	shcore                   = syscall.NewLazyDLL("shcore.dll")
	procCreateStreamOverRA   = shcore.NewProc("CreateStreamOverRandomAccessStream")
	gdiplusDll               = syscall.NewLazyDLL("gdiplus.dll")
	procGdipCreateBmpFromStm = gdiplusDll.NewProc("GdipCreateBitmapFromStream")
	procGdipDisposeImg       = gdiplusDll.NewProc("GdipDisposeImage")
)

const winFiletimeToUnixTicks = 116444736000000000

func winrtDateTimeToTime(ticks int64) time.Time {
	if ticks <= winFiletimeToUnixTicks {
		return time.Time{}
	}
	nsec := (ticks - winFiletimeToUnixTicks) * 100
	return time.Unix(0, nsec).UTC()
}

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

var (
	selectedSessionIndex atomic.Int32
	artworkCacheMu       sync.Mutex
	artworkCacheKey      string
	artworkCacheBitmap   uintptr
	retiredArtwork       = map[uintptr]*time.Timer{}
)

func cachedThumbnail(identity string, props *ole.IInspectable) uintptr {
	artworkCacheMu.Lock()
	defer artworkCacheMu.Unlock()
	if identity != "" && identity == artworkCacheKey && artworkCacheBitmap != 0 {
		return artworkCacheBitmap
	}
	bitmap := FetchThumbnail(props)
	old := artworkCacheBitmap
	artworkCacheKey, artworkCacheBitmap = identity, bitmap
	if bitmap == 0 {
		artworkCacheKey = ""
	}
	if old != 0 && old != bitmap {
		timer := time.AfterFunc(2*time.Second, func() {
			artworkCacheMu.Lock()
			_, stillRetired := retiredArtwork[old]
			delete(retiredArtwork, old)
			artworkCacheMu.Unlock()
			if stillRetired {
				DisposeThumbnail(old)
			}
		})
		retiredArtwork[old] = timer
	}
	return bitmap
}

// ClearThumbnailCache releases artwork resources after the UI has stopped drawing.
func ClearThumbnailCache() {
	artworkCacheMu.Lock()
	bitmaps := make([]uintptr, 0, len(retiredArtwork)+1)
	if artworkCacheBitmap != 0 {
		bitmaps = append(bitmaps, artworkCacheBitmap)
	}
	for bitmap, timer := range retiredArtwork {
		timer.Stop()
		bitmaps = append(bitmaps, bitmap)
	}
	artworkCacheKey, artworkCacheBitmap = "", 0
	retiredArtwork = map[uintptr]*time.Timer{}
	artworkCacheMu.Unlock()
	for _, bitmap := range bitmaps {
		DisposeThumbnail(bitmap)
	}
}

func fetchManager() (*ole.IInspectable, error) {
	roInit()
	factory, err := ole.RoGetActivationFactory(
		"Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager",
		ole.NewGUID("{2050c4ee-11a0-57de-aed7-c97c70338245}"),
	)
	if err != nil {
		return nil, fmt.Errorf("RoGetActivationFactory: %w", err)
	}
	defer factory.Release()

	var asyncOp *ole.IInspectable
	hr := syscallN(slot(factory, 6), uintptr(unsafe.Pointer(factory)), uintptr(unsafe.Pointer(&asyncOp)))
	if int32(hr) < 0 || asyncOp == nil {
		return nil, fmt.Errorf("RequestAsync hr=%d", hr)
	}
	defer asyncOp.Release()

	var manager *ole.IInspectable
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		hr = syscallN(slot(asyncOp, 8), uintptr(unsafe.Pointer(asyncOp)), uintptr(unsafe.Pointer(&manager)))
		if int32(hr) >= 0 {
			break
		}
		if hr != 0x8000000E {
			return nil, fmt.Errorf("GetResults(manager) hr=%d", hr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if manager == nil {
		return nil, fmt.Errorf("GetResults(manager) timeout/nil")
	}
	return manager, nil
}

func getSessionCollection(manager *ole.IInspectable) (*ole.IInspectable, int, error) {
	var sessions *ole.IInspectable
	hr := syscallN(slot(manager, 7), uintptr(unsafe.Pointer(manager)), uintptr(unsafe.Pointer(&sessions)))
	if int32(hr) < 0 || sessions == nil {
		return nil, 0, fmt.Errorf("GetSessions hr=%d", hr)
	}
	var count uint32
	hr = syscallN(slot(sessions, 7), uintptr(unsafe.Pointer(sessions)), uintptr(unsafe.Pointer(&count)))
	if int32(hr) < 0 {
		sessions.Release()
		return nil, 0, fmt.Errorf("GetSessions size hr=%d", hr)
	}
	return sessions, int(count), nil
}

func getSessionAt(sessions *ole.IInspectable, index int) (*ole.IInspectable, error) {
	var session *ole.IInspectable
	hr := syscallN(slot(sessions, 6), uintptr(unsafe.Pointer(sessions)), uintptr(index), uintptr(unsafe.Pointer(&session)))
	if int32(hr) < 0 || session == nil {
		return nil, fmt.Errorf("GetAt(%d) hr=%d", index, hr)
	}
	return session, nil
}

func getCurrentSession(manager *ole.IInspectable) (*ole.IInspectable, error) {
	var session *ole.IInspectable
	hr := syscallN(slot(manager, 6), uintptr(unsafe.Pointer(manager)), uintptr(unsafe.Pointer(&session)))
	if int32(hr) < 0 || session == nil {
		return nil, fmt.Errorf("GetCurrentSession hr=%d", hr)
	}
	return session, nil
}

func getSelectedSession(manager *ole.IInspectable) (*ole.IInspectable, int, int, error) {
	sessions, count, err := getSessionCollection(manager)
	if err != nil {
		session, currentErr := getCurrentSession(manager)
		return session, 0, 1, currentErr
	}
	defer sessions.Release()
	if count == 0 {
		return nil, 0, 0, fmt.Errorf("no active media session")
	}
	index := int(selectedSessionIndex.Load())
	if index < 0 || index >= count {
		index = 0
		selectedSessionIndex.Store(0)
	}
	session, err := getSessionAt(sessions, index)
	if err != nil {
		return nil, 0, count, err
	}
	return session, index, count, nil
}

// CycleSession advances the selected session in the system media session list.
func CycleSession() (int, int, error) {
	manager, err := fetchManager()
	if err != nil {
		return 0, 0, err
	}
	defer manager.Release()
	sessions, count, err := getSessionCollection(manager)
	if err != nil {
		return 0, 0, err
	}
	sessions.Release()
	if count < 2 {
		return 0, count, fmt.Errorf("only %d active media session(s)", count)
	}
	index := (int(selectedSessionIndex.Load()) + 1) % count
	selectedSessionIndex.Store(int32(index))
	return index, count, nil
}

func FetchCurrent() (SessionInfo, error) {
	mgr, err := fetchManager()
	if err != nil {
		return SessionInfo{}, err
	}
	defer mgr.Release()

	session, sessionIndex, sessionCount, err := getSelectedSession(mgr)
	if err != nil {
		return SessionInfo{}, err
	}
	defer session.Release()

	info := SessionInfo{SessionIndex: sessionIndex, SessionCount: sessionCount}
	info.SourceApp, _ = getHString(session, 6)

	var asyncOp2 *ole.IInspectable
	hr := syscallN(slot(session, 7), uintptr(unsafe.Pointer(session)), uintptr(unsafe.Pointer(&asyncOp2)))
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
			info.TrackIdentity = strings.Join([]string{info.SourceApp, info.Title, info.Artist, info.Album}, "\x00")
			info.Thumbnail = cachedThumbnail(info.TrackIdentity, props)
		}
	}

	var timeline *ole.IInspectable
	hr = syscallN(slot(session, 8), uintptr(unsafe.Pointer(session)), uintptr(unsafe.Pointer(&timeline)))
	if int32(hr) >= 0 && timeline != nil {
		defer timeline.Release()
		var end, pos, updated int64
		syscallN(slot(timeline, 7), uintptr(unsafe.Pointer(timeline)), uintptr(unsafe.Pointer(&end)))
		syscallN(slot(timeline, 10), uintptr(unsafe.Pointer(timeline)), uintptr(unsafe.Pointer(&pos)))
		syscallN(slot(timeline, 11), uintptr(unsafe.Pointer(timeline)), uintptr(unsafe.Pointer(&updated)))
		info.Duration = time.Duration(end) * 100
		info.Position = time.Duration(pos) * 100
		uTime := winrtDateTimeToTime(updated)
		if uTime.IsZero() {
			info.PositionUpdatedAt = time.Now()
		} else {
			info.PositionUpdatedAt = uTime
		}
	} else {
		info.PositionUpdatedAt = time.Now()
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
	SourceApp         string
	TrackIdentity     string
	Title             string
	Artist            string
	Album             string
	Status            int32
	Position          time.Duration
	Duration          time.Duration
	PositionUpdatedAt time.Time
	Thumbnail         uintptr
	SessionIndex      int
	SessionCount      int
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

func Play(session *ole.IInspectable)   { control(session, 10) }
func Pause(session *ole.IInspectable)  { control(session, 11) }
func Next(session *ole.IInspectable)   { control(session, 16) }
func Prev(session *ole.IInspectable)   { control(session, 17) }
func Toggle(session *ole.IInspectable) { control(session, 20) }

func FetchSession() (*ole.IInspectable, error) {
	mgr, err := fetchManager()
	if err != nil {
		return nil, err
	}
	defer mgr.Release()
	session, _, _, err := getSelectedSession(mgr)
	return session, err
}
