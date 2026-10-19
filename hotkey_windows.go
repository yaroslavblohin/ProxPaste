//go:build windows && (amd64 || arm64)

package main

import (
	"fmt"
	"strconv"
)

var activeHotkeyID uintptr
var hotkeyBox, hotkeyHint uintptr

func windowsHotkey(h Hotkey) (uintptr, uintptr) {
	flags := uintptr(0x4000)
	if h.Modifiers&modCtrl != 0 {
		flags |= 2
	}
	if h.Modifiers&modAlt != 0 {
		flags |= 1
	}
	if h.Modifiers&modShift != 0 {
		flags |= 4
	}
	if h.Modifiers&modSuper != 0 {
		flags |= 8
	}
	key := uintptr(h.Key[0])
	if len(h.Key) > 1 {
		n, _ := strconv.Atoi(h.Key[1:])
		key = uintptr(0x6F + n)
	}
	return flags, key
}
func prepareWindowsHotkey(h Hotkey) (func(), func(), error) {
	if h == selectedHotkey && activeHotkeyID != 0 {
		return func() {}, func() {}, nil
	}
	id := uintptr(1)
	if activeHotkeyID == 1 {
		id = 2
	}
	flags, key := windowsHotkey(h)
	if call("RegisterHotKey", window, id, flags, key) == 0 {
		return nil, nil, fmt.Errorf("%s занята или зарезервирована системой. Прежнее сочетание сохранено", h.String())
	}
	return func() {
		if activeHotkeyID != 0 {
			call("UnregisterHotKey", window, activeHotkeyID)
		}
		activeHotkeyID = id
	}, func() { call("UnregisterHotKey", window, id) }, nil
}
func applyHotkeyFromUI() {
	if err := changeSettings(textOf(hotkeyBox), textOf(delayBox), textOf(intervalBox), call("SendMessageW", allowBox, 0xF0, 0, 0) == 1, prepareWindowsHotkey); err != nil {
		setStatus(err.Error())
		return
	}
	setText(hotkeyBox, selectedHotkey.String())
	updateHotkeyHint()
}
func updateHotkeyHint() {
	text := "Активно: " + selectedHotkey.String()
	if activeHotkeyID == 0 {
		text = "Сочетание не зарегистрировано"
	}
	setText(hotkeyHint, text)
	lastTrayStatus = ""
}
