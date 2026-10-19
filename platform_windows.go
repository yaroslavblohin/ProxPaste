//go:build windows && (amd64 || arm64)

package main

import (
	"fmt"
	"runtime"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

var user = syscall.NewLazyDLL("user32.dll")
var kernel = syscall.NewLazyDLL("kernel32.dll")

// Keep Go objects passed as syscall arguments alive for the native call.
//
//go:uintptrescapes
func call(name string, args ...uintptr) uintptr {
	r, _, _ := user.NewProc(name).Call(args...)
	return r
}
func wide(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func permission() error     { return nil }
func escapePressed() bool   { return call("GetAsyncKeyState", 0x1B)&0x8000 != 0 }
func modifiersPressed() bool {
	for _, key := range []uintptr{0x10, 0x11, 0x12, 0x5B, 0x5C} {
		if call("GetAsyncKeyState", key)&0x8000 != 0 {
			return true
		}
	}
	return false
}

type keyboardInput struct {
	kind  uint32
	pad   uint32
	vk    uint16
	scan  uint16
	flags uint32
	stamp uint32
	pad2  uint32
	extra uintptr
	tail  [8]byte
}

func keyEvent(vk uint16, up bool) error {
	in := keyboardInput{kind: 1, vk: vk}
	if up {
		in.flags = 2
	}
	if call("SendInput", 1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in)) != 1 {
		return fmt.Errorf("Windows заблокировала SendInput. Проверьте права целевого окна")
	}
	return nil
}
func sendStroke(s stroke) (err error) {
	vk := uint16(s.key)
	codes := map[byte]uint16{'\n': 13, '\t': 9, ';': 0xBA, '=': 0xBB, ',': 0xBC, '-': 0xBD, '.': 0xBE, '/': 0xBF, '`': 0xC0, '[': 0xDB, '\\': 0xDC, ']': 0xDD, '\'': 0xDE}
	if code, ok := codes[s.key]; ok {
		vk = code
	} else if s.key >= 'a' && s.key <= 'z' {
		vk -= 32
	}
	if s.shift {
		if err = keyEvent(0x10, false); err != nil {
			return
		}
		defer func() {
			e := keyEvent(0x10, true)
			if err == nil {
				err = e
			}
		}()
	}
	if err = keyEvent(vk, false); err != nil {
		return
	}
	time.Sleep(12 * time.Millisecond)
	return keyEvent(vk, true)
}

type wndClass struct {
	size       uint32
	style      uint32
	proc       uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menu       *uint16
	name       *uint16
	smallIcon  uintptr
}
type message struct {
	hwnd    uintptr
	id      uint32
	w       uintptr
	l       uintptr
	stamp   uint32
	x       int32
	y       int32
	private uint32
}

var window, editor, delayBox, intervalBox, allowBox, statusLabel uintptr
var controls []uintptr
var closing bool

func textOf(h uintptr) string {
	n := call("GetWindowTextLengthW", h)
	buf := make([]uint16, n+1)
	call("GetWindowTextW", h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}
func setText(h uintptr, s string) { call("SetWindowTextW", h, uintptr(unsafe.Pointer(wide(s)))) }
func add(class, text string, style uintptr, x, y, w, h int, id uintptr) uintptr {
	handle := call("CreateWindowExW", 0, uintptr(unsafe.Pointer(wide(class))), uintptr(unsafe.Pointer(wide(text))), 0x50000000|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), window, id, 0, 0)
	layouts = append(layouts, controlLayout{handle, x, y, w, h})
	return handle
}
func readClipboard() (string, error) {
	if call("OpenClipboard", window) == 0 {
		return "", fmt.Errorf("Не удалось открыть буфер обмена")
	}
	defer call("CloseClipboard")
	h := call("GetClipboardData", 13)
	if h == 0 {
		return "", fmt.Errorf("В буфере нет текста")
	}
	p, _, _ := kernel.NewProc("GlobalLock").Call(h)
	if p == 0 {
		return "", fmt.Errorf("Не удалось прочитать буфер обмена")
	}
	defer kernel.NewProc("GlobalUnlock").Call(h)
	size, _, _ := kernel.NewProc("GlobalSize").Call(h)
	if size < 2 || size > 32*1024*1024 {
		return "", fmt.Errorf("Буфер пуст или превышает 32 МБ")
	}
	data := make([]uint16, size/2)
	kernel.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&data[0])), p, uintptr(len(data)*2))
	return syscall.UTF16ToString(data), nil
}
func loadClipboard() {
	text, err := readClipboard()
	if err != nil {
		setStatus(err.Error())
		return
	}
	setText(editor, text)
	setStatus("Текст загружен. Проверьте его перед вводом.")
}
func windowProc(h uintptr, msg uint32, w, l uintptr) uintptr {
	if taskbarCreated != 0 && msg == taskbarCreated {
		installTray()
		return 0
	}
	switch msg {
	case 0xF:
		paintWindow(h)
		return 0
	case 0x318:
		paintWindowDC(h, w)
		return 0
	case 0x133, 0x135, 0x138:
		return colorControl(w, l)
	case 0x2B:
		drawButton(l)
		return 1
	case 0x312: // WM_HOTKEY: do not activate our window or steal noVNC focus.
		if w == activeHotkeyID && activeHotkeyID != 0 && !closing {
			if busy, _ := snapshot(); !busy {
				text, err := readClipboard()
				if err != nil {
					setStatus(err.Error())
				} else {
					setText(editor, text)
					start(text, textOf(delayBox), textOf(intervalBox), call("SendMessageW", allowBox, 0xF0, 0, 0) == 1)
				}
				if busy, status := snapshot(); !busy {
					trayBalloon(status)
				}
			}
		}
		return 0
	case 0x8001: // tray callback, legacy notification format
		if l == 0x202 || l == 0x203 {
			showWindow()
		}
		if l == 0x205 || l == 0x7B {
			trayMenu()
		}
		return 0
	case 0x2E0: // WM_DPICHANGED
		var rect winRect
		kernel.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&rect)), l, unsafe.Sizeof(rect))
		applyDPI(int(w & 0xFFFF))
		call("SetWindowPos", h, 0, uintptr(rect.left), uintptr(rect.top), uintptr(rect.right-rect.left), uintptr(rect.bottom-rect.top), 0x14)
		return 0
	case 0x112: // WM_SYSCOMMAND
		if w&0xFFF0 == 0xF020 && trayAvailable {
			call("ShowWindow", h, 0)
			return 0
		}
	case 0x111:
		switch w & 0xFFFF {
		case 101:
			if busy, _ := snapshot(); !busy {
				loadClipboard()
			}
		case 102:
			start(textOf(editor), textOf(delayBox), textOf(intervalBox), call("SendMessageW", allowBox, 0xF0, 0, 0) == 1)
		case 103:
			stop()
		case 108:
			applyHotkeyFromUI()
		case 109:
			if busy, _ := snapshot(); !busy {
				setText(hotkeyBox, defaultHotkey.String())
				applyHotkeyFromUI()
			}
		case 201:
			showWindow()
		case 202:
			stop()
		case 203:
			quitWindow()
		}
	case 0x113:
		busy, status := snapshot()
		if textOf(statusLabel) != status {
			setText(statusLabel, status)
		}
		updateTray(status)
		for _, control := range controls {
			enabled := uintptr(1)
			if busy {
				enabled = 0
			}
			call("EnableWindow", control, enabled)
		}
		if closing && !busy {
			call("DestroyWindow", h)
		}
	case 0x10:
		if trayAvailable {
			call("ShowWindow", h, 0)
		} else {
			quitWindow()
		}
		return 0
	case 2:
		if activeHotkeyID != 0 {
			call("UnregisterHotKey", h, activeHotkeyID)
		}
		removeTray()
		destroyTheme()
		call("PostQuitMessage", 0)
		return 0
	}
	return call("DefWindowProcW", h, uintptr(msg), w, l)
}
func runUI() { runWindow(!startInTray) }

