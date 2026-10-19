//go:build windows && (amd64 || arm64)

package main

import (
	"syscall"
	"unsafe"
)

func rgb(r, g, b uintptr) uintptr { return r | g<<8 | b<<16 }

var colorBG = rgb(15, 23, 33)
var colorSurface = rgb(23, 34, 48)
var colorBorder = rgb(45, 61, 78)
var colorInk = rgb(230, 238, 247)
var colorMuted = rgb(155, 173, 192)
var colorAccent = rgb(14, 139, 117)
var colorEditor = rgb(10, 17, 26)
var bgBrush, surfaceBrush, editorBrush, appIcon uintptr

type fontStyle struct {
	size, weight int
	face         string
}
type labelStyle struct {
	font                   fontStyle
	foreground, background uintptr
}

var styles = map[uintptr]labelStyle{}
var fonts = map[fontStyle]uintptr{}

//go:uintptrescapes
func gcall(name string, args ...uintptr) uintptr {
	r, _, _ := gdi.NewProc(name).Call(args...)
	return r
}
func initializeTheme() {
	bgBrush = gcall("CreateSolidBrush", colorBG)
	surfaceBrush = gcall("CreateSolidBrush", colorSurface)
	editorBrush = gcall("CreateSolidBrush", colorEditor)
	appIcon = call("CreateIconFromResourceEx", uintptr(unsafe.Pointer(&iconPNG[0])), uintptr(len(iconPNG)), 1, 0x30000, 256, 256, 0)
	if appIcon == 0 {
		appIcon = call("LoadIconW", 0, 32512)
	}
}
func destroyTheme() {
	for _, font := range fonts {
		gcall("DeleteObject", font)
	}
	for _, brush := range []uintptr{bgBrush, surfaceBrush, editorBrush} {
		if brush != 0 {
			gcall("DeleteObject", brush)
		}
	}
	if appIcon != 0 {
		call("DestroyIcon", appIcon)
	}
}
func getFont(style fontStyle) uintptr {
	if font := fonts[style]; font != 0 {
		return font
	}
	height := -scale(style.size, currentDPI)
	font := gcall("CreateFontW", uintptr(height), 0, 0, 0, uintptr(style.weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wide(style.face))))
	fonts[style] = font
	return font
}
func styleControl(h uintptr, size, weight int, face string, fg, bg uintptr) {
	styles[h] = labelStyle{fontStyle{size, weight, face}, fg, bg}
}
func label(text string, x, y, w, h, size int, strong bool, onCard bool) uintptr {
	handle := add("STATIC", text, 0, x, y, w, h, 0)
	weight := 400
	if strong {
		weight = 600
	}
	bg := colorBG
	if onCard {
		bg = colorSurface
	}
	fg := colorMuted
	if strong {
		fg = colorInk
	}
	styleControl(handle, size, weight, "Segoe UI", fg, bg)
	return handle
}
func button(text string, x, y, w, h int, id uintptr) uintptr {
	handle := add("BUTTON", text, 0x1000B, x, y, w, h, id)
	styleControl(handle, 15, 600, "Segoe UI", colorInk, colorBG)
	return handle
}
func paintRound(dc uintptr, rect winRect, radius int, fill, border uintptr) {
	brush := gcall("CreateSolidBrush", fill)
	pen := gcall("CreatePen", 0, 1, border)
	oldBrush := gcall("SelectObject", dc, brush)
	oldPen := gcall("SelectObject", dc, pen)
	gcall("RoundRect", dc, uintptr(rect.left), uintptr(rect.top), uintptr(rect.right), uintptr(rect.bottom), uintptr(radius), uintptr(radius))
	gcall("SelectObject", dc, oldBrush)
	gcall("SelectObject", dc, oldPen)
	gcall("DeleteObject", brush)
	gcall("DeleteObject", pen)
}

type paintStruct struct {
	dc              uintptr
	erase           int32
	rect            winRect
	restore, update int32
	reserved        [32]byte
}

func paintWindow(h uintptr) {
	var ps paintStruct
	dc := call("BeginPaint", h, uintptr(unsafe.Pointer(&ps)))
	defer call("EndPaint", h, uintptr(unsafe.Pointer(&ps)))
	paintWindowDC(h, dc)
}
func paintWindowDC(h, dc uintptr) {
	var rect winRect
	call("GetClientRect", h, uintptr(unsafe.Pointer(&rect)))
	call("FillRect", dc, uintptr(unsafe.Pointer(&rect)), bgBrush)
	for _, r := range [][4]int{{24, 108, 816, 392}, {24, 406, 324, 572}, {340, 406, 816, 572}} {
		paintRound(dc, winRect{int32(scale(r[0], currentDPI)), int32(scale(r[1], currentDPI)), int32(scale(r[2], currentDPI)), int32(scale(r[3], currentDPI))}, scale(20, currentDPI), colorSurface, colorBorder)
	}
	call("DrawIconEx", dc, uintptr(scale(26, currentDPI)), uintptr(scale(25, currentDPI)), appIcon, uintptr(scale(56, currentDPI)), uintptr(scale(56, currentDPI)), 0, 0, 3)
}
func colorControl(dc, handle uintptr) uintptr {
	style, ok := styles[handle]
	if !ok {
		style = labelStyle{foreground: colorInk, background: colorBG}
	}
	gcall("SetTextColor", dc, style.foreground)
	gcall("SetBkColor", dc, style.background)
	if style.background == colorEditor {
		return editorBrush
	}
	if style.background == colorSurface {
		return surfaceBrush
	}
	return bgBrush
}

type drawItem struct {
	kind, id, item, action, state uint32
	window, dc                    uintptr
	rect                          winRect
	data                          uintptr
}

func drawButton(pointer uintptr) {
	var item drawItem
	kernel.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&item)), pointer, unsafe.Sizeof(item))
	fill, border, fg := colorSurface, colorBorder, colorInk
	if item.id == 102 || item.id == 108 {
		fill = colorAccent
		border = colorAccent
		fg = rgb(255, 255, 255)
	}
	if item.state&1 != 0 {
		fill = rgb(27, 94, 85)
		fg = colorInk
	}
	if item.state&4 != 0 {
		fill = rgb(30, 42, 56)
		border = fill
		fg = rgb(119, 137, 155)
	}
	paintRound(item.dc, item.rect, scale(12, currentDPI), fill, border)
	gcall("SetBkMode", item.dc, 1)
	gcall("SetTextColor", item.dc, fg)
	old := gcall("SelectObject", item.dc, call("SendMessageW", item.window, 0x31, 0, 0))
	defer gcall("SelectObject", item.dc, old)
	text := wide(textOf(item.window))
	call("DrawTextW", item.dc, uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&item.rect)), 0x25)
	if item.state&0x10 != 0 {
		r := item.rect
		r.left += 4
		r.top += 4
		r.right -= 4
		r.bottom -= 4
		call("DrawFocusRect", item.dc, uintptr(unsafe.Pointer(&r)))
	}
}

func darkWindowChrome() {
	proc := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	if proc.Find() != nil {
		return
	}
	enabled := int32(1)
	proc.Call(window, 20, uintptr(unsafe.Pointer(&enabled)), 4)
	// Windows 11 supports explicit caption and border colors; older versions
	// ignore these attributes and retain their system-managed frame.
	for attribute, color := range map[uintptr]uintptr{34: colorBorder, 35: colorBG, 36: colorInk} {
		value := uint32(color)
		proc.Call(window, attribute, uintptr(unsafe.Pointer(&value)), 4)
	}
}
