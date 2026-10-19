//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -mmacosx-version-min=11.0
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices -framework Carbon -framework QuartzCore
#include <stdlib.h>
void nativeRun(int hidden, const void *icon, int iconLength, const void *tray, int trayLength);
int nativePrepareHotkey(unsigned int code, unsigned int modifiers);
void nativeCommitHotkey(void);
void nativeCancelHotkey(void);
int nativePermission(void);
int nativeEscape(void);
int nativeModifiers(void);
int nativeKey(unsigned short code, int down, int shift);
*/
import "C"

import (
	"fmt"
	"runtime"
	"strconv"
	"time"
	"unsafe"
)

// Cocoa must start on the process main thread.
func init() { runtime.LockOSThread() }
func runUI() {
	hidden := C.int(0)
	if startInTray {
		hidden = 1
	}
	C.nativeRun(hidden, unsafe.Pointer(&appPNG[0]), C.int(len(appPNG)), unsafe.Pointer(&trayPNG[0]), C.int(len(trayPNG)))
}
func permission() error {
	if C.nativePermission() == 0 {
		return fmt.Errorf("Разрешите ProxPaste Accessibility и Input Monitoring в настройках macOS, затем перезапустите приложение")
	}
	return nil
}
func escapePressed() bool    { return C.nativeEscape() != 0 }
func modifiersPressed() bool { return C.nativeModifiers() != 0 }

var macCodes = map[byte]uint16{
	'a': 0, 's': 1, 'd': 2, 'f': 3, 'h': 4, 'g': 5, 'z': 6, 'x': 7, 'c': 8, 'v': 9, 'b': 11,
	'q': 12, 'w': 13, 'e': 14, 'r': 15, 'y': 16, 't': 17, '1': 18, '2': 19, '3': 20, '4': 21,
	'6': 22, '5': 23, '=': 24, '9': 25, '7': 26, '-': 27, '8': 28, '0': 29, ']': 30, 'o': 31,
	'u': 32, '[': 33, 'i': 34, 'p': 35, '\n': 36, 'l': 37, 'j': 38, '\'': 39, 'k': 40, ';': 41,
	'\\': 42, ',': 43, '/': 44, 'n': 45, 'm': 46, '.': 47, '\t': 48, ' ': 49, '`': 50,
}

func sendStroke(s stroke) (err error) {
	code, ok := macCodes[s.key]
	if !ok {
		return fmt.Errorf("нет кода клавиши %q", s.key)
	}
	flags := C.int(0)
	if s.shift {
		flags = 1
	}
	event := func(key uint16, down, shift C.int) error {
		if C.nativeKey(C.ushort(key), down, shift) == 0 {
			return fmt.Errorf("не удалось создать событие клавиатуры")
		}
		return nil
	}
	if s.shift {
		if err = event(56, 1, 1); err != nil {
			return
		}
		defer func() {
			e := event(56, 0, 0)
			if err == nil {
				err = e
			}
		}()
	}
	if err = event(code, 1, flags); err != nil {
		return
	}
	time.Sleep(12 * time.Millisecond)
	return event(code, 0, flags)
}

//export goStart
func goStart(text, delay, interval *C.char, allow C.int) {
	start(C.GoString(text), C.GoString(delay), C.GoString(interval), allow != 0)
}

//export goStop
func goStop() { stop() }

//export goBusy
func goBusy() C.int {
	busy, _ := snapshot()
	if busy {
		return 1
	}
	return 0
}

//export goStatus
func goStatus() *C.char { _, status := snapshot(); return C.CString(status) }

//export goSetStatus
func goSetStatus(text *C.char) { setStatus(C.GoString(text)) }

func prepareDarwinHotkey(h Hotkey) (func(), func(), error) {
	var flags uint32
	if h.Modifiers&modCtrl != 0 {
		flags |= 4096
	}
	if h.Modifiers&modAlt != 0 {
		flags |= 2048
	}
	if h.Modifiers&modShift != 0 {
		flags |= 512
	}
	if h.Modifiers&modSuper != 0 {
		flags |= 256
	}
	var code uint16
	if len(h.Key) > 1 {
		n, _ := strconv.Atoi(h.Key[1:])
		codes := []uint16{122, 120, 99, 118, 96, 97, 98, 100, 101, 109, 103, 111}
		code = codes[n-1]
	} else {
		key := h.Key[0]
		if key >= 'A' && key <= 'Z' {
			key += 32
		}
		code = macCodes[key]
	}
	if C.nativePrepareHotkey(C.uint(code), C.uint(flags)) == 0 {
		return nil, nil, fmt.Errorf("%s занята или зарезервирована системой. Прежнее сочетание сохранено", h.String())
	}
	return func() { C.nativeCommitHotkey() }, func() { C.nativeCancelHotkey() }, nil
}

//export goInstallHotkey
func goInstallHotkey() {
	commit, _, err := prepareDarwinHotkey(selectedHotkey)
	if err != nil {
		setStatus(err.Error())
	} else {
		commit()
	}
}

//export goHotkey
func goHotkey() *C.char { return C.CString(selectedHotkey.String()) }

//export goChangeSettings
func goChangeSettings(text, delay, interval *C.char, allow C.int) C.int {
	if err := changeSettings(C.GoString(text), C.GoString(delay), C.GoString(interval), allow != 0, prepareDarwinHotkey); err != nil {
		setStatus(err.Error())
		return 0
	}
	return 1
}

//export goDelaySeconds
func goDelaySeconds() C.int { return C.int(currentSettings.DelaySeconds) }

//export goIntervalMS
func goIntervalMS() C.int { return C.int(currentSettings.IntervalMS) }

//export goAllowEnterTab
func goAllowEnterTab() C.int {
	if currentSettings.AllowEnterTab {
		return 1
	}
	return 0
}