func runWindow(visible bool) {
	runtime.LockOSThread()
	enableDPI()
	initializeTheme()
	instance, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	cls := wndClass{proc: syscall.NewCallback(windowProc), instance: instance, cursor: call("LoadCursorW", 0, 32512), background: bgBrush, icon: appIcon, smallIcon: appIcon, name: wide("ProxPasteWindow")}
	cls.size = uint32(unsafe.Sizeof(cls))
	if call("RegisterClassExW", uintptr(unsafe.Pointer(&cls))) == 0 {
		panic("RegisterClassExW failed")
	}
	window = call("CreateWindowExW", 0, uintptr(unsafe.Pointer(cls.name)), uintptr(unsafe.Pointer(wide("ProxPaste"))), 0x02CA0000, 0x80000000, 0x80000000, 840, 730, 0, 0, instance, 0)
	if window == 0 {
		panic("CreateWindowExW failed")
	}
	darkWindowChrome()
	label("ProxPaste", 104, 22, 420, 40, 30, true, false)
	label("Ваш буфер. Ввод с клавиатуры. Любая консоль.", 106, 66, 650, 24, 15, false, false)
	label("ТЕКСТ ДЛЯ ВВОДА", 44, 128, 350, 24, 12, true, true)
	load := button("Из буфера обмена", 590, 121, 204, 32, 101)
	editor = add("EDIT", "", 0x003110C4, 44, 168, 752, 202, 104)
	styleControl(editor, 16, 400, "Consolas", rgb(226, 237, 245), colorEditor)
	call("SendMessageW", editor, 0xC5, 0x7FFFFFFE, 0)
	label("ТЕМП ВВОДА", 44, 426, 260, 22, 12, true, true)
	label("До старта, сек", 44, 467, 164, 24, 15, false, true)
	delayBox = add("EDIT", strconv.Itoa(currentSettings.DelaySeconds), 0x00812000, 226, 462, 72, 30, 105)
	label("Интервал, мс", 44, 517, 164, 24, 15, false, true)
	intervalBox = add("EDIT", strconv.Itoa(currentSettings.IntervalMS), 0x00812000, 226, 512, 72, 30, 106)
	for _, field := range []uintptr{delayBox, intervalBox} {
		styleControl(field, 16, 400, "Segoe UI", colorInk, colorEditor)
	}
	label("ГОРЯЧАЯ КЛАВИША", 360, 426, 400, 22, 12, true, true)
	hotkeyBox = add("EDIT", selectedHotkey.String(), 0x00810080, 360, 462, 256, 32, 110)
	styleControl(hotkeyBox, 16, 400, "Segoe UI", colorInk, colorEditor)
	apply := button("Сохранить всё", 632, 462, 164, 32, 108)
	hotkeyHint = label("", 360, 507, 258, 22, 13, false, true)
	label("Ctrl / Alt / Shift / Win + A–Z, 0–9, F1–F12", 360, 540, 438, 22, 11, false, true)
	reset := button("По умолчанию", 632, 506, 164, 28, 109)
	allowBox = add("BUTTON", "Разрешить Enter и Tab — Enter может выполнить команду", 0x10003, 26, 586, 790, 28, 107)
	if currentSettings.AllowEnterTab {
		call("SendMessageW", allowBox, 0xF1, 1, 0)
	}
	// Let our palette control checkbox text instead of the light visual style.
	syscall.NewLazyDLL("uxtheme.dll").NewProc("SetWindowTheme").Call(allowBox, uintptr(unsafe.Pointer(wide(""))), uintptr(unsafe.Pointer(wide(""))))
	begin := button("Начать ввод", 24, 630, 226, 42, 102)
	button("Остановить · Esc", 266, 630, 204, 42, 103)
	label("English (US) · Caps Lock выключен", 496, 633, 320, 22, 13, false, false)
	label("Закрытие окна — в трей", 496, 655, 320, 22, 12, false, false)
	statusLabel = label("", 26, 688, 788, 36, 13, false, false)
	controls = []uintptr{load, editor, delayBox, intervalBox, allowBox, begin, hotkeyBox, apply, reset}
	initializeDesktop()
	call("SetTimer", window, 1, 100, 0)
	if visible || !trayAvailable {
		call("ShowWindow", window, 5)
	}
	var m message
	for {
		result := call("GetMessageW", uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if result == 0 || int32(result) == -1 {
			break
		}
		if call("IsDialogMessageW", window, uintptr(unsafe.Pointer(&m))) != 0 {
			continue
		}
		call("TranslateMessage", uintptr(unsafe.Pointer(&m)))
		call("DispatchMessageW", uintptr(unsafe.Pointer(&m)))
	}
}
