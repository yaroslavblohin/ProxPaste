//go:build windows && (amd64 || arm64)

package main

import (
	"syscall"
	"unsafe"
)

var gdi = syscall.NewLazyDLL("gdi32.dll")
var shell = syscall.NewLazyDLL("shell32.dll")
var uiFont uintptr
var trayAvailable bool
var taskbarCreated uint32
var lastTrayStatus string
var currentDPI = 96

type controlLayout struct {
	handle              uintptr
	x, y, width, height int
}

var layouts []controlLayout

type winRect struct{ left, top, right, bottom int32 }

func enableDPI() {
	proc := user.NewProc("SetProcessDpiAwarenessContext")
	if proc.Find() == nil {
		if ok, _, _ := proc.Call(^uintptr(3)); ok != 0 {
			return
		} // PER_MONITOR_AWARE_V2
	}
	call("SetProcessDPIAware")
}
func scale(value, dpi int) int { return (value*dpi + 48) / 96 }
func applyDPI(dpi int) {
	if dpi <= 0 {
		dpi = 96
	}
	currentDPI = dpi
	old := fonts
	fonts = map[fontStyle]uintptr{}
	uiFont = getFont(fontStyle{15, 400, "Segoe UI"})
	for _, item := range layouts {
		font := uiFont
		if style, ok := styles[item.handle]; ok {
			font = getFont(style.font)
		}
		call("SendMessageW", item.handle, 0x30, font, 1)
		call("SetWindowPos", item.handle, 0, uintptr(scale(item.x, dpi)), uintptr(scale(item.y, dpi)), uintptr(scale(item.width, dpi)), uintptr(scale(item.height, dpi)), 0x14)
	}
	for _, font := range old {
		gdi.NewProc("DeleteObject").Call(font)
	}
	call("InvalidateRect", window, 0, 1)
}

// NOTIFYICONDATAW for the supported 64-bit Windows architectures.
type notifyIconData struct {
	size                uint32
	window              uintptr
	id, flags, callback uint32
	icon                uintptr
	tip                 [128]uint16
	state, stateMask    uint32
	info                [256]uint16
	version             uint32
	title               [64]uint16
	infoFlags           uint32
	guid                [16]byte
	balloonIcon         uintptr
}

func trayData() notifyIconData {
	data := notifyIconData{window: window, id: 1}
	data.size = uint32(unsafe.Sizeof(data))
	return data
}
func putUTF16(dst []uint16, text string) {
	for i := range dst {
		dst[i] = 0
	}
	encoded := syscall.StringToUTF16(text)
	if len(encoded) > len(dst) {
		encoded = encoded[:len(dst)-1]
		if len(encoded) > 0 && encoded[len(encoded)-1] >= 0xD800 && encoded[len(encoded)-1] <= 0xDBFF {
			encoded = encoded[:len(encoded)-1]
		}
	}
	copy(dst, encoded)
}
func notifyTray(action uintptr, data *notifyIconData) bool {
	result, _, _ := shell.NewProc("Shell_NotifyIconW").Call(action, uintptr(unsafe.Pointer(data)))
	return result != 0
}
func installTray() {
	data := trayData()
	data.flags = 1 | 2 | 4
	data.callback = 0x8001
	data.icon = appIcon
	putUTF16(data.tip[:], "ProxPaste · "+selectedHotkey.String())
	trayAvailable = notifyTray(0, &data)
	lastTrayStatus = ""
	if !trayAvailable {
		setStatus("Не удалось создать значок трея. Закрытие окна завершит приложение.")
	}
}
func updateTray(status string) {
	if !trayAvailable || status == lastTrayStatus {
		return
	}
	data := trayData()
	data.flags = 4
	putUTF16(data.tip[:], "ProxPaste · "+selectedHotkey.String()+"\n"+status)
	if notifyTray(1, &data) {
		lastTrayStatus = status
	}
}
func trayBalloon(status string) {
	if !trayAvailable {
		return
	}
	data := trayData()
	data.flags = 0x10
	data.infoFlags = 2
	putUTF16(data.title[:], "ProxPaste — ввод не начат")
	putUTF16(data.info[:], status)
	notifyTray(1, &data)
}
func removeTray() { data := trayData(); notifyTray(2, &data); trayAvailable = false }
func showWindow() { call("ShowWindow", window, 9); call("SetForegroundWindow", window) }
func quitWindow() {
	closing = true
	stop()
	if busy, _ := snapshot(); !busy {
		call("DestroyWindow", window)
	}
}
func trayMenu() {
	menu := call("CreatePopupMenu")
	if menu == 0 {
		return
	}
	defer call("DestroyMenu", menu)
	call("AppendMenuW", menu, 0, 201, uintptr(unsafe.Pointer(wide("Открыть ProxPaste"))))
	call("AppendMenuW", menu, 1, 0, uintptr(unsafe.Pointer(wide("Вставка: "+selectedHotkey.String()))))
	call("AppendMenuW", menu, 0, 202, uintptr(unsafe.Pointer(wide("Остановить ввод"))))
	call("AppendMenuW", menu, 0x800, 0, 0)
	call("AppendMenuW", menu, 0, 203, uintptr(unsafe.Pointer(wide("Выход"))))
	var point struct{ x, y int32 }
	call("GetCursorPos", uintptr(unsafe.Pointer(&point)))
	call("SetForegroundWindow", window)
	command := call("TrackPopupMenu", menu, 0x102, uintptr(point.x), uintptr(point.y), 0, window, 0)
	call("PostMessageW", window, 0, 0, 0)
	if command != 0 {
		call("SendMessageW", window, 0x111, command, 0)
	}
}
func initializeDesktop() {
	dpi := 96
	if proc := user.NewProc("GetDpiForWindow"); proc.Find() == nil {
		value, _, _ := proc.Call(window)
		if value != 0 {
			dpi = int(value)
		}
	}
	applyDPI(dpi)
	rect := winRect{right: int32(scale(840, dpi)), bottom: int32(scale(730, dpi))}
	if proc := user.NewProc("AdjustWindowRectExForDpi"); proc.Find() == nil {
		proc.Call(uintptr(unsafe.Pointer(&rect)), 0x00CA0000, 0, 0, uintptr(dpi))
	} else {
		call("AdjustWindowRectEx", uintptr(unsafe.Pointer(&rect)), 0x00CA0000, 0, 0)
	}
	call("SetWindowPos", window, 0, 0, 0, uintptr(rect.right-rect.left), uintptr(rect.bottom-rect.top), 0x16)
	taskbarCreated = uint32(call("RegisterWindowMessageW", uintptr(unsafe.Pointer(wide("TaskbarCreated")))))
	installTray()
	commit, _, err := prepareWindowsHotkey(selectedHotkey)
	if err != nil {
		setStatus(err.Error())
	} else {
		commit()
	}
	updateHotkeyHint()
}
