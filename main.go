package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type stroke struct {
	key   byte
	shift bool
}

const shifted = "~!@#$%^&*()_+{}|:\"<>?"
const unshifted = "`1234567890-=[]\\;',./"

func plan(text string, allow bool) ([]stroke, error) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	result := make([]stroke, 0, len(text))
	for _, c := range text {
		if c == '\n' || c == '\t' {
			if !allow {
				return nil, fmt.Errorf("Разрешите Enter/Tab перед вводом многострочного текста")
			}
		} else if c < 32 || c > 126 {
			return nil, fmt.Errorf("Символ %q (U+%04X) не поддерживается. Нужен English (US)", c, c)
		}
		s := stroke{key: byte(c)}
		if c >= 'A' && c <= 'Z' {
			s.key += 32
			s.shift = true
		} else if i := strings.IndexRune(shifted, c); i >= 0 {
			s.key = unshifted[i]
			s.shift = true
		}
		result = append(result, s)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("Сначала загрузите или введите текст")
	}
	return result, nil
}

var state = struct {
	sync.Mutex
	busy   bool
	status string
	cancel chan struct{}
}{status: "Готово. Текст хранится только в памяти."}

func snapshot() (bool, string) { state.Lock(); defer state.Unlock(); return state.busy, state.status }
func setStatus(s string)       { state.Lock(); state.status = s; state.Unlock() }
func stop() {
	state.Lock()
	defer state.Unlock()
	if state.cancel != nil {
		select {
		case <-state.cancel:
		default:
			close(state.cancel)
		}
	}
}
func start(text, delayText, intervalText string, allow bool) {
	state.Lock()
	busy := state.busy
	state.Unlock()
	if busy {
		return
	}
	steps, err := plan(text, allow)
	if err != nil {
		setStatus(err.Error())
		return
	}
	delay, e1 := strconv.Atoi(strings.TrimSpace(delayText))
	interval, e2 := strconv.Atoi(strings.TrimSpace(intervalText))
	if e1 != nil || e2 != nil || delay < 0 || delay > 60 || interval < 0 || interval > 1000 {
		setStatus("Задержка: 0–60 сек. Интервал: 0–1000 мс.")
		return
	}
	if err = permission(); err != nil {
		setStatus(err.Error())
		return
	}
	cancel := make(chan struct{})
	state.Lock()
	state.busy = true
	state.cancel = cancel
	state.Unlock()
	go func() {
		defer func() { state.Lock(); state.busy = false; state.cancel = nil; state.Unlock() }()
		wait := func(d time.Duration) bool {
			return waitInput(d, cancel, escapePressed)
		}
		for left := delay; left > 0; left-- {
			setStatus(fmt.Sprintf("Старт через %d сек. Щёлкните внутри noVNC. Esc — отмена.", left))
			if !wait(time.Second) {
				setStatus("Отменено до начала ввода.")
				return
			}
		}
		// The hotkey modifiers must not turn the first character into a shortcut.
		if !waitModifiersReleased(modifiersPressed, wait) {
			setStatus("Отменено. Отпустите Ctrl, Alt/Option, Shift и Win/Command.")
			return
		}
		sent := 0
		for _, s := range steps {
			if !wait(0) {
				setStatus(fmt.Sprintf("Остановлено. Отправлено %d из %d.", sent, len(steps)))
				return
			}
			if err := sendStroke(s); err != nil {
				setStatus("Ошибка ввода: " + err.Error())
				return
			}
			sent++
			setStatus(fmt.Sprintf("Отправлено %d из %d. Esc — остановка.", sent, len(steps)))
			if !wait(time.Duration(interval) * time.Millisecond) {
				setStatus(fmt.Sprintf("Остановлено. Отправлено %d из %d.", sent, len(steps)))
				return
			}
		}
		setStatus(fmt.Sprintf("Готово. Отправлено символов: %d.", sent))
	}()
}

// Cancellation is checked even when no inter-character pause is requested.
func waitInput(d time.Duration, cancel <-chan struct{}, escape func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if escape() {
			return false
		}
		select {
		case <-cancel:
			return false
		default:
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return true
		}
		select {
		case <-cancel:
			return false
		case <-time.After(min(10*time.Millisecond, remaining)):
		}
	}
}

func waitModifiersReleased(pressed func() bool, wait func(time.Duration) bool) bool {
	for attempts := 0; attempts < 1000; attempts++ {
		if !pressed() {
			return true
		}
		if !wait(10 * time.Millisecond) {
			return false
		}
	}
	return false
}

var startInTray bool

func main() {
	flag.BoolVar(&startInTray, "tray", false, "Запустить в трее без открытия окна")
	flag.Parse()
	initializeSettings()
	runUI()
}
