package main

import (
	"fmt"
	"runtime"

	"github.com/ngbcoder/Chroma/internal/media"
)

func main() {
	runtime.LockOSThread()
	info, err := media.FetchCurrent()
	if err != nil {
		fmt.Println("ERR:", err)
		return
	}
	fmt.Printf("App=%s\nTitle=%s\nArtist=%s\nAlbum=%s\nStatus=%d\nPos=%v\nDur=%v\n",
		info.SourceApp, info.Title, info.Artist, info.Album, info.Status, info.Position, info.Duration)
}
