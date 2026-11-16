package main

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/ngbcoder/Chroma/internal/ui"
)

func main() {
	w, err := ui.NewWindow(640, 260)
	if err != nil {
		fmt.Println("NewWindow:", err)
		return
	}
	defer w.Destroy()

	// Fill the DIB with a translucent rounded-ish block for the spike.
	bits := w.Bits()
	if bits != 0 {
		n := w.Width * w.Height
		for i := 0; i < n; i++ {
			// premul BGRA: light blue, alpha ~200
			p := (*[4]byte)(unsafe.Pointer(bits + uintptr(i*4)))
			p[0] = 200 // B
			p[1] = 120 // G
			p[2] = 60  // R
			p[3] = 200 // A
		}
	}
	if err := w.Present(200, 100); err != nil {
		fmt.Println("Present:", err)
	}

	// Auto-exit after 5s for the spike.
	time.AfterFunc(5*time.Second, func() {
		// post WM_QUIT
	})
	go func() {
		time.Sleep(5 * time.Second)
		// trigger quit via PostQuitMessage through a no-op; use Destroy + os.Exit
		w.Destroy()
		// break pump by posting quit message via window? Instead just exit process
		// We'll call runtime? Simpler: os.Exit
		// But deferred Destroy already; just exit
		// Actually os.Exit skips defer; fine for spike
		// Use windows? We'll just return from main via channel
	}()

	ui.RunMessagePump()
	fmt.Println("done")
}
