//go:build windows && (amd64 || arm64)

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsNativeLayouts(t *testing.T) {
	if got := unsafe.Sizeof(notifyIconData{}); got != 976 {
		t.Fatalf("NOTIFYICONDATAW size %d, expected 976", got)
	}
	if got := unsafe.Sizeof(keyboardInput{}); got != 40 {
		t.Fatalf("INPUT size %d, expected 40", got)
	}
	if got := unsafe.Offsetof(notifyIconData{}.tip); got != 40 {
		t.Fatalf("tooltip offset %d", got)
	}
}

// Opt-in integration test: creates a hidden real window and a temporary tray
// icon. It never reads the clipboard or sends keyboard input to other apps.
func TestNativeDesktopLifecycle(t *testing.T) {
	if os.Getenv("PROXPASTE_NATIVE_TEST") != "1" {
		t.Skip("set PROXPASTE_NATIVE_TEST=1 for the native desktop test")
	}
	t.Setenv("APPDATA", t.TempDir())
	oldDefault, oldSelected := defaultHotkey, selectedHotkey
	oldSettings := currentSettings
	defaultHotkey = Hotkey{modCtrl | modAlt | modShift, "F11"}
	selectedHotkey = defaultHotkey
	t.Cleanup(func() { defaultHotkey = oldDefault; selectedHotkey = oldSelected; currentSettings = oldSettings })
	initialFlags, initialKey := windowsHotkey(defaultHotkey)
	result := make(chan error, 1)
	go func() {
		var handle uintptr
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			handle = call("FindWindowW", uintptr(unsafe.Pointer(wide("ProxPasteWindow"))), 0)
			var pid uint32
			if handle != 0 {
				call("GetWindowThreadProcessId", handle, uintptr(unsafe.Pointer(&pid)))
			}
			if pid == uint32(os.Getpid()) {
				edit := call("GetDlgItem", handle, 104)
				if edit != 0 && call("SendMessageW", edit, 0x31, 0, 0) != 0 {
					break
				}
			}
			handle = 0
			time.Sleep(20 * time.Millisecond)
		}
		if handle == 0 {
			result <- fmt.Errorf("native window not initialized")
			return
		}
		defer call("PostMessageW", handle, 0x111, 203, 0)
		check := func() error {
			// Synchronous delivery waits until the window's message loop is ready.
			call("SendMessageW", handle, 0, 0, 0)
			if call("IsWindowVisible", handle) != 0 {
				return fmt.Errorf("test window should stay hidden")
			}
			call("SendMessageW", handle, 0x10, 0, 0)
			if call("IsWindow", handle) == 0 {
				return fmt.Errorf("close destroyed the tray app")
			}
			for _, dpi := range []int{192, 144, 96} {
				rect := winRect{right: int32(scale(840, dpi)), bottom: int32(scale(730, dpi))}
				call("AdjustWindowRectExForDpi", uintptr(unsafe.Pointer(&rect)), 0x02CA0000, 0, 0, uintptr(dpi))
				call("SendMessageW", handle, 0x2E0, uintptr(dpi|dpi<<16), uintptr(unsafe.Pointer(&rect)))
				edit := call("GetDlgItem", handle, 104)
				var actual winRect
				call("GetWindowRect", edit, uintptr(unsafe.Pointer(&actual)))
				if actual.right-actual.left != int32(scale(752, dpi)) {
					return fmt.Errorf("editor width at DPI %d: %d", dpi, actual.right-actual.left)
				}
				font := call("SendMessageW", edit, 0x31, 0, 0)
				var logfont [92]byte
				gdi.NewProc("GetObjectW").Call(font, uintptr(len(logfont)), uintptr(unsafe.Pointer(&logfont[0])))
				height := *(*int32)(unsafe.Pointer(&logfont[0]))
				if height != int32(-scale(16, dpi)) {
					return fmt.Errorf("font height at DPI %d: %d", dpi, height)
				}
			}
			if call("RegisterHotKey", handle, 2, initialFlags, initialKey) != 0 {
				call("UnregisterHotKey", handle, 2)
				return fmt.Errorf("expected Ctrl+Alt+V to be already registered")
			}
			field := call("GetDlgItem", handle, 110)
			// A conflict must preserve the old binding and must not write settings.
			if call("RegisterHotKey", handle, 9, 0x4003, 'K') == 0 {
				return fmt.Errorf("test conflict binding unavailable")
			}
			setText(field, "Ctrl+Alt+K")
			call("SendMessageW", handle, 0x111, 108, 0)
			call("UnregisterHotKey", handle, 9)
			path, _ := settingsPath()
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				return fmt.Errorf("failed binding wrote settings")
			}
			setText(field, "Ctrl+Shift+J")
			setText(call("GetDlgItem", handle, 105), "9")
			setText(call("GetDlgItem", handle, 106), "125")
			call("SendMessageW", call("GetDlgItem", handle, 107), 0xF1, 1, 0)
			call("SendMessageW", handle, 0x111, 108, 0)
			stored, err := loadHotkey(path)
			if err != nil || stored != (Hotkey{modCtrl | modShift, "J"}) {
				return fmt.Errorf("new hotkey not persisted: %+v %v", stored, err)
			}
			cfg, err := loadSettings(path)
			if err != nil || cfg.DelaySeconds != 9 || cfg.IntervalMS != 125 || !cfg.AllowEnterTab {
				return fmt.Errorf("input settings not saved: %+v %v", cfg, err)
			}
			// The original shortcut must be available again after successful swap.
			if call("RegisterHotKey", handle, 9, initialFlags, initialKey) == 0 {
				return fmt.Errorf("old hotkey was not released")
			}
			call("UnregisterHotKey", handle, 9)
			call("SendMessageW", handle, 0x111, 109, 0)
			stored, err = loadHotkey(path)
			if err != nil || stored != defaultHotkey {
				return fmt.Errorf("reset did not restore default")
			}
			cfg, err = loadSettings(path)
			if err != nil || cfg.DelaySeconds != 9 || cfg.IntervalMS != 125 || !cfg.AllowEnterTab {
				return fmt.Errorf("hotkey reset overwrote input settings")
			}
			if path := os.Getenv("PROXPASTE_SCREENSHOT"); path != "" {
				setText(call("GetDlgItem", handle, 104), "# ProxPaste - keyboard input\r\necho \"Hello from ProxPaste\"\r\nhostname\r\n")
				call("SendMessageW", handle, 0x113, 1, 0)
				if err := captureWindow(handle, path); err != nil {
					return err
				}
			}
			return nil
		}
		// RegisterHotKey must run on the thread that owns the window. Execute
		// this integration check through a temporary test-only window callback.
		index := ^uintptr(3) // GWLP_WNDPROC (-4)
		previous := call("GetWindowLongPtrW", handle, index)
		callback := syscall.NewCallback(func(h uintptr, msg uint32, w, l uintptr) uintptr {
			if msg == 0x8002 {
				result <- check()
				call("SetWindowLongPtrW", h, index, previous)
				return 0
			}
			return call("CallWindowProcW", previous, h, uintptr(msg), w, l)
		})
		call("SetWindowLongPtrW", handle, index, callback)
		call("SendMessageW", handle, 0x8002, 0, 0)
	}()
	runWindow(false)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestPackagedIcon(t *testing.T) {
	path, err := filepath.Abs("dist/ProxPaste.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("build executable first")
	}
	count, _, _ := shell.NewProc("ExtractIconExW").Call(uintptr(unsafe.Pointer(wide(path))), ^uintptr(0), 0, 0, 0)
	if count == 0 || count == ^uintptr(0) {
		t.Fatalf("exe has no icon resource: %d", count)
	}
}
func TestDPIScaling(t *testing.T) {
	for _, sample := range []struct{ dpi, want int }{{96, 740}, {120, 925}, {144, 1110}, {192, 1480}, {288, 2220}} {
		if got := scale(740, sample.dpi); got != sample.want {
			t.Fatalf("dpi %d: %d", sample.dpi, got)
		}
	}
}
func TestTrayTooltipTermination(t *testing.T) {
	buffer := make([]uint16, 6)
	putUTF16(buffer, "abcd😀z")
	if buffer[5] != 0 || syscall.UTF16ToString(buffer) != "abcd" {
		t.Fatalf("surrogate truncation: %v", buffer)
	}
	putUTF16(buffer, "ok")
	if syscall.UTF16ToString(buffer) != "ok" || buffer[3] != 0 {
		t.Fatalf("old tooltip retained: %v", buffer)
	}
}
