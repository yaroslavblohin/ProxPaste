//go:build windows && (amd64 || arm64)

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"unsafe"
)

// Render only the test-owned hidden window, never the user's desktop.
func captureWindow(window uintptr, path string) error {
	// Standard EDIT controls need a visible HWND to paint their content. Show
	// the test-owned window outside the desktop without activating it.
	position := -30000
	call("SetWindowPos", window, 0, uintptr(position), uintptr(position), 0, 0, 0x15)
	call("ShowWindow", window, 4)
	defer call("ShowWindow", window, 0)
	call("RedrawWindow", window, 0, 0, 0x585)
	var rect winRect
	call("GetWindowRect", window, uintptr(unsafe.Pointer(&rect)))
	w, h := int(rect.right-rect.left), int(rect.bottom-rect.top)
	dc := call("GetDC", window)
	defer call("ReleaseDC", window, dc)
	memory := gcall("CreateCompatibleDC", dc)
	defer gcall("DeleteDC", memory)
	bitmap := gcall("CreateCompatibleBitmap", dc, uintptr(w), uintptr(h))
	defer gcall("DeleteObject", bitmap)
	old := gcall("SelectObject", memory, bitmap)
	call("SendMessageW", window, 0x317, memory, 0x1E)
	gcall("SelectObject", memory, old)
	info := struct {
		Size                   uint32
		Width, Height          int32
		Planes, Bits           uint16
		Compression, ImageSize uint32
		X, Y                   int32
		Used, Important        uint32
	}{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}
	data := make([]byte, w*h*4)
	if gcall("GetDIBits", memory, bitmap, 0, uintptr(h), uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&info)), 0) == 0 {
		return fmt.Errorf("GetDIBits failed")
	}
	for i := 0; i < len(data); i += 4 {
		data[i], data[i+2] = data[i+2], data[i]
		data[i+3] = 255
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, &image.RGBA{Pix: data, Stride: w * 4, Rect: image.Rect(0, 0, w, h)})
}
